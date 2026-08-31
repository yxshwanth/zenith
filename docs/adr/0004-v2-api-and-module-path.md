# ADR 0004: zenith.v2 API and module path, no v1 reinterpretation

Status: Accepted (UPDATE.md, Section 3 and Section 17); module path change
itself is a pending migration step, not yet applied.

## Context

The v1 API (`zenith.proto`) uses an `int64 zookie` /
`required_zookie` pair backed by a Cockroach timestamp. v2 replaces this
with an opaque, epoch/group/store-scoped token (ADR 0003, `UPDATE.md`
Section 9). The current Go module path is `github.com/zenith/zenith`
(mismatched with the `yxshwanth/zenith` repository); `UPDATE.md` Section 17
calls for correcting it as part of the v2 migration sequence.

## Decision

- Introduce a new `zenith.v2` protobuf package and Go client surface. Do
  not repurpose v1 field numbers or reinterpret v1 integers as v2
  revisions.
- Correct the Go module path to `github.com/yxshwanth/zenith` (or apply a
  formal `/v2` module suffix, chosen once and applied consistently) as
  migration step 3, together with updating imports and the protobuf
  `go_package` option in the same change.
- If no application depends on the current API by the time of cutover,
  prefer a clean cutover without a v1 compatibility shim, rather than
  carrying both API surfaces indefinitely.

## Consequences

- v1 and v2 tokens are never comparable or convertible; a v1 zookie must
  never be accepted where a v2 token is expected, and vice versa.
- The module path correction is a single, deliberate, atomic change
  (imports + `go_package` together), not something done incrementally
  file-by-file, to avoid a half-migrated import graph.
- This ADR does not itself change `go.mod` — that happens when the v2
  migration sequence (Section 17) actually starts; Phase 0 skeleton code
  added under the current module path must not be mistaken for that step.
