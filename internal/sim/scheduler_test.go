package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
)

// hashTrace is the Phase 0 gate fingerprint: same seed → same hex digest.
func hashTrace(trace []Delivery) string {
	h := sha256.New()
	for _, d := range trace {
		fmt.Fprintf(h, "%d|%d|%T|", d.Due, d.Target, d.Event)
		switch e := d.Event.(type) {
		case runtime.Tick:
			fmt.Fprintf(h, "%s\n", e.Kind)
		case runtime.PeerMessage:
			fmt.Fprintf(h, "%d|%s\n", e.From, e.Payload)
		default:
			fmt.Fprintf(h, "%v\n", e)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

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

// TestSchedulerDeterministicReplay: identical build/seed gives identical trace.
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

// TestTwoSeedsDifferentOrdersEachReplayable is Phase 0 task 0.7:
// same Schedule set, two seeds → different valid orders; each seed stable.
func TestTwoSeedsDifferentOrdersEachReplayable(t *testing.T) {
	var seedA, seedB uint64
	var orderA, orderB string
	found := false
	for a := uint64(0); a < 40 && !found; a++ {
		for b := a + 1; b < 40; b++ {
			oa := peerOrder(runToyScenario(a))
			ob := peerOrder(runToyScenario(b))
			if oa != ob {
				seedA, seedB, orderA, orderB = a, b, oa, ob
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("could not find two seeds with different peer delivery orders")
	}
	if peerOrder(runToyScenario(seedA)) != orderA {
		t.Fatalf("seed %d not replayable", seedA)
	}
	if peerOrder(runToyScenario(seedB)) != orderB {
		t.Fatalf("seed %d not replayable", seedB)
	}
}

func peerOrder(trace []Delivery) string {
	var order string
	for _, d := range trace {
		if d.Target == 2 || d.Target == 3 {
			order += string(rune('0' + d.Target))
		}
	}
	return order
}

// TestSmokeTraceHashIdentical is the Phase 0 completion gate (task 0.9):
// fixed seed → identical SHA-256 of the delivery trace across two runs.
//
//	go test ./internal/sim -run TestSmokeTraceHashIdentical -count=2
func TestSmokeTraceHashIdentical(t *testing.T) {
	const seed = 42
	h1 := hashTrace(runToyScenario(seed))
	h2 := hashTrace(runToyScenario(seed))
	if h1 != h2 {
		t.Fatalf("identical seed produced different hashes:\n%s\n%s", h1, h2)
	}
	t.Logf("smoke trace hash (seed=%d): %s", seed, h1)
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
