package state

import (
	"errors"
	"testing"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestScenarioStateRegions(t *testing.T) {
	phys := kb.NewKnowledgeBase()
	if err := phys.AddPlatform(&model.PlatformDefinition{ID: "plat-center", Coordinates: model.Motion{X: 0, Y: 0, Z: 0}}); err != nil {
		t.Fatalf("AddPlatform center: %v", err)
	}
	if err := phys.AddPlatform(&model.PlatformDefinition{ID: "plat-far", Coordinates: model.Motion{X: 5_000, Y: 5_000, Z: 0}}); err != nil {
		t.Fatalf("AddPlatform far: %v", err)
	}
	if err := phys.AddNetworkNode(&model.NetworkNode{ID: "node-inside", PlatformID: "plat-center", CountryCode: "US"}); err != nil {
		t.Fatalf("AddNetworkNode inside: %v", err)
	}
	if err := phys.AddNetworkNode(&model.NetworkNode{ID: "node-polygon", PlatformID: "plat-far", CountryCode: "CA"}); err != nil {
		t.Fatalf("AddNetworkNode polygon: %v", err)
	}
	s := NewScenarioState(phys, nil, logging.Noop())

	circle := &model.Region{
		ID:       "circle",
		Type:     model.RegionTypeCircle,
		Center:   model.Coordinates{X: 0, Y: 0, Z: 0},
		RadiusKm: 1,
	}
	if err := s.CreateRegion(circle); err != nil {
		t.Fatalf("CreateRegion(circle) = %v", err)
	}
	if err := s.CreateRegion(circle); err == nil {
		t.Fatalf("CreateRegion duplicate succeeded unexpectedly")
	} else if !errors.Is(err, ErrRegionExists) {
		t.Fatalf("CreateRegion duplicate error = %v, want ErrRegionExists", err)
	}

	polygon := &model.Region{
		ID:   "square",
		Type: model.RegionTypePolygon,
		Vertices: []model.Coordinates{
			{X: 4_000, Y: 4_000},
			{X: 6_000, Y: 4_000},
			{X: 6_000, Y: 6_000},
			{X: 4_000, Y: 6_000},
		},
	}
	if err := s.CreateRegion(polygon); err != nil {
		t.Fatalf("CreateRegion(polygon) = %v", err)
	}

	country := &model.Region{
		ID:          "country",
		Type:        model.RegionTypeCountry,
		CountryCode: "US",
	}
	if err := s.CreateRegion(country); err != nil {
		t.Fatalf("CreateRegion(country) = %v", err)
	}

	if !s.CheckRegionMembership("node-inside", circle.ID) {
		t.Fatalf("node-inside should belong to circle")
	}
	if s.CheckRegionMembership("node-polygon", circle.ID) {
		t.Fatalf("node-polygon unexpectedly in circle")
	}
	if !s.CheckRegionMembership("node-polygon", polygon.ID) {
		t.Fatalf("node-polygon should belong to polygon")
	}
	if s.CheckRegionMembership("node-inside", "missing") {
		t.Fatalf("missing region reported membership")
	}

	insideNodes, err := s.GetNodesInRegion(circle.ID)
	if err != nil {
		t.Fatalf("GetNodesInRegion(circle) = %v", err)
	}
	if len(insideNodes) != 1 || insideNodes[0] != "node-inside" {
		t.Fatalf("GetNodesInRegion(circle) = %v, want [node-inside]", insideNodes)
	}

	polyNodes, err := s.GetNodesInRegion(polygon.ID)
	if err != nil {
		t.Fatalf("GetNodesInRegion(polygon) = %v", err)
	}
	if len(polyNodes) != 1 || polyNodes[0] != "node-polygon" {
		t.Fatalf("GetNodesInRegion(polygon) = %v, want [node-polygon]", polyNodes)
	}
	if !s.CheckRegionMembership("node-inside", country.ID) {
		t.Fatalf("node-inside should belong to country")
	}
	if s.CheckRegionMembership("node-polygon", country.ID) {
		t.Fatalf("node-polygon should not belong to country")
	}
	countryNodes, err := s.GetNodesInRegion(country.ID)
	if err != nil {
		t.Fatalf("GetNodesInRegion(country) = %v", err)
	}
	if len(countryNodes) != 1 || countryNodes[0] != "node-inside" {
		t.Fatalf("GetNodesInRegion(country) = %v, want [node-inside]", countryNodes)
	}
	if nodes, err := s.GetNodesInRegion("missing"); !errors.Is(err, ErrRegionNotFound) {
		t.Fatalf("GetNodesInRegion(missing) = (%v, %v), want ErrRegionNotFound", nodes, err)
	}

	regions := s.ListRegions()
	if len(regions) != 3 {
		t.Fatalf("ListRegions len = %d, want 3", len(regions))
	}
	regions[0].ID = "mutated"
	if _, err := s.GetRegion("mutated"); !errors.Is(err, ErrRegionNotFound) {
		t.Fatalf("mutating returned region leaked to state")
	}
	orig, err := s.GetRegion(circle.ID)
	if err != nil {
		t.Fatalf("GetRegion after mutation = %v", err)
	}
	if orig.ID != circle.ID {
		t.Fatalf("GetRegion returned %q, want %q", orig.ID, circle.ID)
	}

	if err := s.DeleteRegion(circle.ID); err != nil {
		t.Fatalf("DeleteRegion(circle) = %v", err)
	}
	if _, err := s.GetRegion(circle.ID); !errors.Is(err, ErrRegionNotFound) {
		t.Fatalf("GetRegion after delete = %v, want ErrRegionNotFound", err)
	}

	if err := s.CreateRegion(&model.Region{ID: "bad", Type: model.RegionTypeCircle, RadiusKm: 0}); !errors.Is(err, ErrRegionInvalid) {
		t.Fatalf("CreateRegion invalid radius err = %v, want ErrRegionInvalid", err)
	}
	if err := s.CreateRegion(&model.Region{ID: "bad-poly", Type: model.RegionTypePolygon, Vertices: []model.Coordinates{{X: 0, Y: 0}}}); !errors.Is(err, ErrRegionInvalid) {
		t.Fatalf("CreateRegion invalid polygon err = %v, want ErrRegionInvalid", err)
	}
	if err := s.CreateRegion(&model.Region{ID: "bad-country", Type: model.RegionTypeCountry}); !errors.Is(err, ErrRegionInvalid) {
		t.Fatalf("CreateRegion invalid country err = %v, want ErrRegionInvalid", err)
	}
}
