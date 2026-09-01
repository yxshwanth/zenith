# ADR 0005: Deterministic Event/Effect core, shared by real and simulated adapters

Status: Accepted (UPDATE.md, Section 4 and Section 13); implemented in
`internal/runtime` and `internal/sim` (Phase 0).

## Context

The correctness plan requires the same consensus, WAL, MVCC, and evaluator
code to run under a deterministic simulator and under real process
adapters (`UPDATE.md`, Section 13: "The simulator must execute the actual
consensus and storage implementation"). That is only possible if the core
logic never performs I/O, blocks, reads the wall clock, or relies on
ambient randomness or Go map iteration order directly.

## Decision

Every stateful core (replica, WAL, evaluator scheduling) is written against
a `Core` interface:

```go
type Core interface {
    Step(Event) []Effect
}
```

`Event` covers everything a Core reacts to (peer message, client command,
tick, disk completion, snapshot chunk, cancellation, scheduled work).
`Effect` covers everything a Core asks the adapter to do (persist, sync,
send, schedule, apply, reply). Asynchronous effects carry a stable request
ID; the adapter — real or simulated — is responsible for feeding the
matching completion back in as an Event.

The simulator (`internal/sim`) drives `Core` implementations through a
single-threaded, seeded event scheduler. Independently named PRNG streams
are derived from one master seed so that adding draws to one subsystem
cannot perturb another subsystem's sequence (`UPDATE.md`, Section 13,
"Scheduler").

**Covered:** the scheduler stream is drawn at every `Schedule` for
same-due-time tie-breaks. **Partial:** named streams `Workload`, `Network`,
`Disk`, and `Election` exist and are consumed by `internal/replica` when
`Faults` rates are non-zero (`NewClusterWithFaults`, profile `explore`).
Zero-value `Faults` still draws those streams (drop/fail rolls miss, delay
stays 1) so turning rates on later does not reshuffle another stream.
Scripted profiles (`elect-put`, `crash-matrix`) keep rates at zero; their
`--seed` only changes tie-breaks, not the lockstep net/disk/election
timeouts. That is the replay half of the machine, not a randomized explorer.

## Consequences

- A "Ready" batch of effects is not permission to perform them in
  arbitrary order; effect ordering and completion-feedback rules are part
  of each Core's contract, not the adapter's discretion.
- The real adapter may use goroutines and real I/O, but completion order
  must still enter the Core through Events — no side channel.
- Given the same seed, the same sequence of `Scheduler.Schedule` calls, and
  the same Core implementations, two runs must produce a byte-identical
  delivery trace. This is the Phase 0 completion gate
  (`UPDATE.md`, Section 18: "Identical build/seed gives identical trace"),
  and is covered by `internal/sim/scheduler_test.go`.
- This buys replay and shrinking for free later (Section 16) at the cost
  of upfront discipline: any `time.Now`, `time.Sleep`, unguarded
  goroutine, or direct socket/filesystem call inside a Core is a
  correctness bug in the harness, not just a style issue.
