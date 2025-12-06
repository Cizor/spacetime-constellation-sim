// internal/nbi/platform_service.go
package nbi

import (
	"context"
	"errors"
	"fmt"
	"time"

	common "aalyria.com/spacetime/api/common"
	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi/types"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
	log    logging.Logger
}

// NewPlatformService wires a PlatformService to the shared ScenarioState and
// optional logger.
func NewPlatformService(state *sim.ScenarioState, motion MotionModel, log logging.Logger) *PlatformService {
	if log == nil {
		log = logging.Noop()
	}
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
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "platform"),
		logging.String("operation", "create"),
	)

	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if err := ValidatePlatformProto(in); err != nil {
		reqLog.Debug(ctx, "CreatePlatform validation failed",
			logging.String("reason", err.Error()),
			logging.Any("name", in.GetName()),
		)
		return nil, ToStatusError(err)
	}

	dom, err := types.PlatformFromProto(in)
	if err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	}

	// Generate an ID if missing.
	if dom.ID == "" {
		dom.ID = generatePlatformID()
	}

	// Default name to ID if absent.
	if dom.Name == "" {
		dom.Name = dom.ID
	}

	if err := s.state.CreatePlatform(dom); err != nil {
		reqLog.Warn(ctx, "CreatePlatform failed",
			logging.String("entity_id", dom.ID),
			logging.String("error", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	if s.motion != nil {
		if err := s.motion.AddPlatform(dom); err != nil {
			reqLog.Error(ctx, "failed to register platform with motion model",
				logging.String("entity_id", dom.ID),
				logging.String("error", err.Error()),
			)
			// Try to roll back state to keep it consistent with motion model.
			if delErr := s.state.DeletePlatform(dom.ID); delErr != nil {
				reqLog.Warn(ctx, "failed to roll back platform after motion model error",
					logging.String("entity_id", dom.ID),
					logging.String("error", delErr.Error()),
				)
			}
			return nil, status.Error(codes.Internal, "failed to register platform in motion model")
		}
	}

	reqLog.Info(ctx, "platform created",
		logging.String("entity_id", dom.ID),
	)

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
		return nil, ToStatusError(err)
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
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "platform"),
		logging.String("operation", "update"),
	)
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if req == nil || req.GetPlatform() == nil {
		return nil, status.Error(codes.InvalidArgument, "platform is required")
	}

	if err := ValidatePlatformProto(req.GetPlatform()); err != nil {
		reqLog.Debug(ctx, "UpdatePlatform validation failed",
			logging.String("reason", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	dom, err := types.PlatformFromProto(req.GetPlatform())
	if err != nil {
		return nil, ToStatusError(fmt.Errorf("%w: %v", ErrInvalidEntity, err))
	}

	// You must know which platform to update.
	// Ensure it exists so we can map NotFound cleanly.
	if _, err := s.state.GetPlatform(dom.ID); err != nil {
		return nil, ToStatusError(err)
	}

	if err := s.state.UpdatePlatform(dom); err != nil {
		reqLog.Warn(ctx, "UpdatePlatform failed",
			logging.String("entity_id", dom.ID),
			logging.String("error", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	// Note: we *could* update the motion model here if motion-related
	// parameters change (e.g. TLE, motion source), but that's out of scope
	// for this chunk. For now we assume motion-related fields are stable.

	reqLog.Info(ctx, "platform updated",
		logging.String("entity_id", dom.ID),
	)

	return types.PlatformToProto(dom), nil
}

func (s *PlatformService) DeletePlatform(
	ctx context.Context,
	req *v1alpha.DeletePlatformRequest,
) (*emptypb.Empty, error) {
	ctx, reqLog := logging.WithRequestLogger(ctx, s.log)
	reqLog = reqLog.With(
		logging.String("entity_type", "platform"),
		logging.String("operation", "delete"),
	)
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
		level := reqLog.Warn
		if errors.Is(err, sim.ErrPlatformInUse) {
			level = reqLog.Info
		}
		level(ctx, "DeletePlatform failed",
			logging.String("entity_id", req.GetPlatformId()),
			logging.String("error", err.Error()),
		)
		return nil, ToStatusError(err)
	}

	if s.motion != nil {
		if err := s.motion.RemovePlatform(req.GetPlatformId()); err != nil {
			reqLog.Error(ctx, "failed to unregister platform from motion model",
				logging.String("entity_id", req.GetPlatformId()),
				logging.String("error", err.Error()),
			)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	reqLog.Info(ctx, "platform deleted",
		logging.String("entity_id", req.GetPlatformId()),
	)

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
