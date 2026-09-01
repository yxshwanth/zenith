package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestNoLeaderWhenEveryFsyncFails(t *testing.T) {
	c := NewClusterWithFaults(9, []runtime.NodeID{1, 2, 3}, Faults{SyncFailPermille: 1000})
	if got := c.BootstrapElect(25); got != 0 {
		t.Fatalf("leader %d with every fsync failing", got)
	}
}
