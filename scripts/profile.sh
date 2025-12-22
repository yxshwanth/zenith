#!/bin/bash

# Performance profiling script for Zenith

set -e

echo "Zenith Performance Profiling"
echo "============================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Run benchmarks
echo -e "${GREEN}Running benchmarks...${NC}"
echo ""

echo "Service Benchmarks:"
go test -bench=BenchmarkCheck -benchmem ./internal/service

echo ""
echo "Engine Benchmarks:"
go test -bench=BenchmarkExpansion -benchmem ./internal/engine

echo ""
echo "Cache Benchmarks:"
go test -bench=BenchmarkCache -benchmem ./internal/cache

echo ""
echo -e "${YELLOW}CPU Profiling${NC}"
echo "To generate CPU profile, run:"
echo "  go test -cpuprofile=cpu.prof -bench=. ./internal/service"
echo "  go tool pprof cpu.prof"
echo ""

echo -e "${YELLOW}Memory Profiling${NC}"
echo "To generate memory profile, run:"
echo "  go test -memprofile=mem.prof -bench=. ./internal/service"
echo "  go tool pprof mem.prof"
echo ""

echo -e "${YELLOW}Trace Profiling${NC}"
echo "To generate trace, run:"
echo "  go test -trace=trace.out -bench=. ./internal/service"
echo "  go tool trace trace.out"
echo ""

echo -e "${GREEN}Done!${NC}"

