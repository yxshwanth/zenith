# Why Zenith exists, how it got here, and what it can actually prove

This is the project’s argument, not its API reference. Commands live in the [root README](../README.md). Package map: [ARCHITECTURE.md](ARCHITECTURE.md). Numbered contracts: [invariants.md](invariants.md). One-decision records: [adr/](adr/). The original design brief (`docs/UPDATE.md`, `docs/PHASES.md`) is local and gitignored; this file is the public version of that argument.

**Honesty rule used throughout:** “Done” means a gate ran and passed. Planned, partial, and out of scope stay labeled as such. This document does not claim formal verification, absence of bugs, or production security certification.

---

## 1. What the project is

Zenith v2 is a **small replicated authorization database** in Go. It stores relation tuples (object, relation, subject: the Zanzibar/ReBAC vocabulary), replicates them with a **custom Raft** implementation, versions them with **in-memory MVCC** rebuilt from a **custom WAL and snapshots**, and answers `Check` / `ListSubjects` / `WriteTuples` at **one explicit committed revision**. A tiny content-store example binds a published version to an opaque token so a **stale replica cannot authorize newer content** (the new-enemy problem).

The same consensus, WAL, MVCC, and evaluator code is supposed to run in two adapters:

- **Real process:** `cmd/zenithd` (`internal/nodehost`): disk, TCP, gRPC, HTTP.
- **Simulator:** `cmd/zenith-sim` (`internal/replica` + `internal/sim`): one thread, one seed, virtual time.

That pairing is the product. The authorization graph is the workload that makes the pairing worth building. A generic replicated KV would have been smaller; a graph evaluator bolted onto Cockroach (v1) already existed and was judged insufficient.

**It is not:** OpenFGA, SpiceDB, CockroachDB, etcd, or Google Zanzibar. It is not a SQL store, a geo-spanner, a Byzantine-fault-tolerant system, or a certified production IdP. Dataset must fit in memory. Cluster is one Raft group. Non-loopback deployment requires a shared auth token and TLS; that is process-boundary hygiene, not a security product.

Module: `github.com/yxshwanth/zenith`. License: MIT.

---

## 2. How we got here

### v1: Cockroach-backed ReBAC

The repository began as a permission engine in front of CockroachDB. Tuples, a recursive expander, zookies-as-`int64`, docker-compose for one CRDB node, and tests that advertised chaos/property coverage. A source review at commit `a48609be` (2026-08-16) did **not** execute the test suite (no Go toolchain in that review environment). It did inspect the wiring. The findings that drove the rewrite:

| v1 behavior | Why it was rejected |
| --- | --- |
| `ParseZookieString` took a Cockroach decimal timestamp and dropped the fractional part | Not a snapshot protocol; silently coarsens time |
| `CheckDirect` used the requested timestamp; `QueryWithZookie` independently sampled “now” and took a max | Mixed revisions inside one logical query |
| Decision cache hits ignored `RequiredZookie`; keys omitted evaluation revision | Freshness contract unenforced on the hot path |
| `checkStale` fallback returned cached results without the requested-zookie check, on a *different* key format | Intended safety net that could not enforce the contract |
| Some timeouts/batch errors became ordinary DENY | Incomplete evaluation indistinguishable from proven absence of a path |
| “Chaos” tests slept in loops; “property” tests checked a literal increasing slice | Advertised distributed coverage that did not exercise nodes or disks |
| Evaluator imported concrete `db.TupleRepo`; goroutine-fanout expansion | No pinned snapshot handle; scheduling was ambient concurrency |

Preserve: tuple vocabulary, useful graph fixtures, gRPC as a *design* reference. Replace: storage, timestamps, cache policy, evaluator scheduling, test harness. Defer: Redis, Kubernetes, UI, distributed singleflight, performance-as-correctness.

v1 APIs, compose files, and `internal/{db,engine,cache,service,config}` were deleted in the v2 cutover. Historical Cockroach zookies **cannot** be cast into Raft indices. Cutover is a new token scope, not a type pun.

### v2: the subject of the repo is consensus + evidence

The engineering objective is not “ship an authz SaaS.” It is:

