# MKK-Luna

## Project Overview
MKK Luna is a team task management backend built in Go with MySQL and Redis. It provides secure authentication, role-based authorization, task lifecycle management, comments, audit history, analytics endpoints, and production-focused operational features.

## Executive Summary
- MKK Luna API is a production-grade Go REST service.
- JWT auth with refresh rotation.
- RBAC (`owner/admin/member`) for teams and tasks.
- Redis-backed hardening (rate limit, idempotency, lockout, blacklist, cache).
- Observability with Prometheus + Grafana.
- Unit + integration + e2e testing.
- QA automation via Makefile targets.

## Architecture at a Glance
The service is organized in layered form: HTTP transport with `chi`, business logic in services, persistence in repositories, and infrastructure adapters for Redis/email/metrics. Security and consistency checks (RBAC, validation, idempotency, lockout, transactional updates, audit writes) are enforced in service and middleware layers.

MySQL is the source of truth for domain data. Redis is used for rate limiting, idempotency, lockout, JWT blacklist checks, and caching. External email delivery for invites is integrated through a circuit-breaker-enabled client (mocked in Docker Compose). Prometheus scrapes metrics and Grafana visualizes them.

```text
Client
  |
HTTP (chi router)
  |
Services (RBAC, validation, transactions)
  |
Repositories (MySQL - source of truth)
  \
   Redis (rate limit, idempotency, lockout, cache)
```

## Tech Stack
- Go
- MySQL
- Redis
- Docker + Docker Compose
- Prometheus + Grafana
- testcontainers-go

## Repository Layout
- `cmd/api` - API entrypoint
- `cmd/migrator` - DB migrator CLI
- `internal/api` - handlers, router, middleware
- `internal/service` - business logic
- `internal/repository` - MySQL access layer
- `internal/infra` - Redis, metrics, email, lockout, idempotency adapters
- `pkg` - shared utilities
- `migrations` - SQL migrations
- `monitoring` - Prometheus/Grafana provisioning
- `tests/integration` - integration tests (testcontainers)
- `tests/e2e` - end-to-end tests

## Prerequisites
- Go toolchain installed
- Docker Desktop (or Docker Engine) running

## Configuration
1. Create env file:
```bash
cp .env.example .env
```

2. Main app config is loaded from `CONFIG_PATH` (default points to `config/local.yaml`).

3. Keep real secrets out of Git; `.env.example` is template-only.

## Quick Start (Docker Compose)
```bash
docker compose up -d --build
```

Main endpoints (default host mappings):
- API: `http://localhost:8080`
- Grafana: `http://localhost:3000`

## API Documentation (Swagger)
- Swagger UI: `http://localhost:8080/swagger/index.html`
- OpenAPI JSON: `http://localhost:8080/static/swagger/swagger.json`

Swagger UI is served from static files mounted into the API container image.

Main API groups:
- Auth
- Teams
- Tasks / Comments / History
- Stats (owner/admin scoped)
- Admin (system_admin only)

## Database Migrations
Migrations are applied automatically by the API container entrypoint during `docker compose up`.

Manual run (local/dev):
```bash
go run ./cmd/migrator up
```

## Authentication & Security
| Mechanism | Protects Against |
|---|---|
| Rate limiting | brute-force, abuse |
| Login lockout | slow brute-force |
| Idempotency | duplicate writes on client retries |
| JWT blacklist | revoked/stolen token reuse |
| Distributed invite lock | invite race duplication |

Rate-limit policy:
- Global auth-required endpoints: `100 req/min per user`.
- Auth-specific limits are configured separately (`login` and `refresh`).

## Redis Degradation Behavior
If Redis is unavailable:

| Feature | Behavior when Redis is unavailable |
|---|---|
| Rate limit | per-instance in-memory fallback |
| Idempotency | bypassed |
| Login lockout | bypassed |
| JWT blacklist | fail-open by default (`configurable`) |
| Stats cache | bypass to DB |
| Invite lock | fallback to DB UNIQUE constraint |

Availability-first policy is used by default.

## Observability
Metrics endpoint:
- API metrics are served on the metrics server address from config (`metrics.addr`, default `:9090`).
- In Docker Compose this port is currently internal-only (not published to host by default).

Dashboards:
- Prometheus: internal Compose service (`http://prometheus:9090` inside network)
- Grafana: `http://localhost:3000`

Key metrics:
- `http_requests_total`
- `redis_degraded_total`
- `email_circuit_state`
- `idempotency_hits_total`

## Testing & Coverage Gate
Unit tests:
```bash
go test ./...
```

Integration tests (requires Docker):
```bash
INTEGRATION=1 go test -tags integration ./tests/integration -v
```

E2E tests:
```bash
E2E=1 go test -tags e2e ./tests/e2e -v
```
Optional Prometheus assertions in E2E:
```bash
E2E=1 E2E_PROM_URL=http://localhost:9092 go test -tags e2e ./tests/e2e -v
```

Makefile shortcuts:
```bash
make test-unit
make test-integration
make qa-cover
make qa-e2e
make qa
```

Coverage policy:
Critical layer target is `>=85%` (unit + integration) for:
- `AuthService`
- `TeamService`
- `TaskService`
- `StatsService`
- Task history logic

## Troubleshooting
- `405 Method Not Allowed` on task update: use `PUT /api/v1/tasks/{id}`.
- Swagger or route mismatch: rebuild API image/container.
- `can't execute 'sh\r'` in container: convert scripts to LF line endings.
- Port conflict (`8080/3000/9090`): change host mapping in `docker-compose.yml` / `.env`.
- Redis connection refused/timeouts: verify `redis` container health.
- `429` under high request rate: expected when per-user rate limits are exceeded.

## Go Template Compliance
| Area | Status | Notes |
|---|---|---|
| `cmd/` entrypoints | Match | `api`, `migrator` |
| `internal/` boundaries | Match | service/repository/infra separation |
| `pkg/` reusable modules | Partial | intentionally small public surface |
| tests structure | Match | unit/integration/e2e separated |
| infra folders | Match | `migrations`, `monitoring`, `config` |

## Known Tradeoffs / Assumptions
- JWT blacklist defaults to fail-open for availability (`configurable`).
- Some resilience scenarios intentionally bypass strict SLA thresholds.
- Analytics and cache strategies are optimized for current project scope, not unlimited data scale.
