// internal/sim/state/state.go
package state

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
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
	// ErrLinkNotFoundForBeam indicates a link was not found when applying a beam operation.
	ErrLinkNotFoundForBeam = errors.New("link not found for beam")
	// ErrServiceRequestExists indicates a service request already exists.
	ErrServiceRequestExists = errors.New("service request already exists")
	// ErrServiceRequestNotFound indicates a service request was not found.
	ErrServiceRequestNotFound = errors.New("service request not found")
	// ErrPlatformInUse indicates a platform is still referenced by nodes.
	ErrPlatformInUse = errors.New("platform is referenced by nodes")
	// ErrNodeInUse indicates a node is still referenced by other resources.
	ErrNodeInUse = errors.New("node is referenced by links or service requests")
	// ErrInterfaceInUse indicates an interface is still referenced by links.
	ErrInterfaceInUse = errors.New("interface is referenced by links")
)

// ScenarioState coordinates the simulator's major knowledge bases and
// holds transient NBI state like ServiceRequests.
type ScenarioState struct {
	// mu is the coarse scenario-level lock. Take this before touching either KB
	// to maintain the global lock ordering of ScenarioState -> KB locks.
	mu sync.RWMutex

	// physKB is the Scope-1 knowledge base for platforms and nodes.
	physKB *kb.KnowledgeBase

	// netKB is the Scope-2 knowledge base for interfaces and links.
	netKB *network.KnowledgeBase

	// serviceRequests is an in-memory store of active ServiceRequests,
	// keyed by their internal ID.
	serviceRequests map[string]*model.ServiceRequest
	// serviceRequestStatuses track provisioning state / intervals per request.
	serviceRequestStatuses map[string]*model.ServiceRequestStatus
	dtnStorageUsage        map[string]float64

	// motion is an optional motion model used by the simulator; it is
	// reset alongside scenario clears.
	motion motionResetter

	// connectivity is an optional ConnectivityService whose caches need
	// to be flushed when the scenario is cleared.
	connectivity connectivityResetter

	// log is an optional structured logger for state-level events.
	log logging.Logger

	// metrics is an optional recorder for Prometheus-friendly gauges.
	metrics ScenarioMetricsRecorder
}

// ScenarioSnapshot captures a consistent view of all in-memory state
// managed by ScenarioState.
//
// The slices contain pointers owned by the underlying KBs / state;
// callers MUST treat them as read-only.
type ScenarioSnapshot struct {
	Platforms        []*model.PlatformDefinition
	Nodes            []*model.NetworkNode
	Interfaces       []*network.NetworkInterface
	InterfacesByNode map[string][]*network.NetworkInterface
	Links            []*network.NetworkLink
	ServiceRequests  []*model.ServiceRequest
}

// ScenarioMetricsRecorder receives count updates for core scenario entities.
type ScenarioMetricsRecorder interface {
	SetScenarioCounts(platforms, nodes, links, serviceRequests int)
}

type motionResetter interface {
	Reset()
}

type connectivityResetter interface {
	Reset()
}

// ScenarioStateOption customises ScenarioState construction.
type ScenarioStateOption func(*ScenarioState)

// WithMotionModel attaches a motion model whose internal caches should
// be flushed when ClearScenario is invoked.
func WithMotionModel(m motionResetter) ScenarioStateOption {
	return func(s *ScenarioState) {
		s.motion = m
	}
}

// WithConnectivityService attaches a connectivity service whose dynamic
// caches should be cleared alongside scenario data.
func WithConnectivityService(c connectivityResetter) ScenarioStateOption {
	return func(s *ScenarioState) {
		s.connectivity = c
	}
}

// WithMetricsRecorder attaches an optional metrics recorder for entity counts.
func WithMetricsRecorder(m ScenarioMetricsRecorder) ScenarioStateOption {
	return func(s *ScenarioState) {
		s.metrics = m
	}
}

