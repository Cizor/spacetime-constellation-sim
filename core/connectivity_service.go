// core/connectivity_service.go
package core

import (
	"math"
	"strings"
)

// ConnectivityService evaluates which pairs of interfaces are
// connected at a given instant, and annotates links with simple
// quality metrics. Scope 2 extends the earlier version with:
//   - horizon / minimum elevation constraints for sat–ground links,
//   - a basic link-budget style SNR estimate and quality buckets,
//   - automatic dynamic wireless links between all compatible
//     interface pairs (no need for pre-defined NetworkLink entries
//     for every RF hop), and
//   - latency / data-rate metadata on links.
type ConnectivityService struct {
	KB *KnowledgeBase

	// MinElevationDeg is the minimum elevation angle (degrees)
	// required for satellite–ground links.
	MinElevationDeg float64

	// Default latency used for wired links when not specified.
	DefaultWiredLatencyMs float64
}

func NewConnectivityService(kb *KnowledgeBase) *ConnectivityService {
	return &ConnectivityService{
		KB:                    kb,
		MinElevationDeg:       10.0, // a conservative but typical value
		DefaultWiredLatencyMs: 5.0,  // milliseconds
	}
}

// Reset clears any cached connectivity state so a fresh scenario can be
// loaded without leftover dynamic links.
func (cs *ConnectivityService) Reset() {
	if cs == nil {
		return
	}
	if cs.KB != nil {
		cs.KB.ClearDynamicWirelessLinks()
	}
}

// UpdateConnectivity recomputes dynamic wireless connectivity and
// then evaluates all links (static and dynamic) for up/down state
// and quality.
func (cs *ConnectivityService) UpdateConnectivity() {
	// 1) Rebuild dynamic wireless links from the current interface
	// set. This means scenarios only have to describe transceivers
	// and interfaces; every geometry-allowed RF pair is considered.
	cs.rebuildDynamicWirelessLinks()

	// 2) Evaluate every link and update its status in the KB.
	for _, link := range cs.KB.GetAllNetworkLinks() {
		cs.evaluateLink(link)
		cs.KB.UpdateNetworkLink(link)
	}
}

// rebuildDynamicWirelessLinks clears previously discovered wireless
// links and creates a new set based purely on interface metadata and
// transceiver compatibility. It preserves explicit deactivation state
// for links that are recreated.
func (cs *ConnectivityService) rebuildDynamicWirelessLinks() {
	// Before clearing, save which dynamic links were explicitly deactivated
	deactivatedLinkIDs := make(map[string]bool)
	for _, link := range cs.KB.GetAllNetworkLinks() {
		if strings.HasPrefix(link.ID, "dyn-") && link.WasExplicitlyDeactivated {
			deactivatedLinkIDs[link.ID] = true
		}
	}

	// Remove any dynamic wireless links from the last tick.
	cs.KB.ClearDynamicWirelessLinks()

	interfaces := cs.KB.GetAllInterfaces()
	n := len(interfaces)
	for i := 0; i < n; i++ {
		ia := interfaces[i]
		if ia.Medium != MediumWireless {
			continue
		}
		for j := i + 1; j < n; j++ {
			ib := interfaces[j]
			if ib.Medium != MediumWireless {
				continue
			}

			// Both interfaces must reference compatible
			// transceiver models.
			trxA := cs.KB.GetTransceiverModel(ia.TransceiverID)
			trxB := cs.KB.GetTransceiverModel(ib.TransceiverID)
			if trxA == nil || trxB == nil || !trxA.IsCompatible(trxB) {
				continue
			}

			// Create or fetch a dynamic link between them.
			link := cs.KB.UpsertDynamicWirelessLink(ia.ID, ib.ID)
			if link != nil {
				// Restore explicit deactivation state if this link was previously deactivated
				// The link ID is deterministic based on interface IDs, so we can match it
				if deactivatedLinkIDs[link.ID] {
					link.WasExplicitlyDeactivated = true
					link.Status = LinkStatusPotential
					link.IsUp = false
					// Persist the restored state to the knowledge base
					if err := cs.KB.UpdateNetworkLink(link); err != nil {
						// Log error but continue - this is a best-effort restoration
						// The link will be re-evaluated in UpdateConnectivity anyway
					}
				}
			}
		}
	}
}

