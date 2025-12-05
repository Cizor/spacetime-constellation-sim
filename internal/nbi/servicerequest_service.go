// internal/nbi/servicerequest_service.go
package nbi

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
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
	log   Logger
}

// NewServiceRequestService constructs a ServiceRequestService bound to ScenarioState.
func NewServiceRequestService(state *sim.ScenarioState, log Logger) *ServiceRequestService {
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
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "service_request is required")
	}

	dom, err := types.ServiceRequestFromProto(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// The Aalyria ServiceRequest proto does not expose a dedicated ID field.
	// For now we use the `type` string as a stable identifier for CRUD
	// operations, and generate one if it is absent.
	dom.ID = in.GetType()
	if dom.ID == "" {
		dom.ID = generateServiceRequestID()
	}

	if err := s.validateServiceRequest(dom); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.state.CreateServiceRequest(dom); err != nil {
		switch {
		case errors.Is(err, sim.ErrServiceRequestExists):
			return nil, status.Error(codes.AlreadyExists, err.Error())
		default:
			if s.log != nil {
				s.log.Errorw("CreateServiceRequest failed", "err", err)
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

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
		return nil, status.Error(codes.InvalidArgument, "service_request_id is required")
	}

	sr, err := s.state.GetServiceRequest(req.GetServiceRequestId())
	if err != nil {
		if errors.Is(err, sim.ErrServiceRequestNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
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
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetServiceRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "service_request is required")
	}

	dom, err := types.ServiceRequestFromProto(req.GetServiceRequest())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// ID is carried via the proto's `type` field for now.
	dom.ID = req.GetServiceRequest().GetType()
	if dom.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "service_request_id is required")
	}

	existing, err := s.state.GetServiceRequest(dom.ID)
	if err != nil {
		if errors.Is(err, sim.ErrServiceRequestNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	// If in future we have a separate ID path parameter, enforce that it
	// matches the existing ID; for now this is just a sanity check.
	if existing.ID != dom.ID {
		return nil, status.Error(codes.InvalidArgument, "service_request_id cannot be changed")
	}

	if err := s.validateServiceRequest(dom); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.state.UpdateServiceRequest(dom); err != nil {
		if errors.Is(err, sim.ErrServiceRequestNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return attachServiceRequestID(types.ServiceRequestToProto(dom), dom.ID), nil
}

// DeleteServiceRequest removes a ServiceRequest by ID.
func (s *ServiceRequestService) DeleteServiceRequest(
	ctx context.Context,
	req *v1alpha.DeleteServiceRequestRequest,
) (*emptypb.Empty, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetServiceRequestId() == "" {
		return nil, status.Error(codes.InvalidArgument, "service_request_id is required")
	}

	if err := s.state.DeleteServiceRequest(req.GetServiceRequestId()); err != nil {
		if errors.Is(err, sim.ErrServiceRequestNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

// ensureReady verifies the service has been constructed correctly.
func (s *ServiceRequestService) ensureReady() error {
	if s == nil || s.state == nil {
		return status.Error(codes.FailedPrecondition, "scenario state is not configured")
	}
	return nil
}

// validateServiceRequest performs basic sanity checks and referential integrity.
func (s *ServiceRequestService) validateServiceRequest(sr *model.ServiceRequest) error {
	if sr == nil {
		return errors.New("service request is required")
	}
	if sr.ID == "" {
		return errors.New("service_request_id is required")
	}
	if sr.SrcNodeID == "" || sr.DstNodeID == "" {
		return errors.New("src_node_id and dst_node_id are required")
	}

	phys := s.state.PhysicalKB()
	if phys.GetNetworkNode(sr.SrcNodeID) == nil {
		return fmt.Errorf("%w: %q", sim.ErrNodeNotFound, sr.SrcNodeID)
	}
	if phys.GetNetworkNode(sr.DstNodeID) == nil {
		return fmt.Errorf("%w: %q", sim.ErrNodeNotFound, sr.DstNodeID)
	}

	if len(sr.FlowRequirements) == 0 {
		return errors.New("at least one flow requirement is required")
	}
	for i, fr := range sr.FlowRequirements {
		if fr.RequestedBandwidth < 0 {
			return fmt.Errorf("flow requirement %d requested bandwidth cannot be negative", i)
		}
		if fr.MinBandwidth < 0 {
			return fmt.Errorf("flow requirement %d minimum bandwidth cannot be negative", i)
		}
		if fr.MaxLatency < 0 {
			return fmt.Errorf("flow requirement %d latency cannot be negative", i)
		}
		if !fr.ValidFrom.IsZero() && !fr.ValidTo.IsZero() && fr.ValidTo.Before(fr.ValidFrom) {
			return fmt.Errorf("flow requirement %d has invalid time interval: end before start", i)
		}
	}

	return nil
}

func generateServiceRequestID() string {
	return fmt.Sprintf("sr-%d", time.Now().UnixNano())
}

// attachServiceRequestID mirrors the internal ID onto the proto `type` field so
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
