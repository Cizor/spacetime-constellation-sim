package model

// SrPolicy represents a segment routing policy bound to a headend node.
type SrPolicy struct {
	PolicyID      string
	Color         int32
	HeadendNodeID string
	Endpoints     []string
	Segments      []Segment
	Preference    int32
	BindingSID    string
}

// Segment describes a SID (Segment ID) that makes up an SrPolicy.
type Segment struct {
	SID    string
	Type   string // node | adjacency | prefix
	NodeID string // populated for node or adjacency segments
}