// evaluateLink applies LoS, elevation, range and link-budget
// checks, and fills in latency / capacity / SNR / quality fields.
// It respects the link's Status field: only links with Status == LinkStatusActive
// are considered "up" even if geometry/RF allows them.
func (cs *ConnectivityService) evaluateLink(link *NetworkLink) {
	// If the link is administratively impaired, it's always down.
	// Check IsImpaired first to handle un-impairing correctly.
	// Note: We preserve WasExplicitlyDeactivated flag across impairment cycles.
	if link.IsImpaired {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		link.Status = LinkStatusImpaired
		// Preserve WasExplicitlyDeactivated flag - don't clear it during impairment
		return
	}
	// If Status is Impaired but IsImpaired is false, clear the Status
	// to allow normal re-evaluation (handles un-impairing case).
	// However, if the link was explicitly deactivated, preserve that state
	// by setting Status to Potential instead of Unknown.
	if link.Status == LinkStatusImpaired {
		if link.WasExplicitlyDeactivated {
			link.Status = LinkStatusPotential // Preserve explicit deactivation
		} else {
			link.Status = LinkStatusUnknown // Reset to Unknown to allow re-evaluation
		}
	}

	// Wired links are assumed always up (unless impaired) with a
	// simple static latency and large capacity.
	if link.Medium == MediumWired {
		if link.LatencyMs == 0 {
			link.LatencyMs = cs.DefaultWiredLatencyMs
		}
		if link.MaxDataRateMbps == 0 {
			link.MaxDataRateMbps = 1000 // 1 Gbit/s nominal
		}
		// Auto-activate wired links for backward compatibility with Scope 2/3.
		// Auto-activate Unknown links (default/unset status) that were not explicitly deactivated.
		// Also auto-activate Potential links that were auto-downgraded
		// (not explicitly deactivated). Links that were explicitly deactivated
		// (WasExplicitlyDeactivated=true) do NOT auto-activate, ensuring explicit
		// control plane actions are respected. This matches the recovery behavior
		// of wireless links for consistency.
		shouldAutoActivate := (link.Status == LinkStatusUnknown && !link.WasExplicitlyDeactivated) ||
			(link.Status == LinkStatusPotential && !link.WasExplicitlyDeactivated)
		if shouldAutoActivate {
			link.Status = LinkStatusActive
			// Clear explicit deactivation flag only when we actually auto-activate
			link.WasExplicitlyDeactivated = false
		}
		// Link is only "up" if Status is Active
		if link.Status == LinkStatusActive {
			link.IsUp = true
		} else {
			link.IsUp = false
		}
		link.Quality = LinkQualityExcellent
		link.SNRdB = 0 // not meaningful for wired here
		return
	}

	// Wireless link: need geometry and radio checks.
	intfA := cs.KB.GetNetworkInterface(link.InterfaceA)
	intfB := cs.KB.GetNetworkInterface(link.InterfaceB)
	if intfA == nil || intfB == nil || !intfA.IsOperational || !intfB.IsOperational {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		// If Status was not set or was Active, set to Potential to maintain consistency
		// (Active status with IsUp=false is inconsistent)
		if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
			link.Status = LinkStatusPotential
		}
		return
	}

	posA, okA := cs.KB.GetNodeECEFPosition(intfA.ParentNodeID)
	posB, okB := cs.KB.GetNodeECEFPosition(intfB.ParentNodeID)
	if !okA || !okB {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		// If Status was not set or was Active, set to Potential to maintain consistency
		// (Active status with IsUp=false is inconsistent)
		if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
			link.Status = LinkStatusPotential
		}
		return
	}

	// 1) Earth-occlusion LoS check.
	if !hasLineOfSight(posA, posB) {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		// If Status was not set or was Active, set to Potential to maintain consistency
		// (Active status with IsUp=false is inconsistent)
		if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
			link.Status = LinkStatusPotential
		}
		return
	}

	// 2) Minimum elevation constraint for sat–ground pairs.
	if !cs.checkElevationConstraint(posA, posB) {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		// If Status was not set or was Active, set to Potential to maintain consistency
		// (Active status with IsUp=false is inconsistent)
		if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
			link.Status = LinkStatusPotential
		}
		return
	}

	// 3) Radio compatibility and range.
	trxA := cs.KB.GetTransceiverModel(intfA.TransceiverID)
	trxB := cs.KB.GetTransceiverModel(intfB.TransceiverID)
	if trxA == nil || trxB == nil || !trxA.IsCompatible(trxB) {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		// If Status was not set or was Active, set to Potential to maintain consistency
		// (Active status with IsUp=false is inconsistent)
		if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
			link.Status = LinkStatusPotential
		}
		return
	}

	distKm := posA.DistanceTo(posB)
	if trxA.MaxRangeKm > 0 || trxB.MaxRangeKm > 0 {
		maxRange := math.Max(trxA.MaxRangeKm, trxB.MaxRangeKm)
		if distKm > maxRange {
			link.IsUp = false
			link.Quality = LinkQualityDown
			link.SNRdB = 0
			link.MaxDataRateMbps = 0
			// If Status was not set or was Active, set to Potential to maintain consistency
			// (Active status with IsUp=false is inconsistent)
			if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
				link.Status = LinkStatusPotential
			}
			return
		}
	}

	// 4) SNR estimate and quality classification.
	snr := estimateLinkSNRdB(trxA, trxB, distKm)
	link.SNRdB = snr
	classifyLinkBySNR(link, snr)

	// If geometry and RF allow, the link is at least Potential.
	// Its IsUp status then depends on whether the control plane has activated it.
	if link.Quality != LinkQualityDown {
		// Auto-activate for backward compatibility with Scope 2/3 when geometry allows.
		// Auto-activate Unknown links (default/unset status) that were not explicitly deactivated.
		// Also auto-activate Potential links that were auto-downgraded
		// (not explicitly deactivated). Links that were explicitly deactivated
		// (WasExplicitlyDeactivated=true) do NOT auto-activate, ensuring explicit
		// control plane actions are respected for both static and dynamic links.
		//
		// Note: Links auto-downgraded to Potential by geometry/RF checks (lines 181-183,
		// 195-198, etc.) do not set WasExplicitlyDeactivated, so they will auto-activate
		// when geometry improves. This is the intended recovery behavior for temporary
		// geometry failures. Only links explicitly deactivated via DeactivateLink should
		// remain deactivated.
		shouldAutoActivate := (link.Status == LinkStatusUnknown && !link.WasExplicitlyDeactivated) ||
			(link.Status == LinkStatusPotential && !link.WasExplicitlyDeactivated)
		if shouldAutoActivate {
			// Auto-activate when geometry allows (maintains backward compatibility)
			link.Status = LinkStatusActive
			// Clear explicit deactivation flag only when we actually auto-activate
			// This ensures the flag accurately reflects the link's state
			link.WasExplicitlyDeactivated = false
		}
		// Link is only "up" if Status is Active
		if link.Status == LinkStatusActive {
			link.IsUp = true
		} else {
			link.IsUp = false
		}
	} else {
		link.IsUp = false
		// If Status was not set or was Active, set to Potential to maintain consistency
		// (Active status with IsUp=false is inconsistent)
		if link.Status == LinkStatusUnknown || link.Status == LinkStatusActive {
			link.Status = LinkStatusPotential
		}
	}
}

