package nbi

import (
	"context"
	"errors"
	"testing"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeMotionModel is a minimal MotionModel implementation used by tests.
// It records added/removed platform IDs and can be configured to return
// errors for AddPlatform / RemovePlatform.
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

// newScenarioStateForTest constructs an in-memory ScenarioState backed by
// in-memory Scope-1 and Scope-2 knowledge bases.
func newScenarioStateForTest() *sim.ScenarioState {
	return sim.NewScenarioState(kb.NewKnowledgeBase(), core.NewKnowledgeBase(), logging.Noop())
}

// platformProto builds a proto PlatformDefinition from a domain object using
// the real mapping functions.
func platformProto(t *testing.T, dom *model.PlatformDefinition) *common.PlatformDefinition {
	t.Helper()
	pd := types.PlatformToProto(dom)
	if pd == nil {
		t.Fatalf("PlatformToProto returned nil for %+v", dom)
	}
	return pd
}

// --- Happy-path CRUD ---

func TestPlatformServiceCRUDHappyPath(t *testing.T) {
	state := newScenarioStateForTest()
	motion := &fakeMotionModel{}
	svc := NewPlatformService(state, motion, logging.Noop())

	ctx := context.Background()
	base := &model.PlatformDefinition{
		ID:           "platform-crud",
		Name:         "platform-crud",
		Type:         "SATELLITE",
		MotionSource: model.MotionSourceSpacetrack,
	}

	// Create
	createResp, err := svc.CreatePlatform(ctx, platformProto(t, base))
	if err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}
	if createResp.GetName() != base.ID || createResp.GetType() != base.Type {
		t.Fatalf("CreatePlatform response = %#v, want name/type %s/%s", createResp, base.ID, base.Type)
	}
	if len(motion.added) != 1 || motion.added[0] != base.ID {
		t.Fatalf("motion AddPlatform calls = %#v, want [%s]", motion.added, base.ID)
	}
	if stored, err := state.GetPlatform(base.ID); err != nil || stored.Type != base.Type {
		t.Fatalf("scenario state platform = (%#v, %v), want type %s", stored, err, base.Type)
	}

	// Get
	getResp, err := svc.GetPlatform(ctx, &v1alpha.GetPlatformRequest{PlatformId: &base.ID})
	if err != nil {
		t.Fatalf("GetPlatform error: %v", err)
	}
	if getResp.GetName() != base.ID || getResp.GetType() != base.Type {
		t.Fatalf("GetPlatform response = %#v, want name/type %s/%s", getResp, base.ID, base.Type)
	}

	// List
	listResp, err := svc.ListPlatforms(ctx, &v1alpha.ListPlatformsRequest{})
	if err != nil {
		t.Fatalf("ListPlatforms error: %v", err)
	}
	if len(listResp.GetPlatforms()) != 1 || listResp.GetPlatforms()[0].GetName() != base.ID {
		t.Fatalf("ListPlatforms = %#v, want single platform %s", listResp.GetPlatforms(), base.ID)
	}

	// Update
	updated := &model.PlatformDefinition{
		ID:           base.ID,
		Name:         base.Name,
		Type:         "AIRCRAFT",
		MotionSource: model.MotionSourceSpacetrack,
	}
	updateResp, err := svc.UpdatePlatform(ctx, &v1alpha.UpdatePlatformRequest{
		Platform: platformProto(t, updated),
	})
	if err != nil {
		t.Fatalf("UpdatePlatform error: %v", err)
	}
	if updateResp.GetName() != updated.Name || updateResp.GetType() != updated.Type {
		t.Fatalf("UpdatePlatform response = %#v, want name/type %s/%s", updateResp, updated.Name, updated.Type)
	}
	if stored, err := state.GetPlatform(base.ID); err != nil || stored.Name != updated.Name || stored.Type != updated.Type {
		t.Fatalf("scenario state after update = (%#v, %v), want name/type %s/%s", stored, err, updated.Name, updated.Type)
	}

	// Delete
	if _, err := svc.DeletePlatform(ctx, &v1alpha.DeletePlatformRequest{PlatformId: &base.ID}); err != nil {
		t.Fatalf("DeletePlatform error: %v", err)
	}
	if len(motion.removed) != 1 || motion.removed[0] != base.ID {
		t.Fatalf("motion RemovePlatform calls = %#v, want [%s]", motion.removed, base.ID)
	}
	if _, err := state.GetPlatform(base.ID); !errors.Is(err, sim.ErrPlatformNotFound) {
		t.Fatalf("platform should be removed from state, err=%v", err)
	}
}

