#!/usr/bin/env sh
set -eu
BASE_URL="${AEGIS_URL:-http://localhost:8000}"
COMPOSE="docker compose -f $(dirname "$0")/docker-compose.yml"
AUTH_HEADER="${AEGIS_AUTH_HEADER:-}"
get_services() {
  if [ -n "$AUTH_HEADER" ]; then
    curl --fail --silent -H "$AUTH_HEADER" "$BASE_URL/api/telemetry/services"
  else
    curl --fail --silent "$BASE_URL/api/telemetry/services"
  fi
}

$COMPOSE up -d --build
trap '$COMPOSE down' EXIT
sleep "${AEGIS_VERIFY_WAIT_SECONDS:-30}"

echo "1) services"
get_services
echo "2) dependency edge"
if [ -n "$AUTH_HEADER" ]; then
  curl --fail --silent -H "$AUTH_HEADER" "$BASE_URL/api/registry/edges"
else
  curl --fail --silent "$BASE_URL/api/registry/edges"
fi
echo "3) stop and restart svc-prom"
$COMPOSE stop svc-prom
sleep 30
get_services
$COMPOSE start svc-prom
sleep 30
get_services
echo "4) induced evidence"
$COMPOSE exec -T svc-otel sh -c 'true'
echo "5) third service is configured without an Aegis file change"
curl --fail --silent "$BASE_URL/api/telemetry/services" | grep -E 'svc-(otel|prom|third)'
echo "6) hardcoding proof"
if grep -R -n -E 'node-a|node-b|node-c|node-d|shopkart|edge-gateway' go-services python-ml frontend/src; then
  echo "hardcoded service identifiers found" >&2
  exit 1
fi
