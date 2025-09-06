#!/usr/bin/env bash
set -euo pipefail

echo "[pre-push] Running checks (fmt/lint/test${SKIP_SMOKE:+} ${SKIP_SMOKE:+(skip smoke)})"

# 1) Formatting check (no auto-write to avoid modifying staged files)
unformatted=$(gofmt -l . | grep -v '^vendor/' || true)
if [[ -n "$unformatted" ]]; then
  echo "[pre-push] ERROR: Unformatted Go files detected:" >&2
  echo "$unformatted" | sed 's/^/  - /' >&2
  echo "[pre-push] Run: go fmt ./..." >&2
  exit 1
fi

# 2) Lint (golangci-lint if available) + static analysis + unit tests
if [[ "${SKIP_LINT:-}" != "1" ]]; then
  if command -v golangci-lint >/dev/null 2>&1; then
    echo "[pre-push] golangci-lint run"
    golangci-lint run
  else
    echo "[pre-push] golangci-lint not found; running go vet"
    go vet ./...
  fi
fi
go test ./...

# 3) Light end-to-end smoke (can be skipped via SKIP_SMOKE=1)
if [[ "${SKIP_SMOKE:-}" != "1" ]]; then
  PORT=${PORT:-9099} make -s smoke PORT=$PORT
fi

echo "[pre-push] All checks passed."
