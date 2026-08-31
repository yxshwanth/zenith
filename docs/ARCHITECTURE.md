# Architecture (v2)

Zenith is a replicated authorization store: custom Raft, MVCC snapshot reads, ReBAC evaluation, and a deterministic simulator that runs the same core as a live node.

## Processes

- `cmd/zenithd`: real-disk node (WAL, peer TCP, gRPC, HTTP `/status` `/put` `/get`). Use `--dev` on loopback; otherwise `ZENITH_TOKEN_SECRET` + `--auth-token` and TLS for non-loopback.
- `cmd/zenithctl`: gRPC client (`--insecure` on loopback, `--auth-token` / `--tls-ca` otherwise).
- `cmd/zenith-sim`: `run` / `replay` of seed-deterministic profiles (`elect-put`, `crash-matrix`, `new-enemy`).

## Packages

| Package | Role |
| --- | --- |
| `internal/nodehost` | Production adapter: disk WAL, TLS/auth peer transport, effect apply |
| `internal/replica` | **Sim cluster only.** Same Raft/MVCC/session types under `internal/sim`. Not the production node. |
| `internal/raft` | Deterministic Raft `Core` (pre-vote, joint membership, snapshot install) |
| `internal/wal` | Checksummed frames; real file + in-memory sim log |
| `internal/snapshot` | Atomic publish/install of KV+session checkpoints |
| `internal/mvcc` | Revisioned tuple store; `MustApply` panics on order bugs |
| `internal/authz` | ReBAC at one pinned snapshot (union / ∩ / ∖ / TTU) |
| `internal/token` | HMAC-SHA256 opaque revision tokens |
| `internal/session` | Idempotent client retries |
| `internal/checker` | KV linearizability (Porcupine), NewEnemyOK, prefix ledger |
| `internal/runtime` | Event/Effect contract ([ADR 0005](adr/0005-deterministic-core-contract.md)) |

## Docs

- Argument, history, bias, measurements: [RATIONALE.md](RATIONALE.md)
- Invariants: [invariants.md](invariants.md)
- ADRs: [adr/](adr/)
- Local design notes (gitignored): `docs/UPDATE.md`, `docs/PHASES.md`

```bash
make test-v2
make sim-smoke
make cluster-up   # ZENITH_NODES=5, --dev loopback
```
