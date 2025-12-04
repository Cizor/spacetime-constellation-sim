// internal/sim/state/state.go
package state

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// Re-export platform sentinel errors so callers can depend on state.*
// instead of kb.* directly if they want to.
var (
	// ErrPlatformExists indicates a platform already exists.
	ErrPlatformExists = kb.ErrPlatformExists
	// ErrPlatformNotFound indicates a requested platform was not found.
	ErrPlatformNotFound = kb.ErrPlatformNotFound
	// ErrNodeExists indicates a network node already exists.
	ErrNodeExists = kb.ErrNodeExists
	// ErrNodeNotFound indicates a requested node was not found.
	ErrNodeNotFound = kb.ErrNodeNotFound
	// ErrInterfaceExists indicates a network interface already exists.
	ErrInterfaceExists = network.ErrInterfaceExists
	// ErrInterfaceNotFound indicates a requested interface was not found.
	ErrInterfaceNotFound = network.ErrInterfaceNotFound
	// ErrInterfaceInvalid indicates an input interface failed validation.
	ErrInterfaceInvalid = errors.New("invalid interface")
	// ErrTransceiverNotFound indicates a referenced transceiver model is missing.
	ErrTransceiverNotFound = errors.New("transceiver model not found")
	// ErrNodeInvalid indicates a node failed validation.
	ErrNodeInvalid = errors.New("invalid node")
	// ErrLinkNotFound indicates a requested link was not found.
	ErrLinkNotFound = network.ErrLinkNotFound
	// ErrServiceRequestExists indicates a service request already exists.
	ErrServiceRequestExists = errors.New("service request already exists")
	// ErrServiceRequestNotFound indicates a service request was not found.
	ErrServiceRequestNotFound = errors.New("service request not found")
)

// ScenarioState coordinates the simulator's major knowledge bases and
// holds transient NBI state like ServiceRequests.
type ScenarioState struct {
	mu sync.RWMutex

	// physKB is the Scope-1 knowledge base for platforms and nodes.
	physKB *kb.KnowledgeBase

	// netKB is the Scope-2 knowledge base for interfaces and links.
	netKB *network.KnowledgeBase

	// serviceRequests is an in-memory store of active ServiceRequests,
	// keyed by their internal ID.
	serviceRequests map[string]*model.ServiceRequest
}

// ScenarioSnapshot captures a consistent view of all in-memory state
// managed by ScenarioState.
//
// The slices contain pointers owned by the underlying KBs / state;
// callers MUST treat them as read-only.
type ScenarioSnapshot struct {
	Platforms       []*model.PlatformDefinition
	Nodes           []*model.NetworkNode
	Interfaces      []*network.NetworkInterface
	Links           []*network.NetworkLink
	ServiceRequests []*model.ServiceRequest
}

// NewScenarioState wires together the scope-1 and scope-2 knowledge bases
// and prepares an empty ServiceRequest store.
func NewScenarioState(phys *kb.KnowledgeBase, net *network.KnowledgeBase) *ScenarioState {
	return &ScenarioState{
		physKB:          phys,
		netKB:           net,
		serviceRequests: make(map[string]*model.ServiceRequest),
	}
}

// PhysicalKB exposes the scope-1 knowledge base for platforms/nodes.
func (s *ScenarioState) PhysicalKB() *kb.KnowledgeBase {
	return s.physKB
}

// NetworkKB exposes the scope-2 knowledge base for interfaces/links.
func (s *ScenarioState) NetworkKB() *network.KnowledgeBase {
	return s.netKB
}

// Snapshot returns a coherent view of the current scenario state.
//
// It acquires the ScenarioState read lock so that the Scope-1 and
// Scope-2 KBs plus the ServiceRequest map are observed atomically
// from the perspective of ScenarioState callers.
func (s *ScenarioState) Snapshot() *ScenarioSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	srs := make([]*model.ServiceRequest, 0, len(s.serviceRequests))
	for _, sr := range s.serviceRequests {
		srs = append(srs, sr)
	}

	return &ScenarioSnapshot{
		Platforms:       s.physKB.ListPlatforms(),
		Nodes:           s.physKB.ListNetworkNodes(),
		Interfaces:      s.netKB.GetAllInterfaces(),
		Links:           s.netKB.GetAllNetworkLinks(),
		ServiceRequests: srs,
	}
}

