package controller

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
)

func TestDetectBeamConflictsConcurrent(t *testing.T) {
	trx := &core.TransceiverModel{MaxBeams: 1}
	assignments := []BeamAssignment{
		{InterfaceID: "if-1", StartTime: 0, EndTime: 10},
		{InterfaceID: "if-1", StartTime: 5, EndTime: 15},
	}
	conflicts := DetectBeamConflicts("if-1", assignments, trx)
	if len(conflicts) == 0 {
		t.Fatal("expected conflict due to concurrent beams")
	}
	if conflicts[0].ConflictType != "concurrent_beams" {
		t.Fatalf("expected concurrent_beams conflict, got %s", conflicts[0].ConflictType)
	}
}

func TestDetectBeamConflictsPower(t *testing.T) {
	trx := &core.TransceiverModel{TxPowerDBw: 10}
	assignments := []BeamAssignment{
		{InterfaceID: "if-1", PowerDBw: 12},
	}
	conflicts := DetectBeamConflicts("if-1", assignments, trx)
	if len(conflicts) == 0 {
		t.Fatal("expected power conflict")
	}
	if conflicts[0].ConflictType != "power_limit" {
		t.Fatalf("expected power_limit conflict, got %s", conflicts[0].ConflictType)
	}
}

func TestDetectBeamConflictsFrequency(t *testing.T) {
	trx := &core.TransceiverModel{InterferenceThresholdDBw: 1}
	assignments := []BeamAssignment{
		{InterfaceID: "if-1", FrequencyHz: 10, BandwidthHz: 5, PowerDBw: 10, StartTime: 0, EndTime: 10},
		{InterfaceID: "if-1", FrequencyHz: 12, BandwidthHz: 5, PowerDBw: 8, StartTime: 5, EndTime: 15},
	}
	conflicts := DetectBeamConflicts("if-1", assignments, trx)
	if len(conflicts) == 0 {
		t.Fatal("expected frequency conflict")
	}
	if conflicts[0].ConflictType != "frequency" {
		t.Fatalf("expected frequency conflict, got %s", conflicts[0].ConflictType)
	}
}

func TestComputeInterference(t *testing.T) {
	subject := BeamAssignment{
		InterfaceID: "if-1", FrequencyHz: 10e9, BandwidthHz: 1e9, PowerDBw: 10,
		StartTime: 0, EndTime: 10,
	}
	others := []BeamAssignment{
		subject,
		{InterfaceID: "if-1", FrequencyHz: 10.1e9, BandwidthHz: 1e9, PowerDBw: 5, StartTime: 2, EndTime: 8},
	}
	level := ComputeInterference(subject, others)
	if level <= 0 {
		t.Fatalf("expected positive interference level, got %v", level)
	}
}

func TestDetectBeamConflictsFrequencyThreshold(t *testing.T) {
	trx := &core.TransceiverModel{InterferenceThresholdDBw: 1}
	assignments := []BeamAssignment{
		{InterfaceID: "if-1", FrequencyHz: 10e9, BandwidthHz: 1e9, PowerDBw: 10, StartTime: 0, EndTime: 10},
		{InterfaceID: "if-1", FrequencyHz: 10.05e9, BandwidthHz: 1e9, PowerDBw: 9, StartTime: 1, EndTime: 9},
	}
	conflicts := DetectBeamConflicts("if-1", assignments, trx)
	if len(conflicts) == 0 {
		t.Fatal("expected frequency conflict via threshold")
	}
	if conflicts[0].FrequencyDetails == nil {
		t.Fatalf("expected frequency details to be populated")
	}
	if conflicts[0].FrequencyDetails.InterferenceLeveldB <= trx.InterferenceThresholdDBw {
		t.Fatalf("expected interference level > threshold, got %v", conflicts[0].FrequencyDetails.InterferenceLeveldB)
	}
}

func TestResolveConflictsPriority(t *testing.T) {
	beams := []BeamAssignment{
		{ServiceRequestID: "sr-low", InterfaceID: "if-1", Priority: 1},
		{ServiceRequestID: "sr-high", InterfaceID: "if-1", Priority: 10},
	}
	conflict := BeamConflict{InterfaceID: "if-1", ConflictingBeams: beams}
	actions := ResolveConflicts([]BeamConflict{conflict}, StrategyPriority)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Assignment.ServiceRequestID != "sr-low" {
		t.Fatalf("expected low priority beam cancelled, got %s", actions[0].Assignment.ServiceRequestID)
	}
}

func TestResolveConflictsEarliestDeadline(t *testing.T) {
	now := time.Now()
	beams := []BeamAssignment{
		{ServiceRequestID: "sr-late", InterfaceID: "if-1", Deadline: now.Add(10 * time.Second)},
		{ServiceRequestID: "sr-soon", InterfaceID: "if-1", Deadline: now.Add(1 * time.Second)},
	}
	conflict := BeamConflict{InterfaceID: "if-1", ConflictingBeams: beams}
	actions := ResolveConflicts([]BeamConflict{conflict}, StrategyEarliestDeadline)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Assignment.ServiceRequestID != "sr-late" {
		t.Fatalf("expected later deadline cancelled, got %s", actions[0].Assignment.ServiceRequestID)
	}
}

func TestResolveConflictsFairness(t *testing.T) {
	beams := []BeamAssignment{
		{ServiceRequestID: "sr-A", InterfaceID: "if-1", FairnessScore: 5, StartTime: 0},
		{ServiceRequestID: "sr-B", InterfaceID: "if-1", FairnessScore: 1, StartTime: 1},
	}
	conflict := BeamConflict{InterfaceID: "if-1", ConflictingBeams: beams}
	actions := ResolveConflicts([]BeamConflict{conflict}, StrategyFairness)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Assignment.ServiceRequestID != "sr-A" {
		t.Fatalf("expected less fair sr cancelled, got %s", actions[0].Assignment.ServiceRequestID)
	}
}
