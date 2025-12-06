# NBI Client Examples

Minimal clients that talk to the NBI gRPC server, build a tiny scenario, and fetch a snapshot.

## Prerequisites

- An NBI server running and reachable (e.g. `constellation-simulator` `cmd/nbi-server`).
- The server must have a transceiver model that matches the clients' `--transceiver-id` flag (defaults to `trx-ku` from `configs/transceivers.json`).

## Go CLI (`examples/nbi_client_go`)

```
go run ./examples/nbi_client_go --endpoint localhost:50051 --transceiver-id trx-ku
```

What it does:
- Clears the scenario (by default).
- Creates two platforms (`platform-ground`, `platform-sat`).
- Creates two wireless nodes (one per platform) with a single interface each.
- Creates one bidirectional link between the interfaces.
- Calls `ScenarioService.GetScenario` and prints the snapshot.

## Python script (`examples/nbi_client_py`)

No code generation needed; it loads message/service descriptors from `nbi_descriptor.pb`.

```
python -m venv .venv
.venv\Scripts\activate  # or source .venv/bin/activate on Unix
pip install -r examples/nbi_client_py/requirements.txt
python examples/nbi_client_py/main.py --endpoint localhost:50051 --transceiver-id trx-ku
```

Flags:
- `--descriptor` overrides the path to `nbi_descriptor.pb` if you've moved it.
- `--skip-clear` avoids the initial `ClearScenario` call.
- `--timeout` sets a per-RPC timeout in seconds.

Expected output: a short list of the two platforms, two nodes (with interfaces), and one link created by the script.
