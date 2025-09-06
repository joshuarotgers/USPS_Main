# Universal GPS Navigation & Delivery Optimization — Vision

This document describes the product vision for a universal GPS navigation and delivery optimization platform that streamlines operations for over‑the‑road (OTR), regional, and local delivery businesses.

## Problem & Audience
- Shippers, 3PLs, fleets, and couriers struggle to coordinate orders, routes, drivers, and proof workflows across varied service levels and regulations.
- Dispatchers need real‑time situational awareness and quick re‑planning. Drivers need simple, reliable navigation with offline resilience. Admins need compliance, cost control, and performance insights.

## Product Goals
- One platform spanning OTR, regional, and local last‑mile use cases.
- Safe, efficient routing with real‑time adjustments and compliance (HOS, time windows, depot constraints).
- Delightfully simple driver experience with offline support and turn‑by‑turn navigation.
- Extensible: adapters for TMS/WMS/ELD/telematics, webhook events, analytics.
- Secure multi‑tenant foundation with clear RBAC and auditability.

## Personas
- Dispatcher: plans and replans routes, monitors exceptions, communicates with drivers.
- Driver: executes stops, captures PoD, follows optimized navigation, reports events.
- Admin/Ops: configures policies (HOS, priorities), reviews KPIs, manages tenants/users.
- Customer (optional): receives service updates and ETAs.

## Key Capabilities
- Route Planning (VRPTW / ALNS skeleton):
  - Time windows, service times, vehicle capacities, depot constraints.
  - OTR/regional: HOS‑aware planning, break insertion, appointment times.
  - Local: dense stop sequencing, reopt triggers, curb/parking notes.
- Real‑Time Operations:
  - SSE/WebSocket events for route updates, ETAs, exceptions, policy alerts.
  - Dispatcher map with route overlays, geofences, driver locations.
- Navigation & Driver App:
  - Offline‑first data sync (SQLite schema provided), PoD capture, simple UX.
  - Turn‑by‑turn via native maps (deeplinks) with return to app.
- Compliance & Policy:
  - HOS shift/break tracking, auto‑advance policies, policy alerts.
- Integrations & Extensibility:
  - Webhook subscriptions, CSV/SFTP adapter sample, open APIs (OpenAPI + GraphQL reads).
- Observability:
  - Prometheus metrics, Grafana dashboards, logs via Loki/Promtail.

## End‑to‑End Flows
- Orders → Optimize → Assign → Navigate → Events/ETA → PoD → Analytics.
- Reopt on exceptions (late, traffic, failure) with freeze controls.
- Webhooks to downstream systems for status changes and policy alerts.

## Architecture Overview
- Backend (Go): REST + SSE + GraphQL WS bridge; memory store (dev) or Postgres.
- Event Broker: in‑proc or Redis; webhook publisher and worker with retries + DLQ.
- Map UI: Leaflet‑based dispatcher map at `/map` with geofences, routes, SSE.
- Driver App: offline schema (`mobile/local_schema.sql`), planned native shell.
- Metrics & Admin: Prometheus `/metrics`, Grafana provisioning, admin endpoints.

## Data Model (selected)
- Orders & Stops: priorities, locations, time windows, skills.
- Routes & Legs: status, ETAs, cost breakdown, auto‑advance policy.
- Geofences: circle/polygon (polygon optional; PostGIS ready), rules.
- Events: driver/location/arrive/depart/exception/pod, webhooks, DLQ.

## Security & Multi‑Tenancy
- Tenants separated in DB schema; RLS recommended for production.
- Auth modes: dev/hmac/jwks (extensible). RBAC: admin/dispatcher/driver/customer.
- CORS and simple per‑IP rate limiting; request IDs and access logs.

## KPIs & Outcomes
- On‑time performance (OTP), first‑attempt success rate, dwell/detention, miles/time per delivery.
- Reoptization efficiency, webhook success latency, driver utilization, break compliance.

## Current Status (scaffold)
- Implemented: core endpoints, SSE route events, dispatcher map, geofences CRUD, basic optimizer stubs, webhooks infra, metrics/dashboards.
- Gaps: robust optimizer, full route geometry, driver app shell, richer RBAC, deep integrations.

