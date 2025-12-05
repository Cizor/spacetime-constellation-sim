// core/connectivity_service.go
package core

import "math"

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
// transceiver compatibility.
func (cs *ConnectivityService) rebuildDynamicWirelessLinks() {
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
			_ = cs.KB.UpsertDynamicWirelessLink(ia.ID, ib.ID)
		}
	}
}

// evaluateLink applies LoS, elevation, range and link-budget
// checks, and fills in latency / capacity / SNR / quality fields.
func (cs *ConnectivityService) evaluateLink(link *NetworkLink) {
	// Hard administrative impairment.
	if link.IsImpaired {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		return
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
		link.IsUp = true
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
		return
	}

	posA, okA := cs.KB.GetNodeECEFPosition(intfA.ParentNodeID)
	posB, okB := cs.KB.GetNodeECEFPosition(intfB.ParentNodeID)
	if !okA || !okB {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		return
	}

	// 1) Earth-occlusion LoS check.
	if !hasLineOfSight(posA, posB) {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
		return
	}

	// 2) Minimum elevation constraint for sat–ground pairs.
	if !cs.checkElevationConstraint(posA, posB) {
		link.IsUp = false
		link.Quality = LinkQualityDown
		link.SNRdB = 0
		link.MaxDataRateMbps = 0
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
			return
		}
	}

	// 4) SNR estimate and quality classification.
	snr := estimateLinkSNRdB(trxA, trxB, distKm)
	link.SNRdB = snr
	classifyLinkBySNR(link, snr)

	// Link is considered up for any non-down quality bucket.
	link.IsUp = link.Quality != LinkQualityDown
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

	// Simple, fixed noise floor assumption. This is intentionally
	// coarse but sufficient for qualitative classification.
	noiseFloor := -120.0 // dBW

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