1. Define the order of persist → replicate → commit → apply → ack, and test it.
2. Separate linearizable writes, exact historical reads, and minimum-freshness reads.
3. Keep permission semantics across recursion, cache, retry, restart, and lag.
4. Check externally observed results with an **independent** oracle (the checker must not call the production evaluator).
5. Reproduce failures from `(build, seed, workload, trace)`.
6. Document when availability or durability is **no longer promised**.

Work was gated in phases (0 contracts → 1 Raft/KV → 2 recovery → 3 MVCC/ReBAC → 4 content protocol → 5 membership → 6 optional index). A later audit (2026-08-31) found rewrite residue: open network surface, swallowed WAL/MVCC errors, Porcupine built but unused, fake-green sim profiles, v1 docs still in tree. Those were closed with shared-secret auth + optional TLS, `MustApply` / WAL panic, Porcupine on the shared history path, real `new-enemy` profile, deleted stubs, and this documentation split.

---

## 3. The bias we had to possess

Projects like this fail by optimizing the wrong axis. The bias below is **intentional**. It is not a personality trait; it is a filter on every “or” in the design.

**Correctness evidence over feature surface.** A Check that returns DENY because evaluation ran out of budget is a typed error, not a permission decision. Shipping intersection/exclusion/TTU before snapshot pinning would have looked like progress and encoded mixed-revision bugs.

**Same code in sim and production, not a second “test Raft.”** A model that only exists in the simulator cannot find bugs in `internal/raft`. The cost is discipline: no `time.Now`, `time.Sleep`, unguarded map iteration, or I/O inside a `Core.Step`. Completions re-enter as Events.

**Inspectability over cleverness.** Strong reads start as “commit a fence, then read that index.” `ReadIndex` is allowed later, under extra checks. Read leases are excluded because they smuggle clock bounds into safety. Joint membership follows the Raft extended paper only, not etcd-io/raft’s apply-time rules mixed in.

**Logical time over wall clocks.** The committed log index, scoped by epoch and group, is the only data timestamp. Clock skew may kill liveness (elections, timeouts). It must not change what revision `r` means. Distance `|r2-r1|` is not a bound in seconds on staleness.

**One ordering domain over scale-out.** One Raft group. Every tuple and policy share comparable revisions. The new-enemy fence is then a local fact, not a cross-shard token. Sharding, multi-group transactions, and geo-global external consistency are explicit non-goals.

**Memory-sized state over a storage engine.** In-memory MVCC + WAL + snapshots. No LSM. Recovery semantics stay in-repo. The dataset must fit RAM. That is a ceiling, not an oversight (`ponytail:`-style: upgrade when the ceiling is hit, not before).

**Fail closed over fail noisy.** Incomplete Check is not DENY-as-success. Unknown mutation (timeout after submit) is not “did not commit.” Wrong-epoch tokens reject before evaluation. Non-loopback without auth+TLS refuses to start. WAL append failure panics rather than acking a lie. MVCC `rev < applied` panics: that is an invariant, not a log line.

**Deletion over unfinished wiring.** An orphaned membership index, a `shrink` command that ignored the trace it claimed to minimize, and a `new-enemy` profile that always printed pass were worse than absence. The audit deleted or wired them; it did not leave green stubs.

**TigerBeetle-shaped simulation, Zanzibar-shaped content, Raft-paper membership.** Those are the three external biases. Porcupine is the fourth: executable sequential spec for KV histories, not a slide-deck “we did linearizability.”

If this bias feels austere, that is the point. The rejected alternatives (import etcd-io/raft, keep Cockroach, add read leases, shard by store, ship a Leopard index first, treat sleep-loop tests as chaos) were faster to a demo and slower to a sentence of the form “this invariant was checked against a counterexample fixture.”

---

## 4. How decisions were weighted

Every major fork was scored roughly as: **(correctness isolation) × (replayability) × (one-person implementability)**, minus **(hidden coupling to clocks, libraries, or a second model)**. Performance, operational familiarity, and “looks like production” were tie-breakers only after the first three were satisfied.

### Decision table (the ones that actually bent the architecture)

