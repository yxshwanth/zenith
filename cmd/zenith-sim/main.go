// Command zenith-sim runs deterministic simulation profiles.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yxshwanth/zenith/internal/replica"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "replay":
		replayCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: zenith-sim run|replay ...\n")
}

type traceFile struct {
	Seed    uint64   `json:"seed"`
	Profile string   `json:"profile"`
	Assert  string   `json:"assert"` // "leader" | "put"
	Events  []string `json:"events,omitempty"`
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	seed := fs.Uint64("seed", 42, "master seed")
	profile := fs.String("profile", "elect-put", "elect-put|crash-matrix|new-enemy")
	outTrace := fs.String("out-trace", "", "optional path to write scheduler event kinds")
	_ = fs.Parse(args)
	ok, events := runProfile(*seed, *profile)
	if *outTrace != "" {
		b, _ := json.MarshalIndent(traceFile{Seed: *seed, Profile: *profile, Assert: "put", Events: events}, "", "  ")
		_ = os.WriteFile(*outTrace, b, 0o644)
	}
	if !ok {
		os.Exit(1)
	}
}

func runProfile(seed uint64, profile string) (bool, []string) {
	if profile == "new-enemy" {
		if err := replica.RunNewEnemy(seed); err != nil {
			fmt.Println("FAIL", err)
			return false, nil
		}
		fmt.Printf("ok profile=new-enemy seed=%d\n", seed)
		return true, nil
	}
	c := replica.NewCluster(seed, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		fmt.Println("FAIL no leader")
		return false, eventKinds(c)
	}
	switch profile {
	case "elect-put":
		c.ProposePut(leader, "r1", "s", 1, "k", "v")
		c.Run(1000)
		if c.Replicas[leader].KV["k"] != "v" {
			fmt.Println("FAIL put")
			return false, eventKinds(c)
		}
	case "crash-matrix":
		c.ProposePut(leader, "r1", "s", 1, "k", "v")
		c.Run(1000)
		for _, id := range []runtime.NodeID{1, 2, 3} {
			c.Crash(id)
		}
		leader = c.BootstrapElect(80)
		if leader == 0 {
			fmt.Println("FAIL re-elect")
			return false, eventKinds(c)
		}
	default:
		fmt.Println("unknown profile")
		return false, nil
	}
	fmt.Printf("ok profile=%s seed=%d leader=%d\n", profile, seed, leader)
	return true, eventKinds(c)
}

func eventKinds(c *replica.Cluster) []string {
	var out []string
	for _, d := range c.Sched.Trace() {
		out = append(out, fmt.Sprintf("%T", d.Event))
	}
	return out
}

func replayCmd(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	trace := fs.String("trace", "", "path to regression json")
	_ = fs.Parse(args)
	if *trace == "" {
		fmt.Println("need --trace")
		os.Exit(2)
	}
	b, err := os.ReadFile(*trace)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var meta traceFile
	if err := json.Unmarshal(b, &meta); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ok, _ := runProfile(meta.Seed, meta.Profile)
	if !ok {
		os.Exit(1)
	}
}
