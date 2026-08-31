package checker

// PrefixLedger checks R3-style: same (index,term) implies equal command digests.
type PrefixLedger map[uint64]struct {
	Term uint64
	Hash string
}

func (p PrefixLedger) Observe(index, term uint64, hash string) bool {
	prev, ok := p[index]
	if !ok {
		p[index] = struct {
			Term uint64
			Hash string
		}{Term: term, Hash: hash}
		return true
	}
	return prev.Term == term && prev.Hash == hash
}
