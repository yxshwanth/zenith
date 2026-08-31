package runtime

// Event is anything that can drive a Core forward. The set of concrete
// event types is deliberately small: every external occurrence a replica
// reacts to reduces to one of these (UPDATE.md, Section 4).
type Event interface {
	event()
}

// PeerMessage is a message received from another node in the cluster. Peer
// messages may be delayed, dropped, duplicated, or reordered by the
// transport; a Core must not assume any ordering or delivery guarantee
// beyond what the consensus protocol itself establishes.
type PeerMessage struct {
	From    NodeID
	Payload []byte
}

// ClientCommand is a client-submitted request, addressed to this replica.
type ClientCommand struct {
	RequestID string
	Payload   []byte
}

// Tick is a logical timer firing (election timeout, heartbeat, and so on),
// supplied by the runtime rather than read from a wall clock. Kind
// distinguishes timer purposes a Core cares about.
type Tick struct {
	Kind string
}

// DiskCompletion reports that a previously requested Persist or Sync
// Effect has finished. Synced is only meaningful for a Sync completion;
// a Core must not treat a Persist completion alone as durable.
type DiskCompletion struct {
	RequestID string
	Synced    bool
	Err       error
}

// SnapshotChunk delivers one piece of an in-progress snapshot transfer.
type SnapshotChunk struct {
	From     NodeID
	Snapshot string
	Offset   int64
	Data     []byte
	Last     bool
}

// Cancellation reports that a previously scheduled request is no longer of
// interest to the caller — for example, a client gave up, or the local
// node incarnation changed and the pending work is now stale.
type Cancellation struct {
	RequestID string
}

// ScheduledWork fires a previously requested Schedule Effect's callback.
type ScheduledWork struct {
	RequestID string
}

func (PeerMessage) event()    {}
func (ClientCommand) event()  {}
func (Tick) event()           {}
func (DiskCompletion) event() {}
func (SnapshotChunk) event()  {}
func (Cancellation) event()   {}
func (ScheduledWork) event()  {}
