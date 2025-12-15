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

// TimeInterval represents a time interval with start and end times.
type TimeInterval struct {
	StartTime time.Time
	EndTime   time.Time
	Path      *Path
}

type Path struct {
	// ID is an internal identifier for the path.
	ID string
	// Nodes captures the ordered nodes that form the path.
	Nodes []string
	// Latency is the total latency observed for this path while provisioned.
	Latency time.Duration
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
	// MessageSizeBytes describes the DTN message payload size for disruption-tolerant requests.
	MessageSizeBytes uint64
	// CreatedAt records when the service request was created.
	CreatedAt time.Time

	// Federation fields for cross-domain workloads.
	CrossDomain      bool
	SourceDomain     string
	DestDomain       string
	FederationToken  string

	// Status fields updated by the scheduler when paths are provisioned.
	// IsProvisionedNow indicates if the service request is currently provisioned
	// (i.e., a path has been scheduled and actions are in place).
	IsProvisionedNow bool
	// ProvisionedIntervals tracks the time intervals when this service request
	// was/is provisioned. This allows tracking provisioning history.
	ProvisionedIntervals []TimeInterval
	// LastProvisionedAt records when the request was last marked as provisioned.
	LastProvisionedAt time.Time
	// LastUnprovisionedAt records when the request was last marked as not provisioned.
	LastUnprovisionedAt time.Time
}

type ServiceRequestStatus struct {
	IsProvisionedNow    bool
	CurrentInterval     *TimeInterval
	AllIntervals        []TimeInterval
	LastProvisionedAt   time.Time
	LastUnprovisionedAt time.Time
}
