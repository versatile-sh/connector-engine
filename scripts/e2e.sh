#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")/.."
start=$(date +%s)
go test -race -count=1 -timeout 10m -v ./e2e/ "$@" | grep -E '^(\s*--- |ok|FAIL|PASS|\s+\S+_test.go)|running on'
status=${PIPESTATUS[0]}
echo "e2e finished in $(( $(date +%s) - start ))s (exit $status)"
exit "$status"
