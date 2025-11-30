package model

// ServiceRequest describes a request to provision a network flow between two
// nodes, along with QoS / timing constraints.
//
// This mirrors the intent of Aalyria's ServiceRequest proto but is simplified
// for the simulator domain model.
type ServiceRequest struct {
	ID        string
	SrcNodeID string
	DstNodeID string

	FlowRequirements []FlowRequirement

	// Higher value => higher priority.
	Priority int32

	// Whether this flow can tolerate temporary disruptions.
	IsDisruptionTolerant bool

	// Whether resources from partner networks may be used.
	AllowPartnerResources bool

	// Future: status / scheduling details.
	// IsProvisionedNow     bool
	// ProvisionedIntervals []TimeInterval
}

// FlowRequirement captures basic QoS constraints for a single flow.
type FlowRequirement struct {
	// Requested and minimum acceptable bandwidth (in Mbps, for example).
	RequestedBandwidthMbps float64
	MinBandwidthMbps       float64

	// Maximum acceptable one-way latency in milliseconds.
	MaxLatencyMs float64

	// Optional validity window; zero values can mean "always valid".
	ValidFromUnixSec int64
	ValidToUnixSec   int64
}
