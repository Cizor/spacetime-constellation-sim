package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrLinkExists        = errors.New("link already exists")
	ErrLinkNotFound      = errors.New("link not found")
	ErrLinkBadInput      = errors.New("invalid link")
	ErrEmptyLinkID       = errors.New("empty link ID")
	ErrInterfaceMiss     = errors.New("link references unknown interface")
	ErrInterfaceExists   = errors.New("interface already exists")
	ErrInterfaceNotFound = errors.New("interface not found")
	ErrInterfaceBadInput = errors.New("invalid interface")
)

// KnowledgeBase is the Scope-2 network KB: it stores network
// interfaces, links, transceiver models and node positions in ECEF.
//
// NOTE: This KB is now concurrency-safe via an internal RWMutex so
// it can be safely accessed from multiple goroutines (e.g. future
// gRPC/NBI handlers) as long as all access goes through these methods.
type KnowledgeBase struct {
	mu sync.RWMutex

	interfaces       map[string]*NetworkInterface
	links            map[string]*NetworkLink
	linksByInterface map[string]map[string]*NetworkLink
	transceivers     map[string]*TransceiverModel
	nodePositions    map[string]Vec3
}

// NewKnowledgeBase creates an empty network knowledge base.
func NewKnowledgeBase() *KnowledgeBase {
	return &KnowledgeBase{
		interfaces:       make(map[string]*NetworkInterface),
		links:            make(map[string]*NetworkLink),
		linksByInterface: make(map[string]map[string]*NetworkLink),
		transceivers:     make(map[string]*TransceiverModel),
		nodePositions:    make(map[string]Vec3),
	}
}

//
// ---------- Transceiver models ----------
//

