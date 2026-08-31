// Package runtime defines the deterministic event/effect contract shared by
// the real Zenith node adapters and the simulator (UPDATE.md, Section 4
// "Deterministic core contract" and Section 13). A Core implementation may
// only observe the world through Step; it must not perform I/O, block,
// read the wall clock, or use ambient randomness.
package runtime

// NodeID identifies a single replica within a cluster.
type NodeID uint64

// Core is the deterministic state-transition contract for a replica, its
// WAL, its evaluator scheduling, or any other stateful component that must
// run identically under simulation and under real adapters. A Core
// implementation consumes one Event at a time and returns the Effects that
// must be carried out on its behalf. Effects are requests, not
// confirmations: the adapter — real or simulated — is responsible for
// carrying them out and feeding any resulting completion back in as an
// Event.
type Core interface {
	Step(Event) []Effect
}
