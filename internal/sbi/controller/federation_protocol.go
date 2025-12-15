package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/model"
)

// FederationRequest carries metadata for inter-domain coordination.
type FederationRequest struct {
	RequestID    string
	SourceDomain string
	DestDomain   string
	Requirements []model.FlowRequirement
	Token        string
	Deadline     time.Time
}

// FederationResponseStatus describes the outcome of a federation call.
type FederationResponseStatus string

const (
	FederationStatusOK    FederationResponseStatus = "ok"
	FederationStatusError FederationResponseStatus = "error"
)

// FederationResponse contains a path segment reply for a federation request.
type FederationResponse struct {
	RequestID   string
	PathSegment *PathSegment
	Status      FederationResponseStatus
	Error       string
}

// FederationClient describes the federation coordination surface.
type FederationClient interface {
	RequestPathSegment(ctx context.Context, req FederationRequest) (*FederationResponse, error)
}

// InMemoryFederationClient is a stubbed implementation that simulates simple coordination.
type InMemoryFederationClient struct {
	logger logging.Logger
}

// NewInMemoryFederationClient creates a stub federation client.
func NewInMemoryFederationClient(logger logging.Logger) FederationClient {
	if logger == nil {
		logger = logging.Noop()
	}
	return &InMemoryFederationClient{logger: logger}
}

// RequestPathSegment replies with a simple border path segment derived from the request.
func (c *InMemoryFederationClient) RequestPathSegment(ctx context.Context, req FederationRequest) (*FederationResponse, error) {
	if req.RequestID == "" {
		return nil, fmt.Errorf("request ID required")
	}
	if req.Token == "" {
		return &FederationResponse{
			RequestID: req.RequestID,
			Status:    FederationStatusError,
			Error:     "missing token",
		}, nil
	}
	c.logger.Debug(ctx, "FederationRequest",
		logging.String("source", req.SourceDomain),
		logging.String("dest", req.DestDomain),
	)
	segment := &PathSegment{
		DomainID:    req.DestDomain,
		Path:        &model.Path{Nodes: []string{fmt.Sprintf("border-%s", req.DestDomain)}},
		BorderNodes: []string{fmt.Sprintf("border-%s", req.DestDomain)},
	}
	return &FederationResponse{
		RequestID:   req.RequestID,
		PathSegment: segment,
		Status:      FederationStatusOK,
	}, nil
}
