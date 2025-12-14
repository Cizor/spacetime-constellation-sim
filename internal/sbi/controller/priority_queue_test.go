package controller

import (
	"sync"
	"testing"

	"github.com/signalsfoundry/constellation-simulator/model"
)

func TestPriorityQueueOrdersByPriority(t *testing.T) {
	pq := newPriorityQueue()
	pq.Push(&model.ServiceRequest{ID: "low", Priority: 1})
	pq.Push(&model.ServiceRequest{ID: "high", Priority: 10})
	pq.Push(&model.ServiceRequest{ID: "mid", Priority: 5})

	first := pq.Pop()
	if first == nil || first.ID != "high" {
		t.Fatalf("expected high priority first, got %v", first)
	}
	second := pq.Pop()
	if second == nil || second.ID != "mid" {
		t.Fatalf("expected mid priority second, got %v", second)
	}
	third := pq.Pop()
	if third == nil || third.ID != "low" {
		t.Fatalf("expected low priority third, got %v", third)
	}

	if pq.Len() != 0 {
		t.Fatalf("expected queue empty, len=%d", pq.Len())
	}
}

func TestPriorityQueueThreadSafety(t *testing.T) {
	pq := newPriorityQueue()
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(priority int) {
			defer wg.Done()
			pq.Push(&model.ServiceRequest{ID: string(rune(priority)), Priority: int32(priority)})
		}(i)
	}
	wg.Wait()

	if pq.Len() != 100 {
		t.Fatalf("expected 100 entries, got %d", pq.Len())
	}

	seen := map[string]bool{}
	for pq.Len() > 0 {
		sr := pq.Pop()
		if sr == nil {
			t.Fatal("unexpected nil ServiceRequest")
		}
		if seen[sr.ID] {
			t.Fatalf("duplicate ID encountered: %s", sr.ID)
		}
		seen[sr.ID] = true
	}
	if len(seen) != 100 {
		t.Fatalf("expected to see 100 unique entries, saw %d", len(seen))
	}
}
