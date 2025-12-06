package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryInterceptorRecordsMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector, err := NewNBICollector(reg)
	if err != nil {
		t.Fatalf("NewNBICollector: %v", err)
	}

	interceptor := collector.UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/aalyria.spacetime.api.nbi.v1alpha.PlatformService/CreatePlatform"}

	_, err = interceptor(context.Background(), struct{}{}, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		time.Sleep(10 * time.Millisecond)
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("interceptor handler returned error: %v", err)
	}

	if got := testutil.ToFloat64(collector.RPCRequests.WithLabelValues("PlatformService", "CreatePlatform", "OK")); got != 1 {
		t.Fatalf("nbi_requests_total = %v, want 1", got)
	}

	if count := histogramSampleCount(t, reg, "nbi_request_duration_seconds", map[string]string{
		"service": "PlatformService",
		"method":  "CreatePlatform",
	}); count != 1 {
		t.Fatalf("nbi_request_duration_seconds sample_count = %d, want 1", count)
	}
}

func TestUnaryInterceptorRecordsErrorCode(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector, err := NewNBICollector(reg)
	if err != nil {
		t.Fatalf("NewNBICollector: %v", err)
	}

	interceptor := collector.UnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/aalyria.spacetime.api.nbi.v1alpha.NetworkNodeService/CreateNode"}

	_, _ = interceptor(context.Background(), struct{}{}, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.InvalidArgument, "boom")
	})

	if got := testutil.ToFloat64(collector.RPCRequests.WithLabelValues("NetworkNodeService", "CreateNode", "InvalidArgument")); got != 1 {
		t.Fatalf("nbi_requests_total error label = %v, want 1", got)
	}
}

func TestMetricsHandlerExposesScenarioGauges(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector, err := NewNBICollector(reg)
	if err != nil {
		t.Fatalf("NewNBICollector: %v", err)
	}
	collector.SetScenarioCounts(3, 4, 5, 6)
	collector.RPCRequests.WithLabelValues("svc", "method", "OK").Inc()
	collector.RPCDurations.WithLabelValues("svc", "method").Observe(0.01)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	collector.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, metric := range []string{
		"nbi_requests_total",
		"nbi_request_duration_seconds",
		"scenario_platforms",
		"scenario_nodes",
		"scenario_links",
		"scenario_service_requests",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("expected %q in /metrics output", metric)
		}
	}
	if !strings.Contains(body, "3") || !strings.Contains(body, "4") || !strings.Contains(body, "5") || !strings.Contains(body, "6") {
		t.Fatalf("/metrics output missing scenario gauge values: %s", body)
	}
}

func histogramSampleCount(t *testing.T, gatherer prometheus.Gatherer, name string, labels map[string]string) uint64 {
	t.Helper()

	metrics, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, mf := range metrics {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.Metric {
			if matchLabels(m.GetLabel(), labels) && m.GetHistogram() != nil {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func matchLabels(got []*dto.LabelPair, want map[string]string) bool {
	if len(got) < len(want) {
		return false
	}
	matched := 0
	for _, lp := range got {
		if val, ok := want[lp.GetName()]; ok && val == lp.GetValue() {
			matched++
		}
	}
	return matched == len(want)
}
