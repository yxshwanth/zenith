# Invariants

This is the living inventory referenced by `UPDATE.md`, Section 14. Every
invariant has an ID, a precise scope, an observation point, a counterexample
format, and — before it is trusted — a checker validated against an
intentionally broken fixture that proves the checker can detect a
violation.

**Implementation status is honest, not aspirational.** An invariant listed
here as "Planned" has no checker yet; do not cite this document as evidence
that the corresponding property has been tested. See `docs/bugs/` for
actual discovered violations, once any exist.

| ID | Contract | How to observe it | Implementation status |
|---|---|---|---|
| R1 | At most one elected leader per term | Record election evidence, not just nodes' current role labels | Planned — needs `internal/raft` |
| R2 | Durable vote does not change to another candidate in the same term | Inspect persistence transitions and restart state | Planned — needs `internal/raft`, `internal/wal` |
| R3 | Same log index and term imply matching prefix | Compare retained history with a simulator-side committed-prefix ledger | Planned — needs `internal/raft`, `internal/checker` prefix ledger |
| R4 | Every applied command matches the unique committed prefix | Hash canonical command contents at each applied index | Planned — needs `internal/replica` apply path |
| R5 | No apply from an uncommitted entry | Observe commit/apply transitions | Planned — needs `internal/raft`, `internal/replica` |
| D1 | Successful mutation survives supported crash/recovery schedules | Resolve requests after recovery and compare expected state | Planned — needs `internal/wal` recovery, `internal/sim` disk faults |
| D2 | Duplicate request identity does not duplicate effects | Compare session state and independent model outcomes | Planned — needs `internal/session` |
| D3 | Snapshot activation preserves a coherent prefix and configuration | Validate before/after install and restart | Planned — needs `internal/snapshot` |
| M1 | Snapshot reads return the model's version at `r` | Compare `Contains`/`Scan` against independent version histories | Planned — needs `internal/mvcc`, `internal/checker` version oracle |
| M2 | One logical query uses one revision | Instrument every snapshot access; reject revision drift | Planned — needs `internal/mvcc`, `internal/authz` |
| M3 | GC does not alter any admitted pinned snapshot | Read before/after GC and checkpoint transitions | Planned — needs `internal/mvcc` retention |
| A1 | Check equals a reference evaluator at its revision | Generated small graphs and independent traversal | Planned — needs `internal/authz`, `internal/checker` graph oracle |
| A2 | Missing required evidence cannot produce ALLOW | Inject errors into every expression branch and output stage | Planned — needs `internal/authz` |
| A3 | Content version uses a sufficiently fresh authorization snapshot | Observe revocation, fence, publication, and served bytes | Planned — needs `examples/content-service` |
| A4 | Batch results share one snapshot | Compare all reported and accessed revisions | Planned — needs `internal/authz` BatchCheck |
| C1 | Cache hits satisfy the exact selected snapshot | Record key, cached revision, selected revision, and result | Planned — decision caching is deferred (Section 11); no cache to check yet |
| G1 | Every quorum follows the effective configuration | Inspect elections, commitment, and transition histories | Planned — needs `internal/raft` membership |
| L1 | Progress resumes after a modeled healthy suffix | Heal faults, bound delays, drain work, issue fresh requests | Planned — needs `internal/sim` fault injection + full replica stack |

## Checker layers (Section 14)

1. **Internal assertions** — inline, close to the transition they guard. None exist yet; land alongside `internal/raft`.
2. **Independent state-machine oracle** — `internal/checker`. A tiny sequential KV model (`KVModel`) and a brute-force linearizability checker for small histories (`LinearizableKV`) exist as of Phase 0. They must never call production MVCC/cache/evaluator code — and don't; `internal/checker` has no dependency on any other `internal/*` package.
3. **Porcupine** — not yet wired in. `internal/checker.LinearizableKV` is a Phase-0-scale placeholder (brute-force over small histories); it is not a Porcupine replacement and should not be treated as one once Porcupine integration lands.
4. **Application protocol oracle** — not started; depends on `examples/content-service`.

## Phase 0 checker-validation status

`internal/checker/linearizability_test.go` includes both a legal history
(accepted) and an illegal history whose recorded result is impossible under
any operation order (rejected) — the "intentionally broken fixture" this
document requires before a checker is trusted. `internal/sim/scheduler_test.go`
plays the equivalent role for the scheduler's determinism claim: it asserts
same-seed replay is byte-identical and that different seeds can produce
different, individually-reproducible delivery orders.

Neither test exercises Raft, WAL, MVCC, or the evaluator — those checkers
do not exist yet. Do not read this file as claiming R1–L1 are covered.
