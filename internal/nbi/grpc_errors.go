package nbi

import (
	"errors"

	core "github.com/signalsfoundry/constellation-simulator/core"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// ErrNotFound is a package-level sentinel used when an entity cannot be located.
	ErrNotFound = errors.New("not found")
	// ErrInvalidEntity is a package-level sentinel used for client-side validation failures.
	ErrInvalidEntity = errors.New("invalid entity")
)

// ToStatusError maps common simulator errors onto gRPC status codes for NBI services.
func ToStatusError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}

	switch {
	case errors.Is(err, ErrNotFound),
		errors.Is(err, sim.ErrPlatformNotFound),
		errors.Is(err, sim.ErrNodeNotFound),
		errors.Is(err, sim.ErrInterfaceNotFound),
		errors.Is(err, sim.ErrLinkNotFound),
		errors.Is(err, sim.ErrServiceRequestNotFound):
		return status.Error(codes.NotFound, err.Error())

	case errors.Is(err, ErrInvalidEntity),
		errors.Is(err, ErrInvalidPlatform),
		errors.Is(err, ErrInvalidNode),
		errors.Is(err, ErrInvalidInterface),
		errors.Is(err, ErrInvalidLink),
		errors.Is(err, ErrInvalidServiceRequest),
		errors.Is(err, sim.ErrInterfaceInvalid),
		errors.Is(err, sim.ErrNodeInvalid),
		errors.Is(err, sim.ErrTransceiverNotFound),
		errors.Is(err, core.ErrInterfaceMiss):
		return status.Error(codes.InvalidArgument, err.Error())

	case errors.Is(err, sim.ErrPlatformInUse),
		errors.Is(err, sim.ErrNodeInUse),
		errors.Is(err, sim.ErrInterfaceInUse):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, sim.ErrPlatformExists),
		errors.Is(err, sim.ErrNodeExists),
		errors.Is(err, sim.ErrInterfaceExists),
		errors.Is(err, sim.ErrServiceRequestExists),
		errors.Is(err, core.ErrLinkExists):
		return status.Error(codes.AlreadyExists, err.Error())

	default:
		return status.Error(codes.Internal, err.Error())
	}
}
