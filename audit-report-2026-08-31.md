# Codebase Audit Report

**Project:** zenith (github.com/yxshwanth/zenith)
**Date:** 2026-08-31
**Files scanned:** 68 Go source files (plus docs, Makefile, Dockerfile, config); node_modules/vendor/.git/etc excluded (none present)
**Languages:** Go (1.25.5), Protocol Buffers
**Frameworks:** gRPC, Protobuf, OpenTelemetry (indirect), Porcupine (linearizability checker, partially wired)

---

## Executive Summary

Zenith is mid-flight in a v1→v2 rewrite: the old layered service/db/cache architecture has been deleted and replaced with a purpose-built Raft/MVCC/authz core, and the surviving Go code is genuinely clean — no dangling imports, no circular dependencies, no hallucinated references, minimal AI-slop noise. The real risk is concentrated in two places: the network surface is currently wide open (hardcoded token secret, no gRPC auth, no TLS, unauthenticated peer-to-peer Raft transport), and several "done" features are actually inert — a Porcupine-based correctness checker that was built but never wired in, a snapshot-transfer path that nothing ever triggers, and a fault-injection profile that always reports success without testing anything. Five docs still describe the deleted v1 architecture and will actively mislead anyone using them.

**Overall rating:** NEEDS ATTENTION

---

## Critical Issues (fix immediately)

**[C-1] Hardcoded token-signing secret**
- **File:** `internal/nodehost/host.go:48`, `internal/replica/check.go:20`
- **Category:** Security
- **What:** Both the production node and the simulation harness construct `&token.Codec{Secret: []byte("zenith-dev"), ...}` — a literal string in source, with no flag, env var, or config path to override it anywhere in `cmd/zenithd/main.go`.
- **Risk:** Anyone who reads the (public) source code can compute valid opaque revision tokens for any store, defeating the token/session security model entirely.
- **Fix:** Load the signing secret from an env var or config file at startup; fail to start if unset in non-dev mode.

**[C-2] gRPC server has no authentication**
- **File:** `cmd/zenithd/main.go:63-64`
- **Category:** Security
- **What:** `grpc.NewServer()` is created with no `UnaryInterceptor`/`StreamInterceptor`. Every RPC (`Check`, `WriteTuples`, `AddLearner`, `ProposeJoint`, `ProposeFinalize`, etc.) is reachable by any network caller with no identity check.
- **Risk:** `AddLearner`/`ProposeJoint`/`ProposeFinalize` let any caller change Raft cluster membership; `WriteTuples` lets any caller mutate authorization data.
- **Fix:** Add a gRPC auth interceptor (mTLS client certs or a signed-token check) before exposing this beyond localhost.

**[C-3] Unauthenticated peer-to-peer Raft transport**
- **File:** `internal/nodehost/host.go:53-69` (listener), `internal/nodehost/host.go:81` (`readPeer`)
- **Category:** Security
- **What:** The inter-node TCP listener accepts connections from anyone and decodes JSON straight into `Node.Step(runtime.PeerMessage{...})` — directly into the Raft state machine — with no peer identity check.
- **Risk:** Any host that can reach the peer port can inject arbitrary consensus messages (fake votes, fabricated log entries), potentially corrupting the replicated log.
- **Fix:** Authenticate peers (mTLS with a fixed cluster CA, or pre-shared per-node keys) before accepting `Step()` input from a connection.

---

## High Priority (fix this sprint)

**[H-1] Unauthenticated HTTP `/put` and `/get` endpoints**
- **File:** `cmd/zenithd/main.go:70-83`
- **Category:** Security
- **What:** The HTTP mux's `/put` and `/get` handlers accept any request and read/write arbitrary KV state through the Raft log, with no auth middleware.
- **Risk:** Same as C-2 for the HTTP surface — full read/write access to replicated state from anyone who can reach the port.
- **Fix:** Same as C-2; put these behind the same auth layer or remove them from anything but a local debug build.

