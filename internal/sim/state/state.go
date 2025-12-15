// internal/sim/state/state.go
package state

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
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
	// ErrStorageFull indicates there is no more DTN storage available on a node.
	ErrStorageFull = errors.New("dtn storage full")
	// ErrLinkNotFound indicates a requested link was not found.
	ErrLinkNotFound = network.ErrLinkNotFound
	// ErrLinkNotFoundForBeam indicates a link was not found when applying a beam operation.
	ErrLinkNotFoundForBeam = errors.New("link not found for beam")
	// ErrServiceRequestExists indicates a service request already exists.
	ErrServiceRequestExists = errors.New("service request already exists")
	// ErrServiceRequestNotFound indicates a service request was not found.
	ErrServiceRequestNotFound = errors.New("service request not found")
	// ErrRegionExists indicates a region was already defined.
	ErrRegionExists = errors.New("region already exists")
	// ErrRegionNotFound indicates a requested region was not found.
	ErrRegionNotFound = errors.New("region not found")
	// ErrRegionInvalid indicates the supplied region definition is invalid.
	ErrRegionInvalid = errors.New("invalid region")
	// ErrDomainExists indicates a domain already exists.
	ErrDomainExists = errors.New("domain already exists")
	// ErrDomainNotFound indicates a requested domain was not found.
	ErrDomainNotFound = errors.New("domain not found")
	// ErrDomainInvalid indicates the supplied domain definition is invalid.
	ErrDomainInvalid = errors.New("invalid domain")
	// ErrPlatformInUse indicates a platform is still referenced by nodes.
	ErrPlatformInUse = errors.New("platform is referenced by nodes")
	// ErrNodeInUse indicates a node is still referenced by other resources.
	ErrNodeInUse = errors.New("node is referenced by links or service requests")
	// ErrInterfaceInUse indicates an interface is still referenced by links.
	ErrInterfaceInUse = errors.New("interface is referenced by links")
)

const (
	defaultLinkBandwidthBps    = 1_000_000_000
	defaultRegionMembershipTTL = 15 * time.Second
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
	dtnStorage             map[string]*DTNStorage
	// interfacePowerStates tracks allocated power per interface.
	interfacePowerStates map[string]*InterfacePowerState
	// regions store defined geographic regions.
	regions map[string]*model.Region
	// domains store federation scheduling domains.
	domains map[string]*model.SchedulingDomain
	// nodeDomains maps node -> domain ID.
	nodeDomains map[string]string
	// regionMembership caches recent membership snapshots, keyed by region ID.
	regionMembership map[string]*regionMembershipEntry
	// regionMembershipTTL controls how long cached membership is considered fresh.
	regionMembershipTTL time.Duration
	// regionMembershipHook is invoked when membership changes occur.
	regionMembershipHook RegionMembershipHook

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
	// scheduler is used to drive DTN message lifecycle events.
	scheduler sbi.EventScheduler
	// expiryEvents maps message IDs to scheduled expiry event IDs.
	expiryEvents map[string]string
	// messageStates stores the latest MessageState for each message.
	messageStates map[string]MessageState
	// messageHistory tracks transitions per message.
	messageHistory map[string][]MessageStateTransition
}

