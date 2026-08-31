#!/usr/bin/env bash
# Start a real N-node zenithd cluster (default 5), elect, put, verify.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
N="${ZENITH_NODES:-5}"
BASE="${ZENITH_CLUSTER_DIR:-/tmp/zenith-cluster-$$}"
rm -rf "$BASE"
mkdir -p "$BASE"
cd "$ROOT"
go build -o "$BASE/zenithd" ./cmd/zenithd

peers_for() {
  local self=$1 out=()
  for j in $(seq 1 "$N"); do
    if [[ "$j" -ne "$self" ]]; then
      out+=("${j}=127.0.0.1:$((17000+j))")
    fi
  done
  local IFS=,
  echo "${out[*]}"
}

PIDS=()
for i in $(seq 1 "$N"); do
  mkdir -p "$BASE/$i"
  "$BASE/zenithd" --dev --id="$i" --data-dir="$BASE/$i" \
    --listen="127.0.0.1:$((17000+i))" \
    --http="127.0.0.1:$((18000+i))" \
    --grpc="127.0.0.1:$((19000+i))" \
    --peers="$(peers_for "$i")" >"$BASE/$i.log" 2>&1 &
  PIDS+=($!)
done
trap 'kill "${PIDS[@]}" 2>/dev/null || true' EXIT

leader=""
for _ in $(seq 1 120); do
  for i in $(seq 1 "$N"); do
    port=$((18000+i))
    role=$(curl -sf "http://127.0.0.1:$port/status" 2>/dev/null | sed -n 's/.*"role":"\([^"]*\)".*/\1/p' || true)
    if [[ "$role" == "leader" ]]; then
      leader=$port
      break 2
    fi
  done
  sleep 0.1
done
if [[ -z "$leader" ]]; then
  echo "no leader elected" >&2
  tail -n 40 "$BASE"/*.log >&2 || true
  exit 1
fi
echo "leader http=:$leader"
curl -sf "http://127.0.0.1:$leader/put?key=hello&val=world"
echo
need=$((N/2+1))
ok=0
for _ in $(seq 1 80); do
  ok=0
  for i in $(seq 1 "$N"); do
    port=$((18000+i))
    got=$(curl -sf "http://127.0.0.1:$port/get?key=hello" | sed -n 's/.*"value":"\([^"]*\)".*/\1/p' || true)
    if [[ "$got" == "world" ]]; then
      ok=$((ok+1))
    fi
  done
  if [[ "$ok" -ge "$need" ]]; then
    echo "cluster-up ok n=$N leader=:$leader applied_on=$ok/$N dir=$BASE"
    exit 0
  fi
  sleep 0.1
done
echo "put not replicated (got $ok/$N need $need)" >&2
tail -n 80 "$BASE"/*.log >&2 || true
exit 1
