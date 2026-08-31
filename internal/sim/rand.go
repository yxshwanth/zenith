// Package sim implements the deterministic simulation harness (UPDATE.md,
// Section 13): a single-threaded event scheduler that drives real
// runtime.Core implementations through virtual time, plus independently
// seeded PRNG streams so that one master seed reproduces an identical run
// byte-for-byte.
package sim

import "math/bits"

// stream is a deterministic PRNG independently seeded from a master seed
// and a stable name (SplitMix64). Streams derived from the same master
// seed never share state, so adding random draws to one subsystem does not
// perturb another subsystem's sequence (UPDATE.md, Section 13: "Separating
// streams reduces accidental scenario drift").
type stream struct {
	state uint64
}

func newStream(masterSeed uint64, name string) *stream {
	return &stream{state: fnv1aSeed(masterSeed, name)}
}

// next returns the next pseudo-random value from this stream.
func (s *stream) next() uint64 {
	s.state += 0x9E3779B97F4A7C15
	z := s.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Intn returns a deterministic value in [0, n).
func (s *stream) Intn(n int) int {
	if n <= 0 {
		panic("sim: Intn called with n <= 0")
	}
	return int(s.next() % uint64(n))
}

func fnv1aSeed(seed uint64, name string) uint64 {
	h := uint64(14695981039346656037) ^ seed
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= 1099511628211
	}
	if h == 0 {
		// A zero SplitMix64 state stays degenerate for its first output;
		// fold in the seed instead of ever handing that out.
		h = bits.RotateLeft64(seed|1, 17)
	}
	return h
}

// Streams holds every independently seeded PRNG stream a simulation run
// uses. Naming and separating them up front means a subsystem added later
// (e.g. disk fault injection) cannot silently shift the draw sequence
// another subsystem depends on for replay.
type Streams struct {
	Workload  *stream
	Network   *stream
	Disk      *stream
	Election  *stream
	Scheduler *stream
}

// NewStreams derives all named streams from one master seed.
func NewStreams(masterSeed uint64) *Streams {
	return &Streams{
		Workload:  newStream(masterSeed, "workload"),
		Network:   newStream(masterSeed, "network"),
		Disk:      newStream(masterSeed, "disk"),
		Election:  newStream(masterSeed, "election"),
		Scheduler: newStream(masterSeed, "scheduler"),
	}
}
