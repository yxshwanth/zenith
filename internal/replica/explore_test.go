package replica

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func explorerFaults() Faults {
	return Faults{
		MaxNetDelay: 3, DropPermille: 20, DupPermille: 10,
		MaxDiskDelay: 2, SyncFailPermille: 40, ElectJitter: 6,
	}
}

func runExplore(seed uint64) (string, checker.CheckOutcome) {
	c := NewClusterWithFaults(seed, []runtime.NodeID{1, 2, 3}, explorerFaults())
	c.BootstrapElect(80)
	c.ExplorePuts(4)
	c.Faults = Faults{}
	if c.Leader() == 0 {
		c.BootstrapElect(80)
	}
	c.Run(800)
	var b strings.Builder
	for _, d := range c.Sched.Trace() {
		fmt.Fprintf(&b, "%d|%d|%T\n", d.Due, d.Target, d.Event)
	}
	return b.String(), c.CheckHistory()
}

func TestExploreSeedReplayableAndDiverges(t *testing.T) {
	a1, outA := runExplore(42)
	a2, _ := runExplore(42)
	if a1 != a2 {
		t.Fatal("seed 42 not replayable")
	}
	if outA == checker.Fail {
		t.Fatalf("seed 42 history Fail")
	}
	var other uint64
	for seed := uint64(43); seed < 80; seed++ {
		b, out := runExplore(seed)
		if out == checker.Fail {
			t.Fatalf("seed %d history Fail", seed)
		}
		if b != a1 {
			other = seed
			break
		}
	}
	if other == 0 {
		t.Fatal("seeds 43-79 produced the same trace as 42; explorer streams are unused")
	}
	b2, _ := runExplore(other)
	b1, _ := runExplore(other)
	if b1 != b2 {
		t.Fatalf("seed %d not replayable", other)
	}
}

func TestExploreSeedSweep(t *testing.T) {
	raw := os.Getenv("ZENITH_SWEEP")
	if raw == "" {
		t.Skip("set ZENITH_SWEEP=N to scan explore seeds 1..N")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		t.Fatalf("ZENITH_SWEEP must be a positive int, got %q", raw)
	}
	hot := huntFaults()
	checked := 0
	for seed := uint64(1); seed <= uint64(n); seed++ {
		out, dump, err := huntOnce(seed, hot)
		if err != nil {
			t.Fatalf("seed %d prefix: %v\n%s", seed, err, dump)
		}
		if out == checker.Fail {
			t.Fatalf("seed %d FAIL\n%s", seed, dump)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no seed produced a client history")
	}
	t.Logf("porcupine/prefix hunt-checked %d seeds", checked)
}

func huntFaults() Faults {
	return Faults{
		MaxNetDelay: 8, DropPermille: 120, DupPermille: 80,
		MaxDiskDelay: 4, SyncFailPermille: 100, ElectJitter: 16,
	}
}

func huntOnce(seed uint64, f Faults) (checker.CheckOutcome, string, error) {
	c := NewClusterWithFaults(seed, []runtime.NodeID{1, 2, 3}, f)
	c.BootstrapElect(80)
	c.ExploreHunt(40)
	c.Faults = Faults{}
	if c.Leader() == 0 {
		c.BootstrapElect(80)
	}
	c.Run(1200)
	if err := c.prefixOK(); err != nil {
		return checker.Fail, fmt.Sprintf("%v\n%s", err, c), err
	}
	return c.CheckHistory(), fmt.Sprintf("pending=%+v\n%s\n%s", c.pending, c, c.logDump()), nil
}

func (c *Cluster) logDump() string {
	var b strings.Builder
	ids := make([]runtime.NodeID, 0, len(c.Replicas))
	for id := range c.Replicas {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		fmt.Fprintf(&b, "log n%d:", id)
		for _, e := range c.Replicas[id].Node.LogEntries() {
			var cmd command
			if json.Unmarshal(e.Command, &cmd) != nil {
				continue
			}
			fmt.Fprintf(&b, " [%d/%d %s %s=%s]", e.Index, e.Term, cmd.Kind, cmd.Key, cmd.Value)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestHuntOneSeed(t *testing.T) {
	raw := os.Getenv("ZENITH_HUNT_SEED")
	if raw == "" {
		t.Skip("set ZENITH_HUNT_SEED")
	}
	seed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	out, dump, perr := huntOnce(seed, huntFaults())
	if perr != nil {
		t.Fatalf("prefix: %v\n%s", perr, dump)
	}
	t.Log(dump)
	if out == checker.Fail {
		t.Fatalf("seed %d porcupine Fail", seed)
	}
}
