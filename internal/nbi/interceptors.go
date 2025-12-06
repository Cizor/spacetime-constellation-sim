package nbi

import (
	"context"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const requestIDMetadataKey = "x-request-id"

// RequestIDUnaryServerInterceptor ensures a request_id is present on the
// context, sourcing it from inbound metadata if provided, and attaches a
// per-request logger annotated with request_id and method.
func RequestIDUnaryServerInterceptor(base logging.Logger) grpc.UnaryServerInterceptor {
	if base == nil {
		base = logging.Noop()
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if incoming := firstHeader(md, requestIDMetadataKey); incoming != "" {
				ctx = logging.ContextWithRequestID(ctx, incoming)
			}
		}

		ctx, reqLog := logging.WithRequestLogger(ctx, base.With(logging.String("method", info.FullMethod)))
		ctx = logging.ContextWithLogger(ctx, reqLog)

		return handler(ctx, req)
	}
}

func firstHeader(md metadata.MD, key string) string {
	if md == nil {
		return ""
	}
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}
