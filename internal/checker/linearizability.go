package checker

// HistoryEntry records one client-observed KV operation: the operation
// itself, its recorded result, and its invocation/completion order in the
// simulator's global event order (never per-node wall-clock time —
// UPDATE.md, Section 14, "Record client-observed invocation and
// completion in global simulation event order, not skewed node clocks").
type HistoryEntry struct {
	Op          KVOp
	Result      KVResult
	InvokedAt   int64
	CompletedAt int64
}

// LinearizableKV reports whether history has at least one total order,
// consistent with each entry's real-time interval, under which applying
// every operation in that order to a fresh KVModel reproduces every
// recorded result. On success it also returns a witnessing order (indexes
// into history).
//
// This is a brute-force checker intended only for the small histories
// Phase 0 produces (UPDATE.md, Section 14: "Keep those histories
// intentionally small"); it is a placeholder for Porcupine integration,
// not a replacement for it — see docs/invariants.md.
func LinearizableKV(history []HistoryEntry) (bool, []int) {
	used := make([]bool, len(history))
	return search(history, used, nil)
}

func search(history []HistoryEntry, used []bool, order []int) (bool, []int) {
	if len(order) == len(history) {
		witness := append([]int(nil), order...)
		return true, witness
	}
	for i := range history {
		if used[i] || !readyToLinearize(history, used, i) {
			continue
		}

		candidate := append(order, i)
		if !replays(history, candidate) {
			continue
		}

		used[i] = true
		if done, witness := search(history, used, candidate); done {
			return true, witness
		}
		used[i] = false
	}
	return false, nil
}

// readyToLinearize reports whether every not-yet-placed entry that must
// precede candidate i — because it completed before i was invoked — has
// already been placed.
func readyToLinearize(history []HistoryEntry, used []bool, i int) bool {
	for j := range history {
		if j == i || used[j] {
			continue
		}
		if history[j].CompletedAt < history[i].InvokedAt {
			return false
		}
	}
	return true
}

// replays reports whether applying history[order[k]].Op in order to a
// fresh KVModel reproduces every history[order[k]].Result.
func replays(history []HistoryEntry, order []int) bool {
	m := NewKVModel()
	for _, idx := range order {
		got := m.Apply(history[idx].Op)
		if got != history[idx].Result {
			return false
		}
	}
	return true
}