**[H-2] No TLS anywhere on the wire**
- **File:** `cmd/zenithctl/main.go:27` (client uses `insecure.NewCredentials()`), `cmd/zenithd/main.go` (server has no transport credentials), `internal/nodehost/host.go` peer transport (plaintext TCP)
- **Category:** Security
- **What:** Client↔node gRPC, node↔node Raft, and the HTTP control endpoints all run in plaintext.
- **Risk:** Combined with C-2/C-3/H-1, all cluster traffic (including auth-relevant tuple writes and membership changes) is visible and forgeable to anyone on the network path — a real problem if this ever runs across more than one trusted host.
- **Fix:** Add TLS (or mTLS) to the gRPC server/client and the peer transport before any non-localhost deployment.

**[H-3] Porcupine linearizability checker built but never used in production**
- **File:** `internal/checker/porcupine.go:10-64` vs `internal/checker/linearizability.go:76-80`, called from `internal/replica/replica.go:388`
- **Category:** AI Slop / Quality
- **What:** A full adapter to the real `anishathalye/porcupine` library exists and has its own tests, but production code (`replica.go:388`, `CheckHistory()`) still calls the brute-force, exponential-time home-grown checker, whose own doc comment says it's "a placeholder for Porcupine integration, not a replacement for it."
- **Risk:** The project's actual correctness oracle for the deterministic simulation is the weaker placeholder, not the more rigorous integration that was already built and paid for.
- **Fix:** Switch `replica.go:388` to call `CheckKVPorcupine`, or delete the unused Porcupine adapter if it's intentionally shelved.

**[H-4] Snapshot-transfer path is dead code**
- **File:** `internal/raft/raft.go:727` (`SendInstallSnapshot`), `internal/snapshot/snapshot.go:72` (`Store.Bytes()`)
- **Category:** Quality
- **What:** `onInstallSnapshot`/`onInstallSnapshotResp` (raft.go:698-724) fully implement the *receiving* side of leader-triggered snapshot catch-up, but `SendInstallSnapshot` has zero callers anywhere in the codebase — nothing on the leader side ever triggers a snapshot send.
- **Risk:** Follower catch-up after falling far behind presumably only works via full log replay today; the snapshot-based fast-path is untested and possibly non-functional.
- **Fix:** Wire `SendInstallSnapshot` into the leader's replication loop when a follower's needed log entries have been compacted away, and add a test exercising it.

**[H-5] Silently discarded disk-write error on the live node**
- **File:** `internal/nodehost/host.go:99`
- **Category:** Quality
- **What:** `_ = h.Disk.Append(e.Data)` discards the WAL append error; `Sync()` still fires and effects proceed as if the write succeeded.
- **Risk:** A failed disk write is treated as a successful durable commit — a false durability guarantee in the one process that actually touches disk.
- **Fix:** Propagate the error and abort the persist/reply pipeline on append failure.

**[H-6] Silently discarded MVCC apply errors**
- **File:** `internal/nodehost/host.go:140-142`, `internal/replica/replica.go:237,253`
- **Category:** Quality
- **What:** `_ = h.MVCC.ApplyPut/ApplyDelete(...)` discards the error `apply()` returns specifically to catch "rev < applied" ordering violations — an invariant violation, not a routine error.
- **Risk:** An MVCC ordering bug would fail silently instead of surfacing as a correctness alarm, defeating the purpose of the check.
- **Fix:** Log and/or panic on this error rather than discarding it — it should never fire in a correct system, so make it loud when it does.

**[H-7] `new-enemy` simulation profile is a fake-green stub**
- **File:** `cmd/zenith-sim/main.go:85-87`
- **Category:** AI Slop
- **What:** The `"new-enemy"` case prints a placeholder message and unconditionally `return true, ...` — it never runs a crash/adversary schedule or invokes the existing `NewEnemyOK` oracle (`internal/checker/content.go:32`), despite being listed as a real profile in the CLI help text.
- **Risk:** Anyone running `zenith-sim run --profile new-enemy` gets a false "pass" for a scenario that verifies nothing — exactly the kind of test that looks like coverage but isn't.
- **Fix:** Implement the actual new-enemy adversary schedule and call `NewEnemyOK`, or remove the profile from the CLI until it's implemented.

**[H-8] `shrink` delta-debugging command never consumes the trimmed trace**
- **File:** `cmd/zenith-sim/main.go:125-166`
- **Category:** AI Slop / Quality
- **What:** `shrinkCmd` drops entries from `meta.Events` and re-runs `runProfile(meta.Seed, meta.Profile)` to see if the failure persists — but `runProfile` only takes `seed` and `profile`, never `meta.Events`. The result (`ok2`) is identical on every iteration regardless of what was dropped.
- **Risk:** The command always shrinks the event list down to empty and reports that as the "minimized failing trace" — a fabricated result, not an actual minimization.
- **Fix:** Either make `runProfile` (or an alternate replay path) actually accept and replay the trimmed event list, or remove the command until it does.

