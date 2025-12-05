// internal/nbi/scenario_service.go
package nbi

import (
	"context"
	"os"
	"sort"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ScenarioService implements the ScenarioService gRPC server backed by a
// ScenarioState instance.
//
// Semantics:
//   - ClearScenario delegates directly to ScenarioState.ClearScenario().
//   - GetScenario maps the in-memory ScenarioState snapshot into the NBI
//     ScenarioSnapshot proto, reusing existing mapping helpers.
//   - LoadScenario ALWAYS clears the scenario first, then bulk-loads
//     platforms, nodes (with interfaces), links, and service requests in a
//     deterministic order. If any step fails, it attempts to roll back to an
//     empty scenario via ClearScenario.
type ScenarioService struct {
	v1alpha.UnimplementedScenarioServiceServer

	state *sim.ScenarioState
	log   Logger
}

// NewScenarioService constructs a ScenarioService bound to ScenarioState.
func NewScenarioService(state *sim.ScenarioState, log Logger) *ScenarioService {
	return &ScenarioService{
		state: state,
		log:   log,
	}
}

// ClearScenario delegates to ScenarioState.ClearScenario and maps unexpected
// errors to Internal status.
func (s *ScenarioService) ClearScenario(
	ctx context.Context,
	_ *v1alpha.ClearScenarioRequest,
) (*emptypb.Empty, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	if err := s.state.ClearScenario(); err != nil {
		if s.log != nil {
			s.log.Errorw("ClearScenario failed", "err", err)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &emptypb.Empty{}, nil
}

// GetScenario returns a snapshot of the current in-memory scenario using the
// NBI proto surface.
func (s *ScenarioService) GetScenario(
	ctx context.Context,
	_ *v1alpha.GetScenarioRequest,
) (*v1alpha.ScenarioSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	snap := s.state.Snapshot()
	if snap == nil {
		return &v1alpha.ScenarioSnapshot{}, nil
	}

	resp := &v1alpha.ScenarioSnapshot{}

	// Platforms
	for _, pd := range sortDomainPlatforms(snap.Platforms) {
		if pd == nil {
			continue
		}
		resp.Platforms = append(resp.Platforms, types.PlatformToProto(pd))
	}

	// Nodes + embedded interfaces
	for _, node := range sortDomainNodes(snap.Nodes) {
		if node == nil {
			continue
		}
		resp.Nodes = append(resp.Nodes, types.NodeToProtoWithInterfaces(
			node,
			snap.InterfacesByNode[node.ID],
		))
	}

	// Links (group internal NetworkLink objects into BidirectionalLink protos)
	resp.Links = append(resp.Links, groupBidirectionalLinks(snap.Links)...)

	// ServiceRequests
	for _, sr := range sortDomainServiceRequests(snap.ServiceRequests) {
		if sr == nil {
			continue
		}
		resp.ServiceRequests = append(
			resp.ServiceRequests,
			attachServiceRequestID(types.ServiceRequestToProto(sr), sr.ID),
		)
	}

	return resp, nil
}

// LoadScenario clears existing state, optionally loads entities from a
// textproto, and bulk-loads inline scenario entities in a deterministic order.
//
// Semantics:
//   - If scenario_textproto_path is set, it is read and parsed into a
//     LoadScenarioRequest, which then serves as the payload.
//   - Otherwise, the incoming request's inline fields are used.
//   - The load order is fixed for reproducibility:
//     1) Platforms
//     2) Nodes (with interfaces)
//     3) Links
//     4) ServiceRequests
//   - LoadScenario ALWAYS calls ClearScenario first. If any step fails,
//     the method attempts a best-effort rollback by calling ClearScenario
//     again before returning an error.
func (s *ScenarioService) LoadScenario(
	ctx context.Context,
	req *v1alpha.LoadScenarioRequest,
) (_ *emptypb.Empty, retErr error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	payload, err := s.resolveLoadPayload(req)
	if err != nil {
		return nil, err
	}

	// Recommended semantics: always clear first before loading a scenario.
	if err := s.state.ClearScenario(); err != nil {
		if s.log != nil {
			s.log.Errorw("LoadScenario ClearScenario failed", "err", err)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// If any step below fails, try to leave the simulator in a clean empty
	// state by clearing again.
	defer func() {
		if retErr != nil {
			if err := s.state.ClearScenario(); err != nil && s.log != nil {
				s.log.Warnw("failed to roll back scenario after load error", "err", err)
			}
		}
	}()

	// Reuse existing services so we exercise the same NBI mapping and
	// validation logic as normal API calls.
	platSvc := NewPlatformService(s.state, nil, s.log)
	nodeSvc := NewNetworkNodeService(s.state, s.log)
	linkSvc := NewNetworkLinkService(s.state, s.log)
	srSvc := NewServiceRequestService(s.state, s.log)

	// 1) Platforms
	for _, pd := range sortProtoPlatforms(payload.GetPlatforms()) {
		if _, err := platSvc.CreatePlatform(ctx, pd); err != nil {
			retErr = err
			return nil, retErr
		}
	}

	// 2) Nodes (with interfaces)
	for _, node := range sortProtoNodes(payload.GetNodes()) {
		if _, err := nodeSvc.CreateNode(ctx, node); err != nil {
			retErr = err
			return nil, retErr
		}
	}

	// 3) Links
	for _, link := range sortLinks(payload.GetLinks()) {
		if _, err := linkSvc.CreateLink(ctx, link); err != nil {
			retErr = err
			return nil, retErr
		}
	}

	// 4) ServiceRequests
	for _, sr := range sortProtoServiceRequests(payload.GetServiceRequests()) {
		if _, err := srSvc.CreateServiceRequest(ctx, sr); err != nil {
			retErr = err
			return nil, retErr
		}
	}

	return &emptypb.Empty{}, nil
}

// resolveLoadPayload chooses between inline entities and an optional textproto
// reference. If scenario_textproto_path is set it takes precedence over inline
// fields.
func (s *ScenarioService) resolveLoadPayload(req *v1alpha.LoadScenarioRequest) (*v1alpha.LoadScenarioRequest, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	if path := req.GetScenarioTextprotoPath(); path != "" {
		return s.loadScenarioFromTextproto(path)
	}

	return req, nil
}

// loadScenarioFromTextproto reads a textproto file into a LoadScenarioRequest.
// This allows the same schema to be used both inline and on disk.
func (s *ScenarioService) loadScenarioFromTextproto(path string) (*v1alpha.LoadScenarioRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to read scenario textproto: %v", err)
	}

	payload := &v1alpha.LoadScenarioRequest{}
	if err := prototext.Unmarshal(data, payload); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to parse scenario textproto: %v", err)
	}

	return payload, nil
}

// ensureReady validates that the ScenarioService has a configured ScenarioState.
func (s *ScenarioService) ensureReady() error {
	if s == nil || s.state == nil {
		return status.Error(codes.FailedPrecondition, "scenario state is not configured")
	}
	return nil
}

// ---- Sorting helpers for deterministic load / snapshot order ----

func sortDomainPlatforms(platforms []*model.PlatformDefinition) []*model.PlatformDefinition {
	out := make([]*model.PlatformDefinition, 0, len(platforms))
	for _, pd := range platforms {
		if pd != nil {
			out = append(out, pd)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortProtoPlatforms(platforms []*common.PlatformDefinition) []*common.PlatformDefinition {
	out := make([]*common.PlatformDefinition, 0, len(platforms))
	for _, pd := range platforms {
		if pd != nil {
			out = append(out, pd)
		}
	}

	// The Aalyria PlatformDefinition does not expose an ID field in the proto.
	// Using name is sufficient for deterministic ordering here.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].GetName() < out[j].GetName()
	})
	return out
}

func sortDomainNodes(nodes []*model.NetworkNode) []*model.NetworkNode {
	out := make([]*model.NetworkNode, 0, len(nodes))
	for _, n := range nodes {
		if n != nil {
			out = append(out, n)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortProtoNodes(nodes []*resources.NetworkNode) []*resources.NetworkNode {
	out := make([]*resources.NetworkNode, 0, len(nodes))
	for _, n := range nodes {
		if n != nil {
			out = append(out, n)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].GetNodeId() < out[j].GetNodeId()
	})
	return out
}

func sortLinks(links []*resources.BidirectionalLink) []*resources.BidirectionalLink {
	out := make([]*resources.BidirectionalLink, 0, len(links))
	for _, l := range links {
		if l != nil {
			out = append(out, l)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return linkKey(out[i]) < linkKey(out[j])
	})
	return out
}

func sortDomainServiceRequests(reqs []*model.ServiceRequest) []*model.ServiceRequest {
	out := make([]*model.ServiceRequest, 0, len(reqs))
	for _, sr := range reqs {
		if sr != nil {
			out = append(out, sr)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortProtoServiceRequests(reqs []*resources.ServiceRequest) []*resources.ServiceRequest {
	out := make([]*resources.ServiceRequest, 0, len(reqs))
	for _, sr := range reqs {
		if sr != nil {
			out = append(out, sr)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return serviceRequestKey(out[i]) < serviceRequestKey(out[j])
	})
	return out
}

// linkKey produces a stable key based on node + interface IDs for a
// BidirectionalLink. It prefers the explicit *_network_node_id and
// *_tx/_rx_interface_id fields, but will fall back to deprecated LinkEnd
// fields if needed.
func linkKey(link *resources.BidirectionalLink) string {
	if link == nil {
		return ""
	}

	aNode := link.GetANetworkNodeId()
	bNode := link.GetBNetworkNodeId()
	aTx := link.GetATxInterfaceId()
	aRx := link.GetARxInterfaceId()
	bTx := link.GetBTxInterfaceId()
	bRx := link.GetBRxInterfaceId()

	if end := link.GetA(); end != nil && end.GetId() != nil {
		if aNode == "" {
			aNode = end.GetId().GetNodeId()
		}
		if aTx == "" {
			aTx = end.GetId().GetInterfaceId()
		}
		if aRx == "" {
			aRx = end.GetId().GetInterfaceId()
		}
	}

	if end := link.GetB(); end != nil && end.GetId() != nil {
		if bNode == "" {
			bNode = end.GetId().GetNodeId()
		}
		if bTx == "" {
			bTx = end.GetId().GetInterfaceId()
		}
		if bRx == "" {
			bRx = end.GetId().GetInterfaceId()
		}
	}

	return aNode + "|" + aTx + "|" + aRx + "|" + bNode + "|" + bTx + "|" + bRx
}

// serviceRequestKey is a deterministic key for ordering proto ServiceRequests.
// It prefers the overloaded `type` field (used as a stable ID), falling back
// to a simple src|dst composite.
func serviceRequestKey(sr *resources.ServiceRequest) string {
	if sr == nil {
		return ""
	}
	if id := sr.GetType(); id != "" {
		return id
	}
	return sr.GetSrcNodeId() + "|" + sr.GetDstNodeId()
}
