package controller

import (
	"math"
	"sort"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
)

// BeamConflict captures why simultaneous beams on an interface are problematic.
type BeamConflict struct {
	InterfaceID      string
	ConflictingBeams []BeamAssignment
	ConflictType     string // "concurrent_beams", "power_limit", "frequency"
	FrequencyDetails *FrequencyInterference
}

// BeamAssignment describes a single beam scheduled on an interface.
type BeamAssignment struct {
	InterfaceID      string
	Beam             *sbi.BeamSpec
	StartTime        int64 // UnixNano
	EndTime          int64
	FrequencyHz      float64
	BandwidthHz      float64
	PowerDBw         float64
	ServiceRequestID string
	Priority         int
	FairnessScore    float64
	Deadline         time.Time
}

// FrequencyInterference describes aggregated interference around a center frequency.
type FrequencyInterference struct {
	FrequencyGHz        float64
	InterferingBeams    []BeamAssignment
	InterferenceLeveldB float64
}

// BeamActionType describes what to do about a conflicting beam.
type BeamActionType string

const (
	ActionCancel BeamActionType = "cancel"
	ActionDelay  BeamActionType = "delay"
	ActionAdjust BeamActionType = "adjust"
)

// BeamAction is returned by conflict resolution to instruct schedulers how to modify beams.
type BeamAction struct {
	Assignment BeamAssignment
	Action     BeamActionType
	Notes      string
}

// ResolutionStrategy dictates how conflicts should be resolved.
type ResolutionStrategy string

const (
	StrategyPriority         ResolutionStrategy = "priority"
	StrategyEarliestDeadline ResolutionStrategy = "earliest_deadline"
	StrategyFairness         ResolutionStrategy = "fairness"
)

const defaultInterferenceThresholdDBw = 3.0

// ResolveConflicts produces BeamAction plans to resolve conflicts according to strategy.
func ResolveConflicts(conflicts []BeamConflict, strategy ResolutionStrategy) []BeamAction {
	actions := []BeamAction{}
	for _, conflict := range conflicts {
		switch strategy {
		case StrategyPriority:
			actions = append(actions, resolvePriorityConflict(conflict)...)
		case StrategyEarliestDeadline:
			actions = append(actions, resolveDeadlineConflict(conflict)...)
		case StrategyFairness:
			actions = append(actions, resolveFairnessConflict(conflict)...)
		}
	}
	return actions
}

// DetectBeamConflicts inspects the given assignments for violations.
func DetectBeamConflicts(interfaceID string, assignments []BeamAssignment, trx *core.TransceiverModel) []BeamConflict {
	if interfaceID == "" || len(assignments) == 0 {
		return nil
	}

	filtered := make([]BeamAssignment, 0, len(assignments))
	for _, a := range assignments {
		if a.InterfaceID == interfaceID {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	conflicts := []BeamConflict{}

	// Concurrent beams
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].StartTime < filtered[j].StartTime
	})
	for i := range filtered {
		count := 1
		j := i + 1
		for j < len(filtered) && filtered[j].StartTime < filtered[i].EndTime {
			count++
			j++
		}
		if trx != nil && trx.MaxBeams > 0 && count > trx.MaxBeams {
			conflicts = append(conflicts, BeamConflict{
				InterfaceID:      interfaceID,
				ConflictingBeams: filtered[i:j],
				ConflictType:     "concurrent_beams",
			})
			break
		}
	}

	// Power limit
	if trx != nil && trx.TxPowerDBw > 0 {
		var maxPower float64 = trx.TxPowerDBw
		var totalPower float64
		for _, a := range filtered {
			if math.IsNaN(a.PowerDBw) {
				continue
			}
			totalPower = math.Max(totalPower, a.PowerDBw)
		}
		if totalPower > maxPower {
			conflicts = append(conflicts, BeamConflict{
				InterfaceID:      interfaceID,
				ConflictingBeams: filtered,
				ConflictType:     "power_limit",
			})
		}
	}

	threshold := defaultInterferenceThresholdDBw
	if trx != nil && trx.InterferenceThresholdDBw > 0 {
		threshold = trx.InterferenceThresholdDBw
	}
	conflicts = append(conflicts, detectFrequencyConflicts(interfaceID, filtered, threshold)...)

	return conflicts
}

