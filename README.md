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
- API available at `http://localhost:8080`
- Adjust env in `compose.yaml` as needed (DB/Redis URLs, PORT).

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
