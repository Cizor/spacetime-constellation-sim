package kb

import (
    "fmt"
    "sync"

    "github.com/signalsfoundry/constellation-simulator/model"
)

// EventType indicates what kind of change happened in the KB.
type EventType int

const (
    EventPlatformUpdated EventType = iota
)

// Event is emitted to subscribers when something interesting happens.
type Event struct {
    Type     EventType
    Platform model.PlatformDefinition
}

// KnowledgeBase is an in-memory, thread-safe store for platforms and nodes.
type KnowledgeBase struct {
    mu sync.RWMutex

    platforms map[string]*model.PlatformDefinition
    nodes     map[string]*model.NetworkNode

    subs []func(Event)
}

// NewKnowledgeBase constructs an empty KB.
func NewKnowledgeBase() *KnowledgeBase {
    return &KnowledgeBase{
        platforms: make(map[string]*model.PlatformDefinition),
        nodes:     make(map[string]*model.NetworkNode),
    }
}

// AddPlatform adds a new platform. It returns an error if the ID already exists.
func (kb *KnowledgeBase) AddPlatform(p *model.PlatformDefinition) error {
    kb.mu.Lock()
    defer kb.mu.Unlock()

    if _, exists := kb.platforms[p.ID]; exists {
        return fmt.Errorf("platform with ID %q already exists", p.ID)
    }
    // store pointer so that motion models can update in-place
    kb.platforms[p.ID] = p
    return nil
}

// AddNetworkNode adds a new network node. It returns an error if the ID already exists
// or if the referenced platform does not exist.
func (kb *KnowledgeBase) AddNetworkNode(n *model.NetworkNode) error {
    kb.mu.Lock()
    defer kb.mu.Unlock()

    if _, exists := kb.nodes[n.ID]; exists {
        return fmt.Errorf("node with ID %q already exists", n.ID)
    }
    if n.PlatformID != "" {
        if _, ok := kb.platforms[n.PlatformID]; !ok {
            return fmt.Errorf("platform with ID %q not found for node", n.PlatformID)
        }
    }
    kb.nodes[n.ID] = n
    return nil
}

// GetPlatform returns the platform with the given ID, or nil if not found.
func (kb *KnowledgeBase) GetPlatform(id string) *model.PlatformDefinition {
    kb.mu.RLock()
    defer kb.mu.RUnlock()
    return kb.platforms[id]
}

// GetNetworkNode returns the network node with the given ID, or nil if not found.
func (kb *KnowledgeBase) GetNetworkNode(id string) *model.NetworkNode {
    kb.mu.RLock()
    defer kb.mu.RUnlock()
    return kb.nodes[id]
}

// ListPlatforms returns a snapshot slice of all platforms.
func (kb *KnowledgeBase) ListPlatforms() []*model.PlatformDefinition {
    kb.mu.RLock()
    defer kb.mu.RUnlock()

    res := make([]*model.PlatformDefinition, 0, len(kb.platforms))
    for _, p := range kb.platforms {
        res = append(res, p)
    }
    return res
}

// ListNetworkNodes returns a snapshot slice of all network nodes.
func (kb *KnowledgeBase) ListNetworkNodes() []*model.NetworkNode {
    kb.mu.RLock()
    defer kb.mu.RUnlock()

    res := make([]*model.NetworkNode, 0, len(kb.nodes))
    for _, n := range kb.nodes {
        res = append(res, n)
    }
    return res
}

// UpdatePlatformPosition updates a platform's coordinates and notifies subscribers.
func (kb *KnowledgeBase) UpdatePlatformPosition(id string, pos model.Motion) error {
    kb.mu.Lock()
    p, ok := kb.platforms[id]
    if !ok {
        kb.mu.Unlock()
        return fmt.Errorf("platform with ID %q not found", id)
    }
    p.Coordinates = pos
    event := Event{
        Type:     EventPlatformUpdated,
        Platform: *p, // copy for safety
    }
    subs := append([]func(Event){}, kb.subs...)
    kb.mu.Unlock()

    // Notify subscribers outside the lock to avoid deadlocks.
    for _, sub := range subs {
        sub(event)
    }
    return nil
}

// Subscribe registers a callback for KB events. It returns an unsubscribe function.
func (kb *KnowledgeBase) Subscribe(fn func(Event)) (unsubscribe func()) {
    kb.mu.Lock()
    defer kb.mu.Unlock()
    kb.subs = append(kb.subs, fn)
    idx := len(kb.subs) - 1

    return func() {
        kb.mu.Lock()
        defer kb.mu.Unlock()
        if idx < 0 || idx >= len(kb.subs) {
            return
        }
        kb.subs = append(kb.subs[:idx], kb.subs[idx+1:]...)
        idx = -1
    }
}
