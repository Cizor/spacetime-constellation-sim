// core/scenario_loader_test.go
package core

import (
    "strings"
    "testing"
)

func TestLoadNetworkScenario_PopulatesKB(t *testing.T) {
    jsonData := `
{
  "interfaces": [
    {
      "id": "if-gs-ku",
      "name": "GS Ku",
      "medium": "wireless",
      "transceiver_id": "trx-ku",
      "parent_node_id": "gs1",
      "is_operational": true
    },
    {
      "id": "if-sat-ku",
      "name": "SAT Ku",
      "medium": "wireless",
      "transceiver_id": "trx-ku",
      "parent_node_id": "sat1"
    }
  ],
  "links": [
    {
      "id": "link-gs-sat",
      "interface_a": "if-gs-ku",
      "interface_b": "if-sat-ku",
      "medium": "wireless"
    }
  ],
  "positions": {
    "gs1":  { "x": 6371000, "y": 0, "z": 0 },
    "sat1": { "x": 6871000, "y": 0, "z": 0 }
  }
}
`

    kb := NewKnowledgeBase()

    scenario, err := LoadNetworkScenario(kb, strings.NewReader(jsonData))
    if err != nil {
        t.Fatalf("LoadNetworkScenario returned error: %v", err)
    }
    if scenario == nil {
        t.Fatalf("expected non-nil scenario summary")
    }

    // Interfaces
    if len(scenario.InterfaceIDs) != 2 {
        t.Fatalf("expected 2 interfaces in summary, got %d", len(scenario.InterfaceIDs))
    }

    gsIF := kb.GetNetworkInterface("if-gs-ku")
    if gsIF == nil {
        t.Fatalf("expected interface if-gs-ku to be present in KB")
    }
    if gsIF.ParentNodeID != "gs1" {
        t.Errorf("if-gs-ku ParentNodeID = %q, want %q", gsIF.ParentNodeID, "gs1")
    }
    if !gsIF.IsOperational {
        t.Errorf("if-gs-ku IsOperational = false, want true")
    }

    satIF := kb.GetNetworkInterface("if-sat-ku")
    if satIF == nil {
        t.Fatalf("expected interface if-sat-ku to be present in KB")
    }
    if satIF.ParentNodeID != "sat1" {
        t.Errorf("if-sat-ku ParentNodeID = %q, want %q", satIF.ParentNodeID, "sat1")
    }

    // Links
    links := kb.GetAllNetworkLinks()
    if len(links) != 1 {
        t.Fatalf("expected 1 link in KB, got %d", len(links))
    }
    link := links[0]
    if link.ID != "link-gs-sat" {
        t.Errorf("link ID = %q, want %q", link.ID, "link-gs-sat")
    }
    if link.InterfaceA != "if-gs-ku" || link.InterfaceB != "if-sat-ku" {
        t.Errorf("link endpoints = (%s, %s), want (if-gs-ku, if-sat-ku)", link.InterfaceA, link.InterfaceB)
    }
    if link.Medium != MediumWireless {
        t.Errorf("link medium = %v, want MediumWireless", link.Medium)
    }

    // Positions
    if pos, ok := kb.GetNodeECEFPosition("gs1"); !ok {
        t.Fatalf("expected position for node gs1")
    } else {
        if pos.X != 6371000 {
            t.Errorf("gs1.X = %f, want 6371000", pos.X)
        }
    }

    if _, ok := kb.GetNodeECEFPosition("sat1"); !ok {
        t.Fatalf("expected position for node sat1")
    }
}