| Fork | Chosen | Rejected | Weighting |
| --- | --- | --- | --- |
| Own Raft vs etcd-io/raft / hashicorp/raft | Own `internal/raft` ([ADR 0001](adr/0001-custom-raft-implementation.md)) | Import a library | Consensus *is* the homework. A library’s tests do not evidence *this* apply path, snapshot, or membership. Cost: we own pre-vote, joint config, snapshot/log suffix. |
| One group vs shard-by-store | One group ([ADR 0002](adr/0002-single-raft-group.md)) | Independent groups per namespace | Cross-shard ordering is a second consensus problem. New-enemy needs “this fence sees every prior ACL write.” One log gives that. Cost: no independent write scale. |
| Raft index vs HLC / TrueTime / CRDB timestamp | Logical revision ([ADR 0003](adr/0003-logical-revision-as-clock.md)) | Wall-clock or hybrid clocks | Removes clock-skew *correctness* bugs. Cost: cannot say “this revision is ≤ 5s stale.” Admin/fence entries leave gaps; do not assume consecutive tuple changes. |
| Opaque v2 token vs reuse `int64` zookie | New `zenith.v2` ([ADR 0004](adr/0004-v2-api-and-module-path.md)) | Cast old integers | Casting would launder v1 bugs into v2. Cost: no compatibility shim; clean cutover. |
| Event/Effect core vs “just use goroutines” | `Step(Event) []Effect` ([ADR 0005](adr/0005-deterministic-core-contract.md)) | Threads as the concurrency model | Identical seed ⇒ identical trace (Phase 0 gate). Cost: adapters must bounce completions; any I/O inside Core is a harness bug. |
| Paper joint consensus vs etcd apply-time membership | Paper §6 only ([ADR 0006](adr/0006-joint-membership.md)) | Mix the two | Mixing two membership algorithms is a class of silent split-brain. Cost: dual-quorum while joint; strong reads stay on logged fence across membership. |
| Fence-then-read vs read leases | Fence first; `ReadIndex` later, leases never | Clock-bound leases | Leases make safety depend on time. Fence is slower and inspectable. |
| In-memory MVCC vs embed Rocks/Pebble | Memory + custom WAL | Full LSM | Recovery and visibility stay testable in-process. Cost: RAM ceiling. |
| Independent checker vs “assert on the same Engine” | `internal/checker` has no import of authz/mvcc production paths for oracles | Test the implementation with itself | A checker that calls `Engine.Check` cannot catch `Engine.Check`. Porcupine + `GraphAllows` + `NewEnemyOK` + `KVModel`. |
| Shared secret + TLS files vs mTLS CA | Env token + optional certs | Cluster PKI | Audit required a closed network surface. PKI is a product. Loopback `--dev` keeps `cluster-up` and CI alive. |
| Panic on WAL/MVCC invariant vs log-and-continue | Panic | Swallow `_ = err` | A lied ack is worse than a crash. |

**When two options were the same size,** the one that is correct on edge cases won (stdlib `crypto/hmac` over `SHA256(secret‖payload)`, `gofmt` over editorconfig, delete `shrink` rather than fake delta-debug on type-name strings).

**Phase 6 (Leopard-inspired index)** was weighted *after* core gates: a derived index is a second correctness problem (deletion + alternate paths + watermark). It was built as an orphan, then **deleted** in the audit because nothing imported it. Wiring it without an index-vs-oracle differential would have been a green lie. Re-add only with that checker.

---

## 5. What the system actually does

### Replication and durability

Three voters in the real `make cluster-up` path (script default `ZENITH_NODES=5` for broader quorum). Pre-vote. Persist/sync before a vote is visible (R2). Commit only with a majority of the effective configuration; while joint, majority of **both** old and new (G1). Learners get log/snapshots and do not vote. Removed node IDs are not reused.

WAL frames are checksummed. Recovery discards a torn unsynced suffix. A checksum failure *mid-history* is corruption: stop; do not bootstrap a fresh cluster with the old identity. Snapshots publish atomically (stage then rename). Leader sends `InstallSnapshot` when a follower’s `nextIndex` has been compacted away.

