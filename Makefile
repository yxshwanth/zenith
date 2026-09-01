.PHONY: proto generate sim-smoke sim-sweep sim-corpus test-v2 test-race-v2 cluster-up

proto:
	@echo "Generating protobuf code..."
	@mkdir -p api/zenith/v2/pb
	@protoc --proto_path=api/zenith/v2 \
		--go_out=api/zenith/v2/pb --go_opt=paths=source_relative \
		--go-grpc_out=api/zenith/v2/pb --go-grpc_opt=paths=source_relative \
		api/zenith/v2/zenith.proto
	@echo "Protobuf code generated successfully"

generate: proto

sim-smoke:
	go test ./internal/sim -run 'TestSmokeTraceHashIdentical|TestSchedulerDeterministicReplay' -count=2
	go test ./internal/replica -run 'TestElectAndPut|TestCrashRestartPreservesSyncedPut|TestNewEnemy' -count=1
	go run ./cmd/zenith-sim run --seed 42 --profile elect-put

sim-sweep:
	ZENITH_SWEEP=$${ZENITH_SWEEP:-1000} go test ./internal/replica -run TestExploreSeedSweep -count=1 -timeout 15m

sim-corpus:
	go test ./internal/replica -run TestRegressionCorpus -count=1
	@for f in testdata/regressions/*.json; do go run ./cmd/zenith-sim replay --trace "$$f"; done

test-v2:
	go test ./internal/runtime/... ./internal/sim/... ./internal/checker/... \
		./internal/raft/... ./internal/wal/... ./internal/session/... \
		./internal/replica/... ./internal/snapshot/... ./internal/mvcc/... \
		./internal/authz/... ./internal/token/... ./internal/coordinator/... \
		./internal/deccache/... ./internal/nodehost/... \
		./examples/content-service/... ./api/zenith/v2/... -count=1

test-race-v2:
	go test -race ./internal/raft/... ./internal/replica/... ./internal/wal/... -count=1

cluster-up:
	bash scripts/cluster-up.sh
