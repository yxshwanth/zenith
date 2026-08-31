# ADR 0003: Committed Raft log index is the only timestamp

Status: Accepted (UPDATE.md, Section 3).

## Context

The v1 baseline threads a Cockroach decimal HLC-style timestamp through
`ParseZookieString`, discarding its fractional component, and derives
"current" timestamps independently in `CheckDirect` and
`QueryWithZookie` before taking their maximum. That is not a coherent
snapshot protocol (`UPDATE.md`, Section 2).

## Decision

Use the committed Raft log index, scoped by cluster epoch and group, as
the only notion of a data revision. It is a logical revision, not
wall-clock time. No HLC or TrueTime-style clock is implemented.

## Consequences

- A revision is comparable only within the same epoch and group
  (`UPDATE.md`, Section 9); cross-cluster or cross-epoch comparison is
  meaningless and must be rejected, not coerced.
- Clock skew can affect liveness (election timing, deadlines) but cannot
  change what a committed revision means — this removes an entire class
  of clock-skew correctness bugs at the cost of not being able to bound
  staleness in wall-clock seconds from a revision distance alone.
- Administrative commands and read fences consume log indices too, so
  revisions may have gaps between tuple-visible changes; code must not
  assume consecutive revisions correspond to consecutive tuple changes.
- Historical Cockroach zookies (v1) cannot be translated into Raft indices
  by casting; migration requires issuing a new token scope (Section 17),
  not reinterpreting old integers (see ADR 0004).
