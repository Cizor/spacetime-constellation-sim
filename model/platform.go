package model

// MotionSource indicates how a platform's motion is determined.
type MotionSource int

const (
	MotionSourceUnknown    MotionSource = iota
	MotionSourceSpacetrack              // TLE-based orbit propagation
)

// Motion represents a position in ECEF metres.
type Motion struct {
	X float64
	Y float64
	Z float64
}

// PlatformDefinition represents a physical asset (satellite, ground station, etc.).
// Scope 1 focuses on identity and motion; other concerns are deferred to later scopes.
type PlatformDefinition struct {
	ID          string
	Name        string
	Type        string // e.g. "SATELLITE", "GROUND_STATION"
	CategoryTag string

	Coordinates  Motion
	MotionSource MotionSource

	NoradID uint32 // optional; useful when MotionSourceSpacetrack
}
