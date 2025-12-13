package core

import (
	"encoding/json"
	"os"
	"testing"
)

// TestLoadSBIScenario verifies that the SBI example scenario can be loaded
// without errors. This is a smoke test to prevent the scenario from silently
// breaking.
func TestLoadSBIScenario(t *testing.T) {
	kb := NewKnowledgeBase()

	// Load transceivers first (required for interfaces)
	// Try multiple possible paths relative to workspace root
	transceiverPaths := []string{
		"configs/transceivers.json",
		"../configs/transceivers.json",
		"../../configs/transceivers.json",
	}
	var transceiverFile *os.File
	for _, path := range transceiverPaths {
		var err error
		transceiverFile, err = os.Open(path)
		if err == nil {
			break
		}
	}
	if transceiverFile == nil {
		t.Skipf("transceivers.json not found in any expected location, skipping test")
	}
	defer transceiverFile.Close()

	var trxs []*TransceiverModel
	dec := json.NewDecoder(transceiverFile)
	if err := dec.Decode(&trxs); err != nil {
		t.Skipf("failed to decode transceivers.json, skipping test: %v", err)
	}
	for _, trx := range trxs {
		kb.AddTransceiverModel(trx)
	}

	// Load the SBI example scenario
	// Try multiple possible paths relative to workspace root
	scenarioPaths := []string{
		"configs/scope4_sbi_example.json",
		"../configs/scope4_sbi_example.json",
		"../../configs/scope4_sbi_example.json",
	}
	var scenarioFile *os.File
	for _, path := range scenarioPaths {
		var err error
		scenarioFile, err = os.Open(path)
		if err == nil {
			break
		}
	}
	if scenarioFile == nil {
		t.Fatalf("failed to open SBI example scenario in any expected location")
	}
	defer scenarioFile.Close()

	scenario, err := LoadNetworkScenario(kb, scenarioFile)
	if err != nil {
		t.Fatalf("failed to load SBI example scenario: %v", err)
	}

	// Verify basic properties
	if len(scenario.InterfaceIDs) != 2 {
		t.Errorf("expected 2 interfaces, got %d", len(scenario.InterfaceIDs))
	}
	if len(scenario.LinkIDs) != 1 {
		t.Errorf("expected 1 link, got %d", len(scenario.LinkIDs))
	}
	if len(scenario.NodeIDs) != 2 {
		t.Errorf("expected 2 nodes with positions, got %d", len(scenario.NodeIDs))
	}

	// Verify specific interfaces exist
	expectedInterfaces := map[string]bool{
		"if-sat1-down": false,
		"if-gs1-up":    false,
	}
	for _, ifID := range scenario.InterfaceIDs {
		if _, exists := expectedInterfaces[ifID]; exists {
			expectedInterfaces[ifID] = true
		}
	}
	for ifID, found := range expectedInterfaces {
		if !found {
			t.Errorf("expected interface %q not found in scenario", ifID)
		}
	}

	// Verify link exists
	expectedLink := "link-sat1-gs1"
	found := false
	for _, linkID := range scenario.LinkIDs {
		if linkID == expectedLink {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected link %q not found in scenario", expectedLink)
	}

	// Verify nodes have positions
	expectedNodes := map[string]bool{
		"sat1": false,
		"gs1":  false,
	}
	for _, nodeID := range scenario.NodeIDs {
		if _, exists := expectedNodes[nodeID]; exists {
			expectedNodes[nodeID] = true
		}
	}
	for nodeID, found := range expectedNodes {
		if !found {
			t.Errorf("expected node %q not found in scenario positions", nodeID)
		}
	}
}

