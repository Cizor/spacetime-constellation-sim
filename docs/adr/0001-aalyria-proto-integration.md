# ADR 0001 – Aalyria proto integration strategy

- **Status:** Accepted
- **Date:** 2025-11-29
- **Related scope:** Scope 3 – NBI & Scenario Configuration
- **Related issue:** #2

## Context

The goal of this project is to build a **Spacetime-compatible constellation simulator** that can speak the **Aalyria Spacetime APIs**, in particular the **Northbound Interface (NBI)** and related data models (e.g. `PlatformDefinition`, `NetworkNode`, `NetworkLink`, `ServiceRequest`, etc.). :contentReference[oaicite:1]{index=1}

Aalyria publishes:

- A public API repo: [`aalyria/api`] which contains the **canonical `.proto` files** under `api/…`. :contentReference[oaicite:2]{index=2}  
- Generated client libraries (e.g. Go packages such as `aalyria.com/spacetime/...`) that correspond to those protos. :contentReference[oaicite:3]{index=3}  

For this simulator we need:

1. **Stable message types and service definitions** that match Aalyria’s NBI/Model APIs.
2. A way to **generate Go code** so that our NBI server and tests can use those types directly.
3. A repo layout that is:
   - Easy to clone (`git clone && go test ./...` should “just work”).
   - Reasonably self-contained (not fragile to external infra).
   - Explicit about **which version** of the Aalyria APIs we target.

This ADR decides **how we integrate Aalyria’s protos into `spacetime-constellation-sim`** and where they live, so later Scope-3 tickets can implement the NBI services on top.

## Options considered

### Option A – Use Aalyria’s Go modules only (no local protos)

**Description**

- Add a Go module dependency on Aalyria’s published Go APIs (e.g. `aalyria.com/spacetime/...`).
- Use only the generated `.pb.go` and gRPC service interfaces.
- Do **not** store `.proto` files in this repo.

**Pros**

- No local code generation pipeline to maintain.
- Tracks upstream Go client libraries directly.
- Very simple repo layout.

**Cons**

- We lose a clear, local view of the **`.proto` schema** we’re targeting.
- Harder to experiment with partial/experimental APIs or textproto scenarios.
- Tends to couple this simulator tightly to whatever Go packages Aalyria chooses to publish.
- Makes it awkward to write tools/tests that operate directly on proto descriptors or textproto files.

### Option B – Git submodule pointing at `aalyria/api`

**Description**

- Add `aalyria/api` as a Git submodule, e.g. under `third_party/aalyria/api-repo`.
- Reference `.proto` files from there when running `protoc`.

**Pros**

- Keeps an **exact copy** of the upstream tree.
- Updating to a new Spacetime API version can be as simple as bumping the submodule commit.
- No manual copying of files.

**Cons**

- Submodules add friction for contributors (`git submodule init`, `git submodule update`).
- The repo is no longer “just clone and go test”; you must remember the submodule step.
- For this project, we only need a **subset** of the protos (NBI + core common types). Pulling the whole repo as a submodule is overkill.

### Option C – Vendor the relevant `.proto` files into this repo

**Description**

- Copy the subset of Aalyria `.proto` files we need into a **read-only `third_party` tree** in this repo.
- Treat that tree as a **snapshot** of a specific Aalyria API release (matching the `api-main.zip` snapshot you already have).
- Run `protoc` locally (via a small script / Makefile target) to generate Go stubs into an `nbi/gen/...` directory.
- Keep Aalyria copyright and license headers intact and include an Apache-2.0 LICENSE in the third_party subtree.

**Pros**

- `git clone && go test ./...` remains **self-contained**; no submodule steps.
- We can pin to a specific Aalyria API release and record that in this ADR.
- We can limit ourselves to the proto **subset** we actually use (NBI + common types).
- We retain full access to the `.proto` definitions for:
  - Textproto scenario configuration.
  - E2E tests.
  - Experimenting with different NBI surfaces.

**Cons**

- Updating to a new Aalyria API version requires:
  - Refreshing the vendored `.proto` files from a new release.
  - Regenerating Go code.
- Potential for drift if we forget to keep the snapshot in sync with upstream releases we claim to support.

## Decision

We choose **Option C: vendor the relevant Aalyria `.proto` files into this repository** and generate Go code from them.

Rationale:

- Scope-3 and later scopes rely heavily on **NBI semantics** and on the ability to construct scenarios from textproto / gRPC, which is much easier if we have the proto definitions locally.
- This project is intended as a **reference simulator**; reproducibility and clarity are more important than always being on the very latest Aalyria version.
- Avoiding Git submodules keeps contributor ergonomics simple.
- You already have an `api-main.zip` snapshot; vendoring that snapshot (or a subset) is conceptually straightforward.

We will still track the upstream `aalyria/api` repo and its releases, but the **source of truth for this simulator** will be the vendored `.proto` snapshot plus this ADR.

## Implementation

### Directory layout

We will use the following layout for Aalyria-related artifacts:

- `third_party/aalyria/api/`  
  - Contains the **vendored `.proto` files** from Aalyria’s `api/api` subtree (NBI + common + any other needed packages).
  - Read-only in practice: we treat this as an upstream snapshot.
  - Includes an Apache-2.0 `LICENSE` file copied from Aalyria’s repo and any required NOTICE.

- `nbi/gen/`  
  - Contains **generated Go code** from those protos.
  - Subpackages follow the proto structure, e.g.:
    - `nbi/gen/aalyria/api/common`
    - `nbi/gen/aalyria/api/nbi`
    - `nbi/gen/aalyria/api/model/v1` (if needed)
  - These files are generated, not hand-edited.

- `nbi/server/` (or similar; exact name may change in later ADRs)
  - Our own gRPC server implementations, mapping from Aalyria NBI messages into internal `model` / `kb` types.

The exact subset of `.proto` files we plan to vendor includes (at minimum):

- Common types:
  - `api/common/platform.proto`
  - `api/common/platform_antenna.proto`
  - `api/common/coordinates.proto`
  - Other shared types as required by NBI messages. :contentReference[oaicite:4]{index=4}
- NBI & scenario-related APIs:
  - NBI model / entity definitions and services (e.g. platform / node / link / service request).
  - Scenario configuration messages used by Aalyria’s “build a scenario” tutorial. :contentReference[oaicite:5]{index=5}

We will document the exact list of copied files and the upstream commit/tag in a small `third_party/aalyria/README.md` when we vendor them.

### Code generation

We will add:

- A script: `scripts/gen_protos.sh`
- A Makefile target: `make protos`

**Example generation approach (conceptual):**

```bash
#!/usr/bin/env bash
set -euo pipefail

PROTO_ROOT="third_party/aalyria"
OUT_DIR="nbi/gen"

# Clean old generated code (optional, but keeps things tidy)
rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

protoc \
  -I "${PROTO_ROOT}" \
  --go_out="${OUT_DIR}" \
  --go_opt=paths=source_relative \
  --go-grpc_out="${OUT_DIR}" \
  --go-grpc_opt=paths=source_relative \
  api/common/coordinates.proto \
  api/common/platform.proto \
  api/common/platform_antenna.proto \
  api/nbi/*.proto \
  api/model/v1/*.proto
