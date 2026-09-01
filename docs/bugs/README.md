# Bug records

Only record **observed, reproducible** failures here. See [TEMPLATE.md](TEMPLATE.md).

- [BUG-001](BUG-001.md): seed 66, checker dropped in-flight Puts (fixed)
- [BUG-002](BUG-002.md): seed 141, stale StrongGet after acked overwrite (fixed)
- [BUG-003](BUG-003.md): seed 192, KV diverges from identical committed logs (fixed)

Regressions live in `testdata/regressions/`.
