package runtime

// Effect is a side effect a Core has requested. The adapter is responsible
// for carrying it out and feeding any resulting completion back in as an
// Event; returning an Effect is not evidence that it happened
// (docs/adr/0005-deterministic-core-contract.md).
type Effect interface {
	effect()
}

// Persist asks the adapter to durably append Data before any dependent
// Effect (typically Send or Reply) may be released. A Persist completion
// alone does not mean the data survives a crash — see Sync.
type Persist struct {
	RequestID string
	Data      []byte
}

// Sync asks the adapter to fsync previously persisted data. Only after the
// matching DiskCompletion reports Synced=true may durability be assumed.
type Sync struct {
	RequestID string
}

// Send asks the adapter to deliver Payload to peer To. Ordering and
// delivery are not guaranteed; the Core must be able to handle drops,
// duplicates, and reordering of its own messages.
type Send struct {
	To      NodeID
	Payload []byte
}

// Schedule asks the adapter to fire a ScheduledWork event after Delay,
// expressed in the runtime's own logical units (ticks or virtual
// duration) — never as a wall-clock deadline.
// Reserved; raft.Node never returns this effect.
type Schedule struct {
	RequestID string
	Delay     int64
}

// Apply asks the adapter to apply a committed command to the state machine
// at the given log Index/Term and report the deterministic result.
type Apply struct {
	Index   uint64
	Term    uint64
	Command []byte
}

// Reply asks the adapter to deliver a response to a previously received
// ClientCommand identified by RequestID.
type Reply struct {
	RequestID string
	Payload   []byte
	Err       error
}

func (Persist) effect()  {}
func (Sync) effect()     {}
func (Send) effect()     {}
func (Schedule) effect() {}
func (Apply) effect()    {}
func (Reply) effect()    {}
