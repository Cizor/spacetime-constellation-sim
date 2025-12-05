package model

import "time"

type FlowRequirement struct {
	// RequestedBandwidth is the desired bandwidth in bits per second.
	RequestedBandwidth float64
	// MinBandwidth is the minimum acceptable bandwidth in bits per second.
	MinBandwidth float64
	// MaxLatency is the maximum acceptable one-way latency in seconds.
	MaxLatency float64
	// ValidFrom is the start of the interval when this requirement applies.
	ValidFrom time.Time
	// ValidTo is the end of the interval when this requirement applies.
	ValidTo time.Time
}

type ServiceRequest struct {
	// ID is the simulator's stable identifier for this request.
	// It is intended to be unique within a Scenario and is what
	// the NBI layer will use as `request_id` in CRUD operations.
	ID string

	SrcNodeID             string
	DstNodeID             string
	FlowRequirements      []FlowRequirement
	Priority              int32
	IsDisruptionTolerant  bool
	AllowPartnerResources bool

	// status fields for future scopes, e.g.:
	// IsProvisionedNow bool
	// ProvisionedIntervals []TimeInterval
}
