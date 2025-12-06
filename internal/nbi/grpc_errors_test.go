package nbi

import (
	"errors"
	"fmt"
	"testing"

	core "github.com/signalsfoundry/constellation-simulator/core"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToStatusError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		code    codes.Code
		wantNil bool
	}{
		{name: "nil", err: nil, wantNil: true},
		{name: "status passthrough", err: status.Error(codes.PermissionDenied, "denied"), code: codes.PermissionDenied},
		{name: "invalid entity sentinel", err: fmt.Errorf("%w: bad entity", ErrInvalidEntity), code: codes.InvalidArgument},
		{name: "validation sentinel", err: ErrInvalidServiceRequest, code: codes.InvalidArgument},
		{name: "transceiver not found", err: sim.ErrTransceiverNotFound, code: codes.InvalidArgument},
		{name: "not found", err: sim.ErrPlatformNotFound, code: codes.NotFound},
		{name: "referential conflict", err: sim.ErrNodeInUse, code: codes.FailedPrecondition},
		{name: "already exists", err: core.ErrLinkExists, code: codes.AlreadyExists},
		{name: "fallback", err: errors.New("boom"), code: codes.Internal},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ToStatusError(tc.err)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("ToStatusError(nil) = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatalf("ToStatusError(%v) = nil, want error", tc.err)
			}
			if code := status.Code(got); code != tc.code {
				t.Fatalf("ToStatusError(%v) code = %v, want %v", tc.err, code, tc.code)
			}
		})
	}
}