// --- Validation and error cases ---

func TestCreatePlatformValidationErrors(t *testing.T) {
	svc := NewPlatformService(newScenarioStateForTest(), &fakeMotionModel{}, logging.Noop())
	ctx := context.Background()

	missingType := platformProto(t, &model.PlatformDefinition{
		ID:   "no-type",
		Name: "no-type",
	})
	unknownMotion := platformProto(t, &model.PlatformDefinition{
		ID:           "sat-unknown-motion",
		Name:         "sat-unknown-motion",
		Type:         "SATELLITE",
		MotionSource: model.MotionSourceUnknown,
	})

	tests := []struct {
		name string
		pd   *common.PlatformDefinition
		code codes.Code
	}{
		{name: "nil proto", pd: nil, code: codes.InvalidArgument},
		{name: "missing type", pd: missingType, code: codes.InvalidArgument},
		{name: "satellite missing motion source", pd: unknownMotion, code: codes.InvalidArgument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreatePlatform(ctx, tc.pd)
			if status.Code(err) != tc.code {
				t.Fatalf("CreatePlatform(%s) code = %v, want %v (err=%v)", tc.name, status.Code(err), tc.code, err)
			}
		})
	}
}

func TestPlatformNotFoundErrors(t *testing.T) {
	svc := NewPlatformService(newScenarioStateForTest(), &fakeMotionModel{}, logging.Noop())
	ctx := context.Background()

	pid := "missing"

	if _, err := svc.GetPlatform(ctx, &v1alpha.GetPlatformRequest{PlatformId: &pid}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetPlatform missing code = %v, want NotFound (err=%v)", status.Code(err), err)
	}

	updateReq := &v1alpha.UpdatePlatformRequest{
		Platform: platformProto(t, &model.PlatformDefinition{
			ID:           pid,
			Name:         pid,
			Type:         "SATELLITE",
			MotionSource: model.MotionSourceSpacetrack,
		}),
	}
	if _, err := svc.UpdatePlatform(ctx, updateReq); status.Code(err) != codes.NotFound {
		t.Fatalf("UpdatePlatform missing code = %v, want NotFound (err=%v)", status.Code(err), err)
	}

	if _, err := svc.DeletePlatform(ctx, &v1alpha.DeletePlatformRequest{PlatformId: &pid}); status.Code(err) != codes.NotFound {
		t.Fatalf("DeletePlatform missing code = %v, want NotFound (err=%v)", status.Code(err), err)
	}
}

func TestDeletePlatformFailsWhenNodesPresent(t *testing.T) {
	state := newScenarioStateForTest()
	svc := NewPlatformService(state, &fakeMotionModel{}, logging.Noop())
	ctx := context.Background()

	platform := &model.PlatformDefinition{
		ID:           "plat-in-use",
		Name:         "plat-in-use",
		Type:         "SATELLITE",
		MotionSource: model.MotionSourceSpacetrack,
	}
	if _, err := svc.CreatePlatform(ctx, platformProto(t, platform)); err != nil {
		t.Fatalf("CreatePlatform error: %v", err)
	}

	nodeID := "node-uses-platform"
	if err := state.CreateNode(&model.NetworkNode{ID: nodeID, PlatformID: platform.ID}, []*core.NetworkInterface{
		{ID: nodeID + "/if0", ParentNodeID: nodeID, Medium: core.MediumWired},
	}); err != nil {
		t.Fatalf("CreateNode error: %v", err)
	}

	_, err := svc.DeletePlatform(ctx, &v1alpha.DeletePlatformRequest{PlatformId: &platform.ID})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeletePlatform code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}

	if got, err := state.GetPlatform(platform.ID); err != nil || got == nil {
		t.Fatalf("platform should remain after failed delete, got (%+v, %v)", got, err)
	}
}

// --- Motion-model specific behaviours ---

func TestCreatePlatformRegistersMotionModel(t *testing.T) {
	motion := &fakeMotionModel{}
	svc := NewPlatformService(newScenarioStateForTest(), motion, logging.Noop())

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
	svc := NewPlatformService(newScenarioStateForTest(), motion, logging.Noop())

	name := "platform-2"
	typ := "SATELLITE"
	ms := common.PlatformDefinition_SPACETRACK_ORG

	_, err := svc.CreatePlatform(context.Background(), &common.PlatformDefinition{
		Name:         &name,
		Type:         &typ,
		MotionSource: &ms,
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

	svc := NewPlatformService(state, motion, logging.Noop())
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

	svc := NewPlatformService(state, motion, logging.Noop())
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
