# Contributing Guidelines

Thanks for considering a contribution! This repo contains a Go backend API scaffold with smoke tests and a WS demo.

## Prerequisites

- Go 1.21+ (go.mod targets Go 1.23 toolchain)
- Optional: Postgres for DB-backed mode, Redis for Pub/Sub

## Dev Quickstart

- Build: `make build`
- Run: `make run` (defaults to `:8080`)
- Smoke: `make smoke PORT=9099` (runs an end-to-end script)
- WS demo (server already running): `make ws-demo PORT=9099`
- One-shot WS demo (spins server): `make run-ws-demo PORT=9099`

Environment:
- `DATABASE_URL` to enable Postgres store (else memory store is used)
- `REDIS_URL` to enable Redis event broker (else in-memory broker)

## Coding style

- Keep changes focused and minimal; preserve existing style
- Prefer small, composable handlers and clean error handling
- API responses follow problem+json for errors

## Commit messages

- Use conventional commits when possible (`feat:`, `fix:`, `docs:`, `refactor:`, etc.)
- Summaries should be 50 chars or less; body lines wrapped at ~72

## Pull Requests

- Include a brief summary and testing notes
- Update docs/README as needed
- Ensure smoke runs clean locally (`make smoke`)
- Avoid committing secrets/tokens; use `.env` files locally

## Security

- Do not include secrets in code or logs
- Report security issues privately if applicable

## License

- Unless stated otherwise, contributions are under the repo’s license

