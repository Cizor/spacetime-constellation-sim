package model

// Coordinates represents a point in 3D space (ECEF metres).
type Coordinates struct {
	X float64
	Y float64
	Z float64
}

// RegionType identifies the shape of a region.
type RegionType string

const (
	RegionTypeCircle  RegionType = "circle"
	RegionTypePolygon RegionType = "polygon"
	RegionTypeCountry RegionType = "country"
)

// Region defines a geographic collection of nodes.
type Region struct {
	ID          string
	Type        RegionType
	Center      Coordinates
	RadiusKm    float64
	Vertices    []Coordinates
	CountryCode string
}
