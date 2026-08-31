// Command zenithd is a real-disk Raft+MVCC process with gRPC + HTTP status.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/yxshwanth/zenith/api/zenith/v2/pb"
	"github.com/yxshwanth/zenith/internal/nodehost"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func main() {
	id := flag.Int("id", 1, "node id")
	dataDir := flag.String("data-dir", "", "WAL directory")
	listen := flag.String("listen", "127.0.0.1:7001", "peer listen")
	httpAddr := flag.String("http", "127.0.0.1:8001", "HTTP status/put")
	grpcAddr := flag.String("grpc", "127.0.0.1:9001", "gRPC listen")
	peersFlag := flag.String("peers", "", "id=host:port,...")
	dev := flag.Bool("dev", false, "loopback-only: default token secret, no RPC/peer auth")
	authFlag := flag.String("auth-token", "", "cluster auth token (or ZENITH_AUTH_TOKEN)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate PEM")
	tlsKey := flag.String("tls-key", "", "TLS private key PEM")
	flag.Parse()
	if *dataDir == "" {
		*dataDir = fmt.Sprintf("testdata/zenithd-%d", *id)
	}
	if *dev {
		for _, a := range []string{*listen, *httpAddr, *grpcAddr} {
			if !isLoopback(a) {
				log.Fatalf("--dev requires loopback bind, got %s", a)
			}
		}
	}
	authToken := *authFlag
	if authToken == "" {
		authToken = os.Getenv("ZENITH_AUTH_TOKEN")
	}
	secret := os.Getenv("ZENITH_TOKEN_SECRET")
	if secret == "" && !*dev {
		log.Fatal("ZENITH_TOKEN_SECRET required (or --dev)")
	}
	if secret == "" {
		secret = "zenith-dev"
	}
	if !*dev && authToken == "" {
		log.Fatal("ZENITH_AUTH_TOKEN or --auth-token required (or --dev)")
	}
	hasTLS := *tlsCert != "" && *tlsKey != ""
	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("--tls-cert and --tls-key must be set together")
	}
	if !*dev {
		for _, a := range []string{*listen, *httpAddr, *grpcAddr} {
			if !isLoopback(a) && (authToken == "" || !hasTLS) {
				log.Fatalf("non-loopback %s requires --auth-token and --tls-cert/--tls-key", a)
			}
		}
	}

	self := runtime.NodeID(*id)
	peerAddrs := map[runtime.NodeID]string{}
	peerIDs := []runtime.NodeID{self}
	for _, part := range strings.Split(*peersFlag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			log.Fatalf("bad peer %q", part)
		}
		pid, err := strconv.Atoi(kv[0])
		if err != nil {
			log.Fatalf("bad peer id %q: %v", kv[0], err)
		}
		peerAddrs[runtime.NodeID(pid)] = kv[1]
		peerIDs = append(peerIDs, runtime.NodeID(pid))
	}
	h, err := nodehost.Open(self, peerIDs, peerAddrs, *dataDir, []byte(secret))
	if err != nil {
		log.Fatal(err)
	}
	h.AuthToken = authToken
	if hasTLS {
		cert, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatal(err)
		}
		h.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}
	if err := h.StartPeers(*listen); err != nil {
		log.Fatal(err)
	}

	opts := []grpc.ServerOption{grpc.UnaryInterceptor(authUnary(authToken))}
	if hasTLS {
		creds, err := credentials.NewServerTLSFromFile(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatal(err)
		}
		opts = append(opts, grpc.Creds(creds))
	}
	gs := grpc.NewServer(opts...)
	pb.RegisterZenithServer(gs, &grpcAPI{h: h})
	gln, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() { log.Fatal(gs.Serve(gln)) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		role, term, commit, applied := h.Status()
		_ = json.NewEncoder(w).Encode(map[string]any{"id": self, "role": role, "term": term, "commit": commit, "applied": applied})
	})
	mux.HandleFunc("/put", httpAuth(authToken, func(w http.ResponseWriter, r *http.Request) {
		key, val := r.URL.Query().Get("key"), r.URL.Query().Get("val")
		body, _ := json.Marshal(map[string]string{"kind": "Put", "key": key, "value": val})
		reqID := fmt.Sprintf("put-%d", time.Now().UnixNano())
		if err := h.Propose(reqID, body, 3*time.Second); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "ok\n")
	}))
	mux.HandleFunc("/get", httpAuth(authToken, func(w http.ResponseWriter, r *http.Request) {
		h.MuRLock()
		defer h.MuRUnlock()
		key := r.URL.Query().Get("key")
		v, ok := h.KV[key]
		_ = json.NewEncoder(w).Encode(map[string]any{"key": key, "value": v, "ok": ok})
	}))
	log.Printf("zenithd id=%d peer=%s http=%s grpc=%s", self, *listen, *httpAddr, *grpcAddr)
	if hasTLS {
		log.Fatal(http.ListenAndServeTLS(*httpAddr, *tlsCert, *tlsKey, mux))
	}
	log.Fatal(http.ListenAndServe(*httpAddr, mux))
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func bearer(md []string) string {
	if len(md) == 0 {
		return ""
	}
	return strings.TrimPrefix(md[0], "Bearer ")
}

