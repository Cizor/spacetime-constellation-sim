// core/scenario_loader.go
package core

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// NetworkScenario is a small summary of what was loaded from JSON.
// It’s mainly useful for logging or debugging from main().
type NetworkScenario struct {
	InterfaceIDs []string
	LinkIDs      []string
	NodeIDs      []string
}

// internal JSON shapes – keep them unexported so we’re free to evolve them.
type networkScenarioJSON struct {
	Interfaces []networkInterfaceJSON  `json:"interfaces"`
	Links      []networkLinkJSON       `json:"links"`
	Positions  map[string]positionJSON `json:"positions"`
}

type networkInterfaceJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Medium        string `json:"medium"`         // "wired" | "wireless"
	TransceiverID string `json:"transceiver_id"` // maps to NetworkInterface.TransceiverID
	ParentNodeID  string `json:"parent_node_id"` // maps to NetworkInterface.ParentNodeID
	IsOperational *bool  `json:"is_operational"` // optional; defaults to true
	// Future fields (IP addresses, impairment flags, etc.) can be added here
	// and either ignored or plumbed through as we extend Scope 2.
}

type networkLinkJSON struct {
	ID         string `json:"id"`
	InterfaceA string `json:"interface_a"`
	InterfaceB string `json:"interface_b"`
	Medium     string `json:"medium"` // "wired" | "wireless"
	// Future: impairment flags, annotations, etc.
}

type positionJSON struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// LoadNetworkScenario reads a JSON scenario from r, populates the
// KnowledgeBase with interfaces, links, and optional node positions,
// and returns a summary of what was loaded.
//
// It deliberately fails only on JSON / structural errors. Duplicate IDs,
// missing transceivers, etc. are handled the same way our direct Add*()
// calls behave in tests (i.e. we rely on KB invariants & panic/behavior
// there rather than re-validating everything here).
func LoadNetworkScenario(kb *KnowledgeBase, r io.Reader) (*NetworkScenario, error) {
	if kb == nil {
		return nil, fmt.Errorf("LoadNetworkScenario: kb is nil")
	}

	var payload networkScenarioJSON
	dec := json.NewDecoder(r)
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("LoadNetworkScenario: decode failed: %w", err)
	}

	result := &NetworkScenario{
		InterfaceIDs: make([]string, 0, len(payload.Interfaces)),
		LinkIDs:      make([]string, 0, len(payload.Links)),
		NodeIDs:      make([]string, 0, len(payload.Positions)),
	}

	// 1) Positions
	for nodeID, pos := range payload.Positions {
		kb.SetNodeECEFPosition(nodeID, Vec3{
			X: pos.X,
			Y: pos.Y,
			Z: pos.Z,
		})
		result.NodeIDs = append(result.NodeIDs, nodeID)
	}

	// 2) Interfaces
	for _, jsIF := range payload.Interfaces {
		if jsIF.ID == "" {
			return nil, fmt.Errorf("LoadNetworkScenario: interface with empty id")
		}
		medium := mediumFromString(jsIF.Medium)

		isOperational := true
		if jsIF.IsOperational != nil {
			isOperational = *jsIF.IsOperational
		}

		intf := &NetworkInterface{
			ID:            jsIF.ID,
			Name:          jsIF.Name,
			Medium:        medium,
			TransceiverID: jsIF.TransceiverID,
			ParentNodeID:  jsIF.ParentNodeID,
			IsOperational: isOperational,
			// LinkIDs is managed by KB when links are added.
		}

		kb.AddInterface(intf)
		result.InterfaceIDs = append(result.InterfaceIDs, jsIF.ID)
	}

	// 3) Links
	for _, jsL := range payload.Links {
		if jsL.ID == "" {
			return nil, fmt.Errorf("LoadNetworkScenario: link with empty id")
		}
		medium := mediumFromString(jsL.Medium)

		link := &NetworkLink{
			ID:         jsL.ID,
			InterfaceA: jsL.InterfaceA,
			InterfaceB: jsL.InterfaceB,
			Medium:     medium,
			// Quality/SNR/Capacity are evaluated by ConnectivityService.
		}

		kb.AddNetworkLink(link)
		result.LinkIDs = append(result.LinkIDs, jsL.ID)
	}

	return result, nil
}

// mediumFromString maps the JSON "medium" string to our Medium* constants.
//
// We keep this tolerant: unknown / empty values default to MediumWireless,
// because that’s what we mostly use in Scope 2 examples.
// If we add more media later, we extend this switch.
func mediumFromString(s string) MediumType {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "wired", "fiber", "optical", "ethernet":
		return MediumWired
	case "wireless", "radio", "rf", "ku", "ka":
		return MediumWireless
	case "":
		// Default to wireless for now; most Scope 2 examples are RF.
		return MediumWireless
	default:
		return MediumWireless
	}
}