func (kb *KnowledgeBase) AddTransceiverModel(trx *TransceiverModel) error {
	if trx == nil || trx.ID == "" {
		return fmt.Errorf("nil or empty transceiver model")
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if _, exists := kb.transceivers[trx.ID]; exists {
		return fmt.Errorf("transceiver model %q already exists", trx.ID)
	}
	kb.transceivers[trx.ID] = trx
	return nil
}

func (kb *KnowledgeBase) GetTransceiverModel(id string) *TransceiverModel {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.transceivers[id]
}

//
// ---------- Interfaces ----------
//

func (kb *KnowledgeBase) AddInterface(intf *NetworkInterface) error {
	if intf == nil || intf.ID == "" {
		return fmt.Errorf("%w", ErrInterfaceBadInput)
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if _, exists := kb.interfaces[intf.ID]; exists {
		return fmt.Errorf("%w: %q", ErrInterfaceExists, intf.ID)
	}
	kb.interfaces[intf.ID] = intf
	return nil
}

// GetNetworkInterface returns an interface by ID, or nil if not found.
func (kb *KnowledgeBase) GetNetworkInterface(id string) *NetworkInterface {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.interfaces[id]
}

// GetAllInterfaces returns all network interfaces.
func (kb *KnowledgeBase) GetAllInterfaces() []*NetworkInterface {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	out := make([]*NetworkInterface, 0, len(kb.interfaces))
	for _, intf := range kb.interfaces {
		out = append(out, intf)
	}
	return out
}

// GetInterfacesForNode returns interfaces whose ParentNodeID matches nodeID.
func (kb *KnowledgeBase) GetInterfacesForNode(nodeID string) []*NetworkInterface {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var out []*NetworkInterface
	for _, intf := range kb.interfaces {
		if intf != nil && intf.ParentNodeID == nodeID {
			out = append(out, intf)
		}
	}
	return out
}

// ReplaceInterfacesForNode deletes any interfaces attached to the node and
// inserts the provided replacements. Any links attached to removed interfaces
// are also deleted to keep adjacency consistent.
func (kb *KnowledgeBase) ReplaceInterfacesForNode(nodeID string, interfaces []*NetworkInterface) error {
	if nodeID == "" {
		return fmt.Errorf("%w: empty node ID", ErrInterfaceBadInput)
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	for id, iface := range kb.interfaces {
		if iface != nil && iface.ParentNodeID == nodeID {
			kb.deleteInterfaceLocked(id)
		}
	}

	for _, iface := range interfaces {
		if iface == nil || iface.ID == "" {
			return fmt.Errorf("%w", ErrInterfaceBadInput)
		}
		if iface.ParentNodeID == "" {
			iface.ParentNodeID = nodeID
		}
		if iface.ParentNodeID != nodeID {
			return fmt.Errorf("%w: interface %q parent %q does not match node %q", ErrInterfaceBadInput, iface.ID, iface.ParentNodeID, nodeID)
		}
		if _, exists := kb.interfaces[iface.ID]; exists {
			return fmt.Errorf("%w: %q", ErrInterfaceExists, iface.ID)
		}
		kb.interfaces[iface.ID] = iface
	}

	return nil
}

// DeleteInterface removes a single interface and any links that reference it.
func (kb *KnowledgeBase) DeleteInterface(id string) error {
	if id == "" {
		return fmt.Errorf("%w", ErrInterfaceBadInput)
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if _, ok := kb.interfaces[id]; !ok {
		return fmt.Errorf("%w: %q", ErrInterfaceNotFound, id)
	}

	kb.deleteInterfaceLocked(id)
	return nil
}

//
// ---------- Links ----------
//

// AddNetworkLink inserts a new (typically static / scenario-defined)
// link and updates adjacency maps and per-interface LinkIDs.
func (kb *KnowledgeBase) AddNetworkLink(link *NetworkLink) error {
	if link == nil {
		return fmt.Errorf("%w", ErrLinkBadInput)
	}
	if link.ID == "" {
		return fmt.Errorf("%w", ErrEmptyLinkID)
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if _, exists := kb.links[link.ID]; exists {
		return fmt.Errorf("%w: %q", ErrLinkExists, link.ID)
	}

	// Validate that the referenced interfaces exist (when specified).
	if link.InterfaceA != "" {
		if _, ok := kb.interfaces[link.InterfaceA]; !ok {
			return fmt.Errorf("%w: %q references unknown interface %q", ErrInterfaceMiss, link.ID, link.InterfaceA)
		}
	}
	if link.InterfaceB != "" {
		if _, ok := kb.interfaces[link.InterfaceB]; !ok {
			return fmt.Errorf("%w: %q references unknown interface %q", ErrInterfaceMiss, link.ID, link.InterfaceB)
		}
	}

	kb.links[link.ID] = link

	// Adjacency: linksByInterface and interface.LinkIDs.
	kb.attachLinkToInterface(link.ID, link.InterfaceA)
	kb.attachLinkToInterface(link.ID, link.InterfaceB)

	return nil
}

// UpdateNetworkLink overwrites the stored link with the same ID.
// For Scope 2 we assume endpoints do not change at runtime, so we
// don’t need to rebuild adjacency here.
func (kb *KnowledgeBase) UpdateNetworkLink(link *NetworkLink) error {
	if link == nil || link.ID == "" {
		return fmt.Errorf("nil or empty link")
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if _, exists := kb.links[link.ID]; !exists {
		return fmt.Errorf("%w: %q", ErrLinkNotFound, link.ID)
	}
	kb.links[link.ID] = link
	return nil
}

// DeleteNetworkLink removes a link by ID and cleans up adjacency state.
func (kb *KnowledgeBase) DeleteNetworkLink(id string) error {
	if id == "" {
		return fmt.Errorf("%w", ErrEmptyLinkID)
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	link, exists := kb.links[id]
	if !exists {
		return fmt.Errorf("%w: %q", ErrLinkNotFound, id)
	}

	// Remove adjacency before deleting the link entry.
	kb.detachLinkFromInterface(id, link.InterfaceA)
	kb.detachLinkFromInterface(id, link.InterfaceB)
	delete(kb.links, id)
	return nil
}

// GetAllNetworkLinks returns all links in the KB.
func (kb *KnowledgeBase) GetAllNetworkLinks() []*NetworkLink {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	out := make([]*NetworkLink, 0, len(kb.links))
	for _, l := range kb.links {
		out = append(out, l)
	}
	return out
}

// GetNetworkLink returns a single link by ID, or nil if missing.
func (kb *KnowledgeBase) GetNetworkLink(id string) *NetworkLink {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return kb.links[id]
}

// GetLinksForInterface returns all links attached to a given interface.
func (kb *KnowledgeBase) GetLinksForInterface(ifID string) []*NetworkLink {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	m, ok := kb.linksByInterface[ifID]
	if !ok {
		return nil
	}
	out := make([]*NetworkLink, 0, len(m))
	for _, l := range m {
		out = append(out, l)
	}
	return out
}

// GetUpLinks returns all links currently marked as up.
func (kb *KnowledgeBase) GetUpLinks() []*NetworkLink {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	out := make([]*NetworkLink, 0)
	for _, l := range kb.links {
		if l.IsUp {
			out = append(out, l)
		}
	}
	return out
}

// GetNeighbours returns neighbour node IDs reachable from nodeID via
// currently-up links.
func (kb *KnowledgeBase) GetNeighbours(nodeID string) []string {
	if nodeID == "" {
		return nil
	}

	kb.mu.RLock()
	defer kb.mu.RUnlock()

	neigh := make(map[string]struct{})

	for _, link := range kb.links {
		if !link.IsUp {
			continue
		}
		intfA := kb.interfaces[link.InterfaceA]
		intfB := kb.interfaces[link.InterfaceB]
		if intfA == nil || intfB == nil {
			continue
		}
		nodeA := intfA.ParentNodeID
		nodeB := intfB.ParentNodeID

		if nodeA == nodeID && nodeB != "" && nodeB != nodeID {
			neigh[nodeB] = struct{}{}
		}
		if nodeB == nodeID && nodeA != "" && nodeA != nodeID {
			neigh[nodeA] = struct{}{}
		}
	}

	out := make([]string, 0, len(neigh))
	for id := range neigh {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Clear removes interfaces, links, adjacency maps, and node positions,
// leaving transceiver models untouched.
func (kb *KnowledgeBase) Clear() {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	kb.interfaces = make(map[string]*NetworkInterface)
	kb.links = make(map[string]*NetworkLink)
	kb.linksByInterface = make(map[string]map[string]*NetworkLink)
	kb.nodePositions = make(map[string]Vec3)
}

// attachLinkToInterface updates linksByInterface and the interface's
// LinkIDs slice to include linkID.
//
// NOTE: caller must hold kb.mu (write lock).
func (kb *KnowledgeBase) attachLinkToInterface(linkID, ifID string) {
	if ifID == "" {
		return
	}
	m, ok := kb.linksByInterface[ifID]
	if !ok {
		m = make(map[string]*NetworkLink)
		kb.linksByInterface[ifID] = m
	}
	link := kb.links[linkID]
	m[linkID] = link

	if intf := kb.interfaces[ifID]; intf != nil {
		intf.LinkIDs = appendIfMissing(intf.LinkIDs, linkID)
	}
}

// detachLinkFromInterface removes linkID from adjacency maps and
// per-interface LinkIDs.
//
// NOTE: caller must hold kb.mu (write lock).
func (kb *KnowledgeBase) detachLinkFromInterface(linkID, ifID string) {
	if ifID == "" {
		return
	}
	if m, ok := kb.linksByInterface[ifID]; ok {
		delete(m, linkID)
		if len(m) == 0 {
			delete(kb.linksByInterface, ifID)
		}
	}
	if intf := kb.interfaces[ifID]; intf != nil {
		newIDs := make([]string, 0, len(intf.LinkIDs))
		for _, id := range intf.LinkIDs {
			if id != linkID {
				newIDs = append(newIDs, id)
			}
		}
		intf.LinkIDs = newIDs
	}
}

// deleteInterfaceLocked removes the interface with the provided ID and cleans
// up any adjacency state. Caller must hold kb.mu (write lock).
func (kb *KnowledgeBase) deleteInterfaceLocked(id string) {
	for linkID, link := range kb.links {
		if link.InterfaceA == id || link.InterfaceB == id {
			kb.detachLinkFromInterface(linkID, link.InterfaceA)
			kb.detachLinkFromInterface(linkID, link.InterfaceB)
			delete(kb.links, linkID)
		}
	}

	delete(kb.linksByInterface, id)
	delete(kb.interfaces, id)
}

//
// ---------- Dynamic wireless links ----------
//

// ClearDynamicWirelessLinks removes any link that we consider
// "dynamic wireless" based on ID prefix and medium. We treat links
// whose IDs start with "dyn-" and MediumWireless as dynamic.
func (kb *KnowledgeBase) ClearDynamicWirelessLinks() {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	for id, link := range kb.links {
		if link.Medium != MediumWireless {
			continue
		}
		if !strings.HasPrefix(id, "dyn-") {
			continue
		}
		// Remove adjacency first.
		kb.detachLinkFromInterface(id, link.InterfaceA)
		kb.detachLinkFromInterface(id, link.InterfaceB)
		delete(kb.links, id)
	}
}

// UpsertDynamicWirelessLink creates (or reuses) a dynamic wireless
// link between two interfaces. The ID is constructed in a symmetric
// fashion so A–B and B–A share the same link.
//
// NOTE: returns only *NetworkLink (no error) so existing call sites
// like `_ = kb.UpsertDynamicWirelessLink(...)` remain valid.
func (kb *KnowledgeBase) UpsertDynamicWirelessLink(ifA, ifB string) *NetworkLink {
	if ifA == "" || ifB == "" {
		return nil
	}

	ids := []string{ifA, ifB}
	sort.Strings(ids)
	id := fmt.Sprintf("dyn-%s-%s", ids[0], ids[1])

	kb.mu.Lock()
	defer kb.mu.Unlock()

	if existing, ok := kb.links[id]; ok {
		// Ensure adjacency is present in case it was partially
		// removed earlier.
		kb.attachLinkToInterface(id, ifA)
		kb.attachLinkToInterface(id, ifB)
		return existing
	}

	link := &NetworkLink{
		ID:         id,
		InterfaceA: ifA,
		InterfaceB: ifB,
		Medium:     MediumWireless,
	}
	kb.links[id] = link

	kb.attachLinkToInterface(id, ifA)
	kb.attachLinkToInterface(id, ifB)

	return link
}

//
// ---------- Node positions (ECEF) ----------
//

func (kb *KnowledgeBase) SetNodeECEFPosition(nodeID string, pos Vec3) {
	if nodeID == "" {
		return
	}
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.nodePositions[nodeID] = pos
}

func (kb *KnowledgeBase) GetNodeECEFPosition(nodeID string) (Vec3, bool) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	pos, ok := kb.nodePositions[nodeID]
	return pos, ok
}

//
// ---------- Manual impairment helpers ----------
//

// SetInterfaceImpaired marks a network interface as administratively impaired
// or not. We currently model this by toggling IsOperational:
//
//	impaired = true  -> IsOperational = false
//	impaired = false -> IsOperational = true
func (kb *KnowledgeBase) SetInterfaceImpaired(ifID string, impaired bool) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	intf, ok := kb.interfaces[ifID]
	if !ok {
		return fmt.Errorf("SetInterfaceImpaired: interface %q not found", ifID)
	}

	intf.IsOperational = !impaired
	return nil
}

// SetLinkImpaired marks a network link as administratively impaired or not.
// ConnectivityService treats IsImpaired as a hard override on geometry/SNR.
func (kb *KnowledgeBase) SetLinkImpaired(linkID string, impaired bool) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	link, ok := kb.links[linkID]
	if !ok {
		return fmt.Errorf("SetLinkImpaired: link %q not found", linkID)
	}

	link.IsImpaired = impaired
	return nil
}

//
// ---------- Helpers ----------
//

func appendIfMissing(slice []string, id string) []string {
	for _, v := range slice {
		if v == id {
			return slice
		}
	}
	return append(slice, id)
}
