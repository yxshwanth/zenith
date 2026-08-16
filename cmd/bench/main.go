// Command bench seeds a CockroachDB cluster and measures Check latency.
//
//	go run ./cmd/bench -mode=seed
//	go run ./cmd/bench -mode=sql
//	go run ./cmd/bench -mode=load -scenario=all
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zenith/zenith/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const schema = `
CREATE TABLE IF NOT EXISTS relation_tuples (
    namespace STRING NOT NULL,
    object_id STRING NOT NULL,
    relation STRING NOT NULL,
    subject_namespace STRING NOT NULL,
    subject_id STRING NOT NULL,
    subject_relation STRING NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (namespace, object_id, relation, subject_namespace, subject_id, subject_relation),
    INDEX idx_object_relation (namespace, object_id, relation),
    INDEX idx_subject (subject_namespace, subject_id, subject_relation)
);
`

func main() {
	mode := flag.String("mode", "all", "seed | sql | load | all")
	dsn := flag.String("dsn", envOr("ZENITH_DATABASE_CONNECTION_STRING", "postgres://root@localhost:26257/zenith?sslmode=disable"), "CockroachDB DSN")
	grpcAddr := flag.String("grpc", "localhost:50051", "gRPC address")
	directDocs := flag.Int("direct-docs", 50000, "direct-permission documents to seed")
	groups := flag.Int("groups", 1000, "groups to seed")
	members := flag.Int("members", 20, "members per group")
	nestedDocs := flag.Int("nested-docs", 5000, "documents granted via group usersets")
	n := flag.Int("n", 20000, "timed check requests per scenario")
	c := flag.Int("c", 50, "concurrent clients")
	warmup := flag.Int("warmup", 2000, "warmup requests per scenario")
	sqlIters := flag.Int("sql-iters", 3000, "iterations per SQL strategy")
	scenario := flag.String("scenario", "all", "direct_hot | direct_uniform | nested_hot | deny | all")
	flag.Parse()

	switch *mode {
	case "seed":
		must(runSeed(*dsn, *directDocs, *groups, *members, *nestedDocs))
	case "sql":
		must(runSQLBench(*dsn, *sqlIters))
	case "load":
		must(runLoad(*grpcAddr, *directDocs, *nestedDocs, *n, *c, *warmup, *scenario))
	case "all":
		must(runSeed(*dsn, *directDocs, *groups, *members, *nestedDocs))
		must(runSQLBench(*dsn, *sqlIters))
		must(runLoad(*grpcAddr, *directDocs, *nestedDocs, *n, *c, *warmup, *scenario))
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *mode)
		os.Exit(2)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "bench: %v\n", err)
		os.Exit(1)
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping cockroach: %w", err)
	}
	return db, nil
}

