# Universal GPS Navigation & Delivery Optimization Platform

[![Smoke](https://github.com/OWNER/REPO/actions/workflows/smoke.yml/badge.svg)](https://github.com/OWNER/REPO/actions/workflows/smoke.yml)

Note: replace OWNER/REPO in the badge URL with your GitHub repo slug.

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
