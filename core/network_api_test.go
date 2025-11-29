package core

import "testing"

// TestKnowledgeBaseNetworkAPI exercises the KB helper methods that expose
// a simple "network NBI" for higher layers: link getters, up-link queries
// and neighbour discovery.
func TestKnowledgeBaseNetworkAPI(t *testing.T) {
	kb := NewKnowledgeBase()

	// One generic Ku-like transceiver model for all interfaces.
	trx := &TransceiverModel{
		ID: "trx-ku",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		MaxRangeKm: 50000.0,
	}
	kb.AddTransceiverModel(trx)

	// Three interfaces on three different nodes.
	kb.AddInterface(&NetworkInterface{
		ID:            "ifA",
		Name:          "If-A",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeA",
		IsOperational: true,
	})
	kb.AddInterface(&NetworkInterface{
		ID:            "ifB",
		Name:          "If-B",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeB",
		IsOperational: true,
	})
	kb.AddInterface(&NetworkInterface{
		ID:            "ifC",
		Name:          "If-C",
		Medium:        MediumWireless,
		TransceiverID: "trx-ku",
		ParentNodeID:  "nodeC",
		IsOperational: true,
	})

	// Two links: A<->B initially up, B<->C initially down.
	kb.AddNetworkLink(&NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWireless,
		IsUp:       true,
	})
	kb.AddNetworkLink(&NetworkLink{
		ID:         "linkBC",
		InterfaceA: "ifB",
		InterfaceB: "ifC",
		Medium:     MediumWireless,
		IsUp:       false,
	})

	// --- GetNetworkLink ---

	if l := kb.GetNetworkLink("linkAB"); l == nil || l.ID != "linkAB" {
		t.Fatalf("GetNetworkLink(linkAB) = %#v, want non-nil with ID linkAB", l)
	}
	if l := kb.GetNetworkLink("does-not-exist"); l != nil {
		t.Fatalf("expected nil for missing link, got %#v", l)
	}

	// --- GetLinksForInterface ---

	linksForB := kb.GetLinksForInterface("ifB")
	if len(linksForB) != 2 {
		t.Fatalf("expected 2 links for ifB, got %d", len(linksForB))
	}

	// --- GetUpLinks ---

	up := kb.GetUpLinks()
	if len(up) != 1 || up[0].ID != "linkAB" {
		t.Fatalf("expected only linkAB in up links, got %+v", up)
	}

	// --- GetNeighbours ---

	// From nodeB we should see only nodeA, because linkBC is down.
	neighB := kb.GetNeighbours("nodeB")
	if len(neighB) != 1 || neighB[0] != "nodeA" {
		t.Fatalf("expected neighbours of nodeB = [nodeA], got %v", neighB)
	}

	// If we mark linkBC up, nodeB should have both nodeA and nodeC as neighbours.
	if l := kb.GetNetworkLink("linkBC"); l == nil {
		t.Fatalf("expected linkBC to exist")
	} else {
		l.IsUp = true
	}

	neighBAfter := kb.GetNeighbours("nodeB")
	if len(neighBAfter) != 2 {
		t.Fatalf("expected 2 neighbours of nodeB after linkBC up, got %v", neighBAfter)
	}
}
