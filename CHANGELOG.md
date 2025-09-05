# Changelog

All notable changes to this project will be documented here.

## [0.1.0] - 2025-09-05

### Added
- HTTP smoke script (`scripts/smoke.sh`) and Make targets (`smoke`, `ws-demo`, `run-ws-demo`)
- GraphQL WS demo client (`scripts/ws_client.go`)
- Stateful in-memory webhook queue (deliveries + DLQ), webhook metrics aggregation (memory)
- Basic plan metrics in memory mode to populate Admin endpoints
- CI GitHub Actions smoke workflow
- README: CI badge and helper docs

### Fixed
- Postgres store compile errors (string literal, variable names, demand placeholders)

