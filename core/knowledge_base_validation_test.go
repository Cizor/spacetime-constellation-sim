package core

import "testing"

// helper: does slice contain a given ID?
func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// 1) ID collision: adding two interfaces with the same ID should fail.
func TestAddInterface_DuplicateIDFails(t *testing.T) {
	kb := NewKnowledgeBase()

	err := kb.AddInterface(&NetworkInterface{
		ID:           "if-1",
		Name:         "First",
		Medium:       MediumWireless,
		TransceiverID: "",
		ParentNodeID: "nodeA",
	})
	if err != nil {
		t.Fatalf("first AddInterface returned error: %v", err)
	}

	err = kb.AddInterface(&NetworkInterface{
		ID:           "if-1",
		Name:         "Duplicate",
		Medium:       MediumWireless,
		TransceiverID: "",
		ParentNodeID: "nodeB",
	})
	if err == nil {
		t.Fatalf("expected error when adding interface with duplicate ID, got nil")
	}
}

// 2) Bad references: link pointing at a non-existent interface should error.
func TestAddNetworkLink_UnknownInterfaceFails(t *testing.T) {
	kb := NewKnowledgeBase()

	// Only one interface exists.
	if err := kb.AddInterface(&NetworkInterface{
		ID:           "ifA",
		Name:         "If-A",
		Medium:       MediumWireless,
		TransceiverID: "",
		ParentNodeID: "nodeA",
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}

	// InterfaceB does not exist; AddNetworkLink must fail.
	err := kb.AddNetworkLink(&NetworkLink{
		ID:         "linkAB",
		InterfaceA: "ifA",
		InterfaceB: "ifMissing",
		Medium:     MediumWired,
	})
	if err == nil {
		t.Fatalf("expected AddNetworkLink to fail for unknown interface, got nil")
	}
}

// 3) Adjacency consistency: LinkIDs and linksByInterface stay correct
// when links (static + dynamic) are added and dynamic ones are cleared.
func TestInterfaceLinkIDsStayConsistent(t *testing.T) {
	kb := NewKnowledgeBase()

	if err := kb.AddInterface(&NetworkInterface{
		ID:           "ifA",
		Name:         "If-A",
		Medium:       MediumWireless,
		TransceiverID: "",
		ParentNodeID: "nodeA",
	}); err != nil {
		t.Fatalf("AddInterface(ifA) failed: %v", err)
	}
	if err := kb.AddInterface(&NetworkInterface{
		ID:           "ifB",
		Name:         "If-B",
		Medium:       MediumWireless,
		TransceiverID: "",
		ParentNodeID: "nodeB",
	}); err != nil {
		t.Fatalf("AddInterface(ifB) failed: %v", err)
	}

	// Static link between ifA and ifB.
	if err := kb.AddNetworkLink(&NetworkLink{
		ID:         "link-static",
		InterfaceA: "ifA",
		InterfaceB: "ifB",
		Medium:     MediumWired,
	}); err != nil {
		t.Fatalf("AddNetworkLink(link-static) failed: %v", err)
	}

	ifA := kb.GetNetworkInterface("ifA")
	ifB := kb.GetNetworkInterface("ifB")

	if !containsID(ifA.LinkIDs, "link-static") {
		t.Fatalf("ifA.LinkIDs missing link-static, got %v", ifA.LinkIDs)
	}
	if !containsID(ifB.LinkIDs, "link-static") {
		t.Fatalf("ifB.LinkIDs missing link-static, got %v", ifB.LinkIDs)
	}

	linksForA := kb.GetLinksForInterface("ifA")
	if len(linksForA) != 1 || linksForA[0].ID != "link-static" {
		t.Fatalf("expected exactly link-static for ifA, got %+v", linksForA)
	}

	// Now add a dynamic wireless link between the same interfaces.
	dyn := kb.UpsertDynamicWirelessLink("ifA", "ifB")
	if dyn == nil {
		t.Fatalf("expected dynamic link to be created")
	}

	linksForA = kb.GetLinksForInterface("ifA")
	if len(linksForA) != 2 {
		t.Fatalf("expected 2 links for ifA after dynamic upsert, got %d", len(linksForA))
	}

	// Clear dynamic wireless links; static link must remain and LinkIDs updated.
	kb.ClearDynamicWirelessLinks()

	all := kb.GetAllNetworkLinks()
	if len(all) != 1 || all[0].ID != "link-static" {
		t.Fatalf("expected only link-static to remain after ClearDynamicWirelessLinks, got %+v", all)
	}

	if !containsID(ifA.LinkIDs, "link-static") || len(ifA.LinkIDs) != 1 {
		t.Fatalf("ifA.LinkIDs should contain only link-static after clear, got %v", ifA.LinkIDs)
	}
	if !containsID(ifB.LinkIDs, "link-static") || len(ifB.LinkIDs) != 1 {
		t.Fatalf("ifB.LinkIDs should contain only link-static after clear, got %v", ifB.LinkIDs)
	}
}
