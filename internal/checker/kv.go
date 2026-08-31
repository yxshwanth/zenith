// Package checker implements independent reference models used to validate
// the real Zenith implementation and simulator (docs/invariants.md). A
// checker must never call the production MVCC visibility routine, cache,
// or evaluator it is meant to validate — this package has no dependency on
// any other internal/* package, deliberately.
package checker

import "fmt"

// KVOpKind identifies which operation a KVOp performs.
type KVOpKind int

const (
	KVGet KVOpKind = iota
	KVPut
	KVDelete
	KVCompareAndSwap
)

// KVOp is one operation against the reference KV model.
type KVOp struct {
	Kind  KVOpKind
	Key   string
	Value string
	// Expect and Swap are only meaningful for KVCompareAndSwap.
	Expect string
	Swap   string
}

// KVResult is the outcome of applying one KVOp to the model.
type KVResult struct {
	Found bool
	Value string
	// Swapped is only meaningful for KVCompareAndSwap.
	Swapped bool
}

// KVModel is a trivial, single-threaded, sequential reference
// implementation of a key/value store. It exists purely as an oracle: the
// real replicated store's externally observed behavior must match applying
// the same operations, in some legal order, to this model
// (docs/invariants.md, Porcupine models and histories).
type KVModel struct {
	data map[string]string
}

// NewKVModel returns an empty reference model.
func NewKVModel() *KVModel {
	return &KVModel{data: make(map[string]string)}
}

// Apply performs op against the model and returns its result. It is the
// KVModel's only mutator, so the model's entire behavior is defined here.
func (m *KVModel) Apply(op KVOp) KVResult {
	switch op.Kind {
	case KVGet:
		v, ok := m.data[op.Key]
		return KVResult{Found: ok, Value: v}
	case KVPut:
		m.data[op.Key] = op.Value
		return KVResult{Found: true, Value: op.Value}
	case KVDelete:
		_, ok := m.data[op.Key]
		delete(m.data, op.Key)
		return KVResult{Found: ok}
	case KVCompareAndSwap:
		cur, ok := m.data[op.Key]
		if !ok || cur != op.Expect {
			return KVResult{Found: ok, Value: cur, Swapped: false}
		}
		m.data[op.Key] = op.Swap
		return KVResult{Found: true, Value: op.Swap, Swapped: true}
	default:
		panic(fmt.Sprintf("checker: unknown KVOpKind %d", op.Kind))
	}
}
