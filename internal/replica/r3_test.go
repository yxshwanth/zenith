package replica

import (
	"fmt"
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestLogMatchingLeaderChangeMidAppend(t *testing.T) {
	for _, seed := range []uint64{3, 11, 29, 41, 77} {
		if err := logMatchMidAppend(seed); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func logMatchMidAppend(seed uint64) error {
	c := NewCluster(seed, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		return fmt.Errorf("no leader")
	}
	c.ProposePut(leader, "a", "s", 1, "k", "A")
	c.Run(8)
	c.Crash(leader)
	leader2 := c.BootstrapElect(80)
	if leader2 == 0 {
		return fmt.Errorf("no leader after crash\n%s", c)
	}
	c.ProposePut(leader2, "b", "s", 2, "k", "B")
	c.Run(1500)
	if err := c.prefixOK(); err != nil {
		return err
	}
	out := c.CheckHistory()
	if out == checker.Fail {
		return fmt.Errorf("porcupine Fail: %+v", c.History)
	}
	return nil
}
