#!/usr/bin/env bash
set -euo pipefail

IMAGE="${ANDVARI_TEST_IMAGE:-andvari:test}"
CONTAINER_NAME="andvari-integration-test-$$"
PORT="${ANDVARI_TEST_PORT:-18080}"

cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d \
  --name "$CONTAINER_NAME" \
  -e DEV_MODE=true \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if resp=$(curl -fsS "http://127.0.0.1:${PORT}/healthz" 2>/dev/null); then
    if [[ "$resp" == *'"status":"healthy"'* && "$resp" == *'"version":"dev"'* ]]; then
      curl -fsS "http://127.0.0.1:${PORT}/" | grep -qi "<!doctype html>"
      docker exec "$CONTAINER_NAME" wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/healthz
      echo "Container integration test passed: $resp"
      exit 0
    fi
  fi
  sleep 0.5
done

echo "error: container health check failed or timed out" >&2
docker logs "$CONTAINER_NAME" >&2 || true
exit 1
