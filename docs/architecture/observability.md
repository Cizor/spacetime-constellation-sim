## Observability

The NBI surface now exports Prometheus-friendly metrics to help track RPC load, latency, and scenario size. Metrics are registered on the standard Prometheus registry and exposed via `/metrics` (served by `cmd/nbi-server` on `--metrics-addr`, default `:9090`).

### Metric names
- `nbi_requests_total{service,method,code}` – counter incremented on every unary RPC completion.
- `nbi_request_duration_seconds{service,method}` – histogram recording RPC latency in seconds (sub-second buckets up to 10s).
- `scenario_platforms` / `scenario_nodes` / `scenario_links` / `scenario_service_requests` – gauges set from `ScenarioState` mutators after each change.

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
