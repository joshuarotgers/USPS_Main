APP_NAME=api
PORT?=8083
AUTO_ACCEPT?=true

.PHONY: build run smoke ws-demo run-ws-demo hooks-install hooks-uninstall lint

build:
	go build -o bin/$(APP_NAME) ./cmd/api

run: build
	PORT=$(PORT) ./bin/$(APP_NAME)

# Run the HTTP smoke script against a running instance
smoke: build
	PORT=$(PORT) ./scripts/smoke.sh

# Run the GraphQL WS demo client (expects server on PORT)
ws-demo: build
	PORT=$(PORT) go run ./scripts/ws_client.go

# Start server, run WS demo, then cleanup
run-ws-demo: build
	@set -e; \
	PORT=$(PORT) ./bin/$(APP_NAME) >/tmp/api_ws_demo.log 2>&1 & \
	PID=$$!; \
	cleanup() { kill $$PID >/dev/null 2>&1 || true; }; trap cleanup EXIT; \
	for i in $$(seq 1 200); do curl -sS localhost:$(PORT)/healthz >/dev/null 2>&1 && break; sleep 0.05; done; \
	PORT=$(PORT) go run ./scripts/ws_client.go; \
	kill $$PID >/dev/null 2>&1 || true; \
	wait $$PID >/dev/null 2>&1 || true; \
		echo "WS demo done. Logs: /tmp/api_ws_demo.log";

# Install/uninstall Git hooks
hooks-install:
	@mkdir -p .git/hooks
	@install -m 0755 scripts/pre-push.sh .git/hooks/pre-push
	@echo "Installed pre-push hook. Set SKIP_SMOKE=1 to skip smoke during push."

hooks-uninstall:
	@rm -f .git/hooks/pre-push
	@echo "Removed pre-push hook."

# Local lint (requires golangci-lint in PATH). Falls back to vet if unavailable.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; running go vet instead"; \
		go vet ./...; \
	fi

.PHONY: dev-here dev-afk dev-once
dev-here:
	SMOKE=true bash ./scripts/devloop.sh here

dev-afk:
	SMOKE=true AUTO_ACCEPT=$(AUTO_ACCEPT) bash ./scripts/devloop.sh afk

dev-once:
	SMOKE=true bash ./scripts/devloop.sh once