func detectFrequencyConflicts(interfaceID string, assignments []BeamAssignment, threshold float64) []BeamConflict {
	if len(assignments) == 0 || threshold <= 0 {
		return nil
	}
	conflicts := []BeamConflict{}
	for _, assignment := range assignments {
		if assignment.FrequencyHz == 0 || assignment.BandwidthHz == 0 {
			continue
		}
		level := ComputeInterference(assignment, assignments)
		if level <= threshold {
			continue
		}
		interfering := collectInterferingBeams(assignment, assignments)
		if len(interfering) == 0 {
			continue
		}
		conflicts = append(conflicts, BeamConflict{
			InterfaceID:      interfaceID,
			ConflictingBeams: append([]BeamAssignment{assignment}, interfering...),
			ConflictType:     "frequency",
			FrequencyDetails: &FrequencyInterference{
				FrequencyGHz:        assignment.FrequencyHz / 1e9,
				InterferingBeams:    interfering,
				InterferenceLeveldB: level,
			},
		})
		break
	}
	return conflicts
}

// ComputeInterference estimates the dB level generated by beams overlapping in
// time/frequency with the provided assignment.
func ComputeInterference(subject BeamAssignment, assignments []BeamAssignment) float64 {
	var level float64
	for _, other := range assignments {
		if other.InterfaceID != subject.InterfaceID {
			continue
		}
		if !timeOverlap(subject, other) || !frequencyOverlap(subject, other) {
			continue
		}
		freqSep := math.Abs(subject.FrequencyHz - other.FrequencyHz)
		frequencyPenalty := freqSep / math.Max(subject.BandwidthHz, 1)
		halfBW := subject.BandwidthHz/2 + other.BandwidthHz/2
		if halfBW <= 0 {
			continue
		}
		overlapRatio := (halfBW - freqSep) / halfBW
		if overlapRatio < 0 {
			continue
		}
		interference := other.PowerDBw + 10*math.Log10(overlapRatio+1) - frequencyPenalty
		if interference > level {
			level = interference
		}
	}
	return level
}

func collectInterferingBeams(subject BeamAssignment, assignments []BeamAssignment) []BeamAssignment {
	result := make([]BeamAssignment, 0)
	for _, other := range assignments {
		if other.InterfaceID != subject.InterfaceID {
			continue
		}
		if subject == other {
			continue
		}
		if !timeOverlap(subject, other) || !frequencyOverlap(subject, other) {
			continue
		}
		result = append(result, other)
	}
	return result
}

func resolvePriorityConflict(conflict BeamConflict) []BeamAction {
	beams := append([]BeamAssignment(nil), conflict.ConflictingBeams...)
	sort.SliceStable(beams, func(i, j int) bool {
		if beams[i].Priority == beams[j].Priority {
			return beams[i].StartTime < beams[j].StartTime
		}
		return beams[i].Priority > beams[j].Priority
	})
	return cancelBeamsExcept(beams, 0, "higher priority kept")
}

func resolveDeadlineConflict(conflict BeamConflict) []BeamAction {
	beams := append([]BeamAssignment(nil), conflict.ConflictingBeams...)
	sort.SliceStable(beams, func(i, j int) bool {
		return beamDeadline(beams[i]).Before(beamDeadline(beams[j]))
	})
	return cancelBeamsExcept(beams, 0, "earliest deadline kept")
}

func resolveFairnessConflict(conflict BeamConflict) []BeamAction {
	beams := append([]BeamAssignment(nil), conflict.ConflictingBeams...)
	sort.SliceStable(beams, func(i, j int) bool {
		if beams[i].FairnessScore == beams[j].FairnessScore {
			return beams[i].StartTime < beams[j].StartTime
		}
		return beams[i].FairnessScore < beams[j].FairnessScore
	})
	return cancelBeamsExcept(beams, 0, "fairness selection")
}

func cancelBeamsExcept(beams []BeamAssignment, keepIdx int, reason string) []BeamAction {
	actions := make([]BeamAction, 0, len(beams))
	for idx, beam := range beams {
		if idx == keepIdx {
			continue
		}
		actions = append(actions, BeamAction{
			Assignment: beam,
			Action:     ActionCancel,
			Notes:      reason,
		})
	}
	return actions
}

func beamDeadline(beam BeamAssignment) time.Time {
	if !beam.Deadline.IsZero() {
		return beam.Deadline
	}
	if beam.EndTime > 0 {
		return time.Unix(0, beam.EndTime)
	}
	if beam.StartTime > 0 {
		return time.Unix(0, beam.StartTime)
	}
	return time.Now()
}

func timeOverlap(a, b BeamAssignment) bool {
	return a.StartTime < b.EndTime && b.StartTime < a.EndTime
}

func frequencyOverlap(a, b BeamAssignment) bool {
	if a.FrequencyHz == 0 || b.FrequencyHz == 0 {
		return false
	}
	halfBandwidth := a.BandwidthHz/2 + b.BandwidthHz/2
	if halfBandwidth <= 0 {
		return false
	}
	sep := math.Abs(a.FrequencyHz - b.FrequencyHz)
	return sep < halfBandwidth
}
