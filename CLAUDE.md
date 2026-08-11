# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Resolve — a small ticketing system ("Resolve v0 Core Tickets"): a **Go
(chi + pgx) backend** and a **React (Vite + TypeScript) frontend**. The Go
backend is a deliberate port of an earlier NestJS/TypeORM reference
implementation — several comments in the code (`// mirrors the NestJS ...`)
call out where a design choice exists specifically to match that reference's
behavior byte-for-byte (JSON shapes, error messages, id format). Keep that in
mind when a design looks unusual for idiomatic Go: it's often intentional
parity, not an oversight.

## Commands

```bash
make help                 # list every available target
```

**Docker (recommended for running the whole stack):**
```bash
make up                   # Postgres 16 + Go app + frontend, build+start
make down                 # stop (keeps data)
make down-v               # stop and delete the Postgres volume
make logs                 # follow logs
```
The frontend container is a production build served by nginx (proxies `/api`
→ `app`) — no hot reload. Use local dev below for that.

**Local dev:**
```bash
make db                   # just Postgres, in Docker
make backend              # go run ./cmd/api  (listens on :3000)
make install               # npm install (frontend)
make frontend              # npm run dev (http://localhost:5173, proxies /api -> :3000)
```

**Test / lint / typecheck:**
```bash
make test                  # backend (fake repo, no DB) + frontend (Vitest)
make test-backend          # cd backend && go test ./...
make test-frontend         # cd frontend && npm test
make vet                   # go vet
make lint                  # golangci-lint (backend) + oxlint (frontend)
make typecheck-frontend    # tsc -b --noEmit
make ci                    # everything CI runs, in one shot (vet lint test typecheck-frontend build-frontend)
```
Run a single Go test: `cd backend && go test ./internal/tickets/ -run TestName`.
Run a single Vitest file: `cd frontend && npx vitest run src/lib/transitions.test.ts`.

**Build:**
```bash
make all                   # backend binary + frontend/dist
make build-backend         # backend/api
make build-frontend        # frontend/dist
```

Config is env-driven: `cp .env.example .env` to override ports/DB creds
(never commit `.env`). Backend env vars: `PORT`, `DATABASE_URL`, `VERSION`
(see `backend/internal/config/config.go`).

## Architecture

**Backend** (`backend/internal/`), one package per bounded concern, each
following the same shape — `ticket.go`/entity, `repository.go` (interface),
`postgres_repository.go` (impl), `service.go` (business rules), `handler.go`
(chi routes), wired together in `cmd/api/main.go`:

- `tickets/` — the core domain. `service.go` owns all validation and
  business rules (email format, status transitions, pagination bounds);
  `transitions.go` defines the single allowed-status-transition map
  (`AllowedTransitions`) that both the handler and tests rely on.
  `handler.go` is thin — decode, call service, map errors to HTTP status via
  `respondIfError`.
- `audit/` — append-only mutation log. `tickets.Service` depends on it
  through a locally-declared `AuditRecorder` interface (not `audit.Service`
  directly) specifically to avoid an import cycle — the dependency points
  from the consumer, not the provider.
- `stats/`, `health/`, `docs/` — smaller, self-contained handlers, mounted
  the same way in `main.go`.
- `cannedresponses/` — reusable comment text (title/body/tags) agents can
  insert into a ticket comment; `cr_` ids. Deleting one never mutates
  ticket comments seeded from it (see `.claude/specs/canned-responses.md`).
- `migrations/` — `//go:embed schema.sql`, applied at startup
  (`applySchema` in `main.go`). No migration tool: this is a v0
  convenience mirroring TypeORM's `synchronize: true`.
- `httpx/` — shared HTTP helpers: `WriteJSON`/`WriteError` envelopes and
  `Actor(r)`, which reads the `X-Actor` header (default `"api"`) used
  everywhere audit entries need an actor.
- `ids/` — id generation (`tkt_a1b2c3d4` style: prefix + 8 hex chars),
  format matches the NestJS reference.

Error handling pattern: services return typed `*ValidationError` /
`*NotFoundError` (see `tickets/errors.go`); handlers translate those via
`errors.As` in `respondIfError` — add new error types there rather than
returning raw `fmt.Errorf` from a service if a handler needs to distinguish
them.

**Status machine** (`tickets/transitions.go`, mirrored in
`frontend/src/lib/transitions.ts`):
```
new → open → in_progress → resolved → closed
              ↑        ↓
           waiting_customer
```
The backend transition map is the sole source of truth; the frontend copy
only disables buttons for moves that would be rejected anyway. When you
change one, change the other, and check `docs/reopening-tickets-comparison.md`
for prior art on this exact class of change (adding a reopen path) — it
compares three different past implementations and their trade-offs.

**Frontend** (`frontend/src/`): plain `fetch`, no state library. All API
calls go through `api/client.ts`, which injects `X-Actor` (from
`localStorage`, settable via the "Acting as" field in the nav) and unwraps
the `{message}` error envelope into `ApiRequestError`. Pages live in
`pages/`, one per route wired in `App.tsx` (canned responses split a
picker, for inserting into a comment, from a manager, for CRUD).

**Pagination**: `GET /tickets` returns a page envelope
(`{tickets, limit, offset, hasMore}`), not a bare array — `HasMore` is
computed by fetching one row past `limit` rather than a `COUNT(*)`, so cost
stays flat regardless of page depth (see `tickets/repository.go` comments
and `docs/pagination-verdict.md` for why this shape won over a simpler
client-side-only alternative).

**API docs**: Swagger UI at `/docs`, spec at `/openapi.yaml`, checked in at
`backend/internal/docs/openapi.yaml` — keep it in sync when changing request/
response shapes.

## Working conventions in this repo

- A `PreToolUse` hook (`.claude/hooks/protect-audit.js`) blocks
  Edit/Write/MultiEdit on any file path containing `"audit"` — the audit
  trail is intentionally locked down. Don't route around it (shell
  redirection, `sed` via Bash, etc.) if you hit it; treat the block as
  deliberate, per `docs/hook-applied.md`.
- `terraform/` provisions a standalone EC2 host for deploying the compose
  stack; it's infra, not application code — see `terraform/README.md`
  before touching it. State files and `.terraform/` are gitignored.
- `.claude/commands/feature.md` defines a `/feature` workflow
  (explore → plan → implement → test → summary) for building features
  end-to-end in this repo.
- New bounded-concern features are specced first: `.claude/specs/*.md`
  holds numbered acceptance criteria (see `canned-responses.md`) that
  double as a TDD contract before implementation, cross-checked against
  the resulting tests by `.claude/commands/coverage-gaps.md`.
