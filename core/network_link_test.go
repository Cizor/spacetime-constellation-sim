package core

import "testing"

func TestLinkDefaultState(t *testing.T) {
	link := &NetworkLink{
		ID:         "link-1",
		InterfaceA: "a",
		InterfaceB: "b",
		Medium:     MediumWired,
		IsUp:       false,
	}

	// Assert the fields we set.
	if link.ID != "link-1" {
		t.Errorf("expected ID=link-1, got %s", link.ID)
	}
	if link.InterfaceA != "a" {
		t.Errorf("expected InterfaceA=a, got %s", link.InterfaceA)
	}
	if link.InterfaceB != "b" {
		t.Errorf("expected InterfaceB=b, got %s", link.InterfaceB)
	}
	if link.Medium != MediumWired {
		t.Errorf("expected MediumWired, got %v", link.Medium)
	}
	if link.IsUp {
		t.Errorf("link should initially be down")
	}
}
