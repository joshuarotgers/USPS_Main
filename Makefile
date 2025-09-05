APP_NAME=api
PORT?=8080

.PHONY: build run smoke ws-demo run-ws-demo

build:
	go build -o bin/$(APP_NAME) ./cmd/api

run: build
	./bin/$(APP_NAME)

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
