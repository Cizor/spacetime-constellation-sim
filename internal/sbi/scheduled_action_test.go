package sbi

import (
	"testing"
	"time"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestScheduledActionType_BeamActions(t *testing.T) {
	// Test that beam-related action types are defined
	if ScheduledUpdateBeam == ScheduledActionUnknown {
		t.Fatalf("ScheduledUpdateBeam should not equal ScheduledActionUnknown")
	}
	if ScheduledDeleteBeam == ScheduledActionUnknown {
		t.Fatalf("ScheduledDeleteBeam should not equal ScheduledActionUnknown")
	}
	if ScheduledUpdateBeam == ScheduledDeleteBeam {
		t.Fatalf("ScheduledUpdateBeam and ScheduledDeleteBeam should be distinct")
	}
}

func TestScheduledActionType_RouteActions(t *testing.T) {
	// Test that route-related action types are defined
	if ScheduledSetRoute == ScheduledActionUnknown {
		t.Fatalf("ScheduledSetRoute should not equal ScheduledActionUnknown")
	}
	if ScheduledDeleteRoute == ScheduledActionUnknown {
		t.Fatalf("ScheduledDeleteRoute should not equal ScheduledActionUnknown")
	}
	if ScheduledSetRoute == ScheduledDeleteRoute {
		t.Fatalf("ScheduledSetRoute and ScheduledDeleteRoute should be distinct")
	}
}

func TestScheduledActionType_SrPolicyActions(t *testing.T) {
	// Test that SR policy-related action types are defined
	if ScheduledSetSrPolicy == ScheduledActionUnknown {
		t.Fatalf("ScheduledSetSrPolicy should not equal ScheduledActionUnknown")
	}
	if ScheduledDeleteSrPolicy == ScheduledActionUnknown {
		t.Fatalf("ScheduledDeleteSrPolicy should not equal ScheduledActionUnknown")
	}
	if ScheduledSetSrPolicy == ScheduledDeleteSrPolicy {
		t.Fatalf("ScheduledSetSrPolicy and ScheduledDeleteSrPolicy should be distinct")
	}
}

func TestBeamSpec_Construction(t *testing.T) {
	beam := &BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		FrequencyHz:  11e9, // 11 GHz
	}

	if beam.NodeID != "nodeA" {
		t.Fatalf("BeamSpec.NodeID = %q, want %q", beam.NodeID, "nodeA")
	}
	if beam.InterfaceID != "ifA" {
		t.Fatalf("BeamSpec.InterfaceID = %q, want %q", beam.InterfaceID, "ifA")
	}
	if beam.TargetNodeID != "nodeB" {
		t.Fatalf("BeamSpec.TargetNodeID = %q, want %q", beam.TargetNodeID, "nodeB")
	}
	if beam.FrequencyHz != 11e9 {
		t.Fatalf("BeamSpec.FrequencyHz = %v, want %v", beam.FrequencyHz, 11e9)
	}
}

func TestSrPolicySpec_Construction(t *testing.T) {
	srPolicy := &SrPolicySpec{
		PolicyID: "sr-policy-1",
	}

	if srPolicy.PolicyID != "sr-policy-1" {
		t.Fatalf("SrPolicySpec.PolicyID = %q, want %q", srPolicy.PolicyID, "sr-policy-1")
	}
}

func TestScheduledAction_UpdateBeam(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	beam := &BeamSpec{
		NodeID:       "nodeA",
		InterfaceID:  "ifA",
		TargetNodeID: "nodeB",
		TargetIfID:   "ifB",
	}
	meta := ActionMeta{
		RequestID: "req-123",
		SeqNo:     1,
		Token:     "token-abc",
	}

	action := NewBeamAction("entry-1", "agent-1", ScheduledUpdateBeam, when, beam, meta)

	if action.EntryID != "entry-1" {
		t.Fatalf("ScheduledAction.EntryID = %q, want %q", action.EntryID, "entry-1")
	}
	if action.AgentID != "agent-1" {
		t.Fatalf("ScheduledAction.AgentID = %q, want %q", action.AgentID, "agent-1")
	}
	if action.Type != ScheduledUpdateBeam {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, ScheduledUpdateBeam)
	}
	if !action.When.Equal(when) {
		t.Fatalf("ScheduledAction.When = %v, want %v", action.When, when)
	}
	if action.RequestID != "req-123" {
		t.Fatalf("ScheduledAction.RequestID = %q, want %q", action.RequestID, "req-123")
	}
	if action.SeqNo != 1 {
		t.Fatalf("ScheduledAction.SeqNo = %d, want %d", action.SeqNo, 1)
	}
	if action.Token != "token-abc" {
		t.Fatalf("ScheduledAction.Token = %q, want %q", action.Token, "token-abc")
	}
	if action.Beam == nil {
		t.Fatalf("ScheduledAction.Beam should not be nil")
	}
	if action.Beam.NodeID != "nodeA" {
		t.Fatalf("ScheduledAction.Beam.NodeID = %q, want %q", action.Beam.NodeID, "nodeA")
	}
	if action.Route != nil {
		t.Fatalf("ScheduledAction.Route should be nil for beam action")
	}
	if action.SrPolicy != nil {
		t.Fatalf("ScheduledAction.SrPolicy should be nil for beam action")
	}
}

