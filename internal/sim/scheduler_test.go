package sim

import (
	"reflect"
	"testing"

	"github.com/zenith/zenith/internal/runtime"
)

// pingCore is a minimal toy Core used only to exercise the scheduler's
// determinism guarantees. It is not part of the Raft implementation: on a
// Tick it fans out to two peers at the same virtual delay, which forces
// the scheduler to break a same-due-time tie.
type pingCore struct {
	received []string
}

func (c *pingCore) Step(ev runtime.Event) []runtime.Effect {
	switch e := ev.(type) {
	case runtime.Tick:
		return []runtime.Effect{
			runtime.Send{To: 2, Payload: []byte("from-1-a")},
			runtime.Send{To: 3, Payload: []byte("from-1-b")},
		}
	case runtime.PeerMessage:
		c.received = append(c.received, string(e.Payload))
	}
	return nil
}

// sendToDelivery is the EffectHandler for the toy scenario: it turns every
// runtime.Send effect into an immediate (delay 0) PeerMessage delivery.
func sendToDelivery(from runtime.NodeID, effects []runtime.Effect, s *Scheduler) {
	for _, eff := range effects {
		if send, ok := eff.(runtime.Send); ok {
			s.Schedule(0, send.To, runtime.PeerMessage{From: from, Payload: send.Payload})
		}
	}
}

func runToyScenario(seed uint64) []Delivery {
	cores := map[runtime.NodeID]runtime.Core{
		1: &pingCore{},
		2: &pingCore{},
		3: &pingCore{},
	}
	s := NewScheduler(seed, cores, sendToDelivery)
	s.Schedule(0, 1, runtime.Tick{Kind: "start"})
	return s.Run(100)
}

// TestSchedulerDeterministicReplay is the Phase 0 completion gate from
// UPDATE.md Section 18: "Identical build/seed gives identical trace."
func TestSchedulerDeterministicReplay(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		first := runToyScenario(seed)
		second := runToyScenario(seed)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("seed %d: replay produced a different trace:\n%#v\nvs\n%#v", seed, first, second)
		}
	}
}

// TestSchedulerExploresAlternativeOrders shows the tiebreak stream is not
// a fixed insertion-order tiebreak in disguise: across seeds, the two
// same-due-time deliveries to nodes 2 and 3 are ordered both ways.
func TestSchedulerExploresAlternativeOrders(t *testing.T) {
	orders := map[string]bool{}
	for seed := uint64(0); seed < 30; seed++ {
		trace := runToyScenario(seed)
		var order string
		for _, d := range trace {
			if d.Target == 2 || d.Target == 3 {
				order += string(rune('0' + d.Target))
			}
		}
		orders[order] = true
	}
	if len(orders) < 2 {
		t.Fatalf("expected the tiebreak stream to produce more than one delivery order across seeds, got %v", orders)
	}
}

// TestSchedulerDeliversBothMessages is a basic sanity check that the toy
// scenario's effect handler and Core wiring actually work end to end.
func TestSchedulerDeliversBothMessages(t *testing.T) {
	cores := map[runtime.NodeID]runtime.Core{
		1: &pingCore{},
		2: &pingCore{},
		3: &pingCore{},
	}
	s := NewScheduler(42, cores, sendToDelivery)
	s.Schedule(0, 1, runtime.Tick{Kind: "start"})
	s.Run(100)

	c2 := cores[2].(*pingCore)
	c3 := cores[3].(*pingCore)
	if len(c2.received) != 1 || c2.received[0] != "from-1-a" {
		t.Fatalf("node 2 did not receive the expected message: %v", c2.received)
	}
	if len(c3.received) != 1 || c3.received[0] != "from-1-b" {
		t.Fatalf("node 3 did not receive the expected message: %v", c3.received)
	}
}
