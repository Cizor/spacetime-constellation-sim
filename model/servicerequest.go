// model/servicerequest.go

package model

type ServiceRequest struct {
	// ID is the simulator's stable identifier for this request.
	// It is intended to be unique within a Scenario and is what
	// the NBI layer will use as `request_id` in CRUD operations.
	ID string

	// Type is a human-readable classification / label for this request
	// (e.g. "video", "backhaul"), and maps directly to the Aalyria
	// ServiceRequest.type field in the proto.
	Type string

	SrcNodeID             string
	DstNodeID             string
	FlowRequirements      []FlowRequirement
	Priority              int32
	IsDisruptionTolerant  bool
	AllowPartnerResources bool

	// status fields for future scopes, e.g.:
	// IsProvisionedNow    bool
	// ProvisionedIntervals []TimeInterval
}

type FlowRequirement struct {
	RequestedBandwidthMbps float64
	MinBandwidthMbps       float64
	MaxLatencyMs           float64
	ValidFromUnixSec       int64
	ValidToUnixSec         int64
	// (we can add per-flow DTN flags later if needed)
}
