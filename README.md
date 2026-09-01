# Zenith

**Zenith v2** is a replicated authorization database with a custom Raft core, MVCC snapshot reads, a deterministic replay harness (seeded explorer faults are opt-in), and a content-version protocol to prevent the new-enemy problem.

This is an experimental systems project. It is **not** a production replacement for OpenFGA, SpiceDB, CockroachDB, or Google Zanzibar. Correctness claims require reproducible evidence under `make test-v2` / `make sim-smoke`.

**Why it exists, the v1→v2 path, the bias behind the forks, how decisions were weighted, and what we can measure today:** [docs/RATIONALE.md](docs/RATIONALE.md).

Module: `github.com/yxshwanth/zenith` · License: MIT

## Milestone

| Area | Status |
| --- | --- |
| Deterministic sim + KV checkers (+ Porcupine) | Done |
| Custom Raft, WAL, snapshots, session retries | Done |
| MVCC + authz Phase A/B (∪ / ∩ / ∖ / TTU) | Done |
| Opaque tokens, read modes, new-enemy | Done |
| Joint membership, learners, ReadIndex (fixed cfg) | Done |
| Real multi-process `zenithd` + `make cluster-up` | Done (3/5 node) |
| gRPC API + `zenithctl` | Done |
| Decision cache (C1) | Done |
| Legacy Cockroach path | **Removed** |
| UI / Kubernetes / Redis | Out of scope |

Architecture: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Full argument: [`docs/RATIONALE.md`](docs/RATIONALE.md). ADRs: `docs/adr/`. Invariants: `docs/invariants.md`. Local notes (gitignored): `docs/UPDATE.md` / `docs/PHASES.md`.

Loopback only: `zenithd --dev`. Non-loopback needs `ZENITH_TOKEN_SECRET`, `--auth-token`, and TLS.

```bash
make test-v2
make sim-smoke
make generate          # needs protoc + protoc-gen-go{,-grpc}
make cluster-up        # ZENITH_NODES=5, --dev loopback
go run ./cmd/zenith-sim run --seed 42 --profile elect-put
go run ./cmd/zenith-sim run --seed 44 --profile new-enemy
go run ./cmd/zenithd --dev --id=1 --listen=127.0.0.1:7001 --http=127.0.0.1:8001 --grpc=127.0.0.1:9001
go run ./cmd/zenithctl -addr 127.0.0.1:9001 status
```
