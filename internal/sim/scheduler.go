package sim

import (
	"container/heap"

	"github.com/yxshwanth/zenith/internal/runtime"
)

// VirtualTime is the simulation's own clock: a count of scheduler ticks,
// not wall-clock time. A Core must never observe wall-clock time, so this
// is the only notion of "when" available inside a run.
type VirtualTime int64

// scheduledEvent is one entry in the scheduler's queue. tiebreak is drawn
// from the scheduler's own PRNG stream at schedule time, so events due at
// the same VirtualTime are ordered deterministically for a given seed,
// while a different seed can explore a different, equally valid delivery
// order — the two alternative delivery orders a replay needs to compare.
type scheduledEvent struct {
	due      VirtualTime
	tiebreak uint64
	seq      uint64
	target   runtime.NodeID
	event    runtime.Event
}

type eventHeap []scheduledEvent

func (h eventHeap) Len() int { return len(h) }
func (h eventHeap) Less(i, j int) bool {
	if h[i].due != h[j].due {
		return h[i].due < h[j].due
	}
	if h[i].tiebreak != h[j].tiebreak {
		return h[i].tiebreak < h[j].tiebreak
	}
	return h[i].seq < h[j].seq
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)   { *h = append(*h, x.(scheduledEvent)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Delivery is one entry of the trace a Scheduler run produces: the exact
// order in which events were delivered to which node. Two runs with the
// same seed and the same schedule of Schedule calls must produce
// byte-identical traces.
type Delivery struct {
	Due    VirtualTime
	Target runtime.NodeID
	Event  runtime.Event
}

// EffectHandler turns the Effects a Core returned into further scheduler
// activity — e.g. turning a runtime.Send into a future PeerMessage
// delivery on another node, or a runtime.Schedule into a future
// ScheduledWork. It is supplied by the caller so the same Scheduler can
// drive different adapter wiring: a toy core in a Phase 0 test today, real
// replicas once internal/raft exists.
type EffectHandler func(from runtime.NodeID, effects []runtime.Effect, sched *Scheduler)

// Scheduler is a single-threaded, deterministic event loop. It owns no
// goroutines and performs no I/O itself; every side effect flows through
// the supplied EffectHandler.
type Scheduler struct {
	now     VirtualTime
	queue   eventHeap
	seq     uint64
	streams *Streams
	trace   []Delivery
	handler EffectHandler
	cores   map[runtime.NodeID]runtime.Core
}

// NewScheduler builds a scheduler seeded from masterSeed. Given the same
// seed, the same sequence of Schedule calls, and the same Core
// implementations, two Schedulers produce identical traces.
func NewScheduler(masterSeed uint64, cores map[runtime.NodeID]runtime.Core, handler EffectHandler) *Scheduler {
	return &Scheduler{
		streams: NewStreams(masterSeed),
		handler: handler,
		cores:   cores,
	}
}

func (s *Scheduler) NetworkIntn(n int) int  { return s.streams.Network.Intn(n) }
func (s *Scheduler) DiskIntn(n int) int     { return s.streams.Disk.Intn(n) }
func (s *Scheduler) ElectionIntn(n int) int { return s.streams.Election.Intn(n) }
func (s *Scheduler) WorkloadIntn(n int) int { return s.streams.Workload.Intn(n) }

// Now returns the scheduler's current virtual time.
func (s *Scheduler) Now() VirtualTime { return s.now }

// Schedule enqueues event for delivery to target at s.Now()+delay.
func (s *Scheduler) Schedule(delay VirtualTime, target runtime.NodeID, event runtime.Event) {
	if delay < 0 {
		panic("sim: negative delay")
	}
	s.seq++
	heap.Push(&s.queue, scheduledEvent{
		due:      s.now + delay,
		tiebreak: s.streams.Scheduler.next(),
		seq:      s.seq,
		target:   target,
		event:    event,
	})
}

// Run drains the queue, delivering each event to its target Core in
// deterministic order and routing the resulting Effects through the
// EffectHandler, until the queue is empty or maxSteps events have been
// delivered (a safety valve against runaway scheduling; 0 means
// unbounded). It returns the full delivery trace.
func (s *Scheduler) Run(maxSteps int) []Delivery {
	steps := 0
	for s.queue.Len() > 0 {
		if maxSteps > 0 && steps >= maxSteps {
			break
		}
		item := heap.Pop(&s.queue).(scheduledEvent)
		s.now = item.due
		s.trace = append(s.trace, Delivery{Due: item.due, Target: item.target, Event: item.event})

		core, ok := s.cores[item.target]
		if !ok {
			continue
		}
		effects := core.Step(item.event)
		if s.handler != nil && len(effects) > 0 {
			s.handler(item.target, effects, s)
		}
		steps++
	}
	return s.trace
}

// Trace returns the delivery order produced so far.
func (s *Scheduler) Trace() []Delivery { return s.trace }

// Register adds or replaces a core for target (membership / learners).
func (s *Scheduler) Register(target runtime.NodeID, core runtime.Core) {
	s.cores[target] = core
}
