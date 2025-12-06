package nbi

import (
	"context"
	"fmt"
	"strings"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

const tracerName = "github.com/signalsfoundry/constellation-simulator/internal/nbi"

// TracingUnaryServerInterceptor enriches RPC spans with standard attributes and
// ensures a server span exists when tracing interceptors are not configured.
func TracingUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	tracer := otel.Tracer(tracerName)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		service, method := observability.SplitMethod(info.FullMethod)
		span := trace.SpanFromContext(ctx)
		created := false
		if !span.SpanContext().IsValid() {
			spanName := fmt.Sprintf("NBI/%s/%s", service, method)
			ctx, span = tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
			created = true
		} else {
			span.SetName(fmt.Sprintf("NBI/%s/%s", service, method))
		}

		attrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", service),
			attribute.String("rpc.method", method),
			attribute.String("rpc.full_method", strings.TrimPrefix(info.FullMethod, "/")),
		}
		if reqID := logging.RequestIDFromContext(ctx); reqID != "" {
			attrs = append(attrs, attribute.String("request_id", reqID))
		}
		span.SetAttributes(attrs...)

		resp, err := handler(ctx, req)
		if err != nil {
			span.RecordError(err)
		}

		if created {
			span.End()
		}
		return resp, err
	}
}

// StartChildSpan starts a child span for internal operations within handlers.
// entityType and entityID are optional attributes to aid trace navigation.
func StartChildSpan(ctx context.Context, name, entityType, entityID string, extra ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)
	attrs := make([]attribute.KeyValue, 0, len(extra)+2)
	if entityType != "" {
		attrs = append(attrs, attribute.String("entity_type", entityType))
	}
	if entityID != "" {
		attrs = append(attrs, attribute.String("entity_id", entityID))
	}
	attrs = append(attrs, extra...)
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}
