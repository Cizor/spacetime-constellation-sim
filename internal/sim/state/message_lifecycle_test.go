package state

import (
	"testing"
	"time"

	network "github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestMessageStateTransitions(t *testing.T) {
	start := time.Unix(1_000_000, 0)
	scheduler := sbi.NewFakeEventScheduler(start)
	state := newLifecycleState(t, scheduler)

	msg := StoredMessage{
		MessageID:        "msg-life",
		ServiceRequestID: "sr-life",
		SizeBytes:        10,
		Destination:      "node-dst",
	}

	if err := state.StoreMessage("node-src", msg); err != nil {
		t.Fatalf("StoreMessage failed: %v", err)
	}
	status, history, ok := state.GetMessageState(msg.MessageID)
	if !ok || status != MessageStateStored {
		t.Fatalf("expected stored state after StoreMessage, got %v (ok=%v)", status, ok)
	}
	if len(history) == 0 || history[len(history)-1].State != MessageStateStored {
		t.Fatalf("history missing state stored entry: %v", history)
	}

	retrieved, err := state.RetrieveMessage("node-src", msg.MessageID)
	if err != nil {
		t.Fatalf("RetrieveMessage failed: %v", err)
	}
	status, _, _ = state.GetMessageState(msg.MessageID)
	if status != MessageStateInTransit {
		t.Fatalf("expected in_transit state after retrieve, got %s", status)
	}

	retrieved.Destination = "node-dst"
	if err := state.StoreMessage("node-dst", *retrieved); err != nil {
		t.Fatalf("StoreMessage at dest failed: %v", err)
	}
	status, _, _ = state.GetMessageState(msg.MessageID)
	if status != MessageStateDelivered {
		t.Fatalf("expected delivered state after storing at destination, got %s", status)
	}

	delivered := state.MessagesInState(MessageStateDelivered)
	if len(delivered) != 1 || delivered[0].MessageID != msg.MessageID {
		t.Fatalf("MessagesInState missing delivered message: %v", delivered)
	}
}

func TestMessageExpiryScheduling(t *testing.T) {
	start := time.Unix(2_000_000, 0)
	scheduler := sbi.NewFakeEventScheduler(start)
	state := newLifecycleState(t, scheduler)

	node := &model.NetworkNode{
		ID:                   "node-exp",
		StorageCapacityBytes: 128,
	}
	if err := state.CreateNode(node, nil); err != nil {
		t.Fatalf("CreateNode node-exp failed: %v", err)
	}

	expiry := start.Add(2 * time.Minute)
	msg := StoredMessage{
		MessageID:        "msg-exp",
		ServiceRequestID: "sr-exp",
		SizeBytes:        5,
		ExpiryTime:       expiry,
		Destination:      "node-dst",
	}
	if err := state.StoreMessage("node-exp", msg); err != nil {
		t.Fatalf("StoreMessage failed: %v", err)
	}
	status, _, _ := state.GetMessageState(msg.MessageID)
	if status != MessageStateStored {
		t.Fatalf("expected stored, got %s", status)
	}

	scheduler.AdvanceTo(start.Add(time.Minute))
	status, _, _ = state.GetMessageState(msg.MessageID)
	if status != MessageStateStored {
		t.Fatalf("state changed prematurely: %s", status)
	}

	scheduler.AdvanceTo(expiry)
	status, _, _ = state.GetMessageState(msg.MessageID)
	if status != MessageStateExpired {
		t.Fatalf("expected expired after scheduler run, got %s", status)
	}
}

func newLifecycleState(t *testing.T, scheduler sbi.EventScheduler) *ScenarioState {
	t.Helper()
	physKB := kb.NewKnowledgeBase()
	netKB := network.NewKnowledgeBase()
	state := NewScenarioState(physKB, netKB, logging.Noop(), WithEventScheduler(scheduler))

	nodeSrc := &model.NetworkNode{
		ID:                   "node-src",
		StorageCapacityBytes: 128,
	}
	nodeDst := &model.NetworkNode{
		ID:                   "node-dst",
		StorageCapacityBytes: 128,
	}
	for _, node := range []*model.NetworkNode{nodeSrc, nodeDst} {
		if err := state.CreateNode(node, nil); err != nil {
			t.Fatalf("CreateNode %s failed: %v", node.ID, err)
		}
	}
	return state
}
