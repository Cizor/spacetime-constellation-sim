package core

// MediumType describes the physical medium used by a network
// interface. Scope 2 only distinguishes wired vs wireless, which is
// sufficient for connectivity evaluation and link budgeting.
type MediumType string

const (
	MediumWired    MediumType = "wired"
	MediumWireless MediumType = "wireless"
)

// NetworkInterface represents a logical port on a NetworkNode.
//
// In Scope 2 we extend this slightly with optional addressing
// metadata (MAC/IP) and an adjacency list of link IDs. These are
// not yet used for routing, but are useful for diagnostics and
// future integration.
type NetworkInterface struct {
	ID            string     `json:"ID"`
	Name          string     `json:"Name"`
	Medium        MediumType `json:"Medium"`
	TransceiverID string     `json:"TransceiverID"` // Reference to TransceiverModel in KB
	ParentNodeID  string     `json:"ParentNodeID"`  // Node this interface belongs to
	IsOperational bool       `json:"IsOperational"`

	// Optional L2/L3 attributes. They are deliberately simple so
	// that configs can omit them without breaking anything.
	MACAddress string `json:"MACAddress,omitempty"`
	IPAddress  string `json:"IPAddress,omitempty"`

	// LinkIDs tracks which NetworkLink IDs this interface participates in.
	// This is the per-interface adjacency list required by the Scope 2 plan.
	LinkIDs []string `json:"LinkIDs,omitempty"`
}
