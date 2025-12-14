// Package controller contains controller-side SBI logic including telemetry server.
package controller

import (
	"context"
	"time"

	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const nodeIDMetadataKey = "x-node-id"

// TelemetryServer implements the TelemetryService gRPC server.
// It receives metrics from agents and updates TelemetryState.
type TelemetryServer struct {
	telemetrypb.UnimplementedTelemetryServer

	Telemetry *state.TelemetryState
	log       logging.Logger
	Metrics   *sbi.SBIMetrics // optional metrics counter
}

// NewTelemetryServer creates a new TelemetryServer with the given TelemetryState.
func NewTelemetryServer(telemetry *state.TelemetryState, log logging.Logger) *TelemetryServer {
	if log == nil {
		log = logging.Noop()
	}
	return &TelemetryServer{
		Telemetry: telemetry,
		log:       log,
		Metrics:   nil, // optional, can be set after construction
	}
}

// ExportMetrics receives metrics from agents and updates TelemetryState.
// It extracts interface metrics from the proto and stores them internally.
func (s *TelemetryServer) ExportMetrics(
	ctx context.Context,
	req *telemetrypb.ExportMetricsRequest,
) (*emptypb.Empty, error) {
	if s.Telemetry == nil {
		return nil, status.Error(codes.FailedPrecondition, "telemetry state is not configured")
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	// Extract node_id from context metadata
	nodeID := extractNodeIDFromContext(ctx)

	processed := 0
	processedModem := 0
	for _, protoMetrics := range req.GetInterfaceMetrics() {
		if protoMetrics == nil {
			continue
		}

		interfaceID := ""
		if protoMetrics.InterfaceId != nil {
			interfaceID = *protoMetrics.InterfaceId
		}
		if interfaceID == "" {
			// Skip metrics without interface ID
			continue
		}

		// Extract operational state (up/down) from the latest data point
		up := false
		if len(protoMetrics.OperationalStateDataPoints) > 0 {
			latest := protoMetrics.OperationalStateDataPoints[len(protoMetrics.OperationalStateDataPoints)-1]
			if latest.Value != nil && *latest.Value == telemetrypb.IfOperStatus_IF_OPER_STATUS_UP {
				up = true
			}
		}

		// Extract byte counters from the latest statistics data point
		var bytesTx, bytesRx uint64
		if len(protoMetrics.StandardInterfaceStatisticsDataPoints) > 0 {
			latest := protoMetrics.StandardInterfaceStatisticsDataPoints[len(protoMetrics.StandardInterfaceStatisticsDataPoints)-1]
			if latest.TxBytes != nil {
				bytesTx = uint64(*latest.TxBytes)
			}
			if latest.RxBytes != nil {
				bytesRx = uint64(*latest.RxBytes)
			}
		}

		// Create internal metrics
		metrics := &state.InterfaceMetrics{
			NodeID:      nodeID,
			InterfaceID: interfaceID,
			Up:          up,
			BytesTx:     bytesTx,
			BytesRx:     bytesRx,
			// SNRdB and Modulation can be extracted from ModemMetrics if needed later
		}

		s.Telemetry.UpdateMetrics(metrics)
		processed++
	}

	if processed > 0 {
		s.log.Debug(ctx, "exported metrics",
			logging.Int("count", processed),
		)
	}
	for _, protoModem := range req.GetModemMetrics() {
		if protoModem == nil {
			continue
		}

		demodID := protoModem.GetDemodulatorId()
		if demodID == "" {
			continue
		}

		var sinr float64
		var modulation string
		var timestamp time.Time
		if points := protoModem.GetSinrDataPoints(); len(points) > 0 {
			if latest := points[len(points)-1]; latest != nil {
				if latest.SinrDb != nil {
					sinr = *latest.SinrDb
				}
				if latest.ModulatorId != nil {
					modulation = *latest.ModulatorId
				}
				if latest.Time != nil {
					timestamp = latest.Time.AsTime()
				}
			}
		}
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		metrics := &state.ModemMetrics{
			NodeID:      nodeID,
			InterfaceID: demodID,
			SNRdB:       sinr,
			Modulation:  modulation,
			Timestamp:   timestamp,
		}
		if err := s.Telemetry.UpdateModemMetrics(metrics); err != nil {
			s.log.Error(ctx, "telemetry: failed to store modem metrics",
				logging.String("interface", demodID),
				logging.Any("error", err),
			)
			continue
		}
		processedModem++
	}
	if processedModem > 0 {
		s.log.Debug(ctx, "exported modem metrics",
			logging.Int("count", processedModem),
		)
	}

	// Increment metrics counter for each ExportMetrics call
	if s.Metrics != nil {
		s.Metrics.IncTelemetryReports()
	}

	return &emptypb.Empty{}, nil
}

// extractNodeIDFromContext attempts to extract node_id from gRPC context metadata.
// Returns empty string if not found. Agents should include node_id in metadata
// when calling ExportMetrics.
func extractNodeIDFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(nodeIDMetadataKey)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}
