package core

import (
	"time"

	satellite "github.com/joshuaferrara/go-satellite"

	"github.com/signalsfoundry/constellation-simulator/model"
)

// MotionModel updates a platform's position for a given simulation time.
type MotionModel interface {
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

// NewMotionModel chooses an appropriate MotionModel for the platform.
// For now, MotionSourceSpacetrack with non-empty TLE uses SGP4, otherwise static.
func NewMotionModel(p *model.PlatformDefinition, tle1, tle2 string) MotionModel {
	if p.MotionSource == model.MotionSourceSpacetrack && tle1 != "" && tle2 != "" {
		return NewOrbitalModelFromTLE(tle1, tle2)
	}
	return &StaticMotionModel{}
}