// NewScenarioState wires together the scope-1 and scope-2 knowledge bases
// and prepares an empty ServiceRequest store.
func NewScenarioState(phys *kb.KnowledgeBase, net *network.KnowledgeBase, log logging.Logger, opts ...ScenarioStateOption) *ScenarioState {
	if log == nil {
		log = logging.Noop()
	}
	state := &ScenarioState{
		physKB:                 phys,
		netKB:                  net,
		serviceRequests:        make(map[string]*model.ServiceRequest),
		serviceRequestStatuses: make(map[string]*model.ServiceRequestStatus),
		dtnStorageUsage:        make(map[string]float64),
		log:                    log,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(state)
		}
	}
	state.updateMetricsLocked()
	return state
}

// PhysicalKB exposes the scope-1 knowledge base for platforms/nodes.
func (s *ScenarioState) PhysicalKB() *kb.KnowledgeBase {
	return s.physKB
}

// NetworkKB exposes the scope-2 knowledge base for interfaces/links.
func (s *ScenarioState) NetworkKB() *network.KnowledgeBase {
	return s.netKB
}

// WithReadLock executes fn while holding the ScenarioState read lock.
// Callers must not invoke other ScenarioState methods that also take the lock
// from inside fn to avoid self-deadlock.
func (s *ScenarioState) WithReadLock(fn func() error) error {
	if fn == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fn()
}

// MotionUpdater captures the subset of MotionModel needed by the sim loop.
type MotionUpdater interface {
	UpdatePositions(simTime time.Time) error
}

// ConnectivityUpdater captures the subset of ConnectivityService needed by the sim loop.
type ConnectivityUpdater interface {
	UpdateConnectivity()
}