Client ack only after durability, commitment, and apply. Timeout after submit ⇒ **unknown**, retry same session identity and digest (D2). Duplicate identity + different digest is an error.

### Reads and tokens

Tokens are opaque HMAC-SHA256 over epoch, group, store, revision. Wrong scope ⇒ reject before evaluation. Modes ([contracts.md](contracts.md) locally; summarized here):

| Mode | Meaning |
| --- | --- |
| `FullyConsistent` | Strong read: fence (or `ReadIndex` when legal), then that revision. Default if omitted. |
| `AtLeastAsFresh` | Some retained revision ≥ requested lower bound. Not “latest register.” |
| `AtExactRevision` | Exactly `r`, or typed error if compacted / not materialized. |

A replica that is behind a required token revision errors; it does not ALLOW on stale state.

### Authorization

Evaluator receives a snapshot already opened at `r` (`revSeen`). Direct tuples, userset union, then Phase B: intersection, exclusion (stratified), TTU. Budget exhaustion ⇒ Incomplete, never ALLOW (A2). Batches share one snapshot (A4). Decision cache, when on, is keyed by the selected revision (C1).

### New-enemy (A3)

Content bytes live **outside** Zenith (`examples/content-service`). Publish stores bytes with a fence token. Serve only if Check ALLOWs at `evalRev >= token.revision`. Sequence the design cares about: revoke → fence → publish → a Check of the new version must not ALLOW at a revision from before the revoke. `checker.NewEnemyOK` encodes that inequality; `replica.RunNewEnemy` and `zenith-sim --profile new-enemy` run it.

### Process boundary (after the 2026-08-31 audit)

`--dev`: loopback binds only; default token secret; no RPC/peer auth. Otherwise: `ZENITH_TOKEN_SECRET` required; `--auth-token` / `ZENITH_AUTH_TOKEN` on gRPC metadata, HTTP `Authorization` (except public `/status`), and the first JSON frame of the peer TCP connection; `--tls-cert`/`--tls-key` required for any non-loopback listen. This is **not** mTLS-with-a-CA.

`internal/replica` (sim) still uses a fixed sim secret. It is not a network process. `internal/nodehost` is the production node. They share `mvcc.MustApply` so an ordering bug cannot be swallowed on only one path.

---

## 6. Measurements we can produce right now

These are the **actual** observables. Anything not listed is not a current claim.

### Commands (reproducible)

```bash
make test-v2          # unit/integration across runtime, sim, checker, raft, wal,
                      # session, replica, snapshot, mvcc, authz, token,
                      # coordinator, deccache, nodehost, examples/content-service
make sim-smoke        # scheduler determinism (count=2), elect/put + crash + new-enemy,
                      # zenith-sim elect-put seed 42
make sim-sweep        # ZENITH_SWEEP=1000 explore seeds (not default CI)
make sim-corpus       # testdata/regressions replay + zenith-sim replay
make test-race-v2     # -race on raft, replica, wal
make cluster-up       # N real zenithd processes, elect, HTTP put/get (loopback --dev)
go run ./cmd/zenith-sim run --seed 42 --profile elect-put
go run ./cmd/zenith-sim run --seed 44 --profile new-enemy
go run ./cmd/zenith-sim run --seed 42 --profile explore
go run ./cmd/zenith-sim replay --trace testdata/regressions/<file>.json
```

CI (`.github/workflows/v2.yml`): `gofmt -l` empty, `make test-v2`, `make sim-smoke`, `make sim-corpus`, `make test-race-v2`, elect-put, a timed `zenithd --dev` smoke.

Identical build + seed must produce an identical scheduler trace (`TestSchedulerDeterministicReplay`, `TestSmokeTraceHashIdentical`). That is a **measurement**, not a slogan.

### Invariant coverage (from `docs/invariants.md`)