func authUnary(want string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if want == "" {
			return handler(ctx, req)
		}
		md, _ := metadata.FromIncomingContext(ctx)
		if !nodehost.TokenOK(bearer(md["authorization"]), want) {
			return nil, status.Error(codes.Unauthenticated, "unauthenticated")
		}
		return handler(ctx, req)
	}
}

func httpAuth(want string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if want == "" {
			next(w, r)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !nodehost.TokenOK(got, want) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type grpcAPI struct {
	pb.UnimplementedZenithServer
	h *nodehost.Host
}

func (g *grpcAPI) Check(_ context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	ok, tok, err := g.h.Check(req.Store, req.ObjectNamespace, req.ObjectId, req.Relation, req.SubjectNamespace, req.SubjectId)
	if err != nil {
		return &pb.CheckResponse{Error: err.Error()}, nil
	}
	return &pb.CheckResponse{Allowed: ok, Token: tok}, nil
}

func (g *grpcAPI) WriteTuples(_ context.Context, req *pb.WriteTuplesRequest) (*pb.WriteTuplesResponse, error) {
	ops := make([]map[string]any, 0, len(req.Tuples)+len(req.Deletes))
	for _, t := range req.Tuples {
		ops = append(ops, map[string]any{"ons": t.ObjectNamespace, "oid": t.ObjectId, "rel": t.Relation, "sns": t.SubjectNamespace, "sid": t.SubjectId, "srel": t.SubjectRelation})
	}
	for _, t := range req.Deletes {
		ops = append(ops, map[string]any{"ons": t.ObjectNamespace, "oid": t.ObjectId, "rel": t.Relation, "sns": t.SubjectNamespace, "sid": t.SubjectId, "srel": t.SubjectRelation, "delete": true})
	}
	dig := fmt.Sprintf("%s:%d:%d", req.Store, req.Seq, len(ops))
	payload, _ := json.Marshal(map[string]any{"kind": "TupleBatch", "store": req.Store, "session": req.Session, "seq": req.Seq, "digest": dig, "ops": ops})
	reqID := req.RequestId
	if reqID == "" {
		reqID = dig
	}
	if err := g.h.Propose(reqID, payload, 5*time.Second); err != nil {
		return &pb.WriteTuplesResponse{Error: err.Error()}, nil
	}
	_, _, _, applied := g.h.Status()
	tok := g.h.Codec.Mint(req.Store, applied)
	return &pb.WriteTuplesResponse{Token: tok, Revision: applied}, nil
}

func (g *grpcAPI) ListSubjects(_ context.Context, req *pb.ListSubjectsRequest) (*pb.ListSubjectsResponse, error) {
	subs, rev, tok, err := g.h.ListSubjects(req.Store, req.ObjectNamespace, req.ObjectId, req.Relation, int(req.Limit))
	if err != nil {
		return &pb.ListSubjectsResponse{Error: err.Error()}, nil
	}
	out := make([]*pb.Subject, len(subs))
	for i, s := range subs {
		out[i] = &pb.Subject{SubjectNamespace: s.SubjectNamespace, SubjectId: s.SubjectID, SubjectRelation: s.SubjectRelation}
	}
	return &pb.ListSubjectsResponse{Subjects: out, Token: tok, Revision: rev, Complete: true}, nil
}

func (g *grpcAPI) GetClusterStatus(context.Context, *pb.GetClusterStatusRequest) (*pb.GetClusterStatusResponse, error) {
	role, term, commit, applied := g.h.Status()
	return &pb.GetClusterStatusResponse{Role: role, Term: term, CommitIndex: commit, AppliedIndex: applied}, nil
}

func (g *grpcAPI) BatchCheck(ctx context.Context, req *pb.BatchCheckRequest) (*pb.BatchCheckResponse, error) {
	results := make([]*pb.CheckResponse, len(req.Items))
	var rev uint64
	var tok string
	for i, it := range req.Items {
		store := it.Store
		if store == "" {
			store = req.Store
		}
		ok, t, err := g.h.Check(store, it.ObjectNamespace, it.ObjectId, it.Relation, it.SubjectNamespace, it.SubjectId)
		if err != nil {
			results[i] = &pb.CheckResponse{Error: err.Error()}
			continue
		}
		results[i] = &pb.CheckResponse{Allowed: ok, Token: t}
		tok = t
	}
	_, _, _, applied := g.h.Status()
	rev = applied
	return &pb.BatchCheckResponse{Revision: rev, Token: tok, Results: results}, nil
}

func (g *grpcAPI) GetRequestOutcome(_ context.Context, req *pb.GetRequestOutcomeRequest) (*pb.GetRequestOutcomeResponse, error) {
	g.h.MuRLock()
	defer g.h.MuRUnlock()
	rec, ok := g.h.Sessions.Lookup(req.Session, req.Seq)
	if !ok {
		return &pb.GetRequestOutcomeResponse{Found: false}, nil
	}
	return &pb.GetRequestOutcomeResponse{Found: true, Revision: rec.Revision, Result: string(rec.Result)}, nil
}

func (g *grpcAPI) AddLearner(_ context.Context, req *pb.AddLearnerRequest) (*pb.MembershipResponse, error) {
	g.h.MuLock()
	defer g.h.MuUnlock()
	effs := g.h.Node.ProposeAddLearner(runtime.NodeID(req.NodeId))
	g.h.ApplyEffects(effs)
	return &pb.MembershipResponse{Ok: len(effs) > 0}, nil
}

func (g *grpcAPI) ProposeJoint(_ context.Context, req *pb.ProposeJointRequest) (*pb.MembershipResponse, error) {
	voters := make([]runtime.NodeID, len(req.Voters))
	for i, v := range req.Voters {
		voters[i] = runtime.NodeID(v)
	}
	g.h.MuLock()
	defer g.h.MuUnlock()
	effs := g.h.Node.ProposeJoint(voters)
	g.h.ApplyEffects(effs)
	return &pb.MembershipResponse{Ok: len(effs) > 0}, nil
}

func (g *grpcAPI) ProposeFinalize(context.Context, *pb.GetClusterStatusRequest) (*pb.MembershipResponse, error) {
	g.h.MuLock()
	defer g.h.MuUnlock()
	effs := g.h.Node.ProposeFinalize()
	g.h.ApplyEffects(effs)
	return &pb.MembershipResponse{Ok: len(effs) > 0}, nil
}
