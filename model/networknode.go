package model

// RouteEntry represents a static routing table entry for a network node.
// This is used by Scope 4 control-plane components (scheduler, agents) to
// install and remove routes on nodes.
type RouteEntry struct {
	// DestinationCIDR is the destination network prefix in CIDR notation,
	// e.g. "10.0.0.0/24" or "192.168.1.1/32".
	DestinationCIDR string

	// NextHopNodeID is the ID of the next hop node. Empty if the destination
	// is directly connected (no next hop needed).
	NextHopNodeID string

	// OutInterfaceID is the local interface ID used to reach the next hop
	// or destination.
	OutInterfaceID string
}

// NetworkNode represents a logical network endpoint.
// In Scope 1 we only care about its link to a PlatformDefinition.
type NetworkNode struct {
	ID   string
	Name string
	Type string // free-form category, e.g. "ROUTER", "UT", etc.

	// PlatformID links this node to a PlatformDefinition.
	// Consumers can obtain the node's position by looking up the platform.
	PlatformID string
}
