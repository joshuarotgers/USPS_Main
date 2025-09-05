#!/usr/bin/env bash
set -euo pipefail

PORT=${PORT:-9099}
BIN=./bin/api
echo "Building API..." >&2
make build >/dev/null

PORT=$PORT "$BIN" >/tmp/api_smoke.log 2>&1 &
PID=$!
cleanup(){ kill $PID >/dev/null 2>&1 || true; }; trap cleanup EXIT

for i in $(seq 1 200); do
  if curl -sS localhost:$PORT/healthz >/dev/null 2>&1; then break; fi
  sleep 0.05
done

hdr=(-H 'X-Tenant-Id: t_demo' -H 'X-Role: admin')

echo '== Health =='
curl -sS localhost:$PORT/healthz | jq .
curl -sS localhost:$PORT/readyz | jq .

echo '== Orders =='
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"tenantId":"t_demo","orders":[{"externalRef":"ORD-1","priority":5,"stops":[{"type":"pickup","location":{"lat":37.7749,"lng":-122.4194}},{"type":"dropoff","location":{"lat":37.7849,"lng":-122.4094}}]}]}' http://localhost:$PORT/v1/orders | jq .
curl -sS "http://localhost:$PORT/v1/orders?limit=5" "${hdr[@]}" | jq .

echo '== Optimize =='
OPTR='{"tenantId":"t_demo","planDate":"2024-09-05","algorithm":"greedy"}'
resp=$(curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data "$OPTR" http://localhost:$PORT/v1/optimize)
echo "$resp" | jq .
RID=$(printf '%s' "$resp" | jq -r '.routes[0].id')
echo "Route ID: $RID"

echo '== Route get/assign/advance =='
curl -sS "http://localhost:$PORT/v1/routes/$RID" "${hdr[@]}" | jq .
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"driverId":"drv1","vehicleId":"veh1"}' "http://localhost:$PORT/v1/routes/$RID/assign" | jq .

echo '== Subscriptions + Webhooks =='
sub=$(curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"tenantId":"t_demo","url":"https://example.invalid/webhook","events":["stop.advanced"],"secret":"shh"}' http://localhost:$PORT/v1/subscriptions)
echo "$sub" | jq .
curl -sS "http://localhost:$PORT/v1/subscriptions" "${hdr[@]}" | jq .

# Trigger event after subscription exists to enqueue a delivery
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{}' "http://localhost:$PORT/v1/routes/$RID/advance" | jq .

# Allow worker to process once
sleep 1.5

echo '== Admin deliveries (should show pending/retries) =='
curl -sS "http://localhost:$PORT/v1/admin/webhook-deliveries?limit=5" "${hdr[@]}" | jq .
echo '== Admin DLQ =='
curl -sS "http://localhost:$PORT/v1/admin/webhook-dlq?limit=5" "${hdr[@]}" | jq .

echo '== Geofences =='
gf=$(curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"name":"Depot","type":"circle","radiusM":200,"center":{"lat":37.77,"lng":-122.42}}' http://localhost:$PORT/v1/geofences)
echo "$gf" | jq .
gid=$(printf '%s' "$gf" | jq -r '.id')
curl -sS "http://localhost:$PORT/v1/geofences" "${hdr[@]}" | jq .
curl -sS "http://localhost:$PORT/v1/geofences/$gid" "${hdr[@]}" | jq .

echo '== HOS =='
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"ts":"2024-09-05T12:00:00Z"}' "http://localhost:$PORT/v1/drivers/drv1/shift/start" | jq .
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"ts":"2024-09-05T13:00:00Z","type":"meal"}' "http://localhost:$PORT/v1/drivers/drv1/breaks/start" | jq .
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"ts":"2024-09-05T13:30:00Z"}' "http://localhost:$PORT/v1/drivers/drv1/breaks/end" | jq .

echo '== Presign =='
curl -sS -X POST -H 'Content-Type: application/json' "${hdr[@]}" --data '{"tenantId":"t_demo","fileName":"proof.jpg","contentType":"image/jpeg"}' http://localhost:$PORT/v1/media/presign | jq .

echo '== Admin metrics =='
curl -sS "http://localhost:$PORT/v1/admin/routes/stats?planDate=2024-09-05" "${hdr[@]}" | jq .
curl -sS "http://localhost:$PORT/v1/admin/plan-metrics?planDate=2024-09-05" "${hdr[@]}" | jq .
# Set custom latency buckets via optimizer config (used by webhook metrics)
curl -sS -X PUT -H 'Content-Type: application/json' "${hdr[@]}" --data '{"config":{"latencyBuckets":[50,200,800]}}' http://localhost:$PORT/v1/admin/optimizer/config | jq .
curl -sS "http://localhost:$PORT/v1/admin/webhook-metrics?sinceHours=24" "${hdr[@]}" | jq .

echo 'Done.'