**[H-9] `internal/nodehost` has zero test coverage**
- **File:** `internal/nodehost/host.go` (263 lines, no `host_test.go`)
- **Category:** Quality / Testing
- **What:** This is the only package that touches a real disk and a real network socket (WAL persistence, peer TCP framing, effect application) — everything else is exercised only through the simulated `internal/replica` path.
- **Risk:** Given H-5 and H-6 above are both in this exact file, the one place most likely to have real bugs is also the one place with no tests at all.
- **Fix:** Add integration-style tests for `Host` covering disk-append failure, peer message handling, and effect application.

---

## Medium Priority (plan to address)

**[M-1] Compiled 28MB binary tracked in git**
- **File:** `server` (repo root)
- **Category:** Structure
- **What:** `.gitignore` excludes `zenith` and `zenith-server` by name but not the generic name `server`; the binary is confirmed tracked via `git ls-files`.
- **Risk:** Repo bloat and a leftover v1 artifact that doesn't match the current build.
- **Fix:** `git rm --cached server`, add a broader ignore pattern (or build to a `bin/` dir that's gitignored).

**[M-2] Custom MAC construction instead of `crypto/hmac`**
- **File:** `internal/token/token.go:108-113`
- **Category:** Security
- **What:** `mac()` computes `SHA-256(secret || payload)` by hand rather than using `hmac.New(sha256.New, secret)`. This construction is theoretically vulnerable to length-extension since SHA-256 is Merkle–Damgård. The code has a `ponytail:` comment already acknowledging this as a shortcut.
- **Risk:** Lower than C-1 (the fixed-format payload parsing limits practical exploitability), but still a known-weaker construction where the standard-library fix is a one-line change.
- **Fix:** Switch to `crypto/hmac`.

**[M-3] Dockerfile runs as root**
- **File:** `Dockerfile`
- **Category:** Security
- **What:** No `USER` directive in either build stage; the final `alpine:latest` image runs `zenithd` as root by default.
- **Risk:** Standard container-hardening gap — a compromise of the process has root inside the container.
- **Fix:** Add a non-root `USER` before `ENTRYPOINT`.

**[M-4] `internal/index` package is orphaned**
- **File:** `internal/index/index.go`
- **Category:** AI Slop / Quality
- **What:** A "Leopard-inspired positive membership index" with its own tests, but never imported by `authz`, `replica`, or `nodehost`.
- **Risk:** Dead feature code that will bit-rot silently; also confusing for anyone trying to understand what's actually load-bearing.
- **Fix:** Wire it in where it was intended (likely `authz` fast-path checks), or delete it.

**[M-5] `broadcastAppend`'s `heartbeat` parameter has no effect**
- **File:** `internal/raft/raft.go:280,288`
- **Category:** Quality
- **What:** `func (n *Node) broadcastAppend(heartbeat bool)` immediately does `_ = heartbeat` and never branches on it; all 5 call sites pass `true`/`false` for no behavioral difference.
- **Risk:** Low on its own, but suggests an intended heartbeat/append distinction (e.g. skip re-sending entries on pure heartbeats) was lost — worth checking whether that's a correctness gap in the replication loop, not just dead code.
- **Fix:** Either implement the intended distinction or delete the parameter.

**[M-6] `MuRLock`/`MuRUnlock` don't provide read-sharing**
- **File:** `internal/nodehost/host.go:257-260`
- **Category:** Quality
- **What:** `MuLock`/`MuRLock` both lock the same plain `sync.Mutex`; there's no `RWMutex`, so `MuRLock` fully serializes with writers despite its name.
- **Risk:** Misleading API — callers like the `/get` handler (`cmd/zenithd/main.go:82-83`) are written as if reads can proceed concurrently with each other, but they can't.
- **Fix:** Either switch to a real `sync.RWMutex` or rename the methods to stop implying shared-read semantics.

**[M-7] `decodeUserset` carries three unused parameters**
- **File:** `internal/authz/authz.go:174` (call site, discards return `ns`), `internal/authz/authz.go:321-330` (body discards `ons`, `oid`, `rel`)
- **Category:** Quality
- **What:** The function takes 5 params and returns a value, but 3 params are discarded in the body and the return value is discarded at the call site — it's functionally just `decodeKey` with dead plumbing around it.
- **Risk:** Confusing on a hot path (relation-check evaluation); invites a future edit to "use" one of the unused params without realizing it's already dead.
- **Fix:** Drop to the actual signature needed, or document why the extra params exist if they're for a near-term change.

**[M-8] `Engine.check` is a 126-line function with 4-5 levels of nesting**
- **File:** `internal/authz/authz.go:75-201`
- **Category:** Quality
- **What:** Handles intersection/exclusion/TTU/direct-userset rewrite kinds inline in one recursive function.
- **Risk:** This is the core authorization evaluator — hard to unit-test individual rewrite kinds in isolation as currently structured.
- **Fix:** Split each `RelRewrite.Kind` case into its own method (`checkIntersection`, `checkExclusion`, `checkTTU`).

**[M-9] Malformed `--peers` entries silently default to peer id 0**
- **File:** `cmd/zenithd/main.go:46`
- **Category:** Quality
- **What:** `pid, _ := strconv.Atoi(kv[0])` ignores the parse error, while an adjacent check three lines up (`len(kv) != 2`) does `log.Fatalf` for a different malformed-input case in the same loop — inconsistent handling of two similar failure modes.
- **Risk:** A typo in `--peers` silently misconfigures cluster membership instead of failing fast at startup.
- **Fix:** `log.Fatalf` on the `Atoi` error too, matching the sibling check.

**[M-10] Trace-file parse errors ignored in `zenith-sim`**
- **File:** `cmd/zenith-sim/main.go:118,140`
- **Category:** Quality
- **What:** `_ = json.Unmarshal(b, &meta)` on user-supplied trace files in `replayCmd`/`shrinkCmd`.
- **Risk:** A malformed trace file silently becomes a zero-value struct and fails later with a generic "unknown profile" error instead of reporting the actual problem (unreadable trace file).
- **Fix:** Check and report the unmarshal error directly.

**[M-11] Stale doc comment references a helper that doesn't exist**
- **File:** `internal/snapshot/snapshot.go:52-53`
- **Category:** AI Slop
- **What:** Comment says tests call `CrashDropStaging` vs `CrashReplaceTorn`; only `CrashDropStaging` exists anywhere in the codebase.
- **Risk:** Low — cosmetic, but signals an abandoned second test scenario or stale doc that could mislead future contributors.
- **Fix:** Remove the reference or implement the missing helper.

**[M-12] 27 doc-comment references to a file being deleted**
- **File:** 21 non-test `.go` files including `internal/mvcc/mvcc.go`, `internal/raft/raft.go:6`, `internal/checker/linearizability.go:6`, `internal/authz/authz.go`, `internal/token/token.go`, `cmd/zenith-sim/main.go:1`
- **Category:** AI Slop
- **What:** Doc comments cite `UPDATE.md` by section number as the authoritative spec, but `UPDATE.md` is staged for deletion (`git status: D UPDATE.md`).
- **Risk:** Once the deletion lands, all 27 references become dangling pointers to a file no longer in the repo.
- **Fix:** Either keep `UPDATE.md` (it's already gitignored going forward per the new `.gitignore` entries), or update the comments to point at `docs/adr/` / `docs/invariants.md` instead.

**[M-13] gofmt failures on 9 hand-edited files**
- **File:** `internal/authz/authz.go`, `internal/authz/phaseb_test.go`, `internal/checker/content.go`, `internal/index/index.go`, `internal/nodehost/host.go`, `internal/replica/phase4_test.go`, `internal/replica/replica.go`, `internal/snapshot/snapshot.go`, `internal/token/token.go`
- **Category:** AI Slop
- **What:** `gofmt -d` reports diffs (e.g. misaligned struct-field comments in `internal/checker/content.go:16-23`) — consistent with edits from multiple sessions/tools that weren't run through `gofmt`.
- **Risk:** Cosmetic, but a `gofmt` pre-commit hook or CI check would have caught this and is apparently missing.
- **Fix:** Run `gofmt -w .` and add a CI/format check (`.github/workflows/` already exists — verify it runs `gofmt -l` and fails on output).

**[M-14] Five docs describe the deleted v1 architecture**
- **File:** `TESTING.md` (root), `docs/ARCHITECTURE.md`, `docs/DEVELOPMENT.md`, `docs/PERFORMANCE.md`, `docs/DEPLOYMENT.md`
- **Category:** Structure
- **What:** These reference `internal/cache`, `internal/db`, `internal/engine`, `internal/service`, `internal/config`, `internal/middleware`, `internal/observability` — all deleted per `git status` — and `docs/DEVELOPMENT.md:306` even has an import path (`github.com/zenith/zenith/internal/errors`) that never matched the real module path (`github.com/yxshwanth/zenith`). `docs/DEPLOYMENT.md` also instructs using `docker-compose.yml`, which is deleted and not replaced by an equivalent in the new Makefile.
- **Risk:** Anyone (human or AI) using these docs as a reference will confidently describe/build against an architecture that no longer exists.
- **Fix:** Delete or rewrite these five docs to match the v2 architecture (`internal/{authz,coordinator,mvcc,nodehost,raft,replica,session,snapshot,token}`).

**[M-15] Orphaned config example files**
- **File:** `config.example.json`, `config.example.yaml` (repo root)
- **Category:** AI Slop / Structure
- **What:** Zero references anywhere in `.go` files to `config.example`, `LoadConfig`, or `internal/config` (the package they supported was deleted).
- **Risk:** Dead artifacts from the same rewrite that will confuse anyone looking for how to configure the running binaries.
- **Fix:** Delete both files, or replace with examples matching the current `cmd/zenithd` flag-based configuration.

**[M-16] Two differently-named packages both model "a replica"**
- **File:** `internal/nodehost/host.go` (production node, used by `cmd/zenithd`) vs `internal/replica/replica.go` (deterministic in-memory simulation harness, used by `cmd/zenith-sim`)
- **Category:** Structure
- **What:** `internal/replica` doesn't hold the production replica — it's a simulation `Cluster`/`Replica` type built on `internal/sim`. The real node lives in `internal/nodehost`. No shared abstraction connects the two, so the simulated and real paths could silently diverge in behavior even though the project's core value proposition is "deterministic simulation matches production."
- **Risk:** A bug fixed in one path (e.g. the swallowed-error issues in H-5/H-6) could easily not exist in, or differ from, the other path, undermining confidence that sim results generalize to the real node.
- **Fix:** Rename `internal/replica` to something like `internal/simreplica` or `internal/simcluster` to remove the naming collision, and consider whether the two apply-paths can share more logic.

**[M-17] Uninformative test-file names in `internal/replica`**
- **File:** `internal/replica/r1_test.go`, `rc_test.go`, `phase2_test.go`, `phase3_test.go`, `phase4_test.go`, `phase5_test.go`
- **Category:** AI Slop / Structure
- **What:** Named after development phases/checkpoints rather than what they test, unlike the rest of the codebase (`internal/wal/file_test.go`, `internal/token/token_test.go`) which follows normal Go naming.
- **Risk:** Low — makes it hard to find "the test for X" without opening files.
- **Fix:** Rename to describe coverage (e.g. `election_test.go`, `membership_test.go` — some of which already exist alongside the phase-numbered ones).

---

## Low Priority (nice to have)

**[L-1] No LICENSE file**
- **File:** repo root
- **Category:** Structure
- **What:** Public module path (`github.com/yxshwanth/zenith`) with no `LICENSE`.
- **Risk:** Ambiguous terms for anyone wanting to use or contribute to the code.
- **Fix:** Add a LICENSE file matching the intended usage terms.

**[L-2] No `.editorconfig`**
- **File:** repo root
- **Category:** Structure
- **What:** Missing; ties into M-13's formatting drift.
- **Risk:** Minor — Go's `gofmt` covers most of this already.
- **Fix:** Optional; a CI `gofmt -l` check (per M-13) matters more than `.editorconfig` for a single-language Go repo.

**[L-3] `WriteTuples` handler builds its payload inline**
- **File:** `cmd/zenithd/main.go:105-124`
- **Category:** Quality
- **What:** Builds the tuple-op payload and session-digest string directly in the gRPC handler rather than delegating to a helper.
- **Risk:** Minor readability point, not a correctness issue.
- **Fix:** Optional extraction if this handler grows further.

**[L-4] `runtime.SnapshotChunk`, `Cancellation`, and the `Schedule` effect are unreachable**
- **File:** `internal/runtime/events.go`, `internal/runtime/effects.go`
- **Category:** AI Slop
- **What:** Part of the documented "deterministic core" `Event`/`Effect` contract, but no `Core.Step` implementation ever produces/consumes them (`internal/raft` is the only `Core`, and it never returns a `Schedule` effect or handles those two event kinds).
- **Risk:** Low — aspirational contract surface, not actively harmful, but adds noise to what implementers need to consider.
- **Fix:** Either use them (if a planned feature needs them) or trim the contract to what's actually implemented, with a comment noting what's reserved for later.

**[L-5] MVCC `GC` is only ever called from tests**
- **File:** `internal/mvcc/mvcc_test.go` (only caller of `GC`)
- **Category:** Testing
- **What:** Garbage collection of old MVCC revisions is unit-tested in isolation but never exercised against the real apply/replica path.
- **Risk:** Low today (no production caller means no production risk yet), but flags that GC has no integration coverage for when it is wired in.
- **Fix:** Note as a follow-up once GC is actually invoked from `nodehost`/`replica`.

---

## AI Slop Summary

**Total slop instances found:** 12

| Slop Type | Count | Worst Offender |
|-----------|-------|-----------------|
| Placeholder/stub code | 2 | `cmd/zenith-sim/main.go:85` (fake-passing `new-enemy` profile) |
| Noise comments | 1 | `internal/snapshot/snapshot.go:52` (references nonexistent helper) |
| Redundant abstractions | 2 | `internal/checker/porcupine.go` (built, never wired in) |
| Copy-paste duplication | 0 | — none found |
| Hallucinated references | 0 | — none found (build is clean, no fake imports/RPCs) |
| Inconsistency signals | 7 | 27 stale `UPDATE.md` doc-comment refs across 21 files; gofmt drift on 9 files |

**Top 5 most egregious examples:**
1. `cmd/zenith-sim/main.go:85-87` — `new-enemy` fault-injection profile always reports success without testing anything.
2. `cmd/zenith-sim/main.go:125-166` — `shrink` delta-debugging command never actually consumes the trimmed event list, so its "minimized trace" output is fabricated.
3. `internal/checker/porcupine.go` vs `internal/replica/replica.go:388` — a real linearizability checker was built and tested but production still uses the explicitly-labeled placeholder.
4. Five docs (`TESTING.md`, `docs/ARCHITECTURE.md`, `docs/DEVELOPMENT.md`, `docs/PERFORMANCE.md`, `docs/DEPLOYMENT.md`) fully describe a deleted v1 architecture, including a broken import path that never matched the real module.
5. 27 doc comments across 21 files cite `UPDATE.md` by section number, a file currently staged for deletion.

**Pattern analysis:** This isn't scaffolded-and-abandoned AI slop in the usual sense — the surviving Go code itself is coherent and well-structured. The pattern here is specifically **rewrite residue**: real work (Porcupine integration, snapshot transfer, new-enemy testing) was built partway and never flipped on, and documentation from the deleted v1 architecture was never swept out when the packages it described were removed. The fix for most of this is deletion or wiring-up, not rewriting.

---

## Metrics

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Security | 3 | 3 | 3 | 0 | 9 |
| AI Slop | 0 | 2 | 6 | 1 | 9 |
| Code Quality | 0 | 4 | 6 | 3 | 13 |
| Structure | 0 | 0 | 3 | 1 | 4 |
| **Total** | **3** | **9** | **17*** | **5** | **34** |

\* Several findings span two categories (e.g. M-4, M-11, M-12, M-15, M-17 are counted once in their primary category above but touch AI Slop and Quality/Structure simultaneously); the AI Slop Summary table above counts slop-specific instances separately (12) and overlaps with rows in this table rather than adding to it.

**Estimated remediation effort:** ~3-4 developer-days for the Critical+High items (auth/TLS wiring, error-handling fixes, retiring or fixing the two broken sim commands, wiring in Porcupine), plus ~2 days for Medium/Low cleanup (doc rewrites, dead code removal, gofmt/CI hygiene).