// RunSimTick executes a single simulation tick while holding the ScenarioState
// read lock. Callers must keep the work inside fn read-only with respect to
// ScenarioState; writes that touch underlying KBs must follow their own locking.
func (s *ScenarioState) RunSimTick(simTime time.Time, motion MotionUpdater, connectivity ConnectivityUpdater, fn func()) error {
	return s.WithReadLock(func() error {
		if fn != nil {
			fn()
		}
		if motion != nil {
			if err := motion.UpdatePositions(simTime); err != nil {
				return err
			}
		}
		if connectivity != nil {
			connectivity.UpdateConnectivity()
		}
		return nil
	})
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

	platforms := s.physKB.ListPlatforms()
	nodes := s.physKB.ListNetworkNodes()
	interfaces := s.netKB.GetAllInterfaces()

	return &ScenarioSnapshot{
		Platforms:        platforms,
		Nodes:            nodes,
		Interfaces:       interfaces,
		InterfacesByNode: s.interfacesByNodeLocked(nodes, interfaces),
		Links:            s.netKB.GetAllNetworkLinks(),
		ServiceRequests:  srs,
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

	s.updateMetricsLocked()
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

	s.updateMetricsLocked()
	return nil
}

// DeletePlatform removes a platform by ID.
func (s *ScenarioState) DeletePlatform(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.physKB.GetPlatform(id) == nil {
		return ErrPlatformNotFound
	}
	if s.platformHasReferencesLocked(id) {
		// Hard fail semantics: no cascading delete in this scope.
		return ErrPlatformInUse
	}

	if err := s.physKB.DeletePlatform(id); err != nil {
		if errors.Is(err, kb.ErrPlatformNotFound) {
			return ErrPlatformNotFound
		}
		return err
	}

	s.updateMetricsLocked()
	return nil
}

// platformHasReferencesLocked reports whether any nodes reference the platform.
// Caller must hold s.mu.
func (s *ScenarioState) platformHasReferencesLocked(platformID string) bool {
	if platformID == "" {
		return false
	}

	for _, node := range s.physKB.ListNetworkNodes() {
		if node != nil && node.PlatformID == platformID {
			return true
		}
	}
	return false
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

	s.dtnStorageUsage[node.ID] = 0

	s.updateMetricsLocked()
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

// InterfacesByNode returns a map of nodeID -> interfaces while holding
// the ScenarioState read lock, enabling callers to build node/interface
// projections without taking multiple locks.
func (s *ScenarioState) InterfacesByNode() map[string][]*network.NetworkInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := s.physKB.ListNetworkNodes()
	ifaces := s.netKB.GetAllInterfaces()
	return s.interfacesByNodeLocked(nodes, ifaces)
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

	s.updateMetricsLocked()
	return nil
}

// DeleteNode removes a node and its interfaces by ID.
//
// NOTE: Higher-level deletion semantics (e.g. a future "force delete" that
// cascades to links/service requests) are intentionally out of scope here.
func (s *ScenarioState) DeleteNode(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.physKB.GetNetworkNode(id) == nil {
		return ErrNodeNotFound
	}
	if s.nodeHasReferencesLocked(id) {
		return ErrNodeInUse
	}

	if err := s.physKB.DeleteNetworkNode(id); err != nil {
		if errors.Is(err, kb.ErrNodeNotFound) {
			return ErrNodeNotFound
		}
		return err
	}

	if err := s.netKB.ReplaceInterfacesForNode(id, nil); err != nil && !errors.Is(err, network.ErrInterfaceNotFound) {
		return err
	}

	delete(s.dtnStorageUsage, id)

	s.updateMetricsLocked()
	return nil
}

// nodeHasReferencesLocked reports whether any links or service requests point
// at the given node. Caller must hold s.mu.
func (s *ScenarioState) nodeHasReferencesLocked(nodeID string) bool {
	if nodeID == "" {
		return false
	}

	ifaces := s.interfacesForNodeLocked(nodeID)
	interfaceIDs := make(map[string]struct{}, len(ifaces))
	for _, iface := range ifaces {
		if iface != nil && iface.ID != "" {
			interfaceIDs[iface.ID] = struct{}{}
		}
	}

	// Check links for references to any of the node's interfaces.
	if len(interfaceIDs) > 0 {
		for _, link := range s.netKB.GetAllNetworkLinks() {
			if link == nil {
				continue
			}
			if _, ok := interfaceIDs[link.InterfaceA]; ok {
				return true
			}
			if _, ok := interfaceIDs[link.InterfaceB]; ok {
				return true
			}
		}
	}

	// Check service requests that name the node as a source or destination.
	for _, sr := range s.serviceRequests {
		if sr == nil {
			continue
		}
		if sr.SrcNodeID == nodeID || sr.DstNodeID == nodeID {
			return true
		}
	}

	return false
}

// DeleteInterface removes an interface by ID, refusing to delete when it is
// still referenced by links. Cascading deletion could be added in later scopes.
func (s *ScenarioState) DeleteInterface(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty interface ID", ErrInterfaceInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.netKB.GetNetworkInterface(id) == nil {
		return ErrInterfaceNotFound
	}
	if links := s.netKB.GetLinksForInterface(id); len(links) > 0 {
		return ErrInterfaceInUse
	}

	if err := s.netKB.DeleteInterface(id); err != nil {
		if errors.Is(err, network.ErrInterfaceNotFound) {
			return ErrInterfaceNotFound
		}
		return err
	}

	s.updateMetricsLocked()
	return nil
}

// CreateLink inserts a new network link into the Scope-2 knowledge base.
func (s *ScenarioState) CreateLink(link *network.NetworkLink) error {
	return s.CreateLinks(link)
}

