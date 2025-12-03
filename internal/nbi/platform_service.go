// internal/nbi/platform_service.go
package nbi

import (
	"context"
	"errors"
	"fmt"
	"time"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Logger is a minimal interface satisfied by zap.SugaredLogger and similar
// structured loggers.
type Logger interface {
	Debugw(msg string, keysAndValues ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
	Errorw(msg string, keysAndValues ...interface{})
}

// MotionModel describes the subset of motion functionality PlatformService needs.
type MotionModel interface {
	AddPlatform(pd *model.PlatformDefinition) error
	RemovePlatform(platformID string) error
}

// PlatformService implements the PlatformService gRPC server backed by a
// ScenarioState instance for persistence.
type PlatformService struct {
	v1alpha.UnimplementedPlatformServiceServer

	state  *sim.ScenarioState
	motion MotionModel
	log    Logger
}

// NewPlatformService wires a PlatformService to the shared ScenarioState and
// optional logger.
func NewPlatformService(state *sim.ScenarioState, motion MotionModel, log Logger) *PlatformService {
	return &PlatformService{
		state:  state,
		motion: motion,
		log:    log,
	}
}

func (s *PlatformService) CreatePlatform(
	ctx context.Context,
	in *common.PlatformDefinition,
) (*common.PlatformDefinition, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "platform definition is required")
	}

	dom, err := types.PlatformFromProto(in)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Generate an ID if missing.
	if dom.ID == "" {
		dom.ID = generatePlatformID()
	}

	// Default name to ID if absent.
	if dom.Name == "" {
		dom.Name = dom.ID
	}

	// Require a type – detailed validation is deferred to later chunks.
	if dom.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "platform type is required")
	}

	// Orbital platforms need a valid motion source (e.g. TLE-backed propagation).
	if dom.Type == "SATELLITE" && dom.MotionSource == model.MotionSourceUnknown {
		return nil, status.Error(codes.InvalidArgument, "motion source is required for orbital platforms")
	}

	if err := s.state.CreatePlatform(dom); err != nil {
		if errors.Is(err, sim.ErrPlatformExists) {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if s.motion != nil {
		if err := s.motion.AddPlatform(dom); err != nil {
			if s.log != nil {
				s.log.Errorw("failed to register platform with motion model",
					"platform_id", dom.ID,
					"err", err,
				)
			}
			// Try to roll back state to keep it consistent with motion model.
			if delErr := s.state.DeletePlatform(dom.ID); delErr != nil && s.log != nil {
				s.log.Warnw("failed to roll back platform after motion model error",
					"platform_id", dom.ID,
					"err", delErr,
				)
			}
			return nil, status.Error(codes.Internal, "failed to register platform in motion model")
		}
	}

	return types.PlatformToProto(dom), nil
}

func (s *PlatformService) GetPlatform(
	ctx context.Context,
	req *v1alpha.GetPlatformRequest,
) (*common.PlatformDefinition, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if req.GetPlatformId() == "" {
		return nil, status.Error(codes.InvalidArgument, "platform_id is required")
	}

	pd, err := s.state.GetPlatform(req.GetPlatformId())
	if err != nil {
		if errors.Is(err, sim.ErrPlatformNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return types.PlatformToProto(pd), nil
}

func (s *PlatformService) ListPlatforms(
	ctx context.Context,
	_ *v1alpha.ListPlatformsRequest,
) (*v1alpha.ListPlatformsResponse, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	resp := &v1alpha.ListPlatformsResponse{}
	for _, pd := range s.state.ListPlatforms() {
		resp.Platforms = append(resp.Platforms, types.PlatformToProto(pd))
	}
	return resp, nil
}

func (s *PlatformService) UpdatePlatform(
	ctx context.Context,
	req *v1alpha.UpdatePlatformRequest,
) (*common.PlatformDefinition, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetPlatform() == nil {
		return nil, status.Error(codes.InvalidArgument, "platform is required")
	}

	dom, err := types.PlatformFromProto(req.GetPlatform())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// You must know which platform to update.
	if dom.ID == "" {
		return nil, status.Error(codes.InvalidArgument, "platform ID is required")
	}
	if dom.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "platform type is required")
	}

	// Ensure it exists so we can map NotFound cleanly.
	if _, err := s.state.GetPlatform(dom.ID); err != nil {
		if errors.Is(err, sim.ErrPlatformNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := s.state.UpdatePlatform(dom); err != nil {
		if errors.Is(err, sim.ErrPlatformNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Note: we *could* update the motion model here if motion-related
	// parameters change (e.g. TLE, motion source), but that’s out of scope
	// for this chunk. For now we assume motion-related fields are stable.

	return types.PlatformToProto(dom), nil
}

func (s *PlatformService) DeletePlatform(
	ctx context.Context,
	req *v1alpha.DeletePlatformRequest,
) (*emptypb.Empty, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if req.GetPlatformId() == "" {
		return nil, status.Error(codes.InvalidArgument, "platform_id is required")
	}

	if err := s.state.DeletePlatform(req.GetPlatformId()); err != nil {
		if errors.Is(err, sim.ErrPlatformNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if s.motion != nil {
		if err := s.motion.RemovePlatform(req.GetPlatformId()); err != nil {
			if s.log != nil {
				s.log.Errorw("failed to unregister platform from motion model",
					"platform_id", req.GetPlatformId(),
					"err", err,
				)
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *PlatformService) ensureReady() error {
	if s == nil || s.state == nil {
		return status.Error(codes.FailedPrecondition, "scenario state is not configured")
	}
	return nil
}

func generatePlatformID() string {
	return fmt.Sprintf("platform-%d", time.Now().UnixNano())
}
