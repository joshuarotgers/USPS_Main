#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://localhost:8081}
echo "Health:"
curl -sS "$BASE/healthz" | jq . || curl -sS "$BASE/healthz" || true
echo
echo "Docs head:"
curl -sS -I "$BASE/docs" | sed -n '1,5p'
echo
echo "Metrics sample:"
curl -sS "$BASE/metrics" | head -n 10 || true
echo
echo "Optimize demo:"
curl -sS -X POST -H 'Content-Type: application/json' -H 'X-Role: admin' --data '{"tenantId":"t_demo","planDate":"2025-09-05","algorithm":"greedy"}' "$BASE/v1/optimize" | jq . || true

