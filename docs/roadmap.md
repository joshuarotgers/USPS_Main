# Roadmap

Guiding principles: ship a coherent MVP quickly, then iterate in small, observable steps that improve safety, efficiency, and operability across OTR, regional, and local.

## Milestones

### M0 — Foundation Hardening (Completed/In‑Progress)
- REST API stubs, SSE route events, OpenAPI + Swagger UI.
- In‑memory store; optional Postgres schema & migrations.
- Webhook publisher + worker with retries + DLQ; metrics and dashboards.
- Dispatcher Map at `/map`: geofences CRUD, route list + SSE subscription.
- Driver location SSE: publish `driver.location` on events ingest.

### M1 — Dispatcher & Live Tracking
- Route path API for map polylines (done for Postgres via stops).
- Memory‑mode path fallback (synthesize polyline from legs where possible).
- Latest driver location service (endpoint + cache; Postgres table optional).
- Map improvements: multi‑route overlays, driver clustering, exception badges.
- RBAC tightening: dispatcher/admin on write actions, clear error toasts in UI.

Acceptance:
- `/map` renders at least one route polyline in both memory & Postgres modes.
- Live driver marker moves on location events; last known location endpoint returns recent position.

### M2 — Driver App MVP (Offline‑First)
- Minimal driver web app shell (or native wrapper) leveraging `mobile/local_schema.sql`.
- Sync: pull assigned route + stops; push events and PoD (outbox pattern).
- Navigation handoff via native maps deeplink; return to app on completion.
- PoD capture flow with presign + upload; basic retries.

Acceptance:
- Driver completes a simple two‑stop route with arrive/depart/PoD while offline for part of the flow; data reconciles when back online.

### M3 — Optimizer Expansion (ALNS/VRPTW)
- Hard constraints: time windows, capacity, depot, skill, max route duration.
- HOS‑aware planning with break insertion; freeze semantics for reopt.
- Objectives & weights surfaced via config; iterations/time budget controls.
- Metrics: iterations, improvements, acceptance of worse, cost curves; weight snapshots.

Acceptance:
- Given a realistic order set, planner outputs feasible routes respecting HOS and windows; reopt updates active plan without violating freezes.

### M4 — Integrations & Compliance
- ELD/telematics adapter for HOS/position sync.
- Geocoding/traffic provider integration; ETA pipeline feeding SSE.
- Webhook consumers samples (e.g., Slack/Teams, internal message bus).
- Audit trails, basic RLS policies enabled for Postgres tenants.

Acceptance:
- HOS state aligns with ELD feed; ETAs adjust with traffic; auditors can review changes per tenant.

## Backlog & Nice‑to‑Haves
- Curb‑side notes and walking offsets per stop; parking/geofence guidance.
- Batch PoD uploads; barcode scans; temperature seals.
- Multi‑day OTR trip builder with appointments and rest stops.
- Map styles (MapLibre, custom tiles), dark mode, route animation replays.
- Customer tracking links with secure tokens and live ETA.

## Technical Work Items (near‑term)
- Memory path synthesis and unit tests.
- Latest driver location store: in‑mem map + TTL; optional `driver_locations` table.
- Map UI: “Send Test Location” simulator; multi‑route toggle; exception icons.
- SSE consolidation and backpressure safeguards for large fleets.
- RBAC middleware refinements; auth verifier hardening and config.
- Observability: slow query logging, webhook latency buckets configurable per tenant.

## Risks & Mitigations
- Optimization complexity: keep greedy baseline path; gate ALNS with clear defaults.
- Data quality (geocodes): fallbacks and validation; allow dispatcher corrections.
- Mobile offline edge cases: outbox retry/backoff; idempotent server endpoints.
- Scale: push Redis broker and pagination/testing in staging with synthetic load.