// CreatePlatform inserts a new platform into the scenario.
func (s *ScenarioState) CreatePlatform(pd *model.PlatformDefinition) error {
	if pd == nil {
		return errors.New("platform is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.physKB.AddPlatform(pd); err != nil {
		if errors.Is(err, kb.ErrPlatformExists) {
			return ErrPlatformExists
		}
		return err
	}
	return nil
}

// GetPlatform retrieves a platform by ID.
func (s *ScenarioState) GetPlatform(id string) (*model.PlatformDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := s.physKB.GetPlatform(id)
	if p == nil {
		return nil, ErrPlatformNotFound
	}
	return p, nil
}

// ListPlatforms returns all platforms in the scenario.
func (s *ScenarioState) ListPlatforms() []*model.PlatformDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.physKB.ListPlatforms()
}

// UpdatePlatform replaces an existing platform entry.
func (s *ScenarioState) UpdatePlatform(pd *model.PlatformDefinition) error {
	if pd == nil {
		return errors.New("platform is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.physKB.UpdatePlatform(pd); err != nil {
		if errors.Is(err, kb.ErrPlatformNotFound) {
			return ErrPlatformNotFound
		}
		return err
	}
	return nil
}

// DeletePlatform removes a platform by ID.
//
// NOTE: Referential integrity (nodes referencing this platform) is *not*
// enforced here yet; that will be handled in later validation/RI chunks.
func (s *ScenarioState) DeletePlatform(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.physKB.DeletePlatform(id); err != nil {
		if errors.Is(err, kb.ErrPlatformNotFound) {
			return ErrPlatformNotFound
		}
		return err
	}
	return nil
}

// CreateNode inserts a new network node along with its interfaces.
func (s *ScenarioState) CreateNode(node *model.NetworkNode, interfaces []*network.NetworkInterface) error {
	if node == nil || node.ID == "" {
		return fmt.Errorf("%w: empty node", ErrNodeInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if node.PlatformID != "" && s.physKB.GetPlatform(node.PlatformID) == nil {
		return fmt.Errorf("%w: %q", ErrPlatformNotFound, node.PlatformID)
	}
	if existing := s.physKB.GetNetworkNode(node.ID); existing != nil {
		return fmt.Errorf("%w: %q", ErrNodeExists, node.ID)
	}
	if err := s.validateNodeInterfacesLocked(node.ID, interfaces); err != nil {
		return err
	}

	if err := s.physKB.AddNetworkNode(node); err != nil {
		if errors.Is(err, kb.ErrPlatformNotFound) {
			return ErrPlatformNotFound
		}
		if errors.Is(err, kb.ErrNodeExists) {
			return ErrNodeExists
		}
		return err
	}

	added := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		if err := s.netKB.AddInterface(iface); err != nil {
			for _, id := range added {
				_ = s.netKB.DeleteInterface(id)
			}
			_ = s.physKB.DeleteNetworkNode(node.ID)
			if errors.Is(err, network.ErrInterfaceExists) {
				return fmt.Errorf("%w: %q", ErrInterfaceExists, iface.ID)
			}
			if errors.Is(err, network.ErrInterfaceBadInput) {
				return fmt.Errorf("%w: %v", ErrInterfaceInvalid, err)
			}
			return err
		}
		added = append(added, iface.ID)
	}

	return nil
}

// GetNode fetches a node and its interfaces by ID.
func (s *ScenarioState) GetNode(id string) (*model.NetworkNode, []*network.NetworkInterface, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node := s.physKB.GetNetworkNode(id)
	if node == nil {
		return nil, nil, ErrNodeNotFound
	}

	return node, s.interfacesForNodeLocked(id), nil
}

// ListNodes returns all nodes in the scenario.
func (s *ScenarioState) ListNodes() []*model.NetworkNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.physKB.ListNetworkNodes()
}

// ListInterfacesForNode returns all interfaces associated with a node ID.
func (s *ScenarioState) ListInterfacesForNode(nodeID string) []*network.NetworkInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interfacesForNodeLocked(nodeID)
}

