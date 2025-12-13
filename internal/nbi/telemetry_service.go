// Package nbi contains NBI gRPC service implementations.
package nbi

import (
	"context"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TelemetryService implements a read-only NBI service for accessing telemetry metrics.
// This provides a simple way for external tools to inspect interface state.
type TelemetryService struct {
	v1alpha.UnimplementedTelemetryServiceServer

	telemetry *sim.TelemetryState
	log       logging.Logger
}

// NewTelemetryService constructs a TelemetryService bound to TelemetryState.
func NewTelemetryService(telemetry *sim.TelemetryState, log logging.Logger) *TelemetryService {
	if log == nil {
		log = logging.Noop()
	}
	return &TelemetryService{
		telemetry: telemetry,
		log:       log,
	}
}

// ListInterfaceMetrics returns all interface metrics, optionally filtered by node_id and/or interface_id.
func (s *TelemetryService) ListInterfaceMetrics(
	ctx context.Context,
	req *v1alpha.ListInterfaceMetricsRequest,
) (*v1alpha.ListInterfaceMetricsResponse, error) {
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "telemetry"),
		logging.String("operation", "list"),
	)

	if s.telemetry == nil {
		return nil, status.Error(codes.Internal, "telemetry state not available")
	}

	all := s.telemetry.ListAll()

	var out []*v1alpha.InterfaceMetrics

	for _, m := range all {
		if m == nil {
			continue
		}

		// Apply filters
		if req != nil {
			if req.NodeId != nil && *req.NodeId != "" && *req.NodeId != m.NodeID {
				continue
			}
			if req.InterfaceId != nil && *req.InterfaceId != "" && *req.InterfaceId != m.InterfaceID {
				continue
			}
		}

		// Convert to proto (proto2 uses pointers)
		nodeID := m.NodeID
		ifaceID := m.InterfaceID
		up := m.Up
		bytesTx := m.BytesTx
		bytesRx := m.BytesRx

		out = append(out, &v1alpha.InterfaceMetrics{
			NodeId:      &nodeID,
			InterfaceId: &ifaceID,
			Up:          &up,
			BytesTx:     &bytesTx,
			BytesRx:     &bytesRx,
		})
	}

	reqLog.Debug(ctx, "ListInterfaceMetrics completed",
		logging.Int("count", len(out)),
	)

	return &v1alpha.ListInterfaceMetricsResponse{
		Metrics: out,
	}, nil
}

