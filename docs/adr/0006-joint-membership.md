# ADR 0006: Joint consensus membership

## Status

Accepted (Phase 5).

## Decision

Use the Raft extended paper §6 joint consensus algorithm only. Do not mix etcd-io/raft apply-time membership rules.

- One voter-set transition in flight.
- Learners receive log/snapshots but do not count toward quorum.
- While joint, elections and commitment require a majority of both old and new voter sets.
- Append final C_new only after joint config commits.
- Never reuse removed node IDs.

## Consequences

Configuration is persisted in hard state and snapshots. Strong reads stay on the logged-fence path across membership changes.