// checkElevationConstraint applies the minimum elevation limit for
// satellite–ground links. It treats the node closer to the Earth's
// centre as the ground terminal and the further one as the satellite.
// For sat–sat or ground–ground cases the constraint is not applied.
func (cs *ConnectivityService) checkElevationConstraint(posA, posB Vec3) bool {
	rA := posA.Norm()
	rB := posB.Norm()

	// If both are roughly at the same radius, treat this as a
	// non-sat–ground link (e.g. inter-satellite) and skip the
	// elevation constraint.
	const radiusToleranceKm = 50.0
	if math.Abs(rA-rB) < radiusToleranceKm {
		return true
	}

	var ground, sat Vec3
	if rA < rB {
		ground, sat = posA, posB
	} else {
		ground, sat = posB, posA
	}

	// If the "ground" terminal is not actually near the Earth's
	// surface (e.g. a high-altitude platform), skip elevation.
	if ground.Norm() > EarthRadiusKm+100 {
		return true
	}

	elev := ElevationDegrees(ground, sat)
	return elev >= cs.MinElevationDeg
}

// estimateLinkSNRdB computes a very simple SNR estimate based on a
// free-space path loss model. The constants here are deliberately
// conservative and primarily used to derive a monotonic distance
// vs. quality relationship rather than an engineering-grade link
// budget.
func estimateLinkSNRdB(tx, rx *TransceiverModel, distanceKm float64) float64 {
	if distanceKm < 1 {
		distanceKm = 1
	}

	// Use the mid-band frequency for FSPL.
	fGHz := (tx.Band.MinGHz + tx.Band.MaxGHz) / 2
	if fGHz <= 0 {
		fGHz = 10 // fall back to a generic Ku/Ka-like band
	}

	// Free-space path loss in dB: 92.45 + 20 log10(d_km) + 20 log10(f_GHz)
	fspl := 92.45 + 20*math.Log10(distanceKm) + 20*math.Log10(fGHz)

	// Nominal power & gains. If the model specifies explicit values
	// use them, otherwise fall back to reasonable defaults.
	pt := tx.TxPowerDBw
	if pt == 0 {
		pt = 40 // 10 kW EIRP equivalent (placeholder)
	}
	gt := tx.GainTxDBi
	if gt == 0 {
		gt = 30
	}
	gr := rx.GainRxDBi
	if gr == 0 {
		gr = 30
	}

	// Received power in dBW.
	pr := pt + gt + gr - fspl

	// Simple, fixed noise floor assumption extended by noise figure.
	// This keeps the estimator conservative while respecting TLE catalog
	// metadata that was previously dropped.
	noiseFloor := -120.0 + averageNoiseFigure(tx, rx)

	return pr - noiseFloor
}

