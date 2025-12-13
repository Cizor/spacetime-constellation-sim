package core

// LinkStatus is an explicit status for a network link, indicating
// whether it is merely geometrically possible (Potential) or
// actively enabled by the control plane (Active).
type LinkStatus int

const (
	LinkStatusUnknown   LinkStatus = iota // Default/unset
	LinkStatusPotential                   // Geometrically possible, but not activated by control plane
	LinkStatusActive                      // Activated by control plane and geometrically possible
	LinkStatusImpaired                    // Deliberately impaired by control plane, regardless of geometry
)

// LinkQuality is a coarse, human-readable classification of link
// quality derived from the SNR estimate.
type LinkQuality string

const (
	LinkQualityDown      LinkQuality = "down"
	LinkQualityPoor      LinkQuality = "poor"
	LinkQualityFair      LinkQuality = "fair"
	LinkQualityGood      LinkQuality = "good"
	LinkQualityExcellent LinkQuality = "excellent"
)

// NetworkLink connects two NetworkInterfaces. In Scope 2 this is
// used for both static (scenario-defined) links and dynamic wireless
// links that are discovered by the ConnectivityService.
type NetworkLink struct {
	ID         string     `json:"ID"`
	InterfaceA string     `json:"InterfaceA"`
	InterfaceB string     `json:"InterfaceB"`
	Medium     MediumType `json:"Medium"`

	// Status indicates the control-plane state of the link.
	// This is distinct from IsUp, which reflects current physical viability.
	Status LinkStatus `json:"Status"`

	// IsUp indicates if the link is currently physically viable (geometry, RF, etc.)
	// AND administratively active. Maintained for backward compatibility.
	IsUp bool `json:"IsUp"`
	// IsImpaired indicates if the link is administratively forced down.
	// Maintained for backward compatibility.
	IsImpaired bool `json:"IsImpaired"`

	// Performance metadata. Latency and data rate are primarily
	// interesting for wired links, but can also be used as a rough
	// classification for wireless links.
	LatencyMs       float64     `json:"LatencyMs,omitempty"`
	MaxDataRateMbps float64     `json:"MaxDataRateMbps,omitempty"`
	Quality         LinkQuality `json:"Quality,omitempty"`
	SNRdB           float64     `json:"SNRdB,omitempty"`

	// IsStatic marks links that came from a scenario / config file
	// as opposed to dynamic wireless links that are rebuilt on every
	// connectivity update.
	IsStatic bool `json:"IsStatic"`

	// WasExplicitlyDeactivated tracks whether this link was explicitly
	// deactivated via DeactivateLink. This allows us to distinguish
	// between links that were auto-downgraded (should recover) and
	// links that were explicitly deactivated (should NOT auto-activate).
	WasExplicitlyDeactivated bool `json:"WasExplicitlyDeactivated,omitempty"`

	// StatusBeforeImpairment stores the link's status before it was impaired.
	// This is used to restore the original status when the link is un-impaired.
	// Only set when transitioning from non-impaired to impaired state.
	// A pointer is used to distinguish between unset (nil) and explicitly
	// set to Unknown (0 dB is a valid status for links loaded from JSON).
	StatusBeforeImpairment *LinkStatus `json:"StatusBeforeImpairment,omitempty"`
}
