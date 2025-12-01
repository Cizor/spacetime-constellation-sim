// internal/sim/state/state.go
package state

import (
	"errors"
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

// ServiceRequests returns a snapshot of all stored ServiceRequests.
//
// The returned slice is a shallow copy of the internal map values.
// Callers MUST treat the returned ServiceRequests as read-only and
// perform any mutations via ScenarioState methods (to be added in
// later Scope-3 chunks).
func (s *ScenarioState) ServiceRequests() []*model.ServiceRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*model.ServiceRequest, 0, len(s.serviceRequests))
	for _, sr := range s.serviceRequests {
		out = append(out, sr)
	}
	return out
}
