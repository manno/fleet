#!/bin/bash
set -e

date=$(date +"%F_%T")
out="b-$date.json"
FLEET_BENCH_REPORT=${FLEET_BENCH_DB-$out}
FLEET_BENCH_DB=${FLEET_BENCH_DB-"benchmarks/db"}
FLEET_BENCH_TIMEOUT=${FLEET_BENCH_TIMEOUT-"5m"}
FLEET_BENCH_NAMESPACE=${FLEET_BENCH_NAMESPACE-"fleet-local"}

export FLEET_BENCH_TIMEOUT
export FLEET_BENCH_NAMESPACE

n=$(kubectl get clusters.fleet.cattle.io -n "$FLEET_BENCH_NAMESPACE" -l fleet.cattle.io/benchmark=true  -ojson | jq '.items | length')
if [ "$n" -eq 0 ]; then
  echo "No clusters found to benchmark"
  echo "You need at least one cluster with the label fleet.cattle.io/benchmark=true"
  exit 1
fi

date=$(date +"%F_%T")
out="b-$date.json"
ginkgo run --fail-fast --json-report "$FLEET_BENCH_REPORT" ./benchmarks

go run benchmarks/report/main.go report -d benchmarks/db -i "$FLEET_BENCH_REPORT"
