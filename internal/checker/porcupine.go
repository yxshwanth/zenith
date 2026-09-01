package checker

import (
	"fmt"

	"github.com/anishathalye/porcupine"
)

// KVPorcupineModel is a Porcupine model for the sequential KV spec.
func KVPorcupineModel() porcupine.Model {
	return porcupine.Model{
		Init: func() any { return NewKVModel() },
		Step: func(state any, input, output any) (bool, any) {
			m := state.(*KVModel)
			// copy-on-write style: clone map
			cp := NewKVModel()
			for k, v := range m.data {
				cp.data[k] = v
			}
			op := input.(KVOp)
			want := output.(KVResult)
			got := cp.Apply(op)
			return got == want, cp
		},
		Equal: func(a, b any) bool {
			ma, mb := a.(*KVModel), b.(*KVModel)
			if len(ma.data) != len(mb.data) {
				return false
			}
			for k, v := range ma.data {
				if mb.data[k] != v {
					return false
				}
			}
			return true
		},
		DescribeOperation: func(input, output any) string {
			return fmt.Sprintf("%v→%v", input, output)
		},
	}
}

// CheckKVPorcupine classifies a history. Unknown Puts stay in the model as
// invocations with an unbounded return (successful put output) so a later
// completed Get can be explained by an unacked write. Unknown Gets are
// dropped from the model (no sequential output). Any Unknown still yields
// Inconclusive unless the remaining history is already illegal (Fail).
func CheckKVPorcupine(history []HistoryEntry) CheckOutcome {
	hasUnknown := false
	var ops []porcupine.Operation
	for i, e := range history {
		if e.Unknown {
			hasUnknown = true
			if e.Op.Kind != KVPut {
				continue
			}
			ops = append(ops, porcupine.Operation{
				ClientId: i,
				Input:    e.Op,
				Call:     e.InvokedAt,
				Output:   KVResult{Found: true, Value: e.Op.Value},
				Return:   e.InvokedAt + 1<<30,
			})
			continue
		}
		ops = append(ops, porcupine.Operation{
			ClientId: i,
			Input:    e.Op,
			Call:     e.InvokedAt,
			Output:   e.Result,
			Return:   e.CompletedAt,
		})
	}
	if len(ops) > 0 && !porcupine.CheckOperations(KVPorcupineModel(), ops) {
		return Fail
	}
	if hasUnknown {
		return Inconclusive
	}
	return Pass
}