func TestScheduledAction_DeleteBeam(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	beam := &BeamSpec{
		NodeID:      "nodeA",
		InterfaceID: "ifA",
	}
	meta := ActionMeta{
		RequestID: "req-124",
		SeqNo:     2,
		Token:     "token-abc",
	}

	action := NewBeamAction("entry-2", "agent-1", ScheduledDeleteBeam, when, beam, meta)

	if action.Type != ScheduledDeleteBeam {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, ScheduledDeleteBeam)
	}
	if action.Beam == nil {
		t.Fatalf("ScheduledAction.Beam should not be nil")
	}
}

func TestScheduledAction_SetRoute(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	route := &model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
		NextHopNodeID:   "nodeB",
		OutInterfaceID:  "if1",
	}
	meta := ActionMeta{
		RequestID: "req-125",
		SeqNo:     3,
		Token:     "token-abc",
	}

	action := NewRouteAction("entry-3", "agent-1", ScheduledSetRoute, when, route, meta)

	if action.Type != ScheduledSetRoute {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, ScheduledSetRoute)
	}
	if action.Route == nil {
		t.Fatalf("ScheduledAction.Route should not be nil")
	}
	if action.Route.DestinationCIDR != "10.0.0.0/24" {
		t.Fatalf("ScheduledAction.Route.DestinationCIDR = %q, want %q", action.Route.DestinationCIDR, "10.0.0.0/24")
	}
	if action.Beam != nil {
		t.Fatalf("ScheduledAction.Beam should be nil for route action")
	}
	if action.SrPolicy != nil {
		t.Fatalf("ScheduledAction.SrPolicy should be nil for route action")
	}
}

func TestScheduledAction_DeleteRoute(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	route := &model.RouteEntry{
		DestinationCIDR: "10.0.0.0/24",
	}
	meta := ActionMeta{
		RequestID: "req-126",
		SeqNo:     4,
		Token:     "token-abc",
	}

	action := NewRouteAction("entry-4", "agent-1", ScheduledDeleteRoute, when, route, meta)

	if action.Type != ScheduledDeleteRoute {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, ScheduledDeleteRoute)
	}
	if action.Route == nil {
		t.Fatalf("ScheduledAction.Route should not be nil")
	}
}

func TestScheduledAction_SetSrPolicy(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	srPolicy := &SrPolicySpec{
		PolicyID: "sr-policy-1",
	}
	meta := ActionMeta{
		RequestID: "req-127",
		SeqNo:     5,
		Token:     "token-abc",
	}

	action := NewSrPolicyAction("entry-5", "agent-1", ScheduledSetSrPolicy, when, srPolicy, meta)

	if action.Type != ScheduledSetSrPolicy {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, ScheduledSetSrPolicy)
	}
	if action.SrPolicy == nil {
		t.Fatalf("ScheduledAction.SrPolicy should not be nil")
	}
	if action.SrPolicy.PolicyID != "sr-policy-1" {
		t.Fatalf("ScheduledAction.SrPolicy.PolicyID = %q, want %q", action.SrPolicy.PolicyID, "sr-policy-1")
	}
	if action.Beam != nil {
		t.Fatalf("ScheduledAction.Beam should be nil for SR policy action")
	}
	if action.Route != nil {
		t.Fatalf("ScheduledAction.Route should be nil for SR policy action")
	}
}