| Can measure today | How | Status |
| --- | --- | --- |
| ≤1 leader per term (R1) | `TestR1AtMostOneLeaderPerTerm` | Covered |
| Vote durability before grant (R2) | `TestVoteGrantWithheldAcrossFailedSyncStorm`; no leader if every fsync fails | Covered |
| Session retry does not double-apply (D2) | `TestSessionUnknownRetryThenDigestMismatch` | Covered |
| New-enemy (A3) | `TestNewEnemyExploreSeeds`; `new-enemy-explore` profile | Covered |
| Joint quorum (G1) | `TestJointLiveCrashDualQuorum`; dual-quorum block | Covered |
| Commit current-term only (Raft §5.4.2) | `TestMaybeCommitRefusesPreviousTermWithoutCurrentTermEntry`; broken hook + Porcupine fixture | Covered |
| Log prefix agreement (R3) | `TestLogMatchingLeaderChangeMidAppend` | Covered |
| Apply only committed prefix (R4, R5) | apply path / `lastApplied` vs `commitIndex` | Partial / covered |
| Acked mutation survives crash (D1) | crash-matrix replica tests | Covered |
| Snapshot install does not regress prefix (D3) | `TestCrashDuringInstallSnapshotKeepsPrefix`; older snap rejected | Covered |
| MVCC visible-at-r matches oracle (M1) | `TestMVCCMatchesVersionOracle` | Covered |
| One query, one revision (M2) | authz `revSeen` | Covered |
| GC refuses to mutate under a pin (M3) | `TestGCRefusesPinned`; replica `TestGCAfterApplyKeepsCheck` | Covered |
| Check = graph oracle (A1) | `TestAuthzMatchesGraphOracle` | Covered |
| Missing evidence ≠ ALLOW (A2) | budget + closed snap | Covered |
| Batch same snapshot (A4) | `TestBatchCheckSameRevision` | Covered |
| Cache respects revision (C1) | `TestDecisionCacheRespectsRevision` | Covered |
| Progress after heal (L1) | partition heal tests | Covered |

**Pass / fail / inconclusive are three outcomes.** A timeout is never recorded as pass. Porcupine classifies completed KV histories (`CheckKVHistory` → `CheckKVPorcupine`). Unknown ops in the history make the result inconclusive unless the resolved subset is already illegal (fail).

### What we do **not** measure yet (do not cite as results)

- Latency/throughput numbers with hardware, durability settings, and error rate (UPDATE.md §20: publish **after** correctness milestones; none are claimed here).
- Legacy Cockroach benchmarks mixed with v2 (forbidden; v1 path is gone anyway).
- Formal proofs (TLA+, Coq).
- Byzantine or malicious-peer behavior (fault model assumes non-malicious peers; auth token is a shared secret, not BFT).
- Wall-clock staleness SLOs derived from revision distance.
- Production mTLS, cert rotation, or multi-tenant isolation.
- Dataset-larger-than-memory behavior.
- `zenith-sim shrink` was removed; traces are type-name dumps, not replayable Event lists. Replay is seed + profile.
- Index hit-rate / Leopard comparison: package deleted until a differential checker exists.
- Bug ledger in `docs/bugs/` is template only; no filed simulator discoveries at the time of writing. `make sim-sweep` (1000 hot explore seeds) is the search, not a proof of absence. Do not invent bugs to look rigorous.

### Operational counters the code could grow into metrics

Quorum availability, leadership changes, WAL sync failures, commit/apply lag, snapshot installs, retained floor, pin count, unknown mutation outcomes, revision-wait errors, evaluator budget failures, incomplete histories. These are the intended dashboard, not a Prometheus surface today.

---

## 7. Fault model (what “tested” means)

Convention matches [invariants.md](invariants.md): **Covered** is what the
code does today. **Building toward** is the design, not a result.

### Covered

| Fault | How |
| --- | --- |
| Same-due reordering | Scheduler PRNG tie-break on every `Schedule` |
| Manual drop | `Partition` (undirected), `DropDir` (asymmetric), `Frozen` |
| Crash / restart | `Crash` + WAL `CrashUnsynced` (torn unsynced suffix) |
| Scripted crash order | `crash-matrix` crashes nodes 1, 2, 3 in that order |
| Seeded net delay / drop / duplicate | `Faults.MaxNetDelay`, `DropPermille`, `DupPermille` (profile `explore`) |
| Seeded disk delay / failed fsync | `Faults.MaxDiskDelay`, `SyncFailPermille`; `onDisk` retries `Sync` |
| Election jitter | `Faults.ElectJitter` on timeout ticks |
| Workload choice | `ExplorePuts` draws keys/values from the workload stream |
| Healthy suffix | explore zeros `Faults` and drains before judging history |

