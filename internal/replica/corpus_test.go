package replica

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	stdrt "runtime"
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
)

type corpusCase struct {
	Seed    uint64 `json:"seed"`
	Profile string `json:"profile"`
	Expect  string `json:"expect"`
}

func TestRegressionCorpus(t *testing.T) {
	_, file, _, ok := stdrt.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "regressions")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		n++
		path := filepath.Join(dir, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var cs corpusCase
			if err := json.Unmarshal(b, &cs); err != nil {
				t.Fatal(err)
			}
			ok, err := replayCorpus(cs)
			if cs.Expect == "fail" {
				if ok {
					t.Fatal("want fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatal("want ok")
			}
		})
	}
	if n == 0 {
		t.Fatal("empty corpus")
	}
}

func replayCorpus(cs corpusCase) (bool, error) {
	switch cs.Profile {
	case "elect-put":
		c := NewCluster(cs.Seed, []runtime.NodeID{1, 2, 3})
		leader := c.BootstrapElect(50)
		if leader == 0 {
			return false, nil
		}
		c.ProposePut(leader, "r1", "s", 1, "k", "v")
		c.Run(1000)
		return c.Replicas[leader].KV["k"] == "v", nil
	case "crash-matrix":
		c := NewCluster(cs.Seed, []runtime.NodeID{1, 2, 3})
		leader := c.BootstrapElect(50)
		if leader == 0 {
			return false, nil
		}
		c.ProposePut(leader, "r1", "s", 1, "k", "v")
		c.Run(1000)
		for _, id := range []runtime.NodeID{1, 2, 3} {
			c.Crash(id)
		}
		return c.BootstrapElect(80) != 0, nil
	case "new-enemy":
		err := RunNewEnemy(cs.Seed)
		return err == nil, err
	case "new-enemy-explore":
		err := RunNewEnemyExplore(cs.Seed)
		return err == nil, err
	case "explore":
		_, out := runExplore(cs.Seed)
		return out.String() != "fail", nil
	case "hunt":
		out, _, err := huntOnce(cs.Seed, huntFaults())
		if err != nil {
			return false, err
		}
		return out.String() != "fail", nil
	default:
		return false, fmt.Errorf("unknown profile %s", cs.Profile)
	}
}
