## Observability

The NBI surface now exports Prometheus-friendly metrics to help track RPC load, latency, and scenario size. Metrics are registered on the standard Prometheus registry and exposed via `/metrics` (served by `cmd/nbi-server` on `--metrics-addr`, default `:9090`).

### Metric names
- `nbi_requests_total{service,method,code}` – counter incremented on every unary RPC completion.
- `nbi_request_duration_seconds{service,method}` - histogram recording RPC latency in seconds (sub-second buckets up to 10s).
- `scenario_platforms` / `scenario_nodes` / `scenario_links` / `scenario_service_requests` - gauges set from `ScenarioState` mutators after each change.

### Scraping
Point Prometheus at the metrics endpoint exposed by the NBI binary, for example:

```yaml
- job_name: constellation-nbi
  metrics_path: /metrics
  static_configs:
    - targets: ['localhost:9090']
```

### Example PromQL
- QPS per service: `sum(rate(nbi_requests_total[5m])) by (service)`
- p95 latency per method: `histogram_quantile(0.95, sum(rate(nbi_request_duration_seconds_bucket[5m])) by (le,method))`
- Scenario entity counts: `scenario_platforms`, `scenario_nodes`, `scenario_links`, `scenario_service_requests`

### Scheduler observability
- `scheduler_path_computation_duration_seconds` – histogram recorded each time the scheduler computes a new multi-hop path.
- `scheduler_service_requests_queued` – gauge of pending service requests awaiting scheduling.
- `scheduler_preemptions_total` – counter incremented whenever a lower-priority request is preempted.
- `scheduler_contact_window_cache_hit_ratio` – gauge between 0 and 1 describing cache effectiveness.
These metrics are registered via `internal/observability/scheduler_metrics.go` and served alongside the standard Prometheus registry. Use them to build dashboards that highlight planning latency, queue pressure, and cache behavior.

### Debug endpoints and telemetry aggregation
`cmd/nbi-server` now exposes `/debug/scheduler/state` and `/debug/scheduler/decisions` on the same port as `/metrics`. These endpoints return JSON containing:
- scheduler snapshots (`Scheduler.SnapshotDebugState`, `Scheduler.SnapshotDecisions`) for inspection of current beams/routes and recent preemptions.
- aggregated telemetry summaries produced by `TelemetryState.AggregateInterfaceMetricsByNode` and `TelemetryState.AggregateModemMetricsByNode`, so UI clients can see per-node throughput/SNR without re-aggregating.

The telemetry aggregator functions compute node-level totals (interfaces up/down, bytes, average SNR) and modem summaries (counts, throughput, latest timestamp). They are safe for concurrent access and are used both by the `/debug` handlers and anywhere else you need fleet-wide views.

### Tracing
The NBI gRPC server can emit OpenTelemetry traces for each RPC and a few key internal operations (scenario loads, platform/node/link/service request mutations). Tracing is optional and disabled by default.

**Enabling tracing**
- Set `NBI_TRACING_ENABLED=true` when starting `cmd/nbi-server`.
- `NBI_TRACING_EXPORTER` controls where spans go: `stdout` (default) pretty-prints to stdout, `otlp` exports to an OTLP collector (set `NBI_OTLP_ENDPOINT`, default `localhost:4317`). Sampling defaults to `1.0`; override with `NBI_TRACING_SAMPLE_RATIO` (0.0–1.0). `NBI_TRACING_SERVICE_NAME` customises the service.name resource value (default `nbi-grpc`).

**Span shape and attributes**
- Unary interceptor creates/renames a server span per RPC: `NBI/<service>/<method>`. Attributes include `rpc.system=grpc`, `rpc.service`, `rpc.method`, `rpc.full_method`, and `request_id` (from the existing logging correlation ID).
- Child spans are created around ScenarioState mutations (platform/node/link/service request CRUD, scenario clear/load) with `entity_type`/`entity_id` attributes when available.
- Logs produced via `logging.WithRequestLogger` now include `trace_id`, `span_id`, and `request_id` to make correlating logs/traces easier.

**Viewing traces**
- With `NBI_TRACING_EXPORTER=stdout`, spans are printed to the server stdout stream—useful for local debugging.
- With `NBI_TRACING_EXPORTER=otlp`, point `NBI_OTLP_ENDPOINT` at a collector (e.g., Jaeger/Tempo ingest). Use your collector UI to explore traces; RPC spans will include method names and entity identifiers for context.
