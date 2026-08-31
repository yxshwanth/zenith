# ADR 0002: One Raft group for the entire cluster

Status: Accepted (UPDATE.md, Section 3).

## Context

Zenith needs to decide the replication and ordering boundary for all tuple
and policy data. Sharding by store or namespace would allow independent
write scaling, at the cost of cross-shard ordering and transactions.

## Decision

Use exactly one Raft group per cluster for the initial (and likely entire
core) scope. Every tuple, policy, and session write goes through this one
group. Cluster size is three voters for the real deployment, with five
voters used in selected simulations to broaden quorum scenarios.

## Consequences

- Every tuple and policy dependency shares one ordering domain: a
  revision (Raft log index) is comparable across the whole dataset without
  a cross-group commit protocol.
- No independent write-scaling across shards, and no distributed
  transactions across multiple groups — sharding is explicitly out of
  scope (`UPDATE.md`, Section 3, "Explicit non-goals").
- The new-enemy protocol (Section 10) is simpler as a direct consequence:
  keeping all related ACLs in the same group means a fresh fence sees
  every completed prior write, without carrying a token across groups.
- This decision does not generalize automatically if the project later
  adds multiple groups; that would need its own revision-comparison
  design and is explicitly deferred.