func runSeed(dsn string, directDocs, groups, members, nestedDocs int) error {
	db, err := openDB(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}

	start := time.Now()
	fmt.Printf("seeding tuples into cockroach (direct=%d groups=%d×%d nested=%d)...\n",
		directDocs, groups, members, nestedDocs)

	const batch = 250
	type row struct{ ns, oid, rel, sns, sid, srel string }
	flush := func(rows []row) error {
		if len(rows) == 0 {
			return nil
		}
		var b strings.Builder
		args := make([]any, 0, len(rows)*6)
		b.WriteString(`INSERT INTO relation_tuples
			(namespace, object_id, relation, subject_namespace, subject_id, subject_relation)
			VALUES `)
		for i, r := range rows {
			if i > 0 {
				b.WriteByte(',')
			}
			base := i * 6
			fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6)
			args = append(args, r.ns, r.oid, r.rel, r.sns, r.sid, r.srel)
		}
		b.WriteString(" ON CONFLICT DO NOTHING")
		_, err := db.ExecContext(ctx, b.String(), args...)
		return err
	}

	buf := make([]row, 0, batch)
	push := func(r row) error {
		buf = append(buf, r)
		if len(buf) >= batch {
			if err := flush(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
		return nil
	}

	for i := 0; i < directDocs; i++ {
		if err := push(row{
			ns: "doc", oid: fmt.Sprintf("direct_%d", i), rel: "viewer",
			sns: "user", sid: fmt.Sprintf("u_%d", i%10000), srel: "",
		}); err != nil {
			return err
		}
	}
	for g := 0; g < groups; g++ {
		for m := 0; m < members; m++ {
			if err := push(row{
				ns: "group", oid: fmt.Sprintf("g_%d", g), rel: "member",
				sns: "user", sid: fmt.Sprintf("u_%d", (g*members+m)%10000), srel: "",
			}); err != nil {
				return err
			}
		}
	}
	for i := 0; i < nestedDocs; i++ {
		if err := push(row{
			ns: "doc", oid: fmt.Sprintf("nested_%d", i), rel: "viewer",
			sns: "group", sid: fmt.Sprintf("g_%d", i%groups), srel: "member",
		}); err != nil {
			return err
		}
	}
	if err := flush(buf); err != nil {
		return err
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM relation_tuples").Scan(&count); err != nil {
		return err
	}
	fmt.Printf("seed complete: %d tuples in %s\n", count, time.Since(start).Round(time.Millisecond))
	return nil
}

func runSQLBench(dsn string, iters int) error {
	db, err := openDB(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	args := []any{"doc", "direct_0", "viewer", "user", "u_0", ""}

	legacy := func() error {
		var exists int
		err := db.QueryRowContext(ctx, `
			SELECT 1 FROM relation_tuples
			WHERE namespace=$1 AND object_id=$2 AND relation=$3
			  AND subject_namespace=$4 AND subject_id=$5 AND subject_relation=$6
			LIMIT 1`, args...).Scan(&exists)
		if err != nil {
			return err
		}
		var zookie string
		return db.QueryRowContext(ctx, "SELECT cluster_logical_timestamp()::STRING").Scan(&zookie)
	}
	combined := func() error {
		var found bool
		var zookie string
		return db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM relation_tuples
				WHERE namespace=$1 AND object_id=$2 AND relation=$3
				  AND subject_namespace=$4 AND subject_id=$5 AND subject_relation=$6
			), cluster_logical_timestamp()::STRING`, args...).Scan(&found, &zookie)
	}

	timeStrategy := func(name string, fn func() error) (*stats, error) {
		for i := 0; i < 200; i++ {
			if err := fn(); err != nil {
				return nil, fmt.Errorf("%s warmup: %w", name, err)
			}
		}
		durs := make([]time.Duration, 0, iters)
		start := time.Now()
		for i := 0; i < iters; i++ {
			t0 := time.Now()
			if err := fn(); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			durs = append(durs, time.Since(t0))
		}
		s := summarize(name, durs, time.Since(start), 0, 0)
		return &s, nil
	}

	legacyS, err := timeStrategy("legacy 2-RTT (SELECT 1 + GetZookie)", legacy)
	if err != nil {
		return err
	}
	combS, err := timeStrategy("combined 1-RTT (EXISTS + zookie)", combined)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("SQL microbench (same tuple, same connection pool, no gRPC)")
	printStatsHeader()
	printStats(*legacyS)
	printStats(*combS)
	if legacyS.p50 > 0 {
		fmt.Printf("p50 delta: combined is %.0f%% of legacy\n", 100*float64(combS.p50)/float64(legacyS.p50))
	}
	return nil
}

type checkReq func() *api.CheckRequest

func runLoad(addr string, directDocs, nestedDocs, n, conc, warmup int, scenario string) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial %s: %w (is zenith running?)", addr, err)
	}
	defer conn.Close()
	client := api.NewZenithClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Check(ctx, &api.CheckRequest{
		SubjectNamespace: "user", SubjectId: "u_0",
		Namespace: "doc", ObjectId: "direct_0", Relation: "viewer",
	}); err != nil {
		return fmt.Errorf("probe Check: %w", err)
	}

	scenarios := map[string]checkReq{
		"direct_hot": func() *api.CheckRequest {
			return &api.CheckRequest{
				SubjectNamespace: "user", SubjectId: "u_0",
				Namespace: "doc", ObjectId: "direct_0", Relation: "viewer",
			}
		},
		"direct_uniform": func() *api.CheckRequest {
			i := rand.Intn(directDocs)
			return &api.CheckRequest{
				SubjectNamespace: "user", SubjectId: fmt.Sprintf("u_%d", i%10000),
				Namespace: "doc", ObjectId: fmt.Sprintf("direct_%d", i), Relation: "viewer",
			}
		},
		"nested_hot": func() *api.CheckRequest {
			return &api.CheckRequest{
				SubjectNamespace: "user", SubjectId: "u_0",
				Namespace: "doc", ObjectId: "nested_0", Relation: "viewer",
			}
		},
		"deny": func() *api.CheckRequest {
			return &api.CheckRequest{
				SubjectNamespace: "user", SubjectId: "nobody",
				Namespace: "doc", ObjectId: "direct_0", Relation: "viewer",
			}
		},
	}

	order := []string{"direct_hot", "direct_uniform", "nested_hot", "deny"}
	if scenario != "all" {
		if _, ok := scenarios[scenario]; !ok {
			return fmt.Errorf("unknown scenario %q", scenario)
		}
		order = []string{scenario}
	}

	fmt.Println()
	fmt.Printf("gRPC Check load  target=%s  n=%d  c=%d  warmup=%d  go=%s %s/%s\n",
		addr, n, conc, warmup, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	printStatsHeader()
	for _, name := range order {
		s, err := loadScenario(client, name, scenarios[name], n, conc, warmup)
		if err != nil {
			return err
		}
		printStats(s)
	}
	return nil
}

func loadScenario(client api.ZenithClient, name string, next checkReq, n, conc, warmup int) (stats, error) {
	runN := func(count int) ([]time.Duration, int64, int64, error) {
		var (
			idx    atomic.Int64
			errors atomic.Int64
			allows atomic.Int64
			durs   = make([]time.Duration, count)
			wg     sync.WaitGroup
		)
		worker := func() {
			defer wg.Done()
			for {
				i := int(idx.Add(1) - 1)
				if i >= count {
					return
				}
				req := next()
				t0 := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				resp, err := client.Check(ctx, req)
				cancel()
				durs[i] = time.Since(t0)
				if err != nil {
					errors.Add(1)
					continue
				}
				if resp.Allowed {
					allows.Add(1)
				}
			}
		}
		wg.Add(conc)
		for i := 0; i < conc; i++ {
			go worker()
		}
		wg.Wait()
		return durs, errors.Load(), allows.Load(), nil
	}

	if _, _, _, err := runN(warmup); err != nil {
		return stats{}, err
	}
	start := time.Now()
	durs, errs, allows, err := runN(n)
	if err != nil {
		return stats{}, err
	}
	return summarize(name, durs, time.Since(start), errs, allows), nil
}

type stats struct {
	name    string
	n       int
	p50     time.Duration
	p95     time.Duration
	p99     time.Duration
	avg     time.Duration
	qps     float64
	errors  int64
	allowed int64
}

func summarize(name string, durs []time.Duration, wall time.Duration, errors, allowed int64) stats {
	cp := append([]time.Duration(nil), durs...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	var sum time.Duration
	for _, d := range cp {
		sum += d
	}
	n := len(cp)
	avg := time.Duration(0)
	if n > 0 {
		avg = sum / time.Duration(n)
	}
	qps := 0.0
	if wall > 0 {
		qps = float64(n) / wall.Seconds()
	}
	return stats{
		name:    name,
		n:       n,
		p50:     pct(cp, 0.50),
		p95:     pct(cp, 0.95),
		p99:     pct(cp, 0.99),
		avg:     avg,
		qps:     qps,
		errors:  errors,
		allowed: allowed,
	}
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func printStatsHeader() {
	fmt.Printf("%-42s %8s %8s %8s %8s %8s %8s %8s\n",
		"scenario", "n", "p50", "p95", "p99", "avg", "qps", "err/ok")
}

func printStats(s stats) {
	fmt.Printf(" %-41s %8d %8s %8s %8s %8s %8.0f %3d/%-4d\n",
		s.name, s.n,
		fmtDur(s.p50), fmtDur(s.p95), fmtDur(s.p99), fmtDur(s.avg),
		s.qps, s.errors, s.allowed)
}

func fmtDur(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	}
	return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
}
