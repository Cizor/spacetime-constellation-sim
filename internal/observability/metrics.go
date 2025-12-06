package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// NBICollector bundles Prometheus metrics for the NBI surface and provides
// helpers to wire them into gRPC servers and HTTP handlers.
type NBICollector struct {
	gatherer prometheus.Gatherer

	RPCRequests  *prometheus.CounterVec
	RPCDurations *prometheus.HistogramVec

	ScenarioPlatforms       prometheus.Gauge
	ScenarioNodes           prometheus.Gauge
	ScenarioLinks           prometheus.Gauge
	ScenarioServiceRequests prometheus.Gauge
}

// NewNBICollector registers NBI Prometheus metrics against the provided
// registerer, defaulting to the global Prometheus registry when nil.
func NewNBICollector(reg prometheus.Registerer) (*NBICollector, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	gatherer := prometheus.DefaultGatherer
	if g, ok := reg.(prometheus.Gatherer); ok {
		gatherer = g
	}

	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nbi_requests_total",
		Help: "Total number of handled NBI RPCs, labeled by service, method, and gRPC status code.",
	}, []string{"service", "method", "code"})
	requests, err := registerCounterVec(reg, requests, "nbi_requests_total")
	if err != nil {
		return nil, err
	}

	durations := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nbi_request_duration_seconds",
		Help:    "NBI RPC latency in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	}, []string{"service", "method"})
	durations, err = registerHistogramVec(reg, durations, "nbi_request_duration_seconds")
	if err != nil {
		return nil, err
	}

	platforms, err := registerGauge(reg, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "scenario_platforms",
		Help: "Current number of platforms in ScenarioState.",
	}), "scenario_platforms")
	if err != nil {
		return nil, err
	}
	nodes, err := registerGauge(reg, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "scenario_nodes",
		Help: "Current number of network nodes in ScenarioState.",
	}), "scenario_nodes")
	if err != nil {
		return nil, err
	}
	links, err := registerGauge(reg, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "scenario_links",
		Help: "Current number of network links in ScenarioState.",
	}), "scenario_links")
	if err != nil {
		return nil, err
	}
	serviceRequests, err := registerGauge(reg, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "scenario_service_requests",
		Help: "Current number of active service requests in ScenarioState.",
	}), "scenario_service_requests")
	if err != nil {
		return nil, err
	}

	return &NBICollector{
		gatherer:                gatherer,
		RPCRequests:             requests,
		RPCDurations:            durations,
		ScenarioPlatforms:       platforms,
		ScenarioNodes:           nodes,
		ScenarioLinks:           links,
		ScenarioServiceRequests: serviceRequests,
	}, nil
}

// UnaryServerInterceptor records request counts and durations for unary RPCs.
func (c *NBICollector) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)

		if c == nil {
			return resp, err
		}

		fullMethod := ""
		if info != nil {
			fullMethod = info.FullMethod
		}
		service, method := SplitMethod(fullMethod)
		code := status.Code(err).String()

		if c.RPCRequests != nil {
			c.RPCRequests.WithLabelValues(service, method, code).Inc()
		}
		if c.RPCDurations != nil {
			c.RPCDurations.WithLabelValues(service, method).Observe(time.Since(start).Seconds())
		}

		return resp, err
	}
}

// Handler exposes a ready-to-use /metrics handler.
func (c *NBICollector) Handler() http.Handler {
	gatherer := c.gatherer
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

// SetScenarioCounts satisfies the ScenarioMetricsRecorder interface so the
// ScenarioState can drive gauge values directly from its mutators.
func (c *NBICollector) SetScenarioCounts(platforms, nodes, links, serviceRequests int) {
	if c == nil {
		return
	}
	if c.ScenarioPlatforms != nil {
		c.ScenarioPlatforms.Set(float64(platforms))
	}
	if c.ScenarioNodes != nil {
		c.ScenarioNodes.Set(float64(nodes))
	}
	if c.ScenarioLinks != nil {
		c.ScenarioLinks.Set(float64(links))
	}
	if c.ScenarioServiceRequests != nil {
		c.ScenarioServiceRequests.Set(float64(serviceRequests))
	}
}

// SplitMethod parses a fully-qualified gRPC method name into service and method
// components. It tolerates empty strings and partial paths, returning
// "unknown"/"unknown" when parsing fails.
func SplitMethod(fullMethod string) (string, string) {
	if fullMethod == "" {
		return "unknown", "unknown"
	}
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	parts := strings.Split(fullMethod, "/")
	if len(parts) < 2 {
		return "unknown", "unknown"
	}
	service := parts[len(parts)-2]
	method := parts[len(parts)-1]
	if dot := strings.LastIndex(service, "."); dot >= 0 && dot+1 < len(service) {
		service = service[dot+1:]
	}
	if service == "" {
		service = "unknown"
	}
	if method == "" {
		method = "unknown"
	}
	return service, method
}

func registerCounterVec(reg prometheus.Registerer, vec *prometheus.CounterVec, name string) (*prometheus.CounterVec, error) {
	if err := reg.Register(vec); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing, nil
			}
			return nil, fmt.Errorf("collector %s already registered with incompatible type", name)
		}
		return nil, err
	}
	return vec, nil
}

func registerHistogramVec(reg prometheus.Registerer, vec *prometheus.HistogramVec, name string) (*prometheus.HistogramVec, error) {
	if err := reg.Register(vec); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing, nil
			}
			return nil, fmt.Errorf("collector %s already registered with incompatible type", name)
		}
		return nil, err
	}
	return vec, nil
}

func registerGauge(reg prometheus.Registerer, gauge prometheus.Gauge, name string) (prometheus.Gauge, error) {
	if err := reg.Register(gauge); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(prometheus.Gauge); ok {
				return existing, nil
			}
			return nil, fmt.Errorf("collector %s already registered with incompatible type", name)
		}
		return nil, err
	}
	return gauge, nil
}
