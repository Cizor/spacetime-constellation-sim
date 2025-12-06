// internal/nbi/servicerequest_service.go
package nbi

import (
	"context"
	"fmt"
	"time"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ServiceRequestService implements the ServiceRequestService gRPC server backed
// by a ScenarioState instance.
type ServiceRequestService struct {
	v1alpha.UnimplementedServiceRequestServiceServer

	state *sim.ScenarioState
	log   logging.Logger
}

// NewServiceRequestService constructs a ServiceRequestService bound to ScenarioState.
func NewServiceRequestService(state *sim.ScenarioState, log logging.Logger) *ServiceRequestService {
	if log == nil {
		log = logging.Noop()
	}
	return &ServiceRequestService{
		state: state,
		log:   log,
	}
}

// CreateServiceRequest stores a new ServiceRequest after validation.
func (s *ServiceRequestService) CreateServiceRequest(
	ctx context.Context,
	in *resources.ServiceRequest,
) (*resources.ServiceRequest, error) {
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "service_request"),
		logging.String("operation", "create"),
	)
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if err := ValidateServiceRequestProto(in); err != nil {
		reqLog.Debug(ctx, "CreateServiceRequest validation failed",
			logging.String("reason", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	dom, err := types.ServiceRequestFromProto(in)
	if err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	}

	// ID behaviour:
	// For now we overload the `type` field on the proto as the stable ID used
	// by the simulator's internal model. If the caller omits it, we generate
	// one and mirror it back into the response.
	if id := in.GetType(); id != "" {
		dom.ID = id
	} else {
		dom.ID = generateServiceRequestID()
	}

	if err := s.validateServiceRequest(dom); err != nil {
		return nil, ToStatusError(err)
	}

	if err := s.state.CreateServiceRequest(dom); err != nil {
		reqLog.Warn(ctx, "CreateServiceRequest failed",
			logging.String("entity_id", dom.ID),
			logging.String("error", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	reqLog.Info(ctx, "service request created",
		logging.String("entity_id", dom.ID),
		logging.String("src_node_id", dom.SrcNodeID),
		logging.String("dst_node_id", dom.DstNodeID),
	)

	return attachServiceRequestID(types.ServiceRequestToProto(dom), dom.ID), nil
}

// GetServiceRequest retrieves a ServiceRequest by ID.
func (s *ServiceRequestService) GetServiceRequest(
	ctx context.Context,
	req *v1alpha.GetServiceRequestRequest,
) (*resources.ServiceRequest, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetServiceRequestId() == "" {
		return nil, ToStatusError(fmt.Errorf("%w: service_request_id is required", ErrInvalidServiceRequest))
	}

	sr, err := s.state.GetServiceRequest(req.GetServiceRequestId())
	if err != nil {
		return nil, ToStatusError(err)
	}

	return attachServiceRequestID(types.ServiceRequestToProto(sr), sr.ID), nil
}

// ListServiceRequests returns all stored ServiceRequests.
func (s *ServiceRequestService) ListServiceRequests(
	ctx context.Context,
	_ *v1alpha.ListServiceRequestsRequest,
) (*v1alpha.ListServiceRequestsResponse, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	resp := &v1alpha.ListServiceRequestsResponse{}
	for _, sr := range s.state.ListServiceRequests() {
		resp.ServiceRequests = append(resp.ServiceRequests, attachServiceRequestID(
			types.ServiceRequestToProto(sr),
			sr.ID,
		))
	}
	return resp, nil
}

// UpdateServiceRequest replaces an existing ServiceRequest entry.
func (s *ServiceRequestService) UpdateServiceRequest(
	ctx context.Context,
	req *v1alpha.UpdateServiceRequestRequest,
) (*resources.ServiceRequest, error) {
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "service_request"),
		logging.String("operation", "update"),
	)
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetServiceRequest() == nil {
		return nil, ToStatusError(fmt.Errorf("%w: service_request is required", ErrInvalidServiceRequest))
	}

	if err := ValidateServiceRequestProto(req.GetServiceRequest()); err != nil {
		reqLog.Debug(ctx, "UpdateServiceRequest validation failed",
			logging.String("reason", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	dom, err := types.ServiceRequestFromProto(req.GetServiceRequest())
	if err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	}

	// For now we continue the convention that the proto's `type` field carries
	// the stable ID for CRUD operations.
	dom.ID = req.GetServiceRequest().GetType()
	if dom.ID == "" {
		return nil, ToStatusError(fmt.Errorf("%w: service_request_id is required", ErrInvalidServiceRequest))
	}

	existing, err := s.state.GetServiceRequest(dom.ID)
	if err != nil {
		return nil, ToStatusError(err)
	}
	if existing.ID != dom.ID {
		return nil, ToStatusError(fmt.Errorf("%w: service_request_id cannot be changed", ErrInvalidServiceRequest))
	}

	if err := s.validateServiceRequest(dom); err != nil {
		return nil, ToStatusError(err)
	}

	if err := s.state.UpdateServiceRequest(dom); err != nil {
		reqLog.Warn(ctx, "UpdateServiceRequest failed",
			logging.String("entity_id", dom.ID),
			logging.String("error", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	reqLog.Info(ctx, "service request updated",
		logging.String("entity_id", dom.ID),
	)

	return attachServiceRequestID(types.ServiceRequestToProto(dom), dom.ID), nil
}

// DeleteServiceRequest removes a ServiceRequest by ID.
func (s *ServiceRequestService) DeleteServiceRequest(
	ctx context.Context,
	req *v1alpha.DeleteServiceRequestRequest,
) (*emptypb.Empty, error) {
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "service_request"),
		logging.String("operation", "delete"),
	)
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetServiceRequestId() == "" {
		return nil, status.Error(codes.InvalidArgument, "service_request_id is required")
	}

	if err := s.state.DeleteServiceRequest(req.GetServiceRequestId()); err != nil {
		reqLog.Warn(ctx, "DeleteServiceRequest failed",
			logging.String("entity_id", req.GetServiceRequestId()),
			logging.String("error", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	reqLog.Info(ctx, "service request deleted",
		logging.String("entity_id", req.GetServiceRequestId()),
	)

	return &emptypb.Empty{}, nil
}

// ensureReady verifies the service has been constructed correctly.
func (s *ServiceRequestService) ensureReady() error {
	if s == nil || s.state == nil {
		return status.Error(codes.FailedPrecondition, "scenario state is not configured")
	}
	return nil
}

// validateServiceRequest performs referential integrity checks and ensures an ID is set.
func (s *ServiceRequestService) validateServiceRequest(sr *model.ServiceRequest) error {
	if sr == nil {
		return fmt.Errorf("%w: service request is required", ErrInvalidServiceRequest)
	}
	if sr.ID == "" {
		return fmt.Errorf("%w: service_request_id is required", ErrInvalidServiceRequest)
	}

	return s.state.WithReadLock(func() error {
		phys := s.state.PhysicalKB()
		if phys.GetNetworkNode(sr.SrcNodeID) == nil {
			return fmt.Errorf("%w: unknown src node %q", ErrInvalidServiceRequest, sr.SrcNodeID)
		}
		if phys.GetNetworkNode(sr.DstNodeID) == nil {
			return fmt.Errorf("%w: unknown dst node %q", ErrInvalidServiceRequest, sr.DstNodeID)
		}
		return nil
	})
}

func generateServiceRequestID() string {
	return fmt.Sprintf("sr-%d", time.Now().UnixNano())
}

// attachServiceRequestID mirrors the internal ID onto the proto type field so
// callers can read the stable identifier until the proto grows an explicit ID.
func attachServiceRequestID(p *resources.ServiceRequest, id string) *resources.ServiceRequest {
	if p == nil {
		return nil
	}
	if id != "" {
		p.Type = &id
	}
	return p
}
