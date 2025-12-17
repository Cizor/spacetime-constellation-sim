package agent

import (
	"testing"
	"time"

	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	geophys "aalyria.com/spacetime/nmts/v1/proto/types/geophys"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
)

func TestConvertUpdateBeamToBeamSpec_TargetMetadata(t *testing.T) {
	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	scheduler := sbi.NewFakeEventScheduler(time.Now())
	state := state.NewScenarioState(physKB, netKB, logging.Noop())
	agent := NewSimAgent(sbi.AgentID("agent-test"), "node-test", state, scheduler, &fakeTelemetryClient{}, &fakeStream{}, logging.Noop())

	tests := []struct {
		name   string
		target *schedulingpb.BeamTarget
		check  func(t *testing.T, got *sbi.BeamTarget)
	}{
		{
			name: "az_el",
			target: &schedulingpb.BeamTarget{
				Target: &schedulingpb.BeamTarget_AzEl{
					AzEl: &schedulingpb.AzEl{
						AzDeg: 12.3,
						ElDeg: 45.6,
					},
				},
			},
			check: func(t *testing.T, got *sbi.BeamTarget) {
				if got == nil || got.AzEl == nil {
					t.Fatalf("expected az/el target, got %+v", got)
				}
				if got.AzEl.AzimuthDeg != 12.3 {
					t.Fatalf("azimuth = %v, want 12.3", got.AzEl.AzimuthDeg)
				}
				if got.AzEl.ElevationDeg != 45.6 {
					t.Fatalf("elevation = %v, want 45.6", got.AzEl.ElevationDeg)
				}
			},
		},
		{
			name: "cartesian",
			target: &schedulingpb.BeamTarget{
				Target: &schedulingpb.BeamTarget_Cartesian{
					Cartesian: &schedulingpb.Cartesian{
						ReferenceFrame: geophys.CoordinateFrame_COORDINATE_FRAME_ECEF,
						XM:             1.0,
						YM:             2.0,
						ZM:             3.0,
					},
				},
			},
			check: func(t *testing.T, got *sbi.BeamTarget) {
				if got == nil || got.Cartesian == nil {
					t.Fatalf("expected cartesian target, got %+v", got)
				}
				if got.Cartesian.Coordinates.X != 1.0 || got.Cartesian.Coordinates.Y != 2.0 || got.Cartesian.Coordinates.Z != 3.0 {
					t.Fatalf("unexpected coordinates %+v", got.Cartesian.Coordinates)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action := &schedulingpb.UpdateBeam{
				Beam: &schedulingpb.Beam{
					AntennaId: "if-1",
					Target:    tc.target,
				},
			}
			beamSpec, err := agent.convertUpdateBeamToBeamSpec(action)
			if err != nil {
				t.Fatalf("convertUpdateBeamToBeamSpec failed: %v", err)
			}
			tc.check(t, beamSpec.Target)
		})
	}
}
