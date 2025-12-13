package core

// FrequencyBand represents a simple [min,max] GHz band.
type FrequencyBand struct {
	MinGHz float64 `json:"MinGHz"`
	MaxGHz float64 `json:"MaxGHz"`
}

// TransceiverModel describes RF characteristics for a family of radios.
// Scope 2 uses this for range checks, band compatibility and a very
// simple link-budget-style SNR estimate.
type TransceiverModel struct {
	ID   string `json:"ID"`
	Name string `json:"Name"`

	Band FrequencyBand `json:"Band"`

	// MaxRangeKm is the maximum connectivity range in kilometres for
	// simple distance-based cutoffs. 0 = unlimited.
	MaxRangeKm float64 `json:"MaxRangeKm,omitempty"`

	// RF parameters used by the simple SNR estimator in
	// connectivity_service.go. All are optional; if left as zero,
	// the estimator will fall back to conservative defaults.
	TxPowerDBw float64 `json:"TxPowerDBw,omitempty"`
	GainTxDBi  float64 `json:"GainTxDBi,omitempty"`
	GainRxDBi  float64 `json:"GainRxDBi,omitempty"`

	// SystemNoiseFigureDB declares the noise figure (dB) for this
	// transceiver. This field was already supplied in configs but not
	// represented here; it adjusts the noise-floor used by the SNR
	// estimator so that gaps in requirements coverage can be tracked.
	// A pointer is used to distinguish between unset (nil) and explicitly set to 0.
	SystemNoiseFigureDB *float64 `json:"SystemNoiseFigureDB,omitempty"`

	// MaxBeams is metadata describing how many concurrent beams this
	// transceiver can theoretically support.
	//
	// Scope 2: stored only, not enforced. A value of 0 or less is
	// treated as "unspecified" and will be defaulted to 1 when the
	// model is added to the KnowledgeBase.
	MaxBeams int `json:"MaxBeams,omitempty"`
}

// IsCompatible returns true if the frequency bands overlap at all.
func (tm *TransceiverModel) IsCompatible(other *TransceiverModel) bool {
	return !(tm.Band.MaxGHz < other.Band.MinGHz || tm.Band.MinGHz > other.Band.MaxGHz)
}