func TestScheduledAction_DeleteSrPolicy(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	srPolicy := &SrPolicySpec{
		PolicyID: "sr-policy-1",
	}
	meta := ActionMeta{
		RequestID: "req-128",
		SeqNo:     6,
		Token:     "token-abc",
	}

	action := NewSrPolicyAction("entry-6", "agent-1", ScheduledDeleteSrPolicy, when, srPolicy, meta)

	if action.Type != ScheduledDeleteSrPolicy {
		t.Fatalf("ScheduledAction.Type = %v, want %v", action.Type, ScheduledDeleteSrPolicy)
	}
	if action.SrPolicy == nil {
		t.Fatalf("ScheduledAction.SrPolicy should not be nil")
	}
}

func TestScheduledAction_Validate_Success(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	beam := &BeamSpec{
		NodeID:      "nodeA",
		InterfaceID: "ifA",
	}
	meta := ActionMeta{
		RequestID: "req-123",
		SeqNo:     1,
		Token:     "token-abc",
	}

	action := NewBeamAction("entry-1", "agent-1", ScheduledUpdateBeam, when, beam, meta)

	if err := action.Validate(); err != nil {
		t.Fatalf("Validate() returned error for valid action: %v", err)
	}
}

func TestScheduledAction_Validate_UnknownType(t *testing.T) {
	action := &ScheduledAction{
		EntryID: "entry-1",
		AgentID: "agent-1",
		Type:    ScheduledActionUnknown,
		When:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Beam:    &BeamSpec{NodeID: "nodeA", InterfaceID: "ifA"},
	}

	if err := action.Validate(); err == nil {
		t.Fatalf("Validate() should return error for Unknown type")
	}
	if err := action.Validate(); err.Error() != "ScheduledAction.Type must not be ScheduledActionUnknown" {
		t.Fatalf("Validate() error = %q, want %q", err.Error(), "ScheduledAction.Type must not be ScheduledActionUnknown")
	}
}

func TestScheduledAction_Validate_EmptyEntryID(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	beam := &BeamSpec{
		NodeID:      "nodeA",
		InterfaceID: "ifA",
	}

	action := &ScheduledAction{
		EntryID: "",
		AgentID: "agent-1",
		Type:    ScheduledUpdateBeam,
		When:    when,
		Beam:    beam,
	}

	if err := action.Validate(); err == nil {
		t.Fatalf("Validate() should return error for empty EntryID")
	}
	if err := action.Validate(); err.Error() != "ScheduledAction.EntryID must not be empty" {
		t.Fatalf("Validate() error = %q, want %q", err.Error(), "ScheduledAction.EntryID must not be empty")
	}
}

func TestScheduledAction_Validate_ZeroWhen(t *testing.T) {
	beam := &BeamSpec{
		NodeID:      "nodeA",
		InterfaceID: "ifA",
	}

	action := &ScheduledAction{
		EntryID: "entry-1",
		AgentID: "agent-1",
		Type:    ScheduledUpdateBeam,
		When:    time.Time{}, // zero time
		Beam:    beam,
	}

	if err := action.Validate(); err == nil {
		t.Fatalf("Validate() should return error for zero When")
	}
	if err := action.Validate(); err.Error() != "ScheduledAction.When must not be zero" {
		t.Fatalf("Validate() error = %q, want %q", err.Error(), "ScheduledAction.When must not be zero")
	}
}

func TestScheduledAction_Validate_MissingBeamPayload(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	action := &ScheduledAction{
		EntryID: "entry-1",
		AgentID: "agent-1",
		Type:    ScheduledUpdateBeam,
		When:    when,
		Beam:    nil, // missing payload
	}

	if err := action.Validate(); err == nil {
		t.Fatalf("Validate() should return error for missing Beam payload")
	}
}

func TestScheduledAction_Validate_MissingRoutePayload(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	action := &ScheduledAction{
		EntryID: "entry-1",
		AgentID: "agent-1",
		Type:    ScheduledSetRoute,
		When:    when,
		Route:   nil, // missing payload
	}

	if err := action.Validate(); err == nil {
		t.Fatalf("Validate() should return error for missing Route payload")
	}
}

func TestScheduledAction_Validate_MissingSrPolicyPayload(t *testing.T) {
	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	action := &ScheduledAction{
		EntryID:  "entry-1",
		AgentID:  "agent-1",
		Type:     ScheduledSetSrPolicy,
		When:     when,
		SrPolicy: nil, // missing payload
	}

	if err := action.Validate(); err == nil {
		t.Fatalf("Validate() should return error for missing SrPolicy payload")
	}
}

