package observability

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TracingConfig governs how NBI tracing is initialised.
type TracingConfig struct {
	Enabled     bool
	ServiceName string
	Exporter    string // stdout | otlp
	Endpoint    string // used when Exporter == otlp
	SampleRatio float64
}

// TracingConfigFromEnv pulls tracing configuration from environment variables,
// using sensible defaults when unset.
func TracingConfigFromEnv() TracingConfig {
	enabled := strings.EqualFold(os.Getenv("NBI_TRACING_ENABLED"), "true")
	exporter := strings.ToLower(os.Getenv("NBI_TRACING_EXPORTER"))
	if exporter == "" {
		exporter = "stdout"
	}
	service := os.Getenv("NBI_TRACING_SERVICE_NAME")
	if service == "" {
		service = "nbi-grpc"
	}

	ratio := 1.0
	if rawRatio := os.Getenv("NBI_TRACING_SAMPLE_RATIO"); rawRatio != "" {
		if parsed, err := strconv.ParseFloat(rawRatio, 64); err == nil && parsed >= 0 && parsed <= 1 {
			ratio = parsed
		}
	}

	return TracingConfig{
		Enabled:     enabled,
		ServiceName: service,
		Exporter:    exporter,
		Endpoint:    os.Getenv("NBI_OTLP_ENDPOINT"),
		SampleRatio: ratio,
	}
}

// InitTracing wires a tracer provider, exporter, propagators, and sampler based
// on the provided configuration. It returns a shutdown function to flush spans.
func InitTracing(ctx context.Context, cfg TracingConfig, log logging.Logger) (func(context.Context) error, error) {
	if log == nil {
		log = logging.Noop()
	}

	if !cfg.Enabled {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		otel.SetTextMapPropagator(propagation.TraceContext{})
		log.Info(ctx, "tracing disabled; using noop tracer provider")
		return func(context.Context) error { return nil }, nil
	}

	exp, err := exporterFromConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.namespace", "nbi"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	log.Info(ctx, "tracing enabled",
		logging.String("exporter", cfg.Exporter),
		logging.String("service_name", cfg.ServiceName),
		logging.String("sampler", fmt.Sprintf("parentbased_traceidratio_%0.2f", cfg.SampleRatio)),
	)

	return tp.Shutdown, nil
}

func exporterFromConfig(ctx context.Context, cfg TracingConfig) (sdktrace.SpanExporter, error) {
	switch strings.ToLower(cfg.Exporter) {
	case "stdout", "":
		return stdouttrace.New(
			stdouttrace.WithWriter(os.Stdout),
			stdouttrace.WithPrettyPrint(),
			stdouttrace.WithoutTimestamps(),
		)
	case "otlp", "otlpgrpc":
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = "localhost:4317"
		}
		client := otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
		return otlptrace.New(ctx, client)
	default:
		return nil, fmt.Errorf("unsupported tracing exporter: %s", cfg.Exporter)
	}
}

// ShutdownWithTimeout invokes the provided shutdown function with a bounded
// timeout, swallowing errors in the shutdown path.
func ShutdownWithTimeout(ctx context.Context, shutdown func(context.Context) error, log logging.Logger) {
	if shutdown == nil {
		return
	}
	if log == nil {
		log = logging.Noop()
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		log.Warn(ctx, "tracing shutdown failed", logging.String("error", err.Error()))
	}
}
