package state

import (
	"errors"
	"testing"
	"time"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestStoreRetrieveMessage(t *testing.T) {
	state := newStorageState(t, 100)

	msg := StoredMessage{
		MessageID:        "msg-1",
		ServiceRequestID: "sr-1",
		SizeBytes:        10,
		Destination:      "node-dst",
	}
	if err := state.StoreMessage("node-1", msg); err != nil {
		t.Fatalf("StoreMessage failed: %v", err)
	}
	used, capacity, err := state.GetStorageUsage("node-1")
	if err != nil {
		t.Fatalf("GetStorageUsage failed: %v", err)
	}
	if used != 10 || capacity != 100 {
		t.Fatalf("Unexpected storage usage: used=%d capacity=%d", used, capacity)
	}

	retrieved, err := state.RetrieveMessage("node-1", msg.MessageID)
	if err != nil {
		t.Fatalf("RetrieveMessage failed: %v", err)
	}
	if retrieved.MessageID != msg.MessageID {
		t.Fatalf("Retrieved message ID mismatch: got %s want %s", retrieved.MessageID, msg.MessageID)
	}
	used, _, err = state.GetStorageUsage("node-1")
	if err != nil {
		t.Fatalf("GetStorageUsage failed: %v", err)
	}
	if used != 0 {
		t.Fatalf("Expected used bytes to drop to 0 after retrieval, got %d", used)
	}
}

func TestStoreMessageCapacityEnforced(t *testing.T) {
	state := newStorageState(t, 50)

	msg := StoredMessage{
		MessageID:        "msg-full",
		ServiceRequestID: "sr-1",
		SizeBytes:        100,
		Destination:      "node-dst",
	}
	err := state.StoreMessage("node-1", msg)
	if err == nil {
		t.Fatalf("expected StoreMessage to fail due to capacity")
	}
	if !errors.Is(err, ErrStorageFull) {
		t.Fatalf("expected ErrStorageFull, got %v", err)
	}
}

func TestEvictExpiredMessages(t *testing.T) {
	state := newStorageState(t, 100)

	msg := StoredMessage{
		MessageID:        "msg-expired",
		ServiceRequestID: "sr-2",
		SizeBytes:        20,
		ExpiryTime:       time.Now().Add(-time.Minute),
		Destination:      "node-dst",
	}
	if err := state.StoreMessage("node-1", msg); err != nil {
		t.Fatalf("StoreMessage failed: %v", err)
	}

	if err := state.EvictExpiredMessages("node-1", time.Now()); err != nil {
		t.Fatalf("EvictExpiredMessages failed: %v", err)
	}
	used, _, err := state.GetStorageUsage("node-1")
	if err != nil {
		t.Fatalf("GetStorageUsage failed: %v", err)
	}
	if used != 0 {
		t.Fatalf("Expected storage usage 0 after eviction, got %d", used)
	}
	if _, err := state.RetrieveMessage("node-1", msg.MessageID); err == nil {
		t.Fatalf("RetrieveMessage should fail after eviction")
	}
}

func newStorageState(t *testing.T, capacity float64) *ScenarioState {
	t.Helper()
	physKB := kb.NewKnowledgeBase()
	netKB := network.NewKnowledgeBase()
	state := NewScenarioState(physKB, netKB, logging.Noop())
	node := &model.NetworkNode{
		ID:                   "node-1",
		StorageCapacityBytes: capacity,
	}
	if err := state.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	return state
}
