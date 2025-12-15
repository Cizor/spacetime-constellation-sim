package controller

import (
	"context"
	"testing"

	"github.com/signalsfoundry/constellation-simulator/internal/logging"
)

func TestInMemoryFederationClient_RequestPathSegment(t *testing.T) {
	client := NewInMemoryFederationClient(logging.Noop())
	req := FederationRequest{
		RequestID:    "req-1",
		SourceDomain: "dom-a",
		DestDomain:   "dom-b",
		Token:        "tok",
	}
	resp, err := client.RequestPathSegment(context.Background(), req)
	if err != nil {
		t.Fatalf("RequestPathSegment error = %v", err)
	}
	if resp.Status != FederationStatusOK {
		t.Fatalf("Status = %s, want %s", resp.Status, FederationStatusOK)
	}
	if resp.PathSegment == nil || resp.PathSegment.DomainID != req.DestDomain {
		t.Fatalf("PathSegment = %+v", resp.PathSegment)
	}
}

func TestInMemoryFederationClient_MissingToken(t *testing.T) {
	client := NewInMemoryFederationClient(logging.Noop())
	req := FederationRequest{RequestID: "req-2", SourceDomain: "dom-a", DestDomain: "dom-b"}
	resp, err := client.RequestPathSegment(context.Background(), req)
	if err != nil {
		t.Fatalf("RequestPathSegment error = %v", err)
	}
	if resp.Status != FederationStatusError {
		t.Fatalf("Status = %s, want %s", resp.Status, FederationStatusError)
	}
}