// InterfacePowerState tracks the power usage on an interface.
type InterfacePowerState struct {
	InterfaceID    string
	AllocatedPower float64
	assignments    map[string]float64
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

// MessageState represents the lifecycle state for a stored DTN message.
type MessageState string

const (
	MessageStatePending   MessageState = "pending"
	MessageStateInTransit MessageState = "in_transit"
	MessageStateStored    MessageState = "stored"
	MessageStateDelivered MessageState = "delivered"
	MessageStateExpired   MessageState = "expired"
)

// MessageStateTransition records when a message entered a particular state.
type MessageStateTransition struct {
	State MessageState
	Time  time.Time
}

// StoredMessage captures metadata about a DTN message stored on a node.
type StoredMessage struct {
	MessageID        string
	ServiceRequestID string
	SizeBytes        uint64
	ArrivalTime      time.Time
	ExpiryTime       time.Time
	Destination      string
	State            MessageState
}

// RegionMembershipHook is invoked when the cached membership set changes for a region.
type RegionMembershipHook func(regionID string, left, entered []string)

type regionMembershipEntry struct {
	members   map[string]struct{}
	updatedAt time.Time
}

// DTNStorage tracks stored messages and capacity for a single node.
type DTNStorage struct {
	NodeID        string
	CapacityBytes uint64
	UsedBytes     uint64
	Messages      []StoredMessage
	mu            sync.RWMutex
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

// WithEventScheduler attaches an EventScheduler used to drive DTN lifecycle transitions.
func WithEventScheduler(es sbi.EventScheduler) ScenarioStateOption {
	return func(s *ScenarioState) {
		s.scheduler = es
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
		dtnStorage:             make(map[string]*DTNStorage),
		interfacePowerStates:   make(map[string]*InterfacePowerState),
		regions:                make(map[string]*model.Region),
		domains:                make(map[string]*model.SchedulingDomain),
		nodeDomains:            make(map[string]string),
		regionMembership:       make(map[string]*regionMembershipEntry),
		regionMembershipTTL:    defaultRegionMembershipTTL,
		expiryEvents:           make(map[string]string),
		messageStates:          make(map[string]MessageState),
		messageHistory:         make(map[string][]MessageStateTransition),
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

// RunSimTick executes a single simulation tick.
// Callers must keep the work inside fn read-only with respect to ScenarioState;
// writes that touch underlying KBs must follow their own locking.
func (s *ScenarioState) RunSimTick(simTime time.Time, motion MotionUpdater, connectivity ConnectivityUpdater, fn func()) error {
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
	if node.PlatformID != "" && s.physKB.GetPlatform(node.PlatformID) == nil {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrPlatformNotFound, node.PlatformID)
	}
	if existing := s.physKB.GetNetworkNode(node.ID); existing != nil {
		s.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrNodeExists, node.ID)
	}
	if err := s.validateNodeInterfacesLocked(node.ID, interfaces); err != nil {
		s.mu.Unlock()
		return err
	}
	if err := s.physKB.AddNetworkNode(node); err != nil {
		s.mu.Unlock()
		if errors.Is(err, kb.ErrPlatformNotFound) {
			return ErrPlatformNotFound
		}
		if errors.Is(err, kb.ErrNodeExists) {
			return ErrNodeExists
		}
		return err
	}
	s.dtnStorage[node.ID] = newDTNStorage(node)
	s.mu.Unlock()

	added := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		if err := s.netKB.AddInterface(iface); err != nil {
			for _, id := range added {
				_ = s.netKB.DeleteInterface(id)
			}
			_ = s.physKB.DeleteNetworkNode(node.ID)
			s.mu.Lock()
			delete(s.dtnStorage, node.ID)
			s.mu.Unlock()
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

	s.mu.Lock()
	s.updateMetricsLocked()
	s.mu.Unlock()
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

	delete(s.dtnStorage, id)

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

	for _, link := range links {
		s.initLinkBandwidthLocked(link)
	}

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

// ReserveBandwidth reserves bps bits per second on the specified link.
func (s *ScenarioState) ReserveBandwidth(linkID string, bps uint64) error {
	if linkID == "" {
		return errors.New("link ID is empty")
	}
	if bps == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	link := s.netKB.GetNetworkLink(linkID)
	if link == nil {
		return ErrLinkNotFound
	}
	if link.AvailableBandwidthBps < bps {
		return fmt.Errorf("insufficient bandwidth on link %q", linkID)
	}

	link.AvailableBandwidthBps -= bps
	link.ReservedBandwidthBps += bps
	if err := s.netKB.UpdateNetworkLink(link); err != nil {
		return fmt.Errorf("failed to update link bandwidth: %w", err)
	}
	return nil
}

// ReleaseBandwidth frees bps bits per second on the specified link.
func (s *ScenarioState) ReleaseBandwidth(linkID string, bps uint64) error {
	if linkID == "" {
		return errors.New("link ID is empty")
	}
	if bps == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	link := s.netKB.GetNetworkLink(linkID)
	if link == nil {
		return ErrLinkNotFound
	}
	if link.ReservedBandwidthBps < bps {
		return fmt.Errorf("release exceeds reserved bandwidth on link %q", linkID)
	}

	link.ReservedBandwidthBps -= bps
	link.AvailableBandwidthBps += bps
	if link.MaxBandwidthBps > 0 && link.AvailableBandwidthBps > link.MaxBandwidthBps {
		link.AvailableBandwidthBps = link.MaxBandwidthBps
	}
	if err := s.netKB.UpdateNetworkLink(link); err != nil {
		return fmt.Errorf("failed to update link bandwidth: %w", err)
	}
	return nil
}

// GetAvailableBandwidth returns how much bandwidth can still be reserved.
func (s *ScenarioState) GetAvailableBandwidth(linkID string) (uint64, error) {
	if linkID == "" {
		return 0, errors.New("link ID is empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	link := s.netKB.GetNetworkLink(linkID)
	if link == nil {
		return 0, ErrLinkNotFound
	}
	return link.AvailableBandwidthBps, nil
}

func (s *ScenarioState) initLinkBandwidthLocked(link *network.NetworkLink) {
	if link.MaxBandwidthBps == 0 {
		link.MaxBandwidthBps = defaultLinkBandwidthBps
	}
	if link.AvailableBandwidthBps == 0 {
		link.AvailableBandwidthBps = link.MaxBandwidthBps
	}
	if link.ReservedBandwidthBps > link.MaxBandwidthBps {
		link.ReservedBandwidthBps = link.MaxBandwidthBps
	}
	if link.AvailableBandwidthBps > link.MaxBandwidthBps {
		link.AvailableBandwidthBps = link.MaxBandwidthBps
	}
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

	if err := s.populateServiceRequestDomainsLocked(sr); err != nil {
		return err
	}

	if _, exists := s.serviceRequests[sr.ID]; exists {
		return ErrServiceRequestExists
	}
	s.serviceRequests[sr.ID] = sr

	if sr.CreatedAt.IsZero() {
		sr.CreatedAt = time.Now()
	}

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
	hadHistory := len(status.AllIntervals) > 0
	currentTime := s.nowForStatus()

	status.IsProvisionedNow = isProvisioned
	if isProvisioned {
		if intervalCopy == nil {
			intervalCopy = &model.TimeInterval{}
		}
		if intervalCopy.StartTime.IsZero() {
			intervalCopy.StartTime = currentTime
		}
		status.CurrentInterval = intervalCopy
		status.AllIntervals = append(status.AllIntervals, *intervalCopy)
		sr.ProvisionedIntervals = append(sr.ProvisionedIntervals, *intervalCopy)
		status.LastProvisionedAt = intervalCopy.StartTime
	} else {
		if intervalCopy == nil && status.CurrentInterval != nil {
			intervalCopy = copyTimeInterval(status.CurrentInterval)
		}
		if intervalCopy != nil && intervalCopy.EndTime.IsZero() {
			intervalCopy.EndTime = currentTime
		}
		if hadHistory {
			closeProvisionedInterval(intervalCopy, &status.AllIntervals)
			closeProvisionedInterval(intervalCopy, &sr.ProvisionedIntervals)
			if intervalCopy != nil {
				status.LastUnprovisionedAt = intervalCopy.EndTime
			}
		}
		status.CurrentInterval = nil
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

func (s *ScenarioState) nowForStatus() time.Time {
	if s.scheduler != nil {
		return s.scheduler.Now()
	}
	return time.Now()
}

func closeProvisionedInterval(interval *model.TimeInterval, intervals *[]model.TimeInterval) {
	if interval == nil || intervals == nil {
		return
	}
	if len(*intervals) == 0 {
		*intervals = append(*intervals, *interval)
		return
	}
	last := &(*intervals)[len(*intervals)-1]
	if last.StartTime.IsZero() {
		last.StartTime = interval.StartTime
	}
	if !interval.EndTime.IsZero() {
		if last.EndTime.IsZero() || interval.EndTime.After(last.EndTime) {
			last.EndTime = interval.EndTime
		}
	} else if last.EndTime.IsZero() {
		last.EndTime = interval.EndTime
	}
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

func difference(a, b map[string]struct{}) []string {
	if len(a) == 0 {
		return nil
	}
	var diff []string
	for id := range a {
		if b == nil {
			diff = append(diff, id)
			continue
		}
		if _, ok := b[id]; !ok {
			diff = append(diff, id)
		}
	}
	return diff
}

// AllocatePower reserves power for the given interface/entry combination.
func (s *ScenarioState) AllocatePower(interfaceID, entryID string, powerWatts float64) error {
	if interfaceID == "" || entryID == "" {
		return fmt.Errorf("interface and entry IDs are required")
	}
	if powerWatts < 0 {
		return fmt.Errorf("power must be non-negative")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allocatePowerLocked(interfaceID, entryID, powerWatts)
}

func (s *ScenarioState) allocatePowerLocked(interfaceID, entryID string, powerWatts float64) error {
	iface := s.netKB.GetNetworkInterface(interfaceID)
	if iface == nil {
		return fmt.Errorf("interface %s not found", interfaceID)
	}
	trx := s.netKB.GetTransceiverModel(iface.TransceiverID)
	limit := math.Inf(1)
	if trx != nil && trx.MaxPowerWatts > 0 {
		limit = trx.MaxPowerWatts
	}
	state := s.ensureInterfacePowerStateLocked(interfaceID)
	if _, exists := state.assignments[entryID]; exists {
		return fmt.Errorf("entry %s already allocated", entryID)
	}
	newAllocated := state.AllocatedPower + powerWatts
	if limit != math.Inf(1) && newAllocated > limit {
		return fmt.Errorf("power limit exceeded on %s (%.2f > %.2f)", interfaceID, newAllocated, limit)
	}
	state.assignments[entryID] = powerWatts
	state.AllocatedPower = newAllocated
	return nil
}

// ReleasePower removes a previously allocated beam power.
func (s *ScenarioState) ReleasePower(interfaceID, entryID string) {
	if interfaceID == "" || entryID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.interfacePowerStates[interfaceID]
	if state == nil {
		return
	}
	power, ok := state.assignments[entryID]
	if !ok {
		return
	}
	delete(state.assignments, entryID)
	state.AllocatedPower -= power
	if state.AllocatedPower < 0 {
		state.AllocatedPower = 0
	}
}

// GetAvailablePower reports remaining watts on the interface.
func (s *ScenarioState) GetAvailablePower(interfaceID string) float64 {
	if interfaceID == "" {
		return math.Inf(1)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.interfacePowerStates[interfaceID]
	var allocated float64
	if state != nil {
		allocated = state.AllocatedPower
	}

	iface := s.netKB.GetNetworkInterface(interfaceID)
	if iface == nil {
		return math.Inf(1)
	}
	trx := s.netKB.GetTransceiverModel(iface.TransceiverID)
	if trx == nil || trx.MaxPowerWatts <= 0 {
		return math.Inf(1)
	}
	return math.Max(0, trx.MaxPowerWatts-allocated)
}

func (s *ScenarioState) ensureInterfacePowerStateLocked(interfaceID string) *InterfacePowerState {
	if state := s.interfacePowerStates[interfaceID]; state != nil {
		return state
	}
	state := &InterfacePowerState{
		InterfaceID: interfaceID,
		assignments: make(map[string]float64),
	}
	s.interfacePowerStates[interfaceID] = state
	return state
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
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	if bytes <= 0 {
		return nil
	}

	storage, err := s.getStorage(nodeID)
	if err != nil {
		return err
	}
	return storage.reserve(uint64(math.Ceil(bytes)))
}

// ReleaseStorage decrements the DTN storage usage for a node, never dropping below zero.
func (s *ScenarioState) ReleaseStorage(nodeID string, bytes float64) {
	if nodeID == "" || bytes <= 0 {
		return
	}

	s.mu.RLock()
	storage := s.dtnStorage[nodeID]
	s.mu.RUnlock()
	if storage == nil {
		return
	}
	storage.release(uint64(math.Ceil(bytes)))
}

// StorageUsage reports the currently reserved DTN bytes for a node.
func (s *ScenarioState) StorageUsage(nodeID string) float64 {
	if nodeID == "" {
		return 0
	}
	s.mu.RLock()
	storage := s.dtnStorage[nodeID]
	s.mu.RUnlock()
	if storage == nil {
		return 0
	}
	used, _ := storage.usage()
	return float64(used)
}

// StoreMessage stores a DTN message at the specified node if space permits.
func (s *ScenarioState) StoreMessage(nodeID string, msg StoredMessage) error {
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	if msg.MessageID == "" {
		return fmt.Errorf("message ID is required")
	}
	if msg.SizeBytes == 0 {
		return fmt.Errorf("message %q must specify a positive size", msg.MessageID)
	}
	if msg.ArrivalTime.IsZero() {
		msg.ArrivalTime = s.now()
	}
	state := MessageStateStored
	if msg.Destination != "" && msg.Destination == nodeID {
		state = MessageStateDelivered
	}
	msg.State = state
	storage, err := s.getStorage(nodeID)
	if err != nil {
		return err
	}
	if err := storage.storeMessage(msg); err != nil {
		return err
	}
	s.recordMessageState(msg.MessageID, msg.State, msg.ArrivalTime)
	s.cancelExpiryEvent(msg.MessageID)
	s.scheduleExpiryEvent(nodeID, msg)
	return nil
}

// RetrieveMessage returns and removes a stored DTN message from a node.
func (s *ScenarioState) RetrieveMessage(nodeID, msgID string) (*StoredMessage, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	if msgID == "" {
		return nil, fmt.Errorf("message ID is required")
	}
	s.mu.RLock()
	storage := s.dtnStorage[nodeID]
	s.mu.RUnlock()
	if storage == nil {
		return nil, ErrNodeNotFound
	}
	msg, err := storage.retrieveMessage(msgID)
	if err != nil {
		return nil, err
	}
	s.recordMessageState(msg.MessageID, MessageStateInTransit, s.now())
	s.cancelExpiryEvent(msg.MessageID)
	return msg, nil
}

// GetStorageUsage returns the bytes used and configured capacity for a node's DTN storage.
func (s *ScenarioState) GetStorageUsage(nodeID string) (used, capacity uint64, err error) {
	if nodeID == "" {
		return 0, 0, fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	storage, err := s.getStorage(nodeID)
	if err != nil {
		return 0, 0, err
	}
	used, capacity = storage.usage()
	return used, capacity, nil
}

// EvictExpiredMessages removes messages whose expiry time has passed.
func (s *ScenarioState) EvictExpiredMessages(nodeID string, now time.Time) error {
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.mu.RLock()
	storage := s.dtnStorage[nodeID]
	s.mu.RUnlock()
	if storage == nil {
		return ErrNodeNotFound
	}
	expired := storage.evictExpired(now)
	for _, msg := range expired {
		s.recordMessageState(msg.MessageID, MessageStateExpired, now)
		s.cancelExpiryEvent(msg.MessageID)
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

func (s *ScenarioState) getStorage(nodeID string) (*DTNStorage, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	s.mu.RLock()
	storage := s.dtnStorage[nodeID]
	s.mu.RUnlock()
	if storage != nil {
		return storage, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.storageForNodeLocked(nodeID)
}

func (s *ScenarioState) storageForNodeLocked(nodeID string) (*DTNStorage, error) {
	storage := s.dtnStorage[nodeID]
	if storage != nil {
		return storage, nil
	}
	node := s.physKB.GetNetworkNode(nodeID)
	if node == nil {
		return nil, ErrNodeNotFound
	}
	storage = &DTNStorage{
		NodeID:        nodeID,
		CapacityBytes: nodeStorageCapacity(node),
	}
	s.dtnStorage[nodeID] = storage
	return storage, nil
}

func nodeStorageCapacity(node *model.NetworkNode) uint64 {
	if node == nil || node.StorageCapacityBytes <= 0 {
		return 0
	}
	return uint64(math.Max(0, math.Floor(node.StorageCapacityBytes)))
}

func newDTNStorage(node *model.NetworkNode) *DTNStorage {
	if node == nil {
		return &DTNStorage{}
	}
	return &DTNStorage{
		NodeID:        node.ID,
		CapacityBytes: nodeStorageCapacity(node),
	}
}

func (d *DTNStorage) reserve(bytes uint64) error {
	if bytes == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.CapacityBytes > 0 && d.UsedBytes+bytes > d.CapacityBytes {
		return ErrStorageFull
	}
	d.UsedBytes += bytes
	return nil
}

func (d *DTNStorage) release(bytes uint64) {
	if bytes == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.UsedBytes <= bytes {
		d.UsedBytes = 0
	} else {
		d.UsedBytes -= bytes
	}
}

func (d *DTNStorage) usage() (used, capacity uint64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.UsedBytes, d.CapacityBytes
}

func (d *DTNStorage) storeMessage(msg StoredMessage) error {
	if msg.SizeBytes == 0 {
		return fmt.Errorf("message %q has zero size", msg.MessageID)
	}
	if err := d.reserve(msg.SizeBytes); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Messages = append(d.Messages, msg)
	return nil
}

func (d *DTNStorage) retrieveMessage(msgID string) (*StoredMessage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, stored := range d.Messages {
		if stored.MessageID == msgID {
			// Remove from slice
			d.Messages = append(d.Messages[:i], d.Messages[i+1:]...)
			if d.UsedBytes <= stored.SizeBytes {
				d.UsedBytes = 0
			} else {
				d.UsedBytes -= stored.SizeBytes
			}
			stored.State = MessageStateInTransit
			return &stored, nil
		}
	}
	return nil, fmt.Errorf("message %q not found", msgID)
}

func (d *DTNStorage) evictExpired(now time.Time) []StoredMessage {
	d.mu.Lock()
	defer d.mu.Unlock()
	filtered := d.Messages[:0]
	var expired []StoredMessage
	for _, msg := range d.Messages {
		if !msg.ExpiryTime.IsZero() && !msg.ExpiryTime.After(now) {
			msg.State = MessageStateExpired
			if d.UsedBytes <= msg.SizeBytes {
				d.UsedBytes = 0
			} else {
				d.UsedBytes -= msg.SizeBytes
			}
			expired = append(expired, msg)
			continue
		}
		filtered = append(filtered, msg)
	}
	d.Messages = filtered
	return expired
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
	s.interfacePowerStates = make(map[string]*InterfacePowerState)
	s.dtnStorage = make(map[string]*DTNStorage)
	s.regions = make(map[string]*model.Region)
	s.regionMembership = make(map[string]*regionMembershipEntry)
	s.domains = make(map[string]*model.SchedulingDomain)
	s.nodeDomains = make(map[string]string)

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

// CreateRegion registers a geographic region for later membership queries.
func (s *ScenarioState) CreateRegion(region *model.Region) error {
	if region == nil {
		return fmt.Errorf("%w: region is nil", ErrRegionInvalid)
	}
	if region.ID == "" {
		return fmt.Errorf("%w: empty region ID", ErrRegionInvalid)
	}
	if err := validateRegion(region); err != nil {
		return fmt.Errorf("%w: %v", ErrRegionInvalid, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.regions[region.ID]; exists {
		return fmt.Errorf("%w: %q", ErrRegionExists, region.ID)
	}
	s.regions[region.ID] = cloneRegion(region)
	return nil
}

// GetRegion returns the region definition for regionID.
func (s *ScenarioState) GetRegion(regionID string) (*model.Region, error) {
	if regionID == "" {
		return nil, fmt.Errorf("%w: empty region ID", ErrRegionNotFound)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	region := s.regions[regionID]
	if region == nil {
		return nil, fmt.Errorf("%w: %q", ErrRegionNotFound, regionID)
	}
	return cloneRegion(region), nil
}

// ListRegions returns all configured regions.
func (s *ScenarioState) ListRegions() []*model.Region {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*model.Region, 0, len(s.regions))
	for _, region := range s.regions {
		res = append(res, cloneRegion(region))
	}
	return res
}

// DeleteRegion removes the specified region.
func (s *ScenarioState) DeleteRegion(regionID string) error {
	if regionID == "" {
		return fmt.Errorf("%w: empty region ID", ErrRegionNotFound)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.regions[regionID]; !exists {
		return fmt.Errorf("%w: %q", ErrRegionNotFound, regionID)
	}
	delete(s.regions, regionID)
	return nil
}

// CreateDomain registers a federation scheduling domain.
func (s *ScenarioState) CreateDomain(domain *model.SchedulingDomain) error {
	if err := validateDomain(domain); err != nil {
		return fmt.Errorf("%w: %v", ErrDomainInvalid, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	domainID := domain.DomainID
	if _, exists := s.domains[domainID]; exists {
		return fmt.Errorf("%w: %q", ErrDomainExists, domainID)
	}

	uniqueNodes := make([]string, 0, len(domain.Nodes))
	seen := make(map[string]struct{})
	for _, nodeID := range domain.Nodes {
		nodeID = strings.TrimSpace(nodeID)
		if nodeID == "" {
			return fmt.Errorf("%w: domain %q has empty node entry", ErrDomainInvalid, domainID)
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		node := s.physKB.GetNetworkNode(nodeID)
		if node == nil {
			return fmt.Errorf("%w: %q", ErrNodeNotFound, nodeID)
		}
		if existing := s.nodeDomains[nodeID]; existing != "" {
			return fmt.Errorf("%w: node %q already assigned to %q", ErrDomainInvalid, nodeID, existing)
		}
		uniqueNodes = append(uniqueNodes, nodeID)
	}

	cloned := cloneDomain(domain)
	if len(uniqueNodes) > 0 {
		cloned.Nodes = append([]string(nil), uniqueNodes...)
	} else {
		cloned.Nodes = nil
	}

	s.domains[domainID] = cloned
	for _, nodeID := range uniqueNodes {
		s.nodeDomains[nodeID] = domainID
	}
	return nil
}

// GetDomain returns the domain definition for domainID.
func (s *ScenarioState) GetDomain(domainID string) (*model.SchedulingDomain, error) {
	if strings.TrimSpace(domainID) == "" {
		return nil, fmt.Errorf("%w: empty domain ID", ErrDomainNotFound)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	domain := s.domains[domainID]
	if domain == nil {
		return nil, fmt.Errorf("%w: %q", ErrDomainNotFound, domainID)
	}
	return cloneDomain(domain), nil
}

// ListDomains returns all configured domains.
func (s *ScenarioState) ListDomains() []*model.SchedulingDomain {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.SchedulingDomain, 0, len(s.domains))
	for _, domain := range s.domains {
		result = append(result, cloneDomain(domain))
	}
	return result
}

// DeleteDomain removes the specified federation domain.
func (s *ScenarioState) DeleteDomain(domainID string) error {
	if strings.TrimSpace(domainID) == "" {
		return fmt.Errorf("%w: empty domain ID", ErrDomainNotFound)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.domains[domainID]; !exists {
		return fmt.Errorf("%w: %q", ErrDomainNotFound, domainID)
	}
	delete(s.domains, domainID)
	for nodeID, assigned := range s.nodeDomains {
		if assigned == domainID {
			delete(s.nodeDomains, nodeID)
		}
	}
	return nil
}

// SetRegionMembershipTTL configures how long cached membership snapshots remain valid.
func (s *ScenarioState) SetRegionMembershipTTL(ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regionMembershipTTL = ttl
}

// SetRegionMembershipHook registers a callback invoked whenever membership changes.
func (s *ScenarioState) SetRegionMembershipHook(hook RegionMembershipHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regionMembershipHook = hook
}

// UpdateRegionMembership recomputes the nodes currently inside regionID.
func (s *ScenarioState) UpdateRegionMembership(regionID string) error {
	region, err := s.GetRegion(regionID)
	if err != nil {
		return err
	}
	now := s.now()

	s.mu.Lock()
	var (
		prevMembers map[string]struct{}
		entry       = s.regionMembership[regionID]
		hook        RegionMembershipHook
		left        []string
		entered     []string
	)
	defer func() {
		s.mu.Unlock()
		if hook != nil && (len(left) > 0 || len(entered) > 0) {
			hook(regionID, left, entered)
		}
	}()

	if entry != nil && len(entry.members) > 0 {
		prevMembers = make(map[string]struct{}, len(entry.members))
		for nodeID := range entry.members {
			prevMembers[nodeID] = struct{}{}
		}
	}

	newMembers := make(map[string]struct{})
	if s.physKB != nil {
		for _, node := range s.physKB.ListNetworkNodes() {
			if node == nil {
				continue
			}
			if coords, ok := s.nodeCoordinates(node); ok && regionContains(region, coords) {
				newMembers[node.ID] = struct{}{}
			}
		}
	}

	left = difference(prevMembers, newMembers)
	entered = difference(newMembers, prevMembers)

	if entry == nil {
		entry = &regionMembershipEntry{}
	}
	entry.members = newMembers
	entry.updatedAt = now
	s.regionMembership[regionID] = entry
	hook = s.regionMembershipHook
	return nil
}

// CheckRegionMembership reports whether nodeID currently falls inside regionID.
func (s *ScenarioState) CheckRegionMembership(nodeID, regionID string) bool {
	if nodeID == "" || regionID == "" {
		return false
	}
	now := s.now()
	s.mu.RLock()
	entry := s.regionMembership[regionID]
	ttl := s.regionMembershipTTL
	members := entry != nil && entry.members != nil
	var member bool
	if members {
		_, member = entry.members[nodeID]
	}
	stale := entry == nil || ttl <= 0 || now.Sub(entry.updatedAt) > ttl
	s.mu.RUnlock()

	if !stale {
		return member
	}
	if err := s.UpdateRegionMembership(regionID); err != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry = s.regionMembership[regionID]
	if entry == nil || entry.members == nil {
		return false
	}
	_, member = entry.members[nodeID]
	return member
}

// GetDomainForNode returns the domain ID that owns nodeID.
func (s *ScenarioState) GetDomainForNode(nodeID string) (string, error) {
	if strings.TrimSpace(nodeID) == "" {
		return "", fmt.Errorf("%w: empty node ID", ErrNodeInvalid)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.physKB.GetNetworkNode(nodeID) == nil {
		return "", fmt.Errorf("%w: %q", ErrNodeNotFound, nodeID)
	}
	domainID := s.nodeDomains[nodeID]
	if domainID == "" {
		return "", fmt.Errorf("%w: node %q", ErrDomainNotFound, nodeID)
	}
	return domainID, nil
}

// GetNodesInRegion returns the node IDs that currently fall inside regionID.
func (s *ScenarioState) GetNodesInRegion(regionID string) ([]string, error) {
	region, err := s.GetRegion(regionID)
	if err != nil {
		return nil, err
	}
	if s.physKB == nil {
		return nil, nil
	}
	var ids []string
	for _, node := range s.physKB.ListNetworkNodes() {
		if coords, ok := s.nodeCoordinates(node); ok && regionContains(region, coords) {
			ids = append(ids, node.ID)
		}
	}
	return ids, nil
}

func (s *ScenarioState) now() time.Time {
	if s.scheduler != nil {
		return s.scheduler.Now()
	}
	return time.Now()
}

func (s *ScenarioState) recordMessageState(msgID string, state MessageState, at time.Time) {
	if msgID == "" {
		return
	}
	if at.IsZero() {
		at = s.now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messageHistory == nil {
		s.messageHistory = make(map[string][]MessageStateTransition)
	}
	s.messageHistory[msgID] = append(s.messageHistory[msgID], MessageStateTransition{
		State: state,
		Time:  at,
	})
	s.messageStates[msgID] = state
}

func (s *ScenarioState) cancelExpiryEvent(msgID string) {
	if s.scheduler == nil || msgID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expiryEvents == nil {
		return
	}
	id, ok := s.expiryEvents[msgID]
	if !ok {
		return
	}
	delete(s.expiryEvents, msgID)
	s.scheduler.Cancel(id)
}

func (s *ScenarioState) scheduleExpiryEvent(nodeID string, msg StoredMessage) {
	if s.scheduler == nil || msg.MessageID == "" || msg.ExpiryTime.IsZero() {
		return
	}
	when := msg.ExpiryTime
	if when.Before(s.now()) {
		go s.handleMessageExpiry(nodeID, msg.MessageID, when)
		return
	}
	msgID := msg.MessageID
	eventID := s.scheduler.Schedule(when, func() {
		s.handleMessageExpiry(nodeID, msgID, when)
	})
	s.mu.Lock()
	if s.expiryEvents == nil {
		s.expiryEvents = make(map[string]string)
	}
	s.expiryEvents[msgID] = eventID
	s.mu.Unlock()
}

func (s *ScenarioState) handleMessageExpiry(nodeID, msgID string, when time.Time) {
	if nodeID == "" || msgID == "" {
		return
	}
	_ = s.EvictExpiredMessages(nodeID, when)
	s.cancelExpiryEvent(msgID)
	s.recordMessageState(msgID, MessageStateExpired, when)
}

// GetMessageState returns the latest state and history for the given message.
// The bool return indicates whether the message is known to the state tracker.
func (s *ScenarioState) GetMessageState(msgID string) (MessageState, []MessageStateTransition, bool) {
	if msgID == "" {
		return "", nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.messageStates[msgID]
	history := append([]MessageStateTransition(nil), s.messageHistory[msgID]...)
	return state, history, ok
}

// MessagesInState returns the stored messages currently tracked in the specified state.
func (s *ScenarioState) MessagesInState(state MessageState) []StoredMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []StoredMessage
	for msgID, msgState := range s.messageStates {
		if msgState != state {
			continue
		}
		if stored := s.findStoredMessageLocked(msgID); stored != nil {
			results = append(results, *stored)
			continue
		}
		results = append(results, StoredMessage{
			MessageID: msgID,
			State:     msgState,
		})
	}
	return results
}

func (s *ScenarioState) findStoredMessageLocked(msgID string) *StoredMessage {
	for _, storage := range s.dtnStorage {
		if storage == nil {
			continue
		}
		storage.mu.RLock()
		for _, msg := range storage.Messages {
			if msg.MessageID == msgID {
				copy := msg
				storage.mu.RUnlock()
				return &copy
			}
		}
		storage.mu.RUnlock()
	}
	return nil
}

const metresPerKilometre = 1000.0

func validateRegion(region *model.Region) error {
	switch region.Type {
	case model.RegionTypeCircle:
		if region.RadiusKm <= 0 {
			return errors.New("circle radius must be positive")
		}
	case model.RegionTypePolygon:
		if len(region.Vertices) < 3 {
			return errors.New("polygon requires at least 3 vertices")
		}
	case model.RegionTypeCountry:
		if strings.TrimSpace(region.CountryCode) == "" {
			return errors.New("country code required")
		}
	default:
		return fmt.Errorf("unknown region type %q", region.Type)
	}
	return nil
}

func cloneRegion(region *model.Region) *model.Region {
	if region == nil {
		return nil
	}
	clone := *region
	if len(region.Vertices) > 0 {
		clone.Vertices = append([]model.Coordinates(nil), region.Vertices...)
	}
	return &clone
}

func validateDomain(domain *model.SchedulingDomain) error {
	if domain == nil {
		return errors.New("domain is nil")
	}
	if strings.TrimSpace(domain.DomainID) == "" {
		return errors.New("domain ID is required")
	}
	if domain.FederationEndpoint != "" {
		if _, err := url.ParseRequestURI(domain.FederationEndpoint); err != nil {
			return fmt.Errorf("invalid federation endpoint: %w", err)
		}
	}
	return nil
}

func cloneDomain(domain *model.SchedulingDomain) *model.SchedulingDomain {
	if domain == nil {
		return nil
	}
	clone := *domain
	if len(domain.Nodes) > 0 {
		clone.Nodes = append([]string(nil), domain.Nodes...)
	}
	if len(domain.Capabilities) > 0 {
		clone.Capabilities = make(map[string]interface{}, len(domain.Capabilities))
		for key, value := range domain.Capabilities {
			clone.Capabilities[key] = value
		}
	} else {
		clone.Capabilities = nil
	}
	return &clone
}

// populateServiceRequestDomainsLocked ensures domain metadata is valid while holding s.mu.
func (s *ScenarioState) populateServiceRequestDomainsLocked(sr *model.ServiceRequest) error {
	if sr == nil {
		return nil
	}

	sourceDomain := strings.TrimSpace(sr.SourceDomain)
	destDomain := strings.TrimSpace(sr.DestDomain)

	if sr.SrcNodeID != "" {
		if s.physKB.GetNetworkNode(sr.SrcNodeID) == nil {
			return fmt.Errorf("%w: %q", ErrNodeNotFound, sr.SrcNodeID)
		}
		if sourceDomain == "" {
			sourceDomain = s.nodeDomains[sr.SrcNodeID]
		}
		if sourceDomain != "" {
			if _, exists := s.domains[sourceDomain]; !exists {
				return fmt.Errorf("%w: %q", ErrDomainNotFound, sourceDomain)
			}
		}
	}

	if sr.DstNodeID != "" {
		if s.physKB.GetNetworkNode(sr.DstNodeID) == nil {
			return fmt.Errorf("%w: %q", ErrNodeNotFound, sr.DstNodeID)
		}
		if destDomain == "" {
			destDomain = s.nodeDomains[sr.DstNodeID]
		}
		if destDomain != "" {
			if _, exists := s.domains[destDomain]; !exists {
				return fmt.Errorf("%w: %q", ErrDomainNotFound, destDomain)
			}
		}
	}

	crossDomain := sourceDomain != "" && destDomain != "" && sourceDomain != destDomain
	if crossDomain && sr.FederationToken == "" {
		return fmt.Errorf("%w: federation token required for cross-domain request", ErrDomainInvalid)
	}

	sr.SourceDomain = sourceDomain
	sr.DestDomain = destDomain
	sr.CrossDomain = crossDomain
	return nil
}

func (s *ScenarioState) nodeCoordinates(node *model.NetworkNode) (model.Coordinates, bool) {
	if node == nil || node.PlatformID == "" || s.physKB == nil {
		return model.Coordinates{}, false
	}
	platform := s.physKB.GetPlatform(node.PlatformID)
	if platform == nil {
		return model.Coordinates{}, false
	}
	return model.Coordinates{
		X: platform.Coordinates.X,
		Y: platform.Coordinates.Y,
		Z: platform.Coordinates.Z,
	}, true
}

func regionContains(region *model.Region, point model.Coordinates) bool {
	if region == nil {
		return false
	}
	switch region.Type {
	case model.RegionTypeCircle:
		return distanceMeters(region.Center, point) <= region.RadiusKm*metresPerKilometre
	case model.RegionTypePolygon:
		return pointInPolygon(point, region.Vertices)
	case model.RegionTypeCountry:
		return false // country membership not yet implemented
	default:
		return false
	}
}

func distanceMeters(a, b model.Coordinates) float64 {
	return math.Sqrt((a.X-b.X)*(a.X-b.X) + (a.Y-b.Y)*(a.Y-b.Y) + (a.Z-b.Z)*(a.Z-b.Z))
}

func pointInPolygon(point model.Coordinates, vertices []model.Coordinates) bool {
	if len(vertices) < 3 {
		return false
	}
	inside := false
	for i, j := 0, len(vertices)-1; i < len(vertices); j, i = i, i+1 {
		vi := vertices[i]
		vj := vertices[j]
		intersect := ((vi.Y > point.Y) != (vj.Y > point.Y)) &&
			(point.X < (vj.X-vi.X)*(point.Y-vi.Y)/(vj.Y-vi.Y)+vi.X)
		if intersect {
			inside = !inside
		}
	}
	return inside
}