// UpdateNode replaces an existing node and its interfaces.
func (s *ScenarioState) UpdateNode(node *model.NetworkNode, interfaces []*network.NetworkInterface) error {
	if node == nil || node.ID == "" {
		return fmt.Errorf("%w: empty node", ErrNodeInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing := s.physKB.GetNetworkNode(node.ID); existing == nil {
		return ErrNodeNotFound
	}
	if node.PlatformID != "" && s.physKB.GetPlatform(node.PlatformID) == nil {
		return fmt.Errorf("%w: %q", ErrPlatformNotFound, node.PlatformID)
	}
	if err := s.validateNodeInterfacesLocked(node.ID, interfaces); err != nil {
		return err
	}

	if err := s.physKB.UpdateNetworkNode(node); err != nil {
		if errors.Is(err, kb.ErrPlatformNotFound) {
			return ErrPlatformNotFound
		}
		if errors.Is(err, kb.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return err
	}

	if err := s.netKB.ReplaceInterfacesForNode(node.ID, interfaces); err != nil {
		if errors.Is(err, network.ErrInterfaceExists) {
			return fmt.Errorf("%w: %v", ErrInterfaceExists, err)
		}
		if errors.Is(err, network.ErrInterfaceBadInput) {
			return fmt.Errorf("%w: %v", ErrInterfaceInvalid, err)
		}
		return err
	}

	return nil
}

// DeleteNode removes a node and its interfaces by ID.
func (s *ScenarioState) DeleteNode(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.physKB.DeleteNetworkNode(id); err != nil {
		if errors.Is(err, kb.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return err
	}

	if err := s.netKB.ReplaceInterfacesForNode(id, nil); err != nil && !errors.Is(err, network.ErrInterfaceNotFound) {
		return err
	}

	return nil
}

// CreateLink inserts a new network link into the Scope-2 knowledge base.
func (s *ScenarioState) CreateLink(link *network.NetworkLink) error {
	if link == nil {
		return errors.New("link is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.netKB.AddNetworkLink(link)
}

// GetLink returns a network link by ID.
func (s *ScenarioState) GetLink(id string) (*network.NetworkLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link := s.netKB.GetNetworkLink(id)
	if link == nil {
		return nil, ErrLinkNotFound
	}
	return link, nil
}

// ListLinks returns all network links in the Scope-2 KB.
func (s *ScenarioState) ListLinks() []*network.NetworkLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.netKB.GetAllNetworkLinks()
}

// DeleteLink removes a link and updates adjacency.
func (s *ScenarioState) DeleteLink(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.netKB.DeleteNetworkLink(id); err != nil {
		if errors.Is(err, network.ErrLinkNotFound) {
			return ErrLinkNotFound
		}
		return err
	}
	return nil
}

// UpdateLink replaces an existing network link in the Scope-2 knowledge base.
func (s *ScenarioState) UpdateLink(link *network.NetworkLink) error {
	if link == nil {
		return errors.New("link is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.netKB.UpdateNetworkLink(link); err != nil {
		if errors.Is(err, network.ErrLinkNotFound) {
			return ErrLinkNotFound
		}
		return err
	}
	return nil
}

// ServiceRequests returns a snapshot of all stored ServiceRequests.
//
// The returned slice is a shallow copy of the internal map values.
// Callers MUST treat the returned ServiceRequests as read-only and
// perform any mutations via Create/Update/DeleteServiceRequest.
func (s *ScenarioState) ServiceRequests() []*model.ServiceRequest {
	return s.ListServiceRequests()
}

// CreateServiceRequest inserts a new ServiceRequest.
func (s *ScenarioState) CreateServiceRequest(sr *model.ServiceRequest) error {
	if sr == nil {
		return errors.New("service request is nil")
	}
	if sr.ID == "" {
		return errors.New("service request ID is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.serviceRequests[sr.ID]; exists {
		return ErrServiceRequestExists
	}
	s.serviceRequests[sr.ID] = sr
	return nil
}

// GetServiceRequest retrieves a ServiceRequest by ID.
func (s *ScenarioState) GetServiceRequest(id string) (*model.ServiceRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sr, ok := s.serviceRequests[id]
	if !ok {
		return nil, ErrServiceRequestNotFound
	}
	return sr, nil
}

// ListServiceRequests returns a snapshot of ServiceRequests.
func (s *ScenarioState) ListServiceRequests() []*model.ServiceRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*model.ServiceRequest, 0, len(s.serviceRequests))
	for _, sr := range s.serviceRequests {
		out = append(out, sr)
	}
	return out
}

// UpdateServiceRequest replaces an existing ServiceRequest entry.
func (s *ScenarioState) UpdateServiceRequest(sr *model.ServiceRequest) error {
	if sr == nil {
		return errors.New("service request is nil")
	}
	if sr.ID == "" {
		return errors.New("service request ID is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.serviceRequests[sr.ID]; !exists {
		return ErrServiceRequestNotFound
	}
	s.serviceRequests[sr.ID] = sr
	return nil
}

// DeleteServiceRequest removes a ServiceRequest by ID.
func (s *ScenarioState) DeleteServiceRequest(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.serviceRequests[id]; !exists {
		return ErrServiceRequestNotFound
	}
	delete(s.serviceRequests, id)
	return nil
}

// interfacesForNodeLocked returns interfaces attached to the node. Caller must
// hold the ScenarioState lock.
func (s *ScenarioState) interfacesForNodeLocked(nodeID string) []*network.NetworkInterface {
	if nodeID == "" {
		return nil
	}
	return s.netKB.GetInterfacesForNode(nodeID)
}

// validateNodeInterfacesLocked performs basic validation on the provided
// interfaces, ensuring uniqueness per node, parent association, and
// referenced transceiver existence.
//
// Caller must hold the ScenarioState lock.
func (s *ScenarioState) validateNodeInterfacesLocked(nodeID string, interfaces []*network.NetworkInterface) error {
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}

	seen := make(map[string]struct{})

	for _, iface := range interfaces {
		if iface == nil {
			return fmt.Errorf("%w: nil interface", ErrInterfaceInvalid)
		}

		parent, local := splitInterfaceRef(iface.ID)
		if parent != "" && parent != nodeID {
			return fmt.Errorf("%w: interface %q parent %q does not match node %q", ErrInterfaceInvalid, iface.ID, parent, nodeID)
		}

		if local == "" {
			local = iface.ID
		}
		if local == "" {
			return fmt.Errorf("%w: empty interface_id for node %q", ErrInterfaceInvalid, nodeID)
		}

		if iface.ParentNodeID == "" {
			iface.ParentNodeID = nodeID
		}
		if iface.ParentNodeID != nodeID {
			return fmt.Errorf("%w: interface %q parent %q does not match node %q", ErrInterfaceInvalid, iface.ID, iface.ParentNodeID, nodeID)
		}

		if _, ok := seen[local]; ok {
			return fmt.Errorf("%w: duplicate interface_id %q for node %q", ErrInterfaceInvalid, local, nodeID)
		}
		seen[local] = struct{}{}

		if iface.Medium == network.MediumWireless {
			if iface.TransceiverID == "" {
				return fmt.Errorf("%w: wireless interface %q missing transceiver reference", ErrInterfaceInvalid, iface.ID)
			}
			if s.netKB.GetTransceiverModel(iface.TransceiverID) == nil {
				return fmt.Errorf("%w: %q", ErrTransceiverNotFound, iface.TransceiverID)
			}
		}
	}

	return nil
}

func splitInterfaceRef(ref string) (string, string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ref
}

// ClearScenario wipes all in-memory scenario data so a fresh scenario
// can be loaded without dangling references.
func (s *ScenarioState) ClearScenario() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.physKB != nil {
		s.physKB.Clear()
	}
	if s.netKB != nil {
		s.netKB.Clear()
	}
	s.serviceRequests = make(map[string]*model.ServiceRequest)

	return nil
}