// CreateLinks inserts one or more network links into the Scope-2
// knowledge base. If any insert fails, previously added links from
// this call are rolled back to keep adjacency consistent.
func (s *ScenarioState) CreateLinks(links ...*network.NetworkLink) error {
	for _, link := range links {
		if link == nil {
			return errors.New("link is nil")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	added := make([]string, 0, len(links))

	for _, link := range links {
		if err := s.netKB.AddNetworkLink(link); err != nil {
			for _, id := range added {
				_ = s.netKB.DeleteNetworkLink(id)
			}
			return err
		}
		added = append(added, link.ID)
	}

	s.updateMetricsLocked()
	return nil
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

	s.updateMetricsLocked()
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

	s.updateMetricsLocked()
	return nil
}

// ApplyBeamUpdate activates a link by updating its status to Active and setting IsUp to true.
// It finds the link between the specified interfaces and updates it.
func (s *ScenarioState) ApplyBeamUpdate(nodeID string, beam *sbi.BeamSpec) error {
	if beam == nil {
		return errors.New("beam is nil")
	}
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify node exists
	if s.physKB.GetNetworkNode(nodeID) == nil {
		return ErrNodeNotFound
	}

	// Construct interface references
	srcIfRef := fmt.Sprintf("%s/%s", nodeID, beam.InterfaceID)
	targetIfRef := fmt.Sprintf("%s/%s", beam.TargetNodeID, beam.TargetIfID)

	// Find the link between these interfaces
	link := s.findLinkByInterfacesLocked(srcIfRef, targetIfRef)
	if link == nil {
		return ErrLinkNotFoundForBeam
	}

	// Update link status to Active
	link.Status = network.LinkStatusActive
	link.IsUp = true

	if err := s.netKB.UpdateNetworkLink(link); err != nil {
		return fmt.Errorf("failed to update link: %w", err)
	}

	s.updateMetricsLocked()
	return nil
}

// ApplyBeamDelete deactivates a link by updating its status to Potential and setting IsUp to false.
func (s *ScenarioState) ApplyBeamDelete(nodeID, interfaceID, targetNodeID, targetIfID string) error {
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify node exists
	if s.physKB.GetNetworkNode(nodeID) == nil {
		return ErrNodeNotFound
	}

	// Construct interface references
	srcIfRef := fmt.Sprintf("%s/%s", nodeID, interfaceID)
	targetIfRef := fmt.Sprintf("%s/%s", targetNodeID, targetIfID)

	// Find the link between these interfaces
	link := s.findLinkByInterfacesLocked(srcIfRef, targetIfRef)
	if link == nil {
		return ErrLinkNotFoundForBeam
	}

	// Update link status to Potential
	link.Status = network.LinkStatusPotential
	link.IsUp = false

	if err := s.netKB.UpdateNetworkLink(link); err != nil {
		return fmt.Errorf("failed to update link: %w", err)
	}

	s.updateMetricsLocked()
	return nil
}

// findLinkByInterfacesLocked finds a link that connects the two specified interfaces.
// Caller must hold s.mu lock.
func (s *ScenarioState) findLinkByInterfacesLocked(ifA, ifB string) *network.NetworkLink {
	allLinks := s.netKB.GetAllNetworkLinks()
	for _, link := range allLinks {
		if link == nil {
			continue
		}
		// Check both directions (A->B and B->A)
		if (link.InterfaceA == ifA && link.InterfaceB == ifB) ||
			(link.InterfaceA == ifB && link.InterfaceB == ifA) {
			return link
		}
	}
	return nil
}

// ApplySetRoute installs or replaces a route for the specified node.
// If a route with the same DestinationCIDR exists, it is replaced.
func (s *ScenarioState) ApplySetRoute(nodeID string, route model.RouteEntry) error {
	if nodeID == "" {
		return errors.New("nodeID must not be empty")
	}
	if route.DestinationCIDR == "" {
		return errors.New("DestinationCIDR must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return ErrNodeNotFound
	}

	// Find and replace existing route with same DestinationCIDR, or append
	found := false
	for i, r := range node.Routes {
		if r.DestinationCIDR == route.DestinationCIDR {
			node.Routes[i] = route
			found = true
			break
		}
	}
	if !found {
		node.Routes = append(node.Routes, route)
	}

	// Update the node in the KB
	if err := s.physKB.UpdateNetworkNode(node); err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	s.updateMetricsLocked()
	return nil
}

// ApplyDeleteRoute removes a route from the specified node by destination CIDR.
func (s *ScenarioState) ApplyDeleteRoute(nodeID string, destinationCIDR string) error {
	if nodeID == "" {
		return errors.New("nodeID must not be empty")
	}
	if destinationCIDR == "" {
		return errors.New("destinationCIDR must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return ErrNodeNotFound
	}

	// Remove route with matching DestinationCIDR
	newRoutes := make([]model.RouteEntry, 0, len(node.Routes))
	found := false
	for _, r := range node.Routes {
		if r.DestinationCIDR != destinationCIDR {
			newRoutes = append(newRoutes, r)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("route not found for destination %q", destinationCIDR)
	}

	node.Routes = newRoutes

	// Update the node in the KB
	if err := s.physKB.UpdateNetworkNode(node); err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	s.updateMetricsLocked()
	return nil
}

// InstallRoute installs a route for the specified node (idempotent).
// If a route with the same DestinationCIDR exists, it is replaced.
func (s *ScenarioState) InstallRoute(nodeID string, route model.RouteEntry) error {
	return s.ApplySetRoute(nodeID, route)
}

// InstallMultiHopRoute installs a routing entry that includes a full multi-hop path.
func (s *ScenarioState) InstallMultiHopRoute(nodeID string, route model.RouteEntry) error {
	if len(route.Path) == 0 {
		return fmt.Errorf("multi-hop route path must include at least one node")
	}
	return s.InstallRoute(nodeID, route)
}

// RemoveRoute removes a route from the specified node by destination CIDR.
func (s *ScenarioState) RemoveRoute(nodeID string, destinationCIDR string) error {
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	if destinationCIDR == "" {
		return errors.New("destination CIDR is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return ErrNodeNotFound
	}

	// Remove route with matching DestinationCIDR
	newRoutes := make([]model.RouteEntry, 0, len(node.Routes))
	found := false
	for _, r := range node.Routes {
		if r.DestinationCIDR != destinationCIDR {
			newRoutes = append(newRoutes, r)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("route not found for destination %q", destinationCIDR)
	}

	node.Routes = newRoutes

	// Update the node in the KB
	if err := s.physKB.UpdateNetworkNode(node); err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	s.updateMetricsLocked()
	return nil
}

// GetRoutePath returns the stored path for the route installed at srcNodeID toward dstNodeID.
func (s *ScenarioState) GetRoutePath(srcNodeID, dstNodeID string) ([]string, error) {
	if srcNodeID == "" || dstNodeID == "" {
		return nil, fmt.Errorf("%w: source and destination node IDs are required", ErrNodeInvalid)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	node := s.physKB.GetNetworkNode(srcNodeID)
	if node == nil {
		return nil, ErrNodeNotFound
	}

	for _, route := range node.Routes {
		if len(route.Path) > 0 && route.Path[0] == srcNodeID && route.Path[len(route.Path)-1] == dstNodeID {
			pathCopy := append([]string(nil), route.Path...)
			return pathCopy, nil
		}
	}
	return nil, fmt.Errorf("path from %q to %q not found", srcNodeID, dstNodeID)
}

// InvalidateExpiredRoutes removes routes whose ValidUntil timestamp has elapsed.
func (s *ScenarioState) InvalidateExpiredRoutes(now time.Time) error {
	if now.IsZero() {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nodes := s.physKB.ListNetworkNodes()
	for _, node := range nodes {
		updated := false
		newRoutes := make([]model.RouteEntry, 0, len(node.Routes))
		for _, route := range node.Routes {
			if !route.ValidUntil.IsZero() && !now.Before(route.ValidUntil) {
				updated = true
				continue
			}
			newRoutes = append(newRoutes, route)
		}
		if updated {
			node.Routes = newRoutes
			if err := s.physKB.UpdateNetworkNode(node); err != nil {
				return fmt.Errorf("failed to update node %q: %w", node.ID, err)
			}
		}
	}
	return nil
}

// GetRoutes returns all routes for the specified node.
func (s *ScenarioState) GetRoutes(nodeID string) ([]model.RouteEntry, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return nil, ErrNodeNotFound
	}

	// Return a copy of the routes
	routes := make([]model.RouteEntry, len(node.Routes))
	copy(routes, node.Routes)
	return routes, nil
}

// GetRoute returns a specific route for the specified node by destination CIDR.
// Returns (route, true) if found, (nil, false) if not found.
func (s *ScenarioState) GetRoute(nodeID string, destinationCIDR string) (*model.RouteEntry, bool) {
	if nodeID == "" {
		return nil, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return nil, false
	}

	// Find route with matching DestinationCIDR
	for _, r := range node.Routes {
		if r.DestinationCIDR == destinationCIDR {
			// Return a copy
			route := r
			return &route, true
		}
	}

	return nil, false
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

	s.ensureServiceRequestStatusLocked(sr.ID)

	s.updateMetricsLocked()
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

	s.updateMetricsLocked()
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
	delete(s.serviceRequestStatuses, id)

	s.updateMetricsLocked()
	return nil
}

// UpdateServiceRequestStatus updates the provisioning metadata tracked for a ServiceRequest.
func (s *ScenarioState) UpdateServiceRequestStatus(srID string, isProvisioned bool, interval *model.TimeInterval) error {
	if srID == "" {
		return errors.New("service request ID is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sr, exists := s.serviceRequests[srID]
	if !exists {
		return ErrServiceRequestNotFound
	}

	status := s.ensureServiceRequestStatusLocked(srID)
	intervalCopy := copyTimeInterval(interval)

	status.IsProvisionedNow = isProvisioned
	if intervalCopy != nil {
		status.AllIntervals = append(status.AllIntervals, *intervalCopy)
	}
	if isProvisioned {
		status.CurrentInterval = intervalCopy
		if intervalCopy != nil {
			status.LastProvisionedAt = intervalCopy.StartTime
			sr.ProvisionedIntervals = append(sr.ProvisionedIntervals, *intervalCopy)
		}
	} else {
		status.CurrentInterval = nil
		if intervalCopy != nil {
			status.LastUnprovisionedAt = intervalCopy.EndTime
		}
	}
	sr.IsProvisionedNow = isProvisioned
	sr.LastProvisionedAt = status.LastProvisionedAt
	sr.LastUnprovisionedAt = status.LastUnprovisionedAt

	return nil
}

// GetServiceRequestStatus returns a copy of the provisioning metadata for the request.
func (s *ScenarioState) GetServiceRequestStatus(srID string) (*model.ServiceRequestStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.serviceRequests[srID]; !exists {
		return nil, ErrServiceRequestNotFound
	}

	status := s.serviceRequestStatuses[srID]
	if status == nil {
		return &model.ServiceRequestStatus{}, nil
	}
	return cloneServiceRequestStatus(status), nil
}

// updateMetricsLocked pushes current entity counts into the metrics recorder.
// Caller must hold s.mu when invoking this helper.
func (s *ScenarioState) updateMetricsLocked() {
	if s == nil || s.metrics == nil {
		return
	}
	platforms := 0
	nodes := 0
	links := 0
	if s.physKB != nil {
		platforms = len(s.physKB.ListPlatforms())
		nodes = len(s.physKB.ListNetworkNodes())
	}
	if s.netKB != nil {
		links = len(s.netKB.GetAllNetworkLinks())
	}
	s.metrics.SetScenarioCounts(platforms, nodes, links, len(s.serviceRequests))
}

func (s *ScenarioState) ensureServiceRequestStatusLocked(srID string) *model.ServiceRequestStatus {
	if status := s.serviceRequestStatuses[srID]; status != nil {
		return status
	}
	status := &model.ServiceRequestStatus{}
	s.serviceRequestStatuses[srID] = status
	return status
}

func copyTimeInterval(interval *model.TimeInterval) *model.TimeInterval {
	if interval == nil {
		return nil
	}
	clone := *interval
	if interval.Path != nil {
		pathCopy := *interval.Path
		clone.Path = &pathCopy
	}
	return &clone
}

func cloneServiceRequestStatus(src *model.ServiceRequestStatus) *model.ServiceRequestStatus {
	if src == nil {
		return nil
	}
	dst := *src
	if src.CurrentInterval != nil {
		intervalCopy := *src.CurrentInterval
		dst.CurrentInterval = &intervalCopy
	}
	if len(src.AllIntervals) > 0 {
		dst.AllIntervals = append([]model.TimeInterval(nil), src.AllIntervals...)
	}
	return &dst
}

// interfacesByNodeLocked builds a map of nodeID -> interfaces for callers that
// already hold the ScenarioState lock.
func (s *ScenarioState) interfacesByNodeLocked(nodes []*model.NetworkNode, interfaces []*network.NetworkInterface) map[string][]*network.NetworkInterface {
	byNode := make(map[string][]*network.NetworkInterface, len(nodes))

	for _, iface := range interfaces {
		if iface == nil {
			continue
		}
		parent := iface.ParentNodeID
		byNode[parent] = append(byNode[parent], iface)
	}

	for _, node := range nodes {
		if node == nil {
			continue
		}
		if _, ok := byNode[node.ID]; !ok {
			byNode[node.ID] = nil
		}
	}

	return byNode
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

// ReserveStorage increments the DTN storage usage for a node, respecting capacity limits.
func (s *ScenarioState) ReserveStorage(nodeID string, bytes float64) error {
	if nodeID == "" {
		return fmt.Errorf("node ID is empty")
	}
	if bytes <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return ErrNodeNotFound
	}

	usage := s.dtnStorageUsage[nodeID]
	capacity := node.StorageCapacityBytes
	if capacity > 0 && usage+bytes > capacity {
		return fmt.Errorf("storage capacity exceeded for node %q: %.0f/%.0f", nodeID, usage+bytes, capacity)
	}

	s.dtnStorageUsage[nodeID] = usage + bytes
	return nil
}

// ReleaseStorage decrements the DTN storage usage for a node, never dropping below zero.
func (s *ScenarioState) ReleaseStorage(nodeID string, bytes float64) {
	if nodeID == "" || bytes <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	usage := s.dtnStorageUsage[nodeID]
	usage -= bytes
	if usage < 0 {
		usage = 0
	}
	s.dtnStorageUsage[nodeID] = usage
}

// StorageUsage reports the currently reserved DTN bytes for a node.
func (s *ScenarioState) StorageUsage(nodeID string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dtnStorageUsage[nodeID]
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
func (s *ScenarioState) ClearScenario(ctx context.Context) error {
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)

	s.mu.Lock()
	defer s.mu.Unlock()

	platforms := 0
	nodes := 0
	links := 0
	interfaces := 0
	if s.physKB != nil {
		platforms = len(s.physKB.ListPlatforms())
		nodes = len(s.physKB.ListNetworkNodes())
	}
	if s.netKB != nil {
		interfaces = len(s.netKB.GetAllInterfaces())
		links = len(s.netKB.GetAllNetworkLinks())
	}
	serviceRequests := len(s.serviceRequests)
	reqLog.Debug(ctx, "clearing scenario",
		logging.String("entity_type", "scenario"),
		logging.String("operation", "clear"),
		logging.Int("platforms", platforms),
		logging.Int("nodes", nodes),
		logging.Int("interfaces", interfaces),
		logging.Int("links", links),
		logging.Int("service_requests", serviceRequests),
	)

	if s.physKB != nil {
		s.physKB.Clear()
	}
	if s.netKB != nil {
		s.netKB.Clear()
	}
	s.serviceRequests = make(map[string]*model.ServiceRequest)
	s.serviceRequestStatuses = make(map[string]*model.ServiceRequestStatus)
	s.dtnStorageUsage = make(map[string]float64)

	if s.motion != nil {
		s.motion.Reset()
	}
	if s.connectivity != nil {
		s.connectivity.Reset()
	}

	s.updateMetricsLocked()

	reqLog.Debug(ctx, "scenario cleared",
		logging.String("entity_type", "scenario"),
		logging.String("operation", "clear"),
		logging.Int("platforms", platforms),
		logging.Int("nodes", nodes),
		logging.Int("interfaces", interfaces),
		logging.Int("links", links),
		logging.Int("service_requests", serviceRequests),
	)

	return nil
}
