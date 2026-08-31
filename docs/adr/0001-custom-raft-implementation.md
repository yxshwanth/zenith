# ADR 0001: Implement Raft ourselves rather than importing a library

Status: Accepted (UPDATE.md, Section 3).

## Context

Zenith v2 replaces the CockroachDB-backed storage path with a replicated
authorization database. Mature Raft implementations exist (etcd-io/raft,
hashicorp/raft) and would reach a working cluster faster than writing
consensus from scratch.

## Decision

Implement the Raft consensus core in this repository (`internal/raft`),
using the extended Raft paper and etcd-io/raft as references, not
dependencies (see `UPDATE.md` References T1, T2, T3).

## Consequences

- Consensus code is the subject of the project, not a wrapped dependency;
  correctness evidence must come from Zenith's own implementation and
  simulator, not from an upstream library's test suite.
- We own election, log matching, snapshot, and joint-membership edge cases
  ourselves, including the ones that are easy to get wrong (pre-vote,
  configuration overwrite rules, snapshot/log-suffix reconciliation).
- We must not silently blend a library's apply-time membership scheme with
  the paper's activation rules — one algorithm, specified in ADR 0005
  before it is enabled.
- Slower to reach a working cluster than adopting etcd-io/raft directly;
  accepted because the project's engineering objective is the consensus
  and correctness-testing work itself (`UPDATE.md`, Section 1).
