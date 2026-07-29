# Resolve

A small ticketing system — "Resolve v0 Core Tickets" — ported to a
**Go (chi + pgx) backend** and a **React (Vite + TypeScript) frontend**,
from an original NestJS + TypeORM reference implementation. Behavior
(validation rules, status machine, audit trail, stats) is preserved
1:1 with the reference — see
`backend/internal/tickets/service_test.go` for the behavioral contract.

## Stack

- `backend/` — Go 1.25, [chi](https://github.com/go-chi/chi) router,
  [pgx](https://github.com/jackc/pgx) (no ORM), Postgres 16.
- `frontend/` — Vite + React + TypeScript, react-router-dom, plain
  `fetch` (no state library).

## Quick start

```bash
make help    # list every available target
```

## Run everything with Docker (recommended)

```bash
make up                   # Postgres 16 + the Go app
curl localhost:3000/stats
```

Port 3000 busy? `APP_PORT=3300 make up`.
Config is env-driven — `cp .env.example .env` to override ports or
database credentials (never commit `.env`). Data lives in the
`pgdata` volume — it survives restarts and rebuilds. Reset everything:
`make down-v`.

> Note: if your Docker install's buildkit fails with a bind-mount
> error under WSL, use `make up-legacy` instead (buildkit disabled).

Other useful targets: `make down`, `make ps`, `make logs`.

Then, separately, run the frontend dev server (see below) — it isn't
containerized in v0.

## Run locally (dev)

**Backend**
```bash
make db         # just the database, in Docker
make backend    # go run ./cmd/api, listens on :3000
```

**Frontend**
```bash
make install    # npm install
make frontend   # npm run dev — http://localhost:5173, proxies /api -> :3000
```

## Test

```bash
make test                 # Go unit tests against a fake in-memory repository, no DB needed
make vet                   # go vet
make typecheck-frontend    # tsc -b --noEmit
make build-frontend        # production build
```

## Endpoints (v0)

- `POST /tickets` — `{ subject, description, customerEmail, priority }`
  (priority: `low | normal | high | urgent`)
- `GET /tickets?status=&priority=` — list (filterable)
- `GET /tickets/:id` — one ticket, including comments
- `POST /tickets/:id/status` — `{ "to": "open" | ... }` (whitelisted
  transitions; illegal moves → 400 listing allowed next states)
- `POST /tickets/:id/comments` — `{ author, body, internal }`
  (`internal: true` = agent-only note; never exposed to customers)
- `GET /audit` — every mutation, with actor (from `X-Actor` header)
- `GET /stats` — counts by status/priority + average resolution minutes

## Status machine

```
new → open → in_progress → resolved → closed
              ↑        ↓
           waiting_customer
```

## Frontend pages

- **Tickets** — list with status/priority filters
- **New Ticket** — create form
- **Ticket detail** — status transition buttons (only legal next states
  enabled), comment thread (internal notes visually distinct), add-comment
  form
- **Audit Log** — global or per-ticket (`?ticketId=`) mutation history
- **Stats** — totals, breakdowns by status/priority, average resolution time

The "Acting as" field in the nav bar sets the `X-Actor` header sent with
every request (defaults to `api`).