Zero-value `Faults` (elect-put, most unit tests): streams are drawn but
rolls miss, delay is 1, fsync succeeds, timeouts stay `3+3*id`. `--seed 42`
and `--seed 44` on `elect-put` are not two different executions.

### Building toward

Delayed/duplicated/reordered messages and failed syncs as the **default**
for every profile; a recorded fault schedule (not only Bernoulli rolls);
wall-clock jumps (liveness only); CI over seeds 1..N with Porcupine as the
judge; a `docs/bugs/` entry with a reproducing seed. Unbounded chaos
without a healthy suffix still must not be scored as a liveness failure
(TigerBeetle split, [T7](https://tigerbeetle.com/blog/2023-07-06-simulation-testing-for-liveness/)).

**Assumptions:** peers are not Byzantine; a successful `fsync` matches the
model; unsynced data may vanish, survive, or partially persist.

**Real vs sim:** the simulator does not replace disk-full, kernel, or NIC
behavior. `internal/nodehost` tests cover WAL append failure (panic), MVCC
order panic, and snapshot install restore. `make cluster-up` is a real
multi-process elect/put/get. That is the current real-adapter evidence,
thin compared to the sim matrix, and labeled as such.

---

## 8. What we refused so the core could exist

Out of scope on purpose (not “later” unless a gate says so): Byzantine FT; geo-global external consistency; multi-group revision comparison; recovery after **all** durable copies are gone; arbitrary SQL; distributed transactions with the content store; transparent rolling upgrades; production security certification; read leases; Redis; Kubernetes packaging; UI; treating sleep-loop tests as distributed coverage; shipping an index without an oracle; keeping v1 docs that described deleted packages.

`docs/API.md` still describes the **v1** zookie API and “no authentication.” It is not the v2 contract. Until it is rewritten, treat [ARCHITECTURE.md](ARCHITECTURE.md) + `api/zenith/v2` protobuf + this file as truth.

---

## 9. How to read the rest of the tree

| If you want… | Read |
| --- | --- |
| Clone, run, milestone table | [README](../README.md) |
| Package/process map | [ARCHITECTURE.md](ARCHITECTURE.md) |
| This argument | this file |
| One decision and its rejected alternative | [adr/](adr/) |
| Numbered invariants and checker names | [invariants.md](invariants.md) |
| Fault model + read modes (local) | `docs/contracts.md` (gitignored; summary in §5) |
| Original 22-section brief | `docs/UPDATE.md` (gitignored) |
| Phase checkboxes | `docs/PHASES.md` (gitignored) |
| Observed bugs only | [bugs/](bugs/) (empty until a replayable failure is filed) |
| Content integration | `examples/content-service` |

Implementation reading order (same as the brief): Raft + persistence, KV sim, snapshots/recovery, MVCC visibility, Zanzibar content protocol, membership, then derived indexes.

External references that actually shaped code: Raft extended paper and Ongaro dissertation; etcd-io/raft **as documentation, not a dependency**; Zanzibar (content checks, config consistency, Leopard as optional inspiration); Porcupine; TigerBeetle architecture and liveness-testing split.

---

## 10. One-paragraph version

Zenith v1 was a Cockroach-backed ReBAC engine whose timestamps, caches, and tests did not implement a snapshot protocol. v2 makes consensus and evidence the product: one Raft group, logical revisions, an Event/Effect core shared by a seeded simulator and a real node, an evaluator that cannot see more than one revision, and a content token so lag cannot bless new bytes. Decisions were weighted for replayability and isolation from clocks and second models, not for demo speed. What we can show today is `make test-v2` / `sim-smoke` / `cluster-up` plus the invariant table (pass, fail, or inconclusive), not a latency leaderboard and not a proof of absence of bugs.
