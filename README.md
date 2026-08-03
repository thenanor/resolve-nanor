# Resolve

A small ticketing system — "Resolve v0 Core Tickets" — built in 
**Go (chi + pgx) backend** and a **React (Vite + TypeScript) frontend**.

## Stack

- `backend/` — Go 1.26, [chi](https://github.com/go-chi/chi) router,
  [pgx](https://github.com/jackc/pgx) (no ORM), Postgres 16.
- `frontend/` — Vite + React + TypeScript, react-router-dom, plain
  `fetch` (no state library).

## Quick start

```bash
make help    # list every available target
```

## Run everything with Docker (recommended)

```bash
make up                   # Postgres 16 + the Go app + the frontend
curl localhost:3000/stats
open http://localhost:5173
```

Port 3000 or 5173 busy? `APP_PORT=3300 make up` / `FRONTEND_PORT=8080 make up`.
Config is env-driven — `cp .env.example .env` to override ports or
database credentials (never commit `.env`). Data lives in the
`pgdata` volume — it survives restarts and rebuilds. Reset everything:
`make down-v`.

The frontend container is a production build (`vite build`) served by
nginx, which proxies `/api` to the `app` service — there's no hot
reload here. For that, use the local dev server below.

Other useful targets: `make down`, `make ps`, `make logs`.

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

## Test & lint

```bash
make test                 # Go unit tests (fake repository, no DB) + frontend unit tests (Vitest)
make test-backend          # just the Go tests
make test-frontend         # just the Vitest tests
make vet                   # go vet
make lint                   # golangci-lint (backend) + oxlint (frontend)
make typecheck-frontend    # tsc -b --noEmit

make all                    # build backend/api + frontend/dist (no Docker)
make build-backend          # just the Go binary
make build-frontend         # just the production frontend bundle

make ci                     # everything CI runs, in one shot
```

Frontend unit tests (Vitest + React Testing Library) cover the pure
status-transition map, the actor-storage logic in the API client, and
badge component rendering — see `frontend/src/**/*.test.{ts,tsx}`.

## CI

`.github/workflows/ci.yml` runs on every pull request: backend build,
`go vet`, `go test`, and `golangci-lint`; frontend `oxlint`, Vitest,
a TypeScript typecheck, and a production build.

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
        ↑     ↑        ↓       │
        └─────┴────────┘       │
           waiting_customer    │
        ↑                      │
        └── reopen ────────────┘
```

A resolved ticket can be reopened back to `open` (e.g. the customer replies with a
new problem); `resolvedAt` is cleared until it's resolved again. `closed` remains
terminal.

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