// classifyLinkBySNR fills in quality and nominal capacity based on
// SNR. Thresholds are intentionally soft and for demonstration only.
func classifyLinkBySNR(link *NetworkLink, snr float64) {
	switch {
	case snr < 0:
		link.Quality = LinkQualityDown
		link.MaxDataRateMbps = 0
	case snr < 5:
		link.Quality = LinkQualityPoor
		if link.MaxDataRateMbps == 0 {
			link.MaxDataRateMbps = 10
		}
	case snr < 10:
		link.Quality = LinkQualityFair
		if link.MaxDataRateMbps == 0 {
			link.MaxDataRateMbps = 50
		}
	case snr < 20:
		link.Quality = LinkQualityGood
		if link.MaxDataRateMbps == 0 {
			link.MaxDataRateMbps = 200
		}
	default:
		link.Quality = LinkQualityExcellent
		if link.MaxDataRateMbps == 0 {
			link.MaxDataRateMbps = 1000
		}
	}
}

func averageNoiseFigure(tx, rx *TransceiverModel) float64 {
	sum := 0.0
	count := 0
	for _, model := range []*TransceiverModel{tx, rx} {
		if model == nil {
			continue
		}
		if nf := model.SystemNoiseFigureDB; nf != 0 {
			sum += nf
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
