// Command zenithctl talks to a zenithd gRPC endpoint.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yxshwanth/zenith/api/zenith/v2/pb"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9001", "gRPC address")
	authToken := flag.String("auth-token", "", "cluster auth token (or ZENITH_AUTH_TOKEN)")
	tlsCA := flag.String("tls-ca", "", "PEM CA / server cert for TLS")
	insec := flag.Bool("insecure", false, "plaintext (loopback only)")
	flag.Parse()
	if *authToken == "" {
		*authToken = os.Getenv("ZENITH_AUTH_TOKEN")
	}
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: zenithctl [-addr host:port] status|check|write|list ...")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dialOpts, err := clientDialOpts(*addr, *tlsCA, *insec, *authToken)
	if err != nil {
		logFatal(err)
	}
	conn, err := grpc.NewClient(*addr, dialOpts...)
	if err != nil {
		logFatal(err)
	}
	defer conn.Close()
	c := pb.NewZenithClient(conn)
	switch args[0] {
	case "status":
		st, err := c.GetClusterStatus(ctx, &pb.GetClusterStatusRequest{})
		must(err)
		fmt.Printf("%+v\n", st)
	case "check":
		// check store ons oid rel sns sid
		need(args, 7)
		resp, err := c.Check(ctx, &pb.CheckRequest{
			Store: args[1], ObjectNamespace: args[2], ObjectId: args[3], Relation: args[4],
			SubjectNamespace: args[5], SubjectId: args[6],
		})
		must(err)
		fmt.Printf("allowed=%v token=%s err=%s\n", resp.Allowed, resp.Token, resp.Error)
	case "write":
		// write store session seq ons oid rel sns sid
		need(args, 9)
		var seq uint64
		fmt.Sscanf(args[3], "%d", &seq)
		resp, err := c.WriteTuples(ctx, &pb.WriteTuplesRequest{
			Store: args[1], Session: args[2], Seq: seq, RequestId: args[2] + "-" + args[3],
			Tuples: []*pb.Tuple{{
				ObjectNamespace: args[4], ObjectId: args[5], Relation: args[6],
				SubjectNamespace: args[7], SubjectId: args[8],
			}},
		})
		must(err)
		fmt.Printf("rev=%d token=%s err=%s\n", resp.Revision, resp.Token, resp.Error)
	case "list":
		need(args, 5)
		resp, err := c.ListSubjects(ctx, &pb.ListSubjectsRequest{
			Store: args[1], ObjectNamespace: args[2], ObjectId: args[3], Relation: args[4],
		})
		must(err)
		fmt.Printf("complete=%v rev=%d n=%d err=%s\n", resp.Complete, resp.Revision, len(resp.Subjects), resp.Error)
		for _, s := range resp.Subjects {
			fmt.Printf("  %s:%s\n", s.SubjectNamespace, s.SubjectId)
		}
	default:
		logFatal(fmt.Errorf("unknown command %q", args[0]))
	}
}

func need(args []string, n int) {
	if len(args) < n {
		logFatal(fmt.Errorf("need %d args", n))
	}
}
func must(err error) {
	if err != nil {
		logFatal(err)
	}
}
func logFatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func clientDialOpts(addr, caFile string, insec bool, token string) ([]grpc.DialOption, error) {
	host, _, _ := net.SplitHostPort(addr)
	loopback := host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	var creds credentials.TransportCredentials
	switch {
	case caFile != "":
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certs in %s", caFile)
		}
		creds = credentials.NewTLS(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	case insec && loopback:
		creds = insecure.NewCredentials()
	case loopback:
		creds = insecure.NewCredentials()
	default:
		return nil, fmt.Errorf("need --tls-ca or loopback --insecure")
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(tokenRPC{token: token, insecure: insec || loopback && caFile == ""}))
	}
	return opts, nil
}

type tokenRPC struct {
	token    string
	insecure bool
}

func (t tokenRPC) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + t.token}, nil
}
func (t tokenRPC) RequireTransportSecurity() bool { return !t.insecure }
