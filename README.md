# Universal GPS Navigation & Delivery Optimization Platform

[![Smoke](https://github.com/joshuarotgers/USPS_Main/actions/workflows/smoke.yml/badge.svg)](https://github.com/joshuarotgers/USPS_Main/actions/workflows/smoke.yml)

Note: the badge links to this repository's Smoke workflow.

This repository contains a foundational scaffold for a universal GPS navigation and delivery optimization platform designed for postal, courier, food, parcel, and freight operations.

- Backend: Go (HTTP API, stubs for core endpoints)
- Specs: OpenAPI (REST writes), GraphQL (reads)
- Data: Postgres schema migration (multi-tenant), SQLite mobile schema
- Optimization: VRPTW/ALNS skeleton for route planning
- Integrations: Adapter interface + minimal CSV/SFTP example
- Docs: Events taxonomy, policy rules sample

## Structure

- `openapi/openapi.yaml` — REST API spec (core flows)
- `graphql/schema.graphql` — Query schema for reads
- `db/migrations/001_init.sql` — Initial Postgres schema
- `cmd/api/main.go` — API server entrypoint (Go)
- `internal/` — Packages for api, models, optimization, integrations, etc.
- `mobile/local_schema.sql` — Offline-first SQLite schema for driver app
- `docs/` — Events taxonomy and policy examples

## Quick Start (Dev)

- Requirements: Go 1.21+ (no external deps for stubs)
- Build: `make build`
- Run: `make run` (serves on `:8080`)
  - Optional: set `DATABASE_URL` for Postgres (e.g., `postgres://user:pass@localhost:5432/dbname`); otherwise uses in-memory store.

Dev helpers:
- Smoke (HTTP): `make smoke PORT=9099` — builds, runs a quick end-to-end script hitting core endpoints, webhooks, and admin views.
- WS demo: `make ws-demo PORT=9099` — runs a small GraphQL WS client that subscribes to `routeEvents` and prints frames (expects server running on PORT).
- One-shot WS demo: `make run-ws-demo PORT=9099` — starts the server, runs the WS client, then cleans up.

## Docker

- Build image: `docker build -t gpsnav-api .`
- Run: `docker run --rm -p 8080:8080 -e PORT=8080 gpsnav-api`

### Docker Compose (with Postgres + Redis)

- Start stack: `docker compose up --build`
 - API available at `http://localhost:8081` (compose maps host 8081 -> container 8080)
- Adjust env in `compose.yaml` as needed (DB/Redis URLs, PORT).

Optional services included:
- Prometheus at `http://localhost:9090` (scrapes API `/metrics`)
- Grafana at `http://localhost:3000` (admin/admin by default)
  - Datasource pre-provisioned to Prometheus
  - Dashboards auto-imported:
    - Metrics: `grafana/dashboard-api.json`
    - Logs: `grafana/dashboard-logs.json`
  - After `docker compose up`, open Grafana and verify dashboards.
- Loki + Promtail for logs:
  - Loki at `http://localhost:3100`
  - Promtail tails Docker container logs and ships to Loki (config in `configs/promtail-config.yml.example`)
  - Add a Log panel in Grafana using the `Loki` datasource to explore logs.

### CORS, Rate Limiting, and Metrics

- CORS: set `ALLOW_ORIGINS` (comma-separated) or `*` to allow all.
- Rate limit: per-IP with token bucket
  - `RATE_RPS` (default 20), `RATE_BURST` (default 40)
- Metrics: Prometheus endpoint at `GET /metrics` (Prometheus format)

Prometheus scrape example (`configs/prometheus.yml.example`):

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'gpsnav-api'
    static_configs:
      - targets: ['api:8080']   # or 'localhost:8080' if running locally
```

Grafana dashboard JSON is provided at `grafana/dashboard-api.json` — import it into Grafana and point it at your Prometheus datasource.

## Systemd

Sample unit and env file in `deploy/systemd/`:
- Copy binary to `/usr/local/bin/api`
- Create data dir: `/var/lib/gpsnav` (owned by `www-data`)
- Copy `deploy/systemd/api.service` to `/etc/systemd/system/api.service`
- Copy `deploy/systemd/api.env.example` to `/etc/gpsnav/api.env` and edit
- Enable + start: `sudo systemctl enable --now api`

## License

MIT — see `LICENSE`.

## CI Overview

- Smoke: runs an end-to-end HTTP script on pushes/PRs.
- Tests: builds and runs `go test ./...` on pushes/PRs to `main`.
- Releases: tag `v*` builds binaries for Linux/macOS/Windows and uploads to Releases.
- Docker Publish: tag `v*` builds multi-arch images and pushes to Docker Hub.
- Postgres Integration (opt-in):
  - Run manually from Actions (workflow_dispatch), or for PRs labeled `pg-integration`, or on tag pushes `v*`.
  - Locally: `go test -tags postgres_integration ./internal/store` with `DATABASE_URL`.

## Configuration Reference

- Core:
  - `PORT`: HTTP listen port (default 8080)
  - `DATABASE_URL`: Postgres DSN to enable DB-backed store (if unset, in-memory)
  - `DB_MIGRATE`: set to `false` to skip auto-migrations (or CLI `-migrate=false`)
  - `REDIS_URL`: Redis URL to enable cross-process event broker (optional)
- Auth:
  - `AUTH_MODE`: `dev` | `hmac` | `jwks`
  - `AUTH_HMAC_SECRET`, `AUTH_JWKS_URL`, `AUTH_TENANT_CLAIM`, `AUTH_ROLE_CLAIM`, `AUTH_DRIVER_CLAIM`
- Webhooks:
  - `WEBHOOK_MAX_ATTEMPTS`: max retries before DLQ
- CORS/Rate limit:
  - `ALLOW_ORIGINS`: `*` or comma-separated origins
  - `RATE_RPS`, `RATE_BURST`: per-IP limits

Endpoints (stubbed):
- `POST /v1/orders` — bulk import orders
- `POST /v1/optimize` — plan/replan routes
- `GET /v1/routes/{id}` — fetch route details
- `POST /v1/routes/{id}/assign` — assign driver/vehicle
- `PATCH /v1/routes/{id}` — update route (If-Match style)
- `POST /v1/routes/{id}/advance` — auto/manual advance to next stop
- `GET /v1/routes/{id}/events/stream` — route events SSE
- `POST /v1/driver-events` — ingest driver/location events
- `POST /v1/pod` — upload Proof of Delivery metadata
- `POST /v1/subscriptions` — configure webhooks
- `GET /v1/eta/stream` — SSE ETA updates (demo)
- `POST /v1/drivers/{driverId}/shift/start|end` — driver shift control
- `POST /v1/drivers/{driverId}/breaks/start|end` — break control
- `GET/POST /v1/geofences` and `GET/PATCH/DELETE /v1/geofences/{id}` — geofence CRUD
- `POST /v1/media/presign` — request presigned URL for PoD media
- `/healthz`, `/readyz` — health probes

This is a scaffold, not production-ready. Extend services, storage, auth, and optimization per the design.
