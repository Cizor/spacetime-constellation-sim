// core/motion.go
package core

import (
	"errors"
	"sync"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"

	"github.com/signalsfoundry/constellation-simulator/model"
)

// MotionModel manages propagators for a set of platforms and updates
// their positions as simulation time advances.
type MotionModel struct {
	mu sync.RWMutex

	entries map[string]motionEntry

	posUpdater positionUpdater
	tleFetcher TLEFetcher
}

// positionUpdater is satisfied by KnowledgeBase (and any other type)
// that can persist updated platform coordinates.
type positionUpdater interface {
	UpdatePlatformPosition(id string, pos model.Motion) error
}

// TLEFetcher provides TLE lines for a platform, enabling SGP4 motion
// when MotionSourceSpacetrack is set.
type TLEFetcher func(pd *model.PlatformDefinition) (line1, line2 string)

// MotionModelOption customises MotionModel construction.
type MotionModelOption func(*MotionModel)

// WithPositionUpdater wires an optional sink for persisted coordinates.
func WithPositionUpdater(updater positionUpdater) MotionModelOption {
	return func(mm *MotionModel) {
		mm.posUpdater = updater
	}
}

// WithTLEFetcher supplies a function that returns TLE lines for a platform.
func WithTLEFetcher(fetcher TLEFetcher) MotionModelOption {
	return func(mm *MotionModel) {
		mm.tleFetcher = fetcher
	}
}

// NewMotionModel constructs an empty motion model with optional hooks.
func NewMotionModel(opts ...MotionModelOption) *MotionModel {
	mm := &MotionModel{
		entries: make(map[string]motionEntry),
	}
	for _, opt := range opts {
		opt(mm)
	}
	return mm
}

// AddPlatform registers a platform for motion propagation.
func (m *MotionModel) AddPlatform(pd *model.PlatformDefinition) error {
	if pd == nil || pd.ID == "" {
		return errors.New("platform is nil or missing ID")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[pd.ID]; exists {
		return errors.New("platform already registered in motion model: " + pd.ID)
	}

	copy := clonePlatform(pd)
	prop := newPlatformPropagator(copy, m.tleFetcher)
	m.entries[pd.ID] = motionEntry{
		platform:   copy,
		original:   pd, // Keep reference to original so we can update it
		propagator: prop,
	}
	return nil
}

// RemovePlatform unregisters a platform from motion propagation.
func (m *MotionModel) RemovePlatform(platformID string) error {
	if platformID == "" {
		return errors.New("platform ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[platformID]; !exists {
		return errors.New("platform not registered in motion model: " + platformID)
	}
	delete(m.entries, platformID)
	return nil
}

// Reset clears all registered platforms and their propagators, returning
// the motion model to an empty state.
func (m *MotionModel) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[string]motionEntry)
}

// UpdatePositions advances all registered platforms to simTime.
func (m *MotionModel) UpdatePositions(simTime time.Time) error {
	m.mu.RLock()
	entries := make([]motionEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}
	updater := m.posUpdater
	m.mu.RUnlock()

	var errs []error
	for _, entry := range entries {
		if entry.propagator == nil || entry.platform == nil {
			continue
		}
		entry.propagator.UpdatePosition(simTime, entry.platform)
		// Update the original platform's coordinates so external code can observe changes
		if entry.original != nil {
			entry.original.Coordinates = entry.platform.Coordinates
		}
		if updater != nil {
			if err := updater.UpdatePlatformPosition(entry.platform.ID, entry.platform.Coordinates); err != nil {
				errs = append(errs, err)
			}
		}
	}
	// Return the first error encountered, or nil if no errors
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

type motionEntry struct {
	platform   *model.PlatformDefinition // Cloned copy for internal use
	original   *model.PlatformDefinition // Reference to original for external updates
	propagator platformPropagator
}

type platformPropagator interface {
	UpdatePosition(simTime time.Time, p *model.PlatformDefinition)
}

// StaticMotionModel leaves the platform's position unchanged.
type StaticMotionModel struct{}

// UpdatePosition for static motion does nothing.
func (m *StaticMotionModel) UpdatePosition(simTime time.Time, p *model.PlatformDefinition) {
	// no-op
}

// OrbitalSGP4MotionModel uses a TLE and SGP4 to update platform position.
type OrbitalSGP4MotionModel struct {
	sat satellite.Satellite
}

// NewOrbitalModelFromTLE constructs an orbital model from TLE lines.
func NewOrbitalModelFromTLE(line1, line2 string) *OrbitalSGP4MotionModel {
	sat := satellite.TLEToSat(line1, line2, satellite.GravityWGS72)
	return &OrbitalSGP4MotionModel{sat: sat}
}

// UpdatePosition propagates the satellite to the given simulation time and updates p.Coordinates.
// go-satellite works in kilometres; we store metres in the model.
func (m *OrbitalSGP4MotionModel) UpdatePosition(simTime time.Time, p *model.PlatformDefinition) {
	year, month, day := simTime.Date()
	hour, min, sec := simTime.Clock()

	posECI, _ := satellite.Propagate(m.sat, year, int(month), day, hour, min, sec)
	jd := satellite.JDay(year, int(month), day, hour, min, sec)
	gmst := satellite.ThetaG_JD(jd)
	posECEF := satellite.ECIToECEF(posECI, gmst)

	const kmToM = 1000.0
	p.Coordinates = model.Motion{
		X: posECEF.X * kmToM,
		Y: posECEF.Y * kmToM,
		Z: posECEF.Z * kmToM,
	}
}

// NewPlatformPropagator chooses a per-platform propagator based on the
// platform's MotionSource. MotionSourceSpacetrack with non-empty TLE uses
// SGP4, otherwise a static model is used.
//
// This helper exists mostly for compatibility with earlier code.
func NewPlatformPropagator(p *model.PlatformDefinition, tle1, tle2 string) platformPropagator {
	return newPlatformPropagator(p, func(_ *model.PlatformDefinition) (string, string) {
		return tle1, tle2
	})
}

func newPlatformPropagator(p *model.PlatformDefinition, fetcher TLEFetcher) platformPropagator {
	var tle1, tle2 string
	if fetcher != nil {
		tle1, tle2 = fetcher(p)
	}

	if p.MotionSource == model.MotionSourceSpacetrack && tle1 != "" && tle2 != "" {
		return NewOrbitalModelFromTLE(tle1, tle2)
	}
	return &StaticMotionModel{}
}

func clonePlatform(pd *model.PlatformDefinition) *model.PlatformDefinition {
	if pd == nil {
		return nil
	}
	cp := *pd
	return &cp
}
