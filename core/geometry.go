package core

import "math"

// EarthRadiusKm is the mean Earth radius used for all simple
// geometry calculations in the connectivity layer (kilometres).
const EarthRadiusKm = 6371.0

// Vec3 is an ECEF-style vector in kilometres.
type Vec3 struct {
	X, Y, Z float64
}

// DistanceTo returns the straight-line distance between two points.
func (v Vec3) DistanceTo(other Vec3) float64 {
	dx := v.X - other.X
	dy := v.Y - other.Y
	dz := v.Z - other.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// Norm returns the Euclidean norm of the vector.
func (v Vec3) Norm() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
}

// Sub returns v - other.
func (v Vec3) Sub(other Vec3) Vec3 {
	return Vec3{X: v.X - other.X, Y: v.Y - other.Y, Z: v.Z - other.Z}
}

// Dot returns the dot product of two vectors.
func (v Vec3) Dot(other Vec3) float64 {
	return v.X*other.X + v.Y*other.Y + v.Z*other.Z
}

// hasLineOfSight checks whether the straight segment between p1 and p2
// intersects the Earth sphere. If it does, the Earth blocks the line-of-sight
// and the function returns false.
//
// All positions are ECEF in kilometres.
func hasLineOfSight(p1, p2 Vec3) bool {
	v := p2.Sub(p1)
	a := v.Dot(v)
	if a == 0 {
		// Degenerate case: same point. If it's outside Earth, treat as LoS;
		// if inside, treat as blocked.
		return p1.Dot(p1) > EarthRadiusKm*EarthRadiusKm
	}

	// Find the closest point on the segment to the Earth's centre (origin).
	// t* minimises |p1 + t v|^2 over t ∈ ℝ.
	t := -p1.Dot(v) / a
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	closest := Vec3{
		X: p1.X + v.X*t,
		Y: p1.Y + v.Y*t,
		Z: p1.Z + v.Z*t,
	}

	// If the closest point lies inside or on the Earth sphere, the
	// segment intersects the Earth -> no LoS.
	return closest.Dot(closest) > EarthRadiusKm*EarthRadiusKm
}

// ElevationDegrees returns the elevation angle of the target as seen from
// the observer, in degrees. 0° = geometric horizon, 90° = overhead.
func ElevationDegrees(observer, target Vec3) float64 {
	// Vector from observer to target.
	v := target.Sub(observer)
	vNorm := v.Norm()
	if vNorm == 0 {
		return 90
	}

	// Local zenith at observer is its normalised position vector.
	r := observer.Norm()
	if r == 0 {
		return 90
	}
	zenith := Vec3{
		X: observer.X / r,
		Y: observer.Y / r,
		Z: observer.Z / r,
	}

	// Angle between v and zenith.
	cosGamma := v.Dot(zenith) / vNorm
	if cosGamma > 1 {
		cosGamma = 1
	} else if cosGamma < -1 {
		cosGamma = -1
	}
	gammaDeg := math.Acos(cosGamma) * 180.0 / math.Pi

	// Elevation is measured from local horizon (90° − zenith angle).
	return 90.0 - gammaDeg
}
