package nbi

import (
	"context"
	"errors"
	"testing"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeMotionModel struct {
	added     []string
	removed   []string
	addErr    error
	removeErr error
}

func (f *fakeMotionModel) AddPlatform(pd *model.PlatformDefinition) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, pd.ID)
	return nil
}

func (f *fakeMotionModel) RemovePlatform(platformID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, platformID)
	return nil
}

func newScenarioStateForTest() *sim.ScenarioState {
	return sim.NewScenarioState(kb.NewKnowledgeBase(), core.NewKnowledgeBase())
}

func TestCreatePlatformRegistersMotionModel(t *testing.T) {
	motion := &fakeMotionModel{}
	svc := NewPlatformService(newScenarioStateForTest(), motion, nil)

	name := "platform-1"
	typ := "SATELLITE"
	ms := common.PlatformDefinition_SPACETRACK_ORG

	resp, err := svc.CreatePlatform(context.Background(), &common.PlatformDefinition{
		Name:         &name,
		Type:         &typ,
		MotionSource: &ms,
	})
	if err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	// Proto round-trip sanity: name should match.
	if resp.GetName() != name {
		t.Fatalf("unexpected platform name in response: %s", resp.GetName())
	}

	// We don’t have an ID field on the proto, so read from ScenarioState.
	plats := svc.state.ListPlatforms()
	if len(plats) != 1 {
		t.Fatalf("expected exactly 1 platform in state, got %d", len(plats))
	}
	id := plats[0].ID
	if id == "" {
		t.Fatalf("expected platform in state to have a non-empty ID")
	}

	if len(motion.added) != 1 {
		t.Fatalf("expected motion model registration for one platform, got %d", len(motion.added))
	}
	if motion.added[0] != id {
		t.Fatalf("expected motion model registration for id %q, got %q", id, motion.added[0])
	}
}

func TestCreatePlatformMotionModelErrorRollsBack(t *testing.T) {
	motion := &fakeMotionModel{addErr: errors.New("motion failure")}
	svc := NewPlatformService(newScenarioStateForTest(), motion, nil)

	name := "platform-2"
	typ := "SATELLITE"

	_, err := svc.CreatePlatform(context.Background(), &common.PlatformDefinition{
		Name: &name,
		Type: &typ,
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error from motion model failure, got %v", err)
	}

	// We don’t know the generated ID on failure, but we *do* know that
	// there should be no platforms left in state.
	if plats := svc.state.ListPlatforms(); len(plats) != 0 {
		t.Fatalf("expected no platforms in state after rollback, got %d", len(plats))
	}
}

func TestDeletePlatformUnregistersMotionModel(t *testing.T) {
	motion := &fakeMotionModel{}
	state := newScenarioStateForTest()
	if err := state.CreatePlatform(&model.PlatformDefinition{
		ID:   "platform-3",
		Name: "platform-3",
		Type: "SATELLITE",
	}); err != nil {
		t.Fatalf("seed platform error: %v", err)
	}

	svc := NewPlatformService(state, motion, nil)
	pid := "platform-3"
	if _, err := svc.DeletePlatform(context.Background(), &v1alpha.DeletePlatformRequest{
		PlatformId: &pid,
	}); err != nil {
		t.Fatalf("DeletePlatform error: %v", err)
	}

	if len(motion.removed) != 1 || motion.removed[0] != "platform-3" {
		t.Fatalf("expected motion removal for platform-3, got %#v", motion.removed)
	}
	if _, err := state.GetPlatform("platform-3"); !errors.Is(err, sim.ErrPlatformNotFound) {
		t.Fatalf("platform should be removed from state, err=%v", err)
	}
}

func TestDeletePlatformMotionModelError(t *testing.T) {
	motion := &fakeMotionModel{removeErr: errors.New("cannot remove")}
	state := newScenarioStateForTest()
	if err := state.CreatePlatform(&model.PlatformDefinition{
		ID:   "platform-4",
		Name: "platform-4",
		Type: "SATELLITE",
	}); err != nil {
		t.Fatalf("seed platform error: %v", err)
	}

	svc := NewPlatformService(state, motion, nil)
	pid := "platform-4"
	_, err := svc.DeletePlatform(context.Background(), &v1alpha.DeletePlatformRequest{
		PlatformId: &pid,
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error when motion removal fails, got %v", err)
	}
	if _, err := state.GetPlatform("platform-4"); !errors.Is(err, sim.ErrPlatformNotFound) {
		t.Fatalf("platform should already be removed from state despite motion error, err=%v", err)
	}
}
