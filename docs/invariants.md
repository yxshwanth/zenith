# Invariants

This is the living invariant inventory. Every invariant has an ID, a precise
scope, an observation point, a counterexample format, and (before it is
trusted) a checker validated against an intentionally broken fixture.

**Implementation status is honest, not aspirational.**

| ID | Contract | How to observe it | Implementation status |
|---|---|---|---|
| R1 | At most one elected leader per term | Record election evidence | Covered: `TestR1AtMostOneLeaderPerTerm` |
| R2 | Durable vote does not change to another candidate in the same term | Persist/Sync before grant | Covered: `TestR2VotePersistsBeforeGrantVisible`; `TestVoteGrantWithheldAcrossFailedSyncStorm` |
| R3 | Same log index and term imply matching prefix | Prefix ledger | Covered: `TestLogMatchingLeaderChangeMidAppend`; ledger keys `(index,term)` |
| R4 | Every applied command matches the unique committed prefix | Apply path | Covered: `afterSync` append guarantees no lost applies; Porcupine hunt (2000 seeds) validates linearizability under faults |
| R5 | No apply from an uncommitted entry | commit/apply transitions | Covered: `lastApplied` tracks `commitIndex` |
| D1 | Successful mutation survives crash/recovery | Crash matrix | Covered: Phase 1–2 tests |
| D2 | Duplicate request identity does not duplicate effects | Session table | Covered: `TestSessionRetryNoDup`; `TestSessionUnknownRetryThenDigestMismatch` |
| D3 | Snapshot activation preserves coherent prefix | Snapshot publish/install | Covered: older snap rejected; `TestCrashDuringInstallSnapshotKeepsPrefix`; crash prefers newer snap over older WAL |
| M1 | Snapshot reads return model version at `r` | Version oracle | Covered: `TestMVCCMatchesVersionOracle` |
| M2 | One logical query uses one revision | authz `revSeen` | Covered |
| M3 | GC does not alter pinned snapshots | GC refuse | Covered: `TestGCRefusesPinned` |
| A1 | Check equals reference evaluator | Graph oracle | Covered: `TestAuthzMatchesGraphOracle` |
| A2 | Missing evidence cannot produce ALLOW | Budget Incomplete | Covered: budget + closed snap |
| A3 | Content version uses sufficiently fresh authz | New-enemy | Covered: `TestNewEnemyStaleReplicaBlocked`; `TestNewEnemyExploreSeeds` |
| A4 | Batch results share one snapshot | BatchCheck | Covered: `TestBatchCheckSameRevision` |
| C1 | Cache hits satisfy selected snapshot | Cache | Covered: `TestDecisionCacheRespectsRevision` |
| G1 | Quorum follows effective configuration | Joint config | Covered: learner non-quorum; `TestJointLiveCrashDualQuorum`; dual-quorum block |
| L1 | Progress after healthy suffix | Partition heal | Covered: partition heal + dual-quorum block |

## Checker layers

1. Internal assertions near Raft persist/sync.
2. `internal/checker`: KV, linearizability, Porcupine, VisibleAt, GraphAllows, NewEnemyOK, PrefixLedger.
3. Porcupine wired for KV (`CheckKVPorcupine`).
4. Application protocol oracle: `NewEnemyOK` + content-service example.

## Phases 0–6

Gate-minimal completion under `make test-v2` / `make sim-smoke`.
