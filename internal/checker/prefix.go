package checker

// prefixKey is (index, term). R3: same pair implies the same command.
// Different terms at one index are log overwrite, not a conflict.
type prefixKey struct {
	Index uint64
	Term  uint64
}

// PrefixLedger checks R3: same (index, term) implies equal command hashes.
type PrefixLedger map[prefixKey]string

func (p PrefixLedger) Observe(index, term uint64, hash string) bool {
	k := prefixKey{Index: index, Term: term}
	prev, ok := p[k]
	if !ok {
		p[k] = hash
		return true
	}
	return prev == hash
}
