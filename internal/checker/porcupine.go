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

// CheckKVPorcupine runs Porcupine on a completed history (no Unknown entries).
// Returns Pass, Fail, or Inconclusive if any Unknown present.
func CheckKVPorcupine(history []HistoryEntry) CheckOutcome {
	var ops []porcupine.Operation
	for i, e := range history {
		if e.Unknown {
			return Inconclusive
		}
		ops = append(ops, porcupine.Operation{
			ClientId: i,
			Input:    e.Op,
			Call:     e.InvokedAt,
			Output:   e.Result,
			Return:   e.CompletedAt,
		})
	}
	ok := porcupine.CheckOperations(KVPorcupineModel(), ops)
	if ok {
		return Pass
	}
	return Fail
}
