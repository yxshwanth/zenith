.PHONY: proto generate

proto:
	@echo "Generating protobuf code..."
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		internal/api/zenith.proto
	@echo "Protobuf code generated successfully"

generate: proto

