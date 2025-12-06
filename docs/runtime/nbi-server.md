# nbi-server runtime

`cmd/nbi-server` exposes the constellation simulator through the Scope-3 NBI gRPC surface. It keeps the physical and network knowledge bases in memory, runs the simulation loop, and publishes metrics/traces for observability.

## Configuration
- `--listen-address` / `NBI_LISTEN_ADDRESS` (default `0.0.0.0:50051`): gRPC listen address.
- `--metrics-address` / `NBI_METRICS_ADDRESS` (default `:9090`, empty to disable): HTTP address serving `/metrics`.
- `--enable-tls` / `NBI_ENABLE_TLS` (default `false`): enable TLS on the gRPC listener. Requires `--tls-cert`/`NBI_TLS_CERT` and `--tls-key`/`NBI_TLS_KEY`.
- `--log-level` / `LOG_LEVEL` (default `info`): log verbosity (`debug|info|warn`).
- `--log-format` / `LOG_FORMAT` (default `text`): `text` or `json`.
- `--transceivers` / `NBI_TRANSCEIVERS_PATH` (default `configs/transceivers.json`): transceiver model catalog loaded at startup.
- `--network-scenario` / `NBI_NETWORK_SCENARIO` (optional): JSON scenario with initial interfaces/links/positions.
- `--tick` / `NBI_TICK_INTERVAL` (default `1s`): simulation tick interval.
- `--accelerated` / `NBI_ACCELERATED` (default `true`): run ticks in accelerated mode.
- Tracing: `NBI_TRACING_ENABLED` (`true|false`), `NBI_TRACING_EXPORTER` (`stdout|otlp`), `NBI_TRACING_SERVICE_NAME`, `NBI_TRACING_SAMPLE_RATIO` (`0-1`), `NBI_OTLP_ENDPOINT` (for OTLP exporter).

## Running locally
```sh
go run ./cmd/nbi-server \
  --listen-address=:50051 \
  --metrics-address=:9090 \
  --transceivers=configs/transceivers.json \
  --network-scenario=configs/network_scenario.json \
  --log-level=info
```
Logs are emitted to stdout/stderr. Metrics are exposed on `/metrics` over HTTP if `--metrics-address` is set.

## Docker
Build the image from the repo root:
```sh
docker build -t nbi-server:local .
```

Run the container, publishing gRPC and metrics ports:
```sh
docker run --rm -p 50051:50051 -p 9090:9090 nbi-server:local \
  --listen-address=0.0.0.0:50051 \
  --metrics-address=:9090 \
  --log-level=info
```
Pass scenario files or TLS material via bind mounts or environment variables, e.g. `-v %cd%/configs:/configs` and `-e NBI_NETWORK_SCENARIO=/configs/network_scenario.json`.

## Metrics and health
- Prometheus metrics: scrape `http://<metrics-address>/metrics` (default `http://localhost:9090/metrics`).
- Tracing: enable with `NBI_TRACING_ENABLED=true` and configure exporters as needed.

## Notes and assumptions
- Knowledge bases are in-memory; there is no persistent storage for scenarios or state.
- The server is intended to be driven by NBI clients (including the textproto loader) rather than a built-in UI.
- Default assets (transceivers and example scenarios) are copied into the container under `/configs`.
