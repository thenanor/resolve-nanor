# Triage service: AI ticket categorization + prioritization

## Context

Resolve tickets currently carry a user-chosen `priority` and no `category` at
all. The goal is a new **`triage` service** — a genuinely separate process
("beside" the main app, per your framing), triggered whenever a ticket is
created, that calls the Claude API to classify the ticket into one of six
categories (`billing`, `bug`, `account_access`, `feature_request`, `how_to`,
`other`) and re-derive its priority (`low`, `normal`, `high`, `urgent`,
reusing the existing `Priority` enum), then writes both back onto the ticket.

Confirmed decisions driving this plan:
- **Model:** Claude Haiku 4.5 (`claude-haiku-4-5`) — cheap/fast, appropriate
  for a mechanical enum-classification call that fires on every ticket.
- **Priority:** a **high-confidence** AI result overwrites the ticket's
  `priority` field (no separate "suggested priority" column in that case).
  Category is new either way.
- **Confidence-gated human review:** the model also returns a `confidence`
  (`low`/`medium`/`high`) alongside its category/priority call. `medium`/
  `high` results auto-apply as above. `low`-confidence results are **not**
  applied automatically — they're stored as a pending suggestion for a human
  to accept or reject, so an uncertain classification never silently
  overwrites a ticket's priority.
- **Trigger:** the main app POSTs to the triage service **fire-and-forget**
  from a goroutine spawned in `tickets.Service.Create`, so ticket-creation
  latency is unaffected by LLM round-trip time.
- **Write-back:** the triage service is stateless (no DB). It classifies via
  Claude, then PATCHes the result back to the main app's HTTP API using the
  existing `X-Actor` convention — so the write goes through
  `tickets.Service` and reuses the existing audit-trail plumbing instead of
  a second, duplicate DB-write path.
- **Structured output:** a single forced tool call (`tool_choice` pinned to
  one tool) with `category`/`priority`/`confidence` as JSON Schema enums —
  more reliable for this shape than JSON-mode/`output_config.format`, and
  more reliable than asking the model for a calibrated numeric score.
- **No queue/retry infra.** This repo has zero async code anywhere today
  (confirmed by grep — no `go func()`, no worker, no queue). Triage failures
  are best-effort: log and move on; the ticket keeps its original priority
  and a null category. This is deliberately the *first* goroutine in the
  codebase, so it needs to be narrowly scoped and obviously safe.

I verified the plan below against the actual current contents of
`tickets/service.go`, `tickets/ticket.go`, `tickets/postgres_repository.go`,
and `cmd/api/main.go` — not just a first-pass summary.

## Key existing patterns being followed

- **Consumer-declared interfaces to avoid import cycles**, exactly like
  `AuditRecorder` (`tickets/service.go:30-36`): `tickets` will declare its
  own local `TriageNotifier` interface (one method,
  `NotifyTicketCreated(ctx, ticketID, subject, description) error`); the
  concrete HTTP-calling implementation is built and wired in
  `cmd/api/main.go`, mirroring `ticketsAuditAdapter` (`main.go:70-97`)
  exactly. `triage` in turn declares its own local `Classifier` and
  `TicketUpdater` interfaces for the same reason.
- **Enum type pattern**: `Category` (and a small `Confidence` type) mirror
  `Priority` in `ticket.go:14-32` precisely — a string type, a const block,
  an `AllX` slice, and a `.Valid()` method.
- **Service-method shape**: the new `ApplyTriage`/`ReviewTriage` methods
  mirror `ChangeStatus` (`service.go:107-144`): fetch → validate → mutate →
  persist → `s.audit.Record(...)` (the **already-injected** `AuditRecorder`
  — no new audit dependency needed) → return.
- **Repository writes stay narrow.** `UpdateTicket` today only persists
  `status`/`updated_at`/`resolved_at` (`postgres_repository.go:29-35`) — it
  has never written `priority`, and widening it would silently couple the
  status state-machine's write path to an unrelated AI write-back. Add
  dedicated, single-purpose repository methods instead (below), matching
  the repo's existing one-method-per-write-intent granularity
  (`CreateTicket`, `UpdateTicket`, `AddComment`).

## Backend: new `triage` package + binary

**`backend/internal/triage/`** (new package):
- `triage.go` — `Result{Category, Priority, Confidence string}`; local
  interfaces `Classifier` (`Classify(ctx, subject, description) (Result,
  error)`) and `TicketUpdater` (`UpdateTriage(ctx, ticketID, category,
  priority, confidence string) error`).
- `service.go` — `Service{classifier, updater}`; `Triage(ctx, ticketID,
  subject, description) error` — classify, then write back, fully
  synchronous (see below for why).
- `anthropic_classifier.go` — wraps `github.com/anthropics/anthropic-sdk-go`
  (`go get` it; module is Go 1.26, no `replace` directives, clean fit).
  Split into pure, independently-testable pieces:
  - `buildRequest(subject, description) anthropic.MessageNewParams` — model
    `"claude-haiku-4-5"` (bare alias string), a system prompt listing all
    six categories and four priorities with one-line definitions, *plus*
    explicit guidance on `confidence`: **`low`** when the ticket text is
    short, ambiguous, or plausibly fits more than one category/priority;
    **`high`** when the categorization is unambiguous; **`medium`**
    otherwise. One tool (`classify_ticket`) whose `input_schema` has
    `category`, `priority`, and `confidence` as JSON Schema `enum` arrays
    (all three `required`), `tool_choice` forced to that tool, `MaxTokens:
    256`.
  - `parseResult(*anthropic.Message) (Result, error)` — extracts the
    `ToolUseBlock`, unmarshals its JSON input, and validates all three
    fields against `tickets.AllCategories`/`AllPriorities`/`AllConfidence`
    before trusting them (defense in depth — schemas constrain but don't
    perfectly guarantee).
  - **Import `resolve/internal/tickets` for the enum source of truth**
    (`Category`, `Priority`, `Confidence`, `AllCategories`,
    `AllPriorities`, `AllConfidence`, `.Valid()`). One-directional
    (`triage → tickets`, never the reverse), so no cycle. The alternative —
    hand-duplicating the strings in `triage` — is a guaranteed drift bug the
    first time someone adds a category and forgets the second copy;
    importing is worth the (compile-time-only) extra dependency weight.
- `http_ticket_updater.go` — `HTTPTicketUpdater{baseURL, client}` PATCHes
  `{baseURL}/tickets/{id}/triage` with `{category, priority, confidence}`
  and header `X-Actor: triage-service` — that header is what threads the
  write through the existing `httpx.Actor(r)` → audit-trail path
  unmodified.
- `handler.go` — `POST /triage` accepts `{ticketId, subject, description}`,
  calls `Service.Triage` **synchronously**, returns `204`/`500`. Not part of
  the public API surface, so no OpenAPI entry and no `respondIfError`-style
  typed-error handling needed.

**Why the triage handler stays synchronous** rather than spawning its own
internal goroutine: the main app already decouples ticket-creation latency
via *its* goroutine. If triage's handler also returned early and finished
in the background, there'd be two independent fire-and-forget layers with
no way for the outer one to observe whether the inner one ever completed.
One goroutine, one timeout (the main app's dispatch goroutine's context,
which bounds the whole classify+write-back round trip) is much easier to
reason about — and blocking inside an already-backgrounded goroutine is
free, since the ticket-creation HTTP response already returned.

**`backend/cmd/triage/main.go`** (new binary): stateless — no `pgxpool`, no
`applySchema`. `config.Load()` → fail fast (`log.Fatal`) if
`ANTHROPIC_API_KEY` is unset (a service that can never succeed should
refuse to start, not accept traffic and silently no-op forever) → build
`anthropic.NewClient(...)` → `triage.NewService(classifier, updater)` → chi
router with the same minimal `Logger`+`Recoverer` middleware as `cmd/api` →
mount the triage handler and `health.NewHandler` (reused as-is, gives
`/health` for free) → `ListenAndServe`.

## Backend: changes to `tickets`

- **`ticket.go`**:
  - New `Category` type (mirrors `Priority` exactly — see pattern above).
  - New `Confidence` type (`low`/`medium`/`high`) — used only as request
    input to `ApplyTriage`, not stored as its own ticket field, but typed
    for the same `.Valid()`-based validation consistency as the rest of
    the domain.
  - `Ticket` gains four new nullable pointer fields, following the existing
    `ResolvedAt *string` nullability convention:
    - `Category *Category` — the **applied** category (set once triage
      auto-applies, or once a pending suggestion is accepted).
    - `PendingCategory *Category`, `PendingPriority *Priority` — a
      low-confidence suggestion awaiting human review. Both non-nil
      together or both nil; never partially set.
- **`service.go`**:
  - Local `TriageNotifier` interface next to `AuditRecorder`; `Service`
    gains a `triage TriageNotifier` field; `NewService` gains a third
    parameter (touches 6 call sites — see Testing below).
  - In `Create`, **after** `s.audit.Record(...)` succeeds, dispatch:
    ```go
    go func(id, subject, description string) {
        ctx, cancel := context.WithTimeout(context.Background(), triageNotifyTimeout)
        defer cancel()
        if err := s.triage.NotifyTicketCreated(ctx, id, subject, description); err != nil {
            log.Printf("triage notify failed for ticket %s: %v", id, err)
        }
    }(t.ID, t.Subject, t.Description)
    ```
    **The one detail that will silently break in production if missed:**
    use `context.Background()` with its own timeout, *not* the request's
    `ctx` — chi cancels the request context the moment the handler returns,
    which happens right after this line, so a goroutine inheriting it would
    fail with `context.Canceled` on effectively every call. Close over
    `t.ID`/`t.Subject`/`t.Description` by value, not `t` by reference.
    `triageNotifyTimeout` (~20s, a tunable starting point, not a researched
    number) needs to cover the HTTP hop to triage + Claude latency + the
    PATCH-back hop — generous is cheap since this is invisible to the
    end user regardless.
  - New `ApplyTriage(ctx, actor, id, category, priority, confidence string)
    (*Ticket, error)` — shaped like `ChangeStatus` (see pattern above):
    validates all three enums via `.Valid()`, then branches on confidence:
    - **`medium`/`high`**: sets `t.Category`/`t.Priority` directly, clears
      any stale pending fields, calls `repo.SetTriage(...)`, records a
      `"ticket.triaged"` audit entry with `category`/`priority`/
      `confidence` in `details`.
    - **`low`**: leaves `t.Category`/`t.Priority` untouched, sets
      `t.PendingCategory`/`t.PendingPriority`, calls
      `repo.SetPendingTriage(...)`, records a
      `"ticket.triage_needs_review"` audit entry with the suggested
      `category`/`priority`/`confidence` in `details`.
  - New `ReviewTriage(ctx, actor, id string, accept bool) (*Ticket, error)`
    — fetches the ticket, returns an `invalid(...)` error if there's no
    pending suggestion (`PendingCategory == nil`), then:
    - **accept**: calls `repo.AcceptPendingTriage(...)` (applies pending →
      actual, clears pending, in one atomic `UPDATE`), updates the
      in-memory ticket to match, records `"ticket.triage_reviewed"` with
      `decision: "accept"`.
    - **reject**: calls `repo.RejectPendingTriage(...)` (clears pending
      only), records `"ticket.triage_reviewed"` with `decision: "reject"`.
- **`handler.go`**:
  - `PATCH /{id}/triage` → decode `{category, priority, confidence}` →
    `service.ApplyTriage(...)` → existing `respondIfError` → `200` +
    ticket.
  - New `POST /{id}/triage/review` → decode `{decision: "accept" |
    "reject"}` → `service.ReviewTriage(...)` → `200` + ticket. (`POST`
    rather than `PATCH` since it's an action/decision, not a partial
    resource update — consistent with how `ChangeStatus` is exposed today,
    check the existing route verb for `/status` and mirror it exactly.)
- **`repository.go` / `postgres_repository.go`**: add to the `Repository`
  interface and its Postgres implementation:
  - `SetTriage(ctx, id, category Category, priority Priority, updatedAt
    string) error` — `UPDATE tickets SET category = $2, priority = $3,
    pending_category = NULL, pending_priority = NULL, updated_at = $4
    WHERE id = $1` (also clears any stale pending fields, defensively).
  - `SetPendingTriage(ctx, id, category Category, priority Priority,
    updatedAt string) error` — `UPDATE tickets SET pending_category = $2,
    pending_priority = $3, updated_at = $4 WHERE id = $1` (leaves
    `category`/`priority` untouched).
  - `AcceptPendingTriage(ctx, id, updatedAt string) error` — `UPDATE
    tickets SET category = pending_category, priority =
    COALESCE(pending_priority, priority), pending_category = NULL,
    pending_priority = NULL, updated_at = $2 WHERE id = $1` — reads its own
    pending columns inside the single `UPDATE`, so there's no read-then-
    write race between fetch and apply.
  - `RejectPendingTriage(ctx, id, updatedAt string) error` — `UPDATE
    tickets SET pending_category = NULL, pending_priority = NULL,
    updated_at = $2 WHERE id = $1`.

## Backend: column-list plumbing (positional, must stay in lockstep)

`postgres_repository.go` has **no named-column scanning** — `category`,
`pending_category`, and `pending_priority` need to be threaded through
three places in the same order every time:
- `CreateTicket`'s `INSERT` column list + `VALUES` (write `nil` for all
  three at creation — every ticket starts untriaged).
- The shared `SELECT` in `FindByID` and `findTicketsQuery`.
- `scanTicketRow` — add `*string` scan targets for all three, convert to
  the appropriate pointer types.

This is the single riskiest hand-edit in the change (get the SELECT and
Scan column order out of sync and you silently scan the wrong values into
the wrong fields) — worth a one-line comment in the diff.

## Migration

`backend/internal/migrations/schema.sql` currently has **zero `ALTER
TABLE` statements** — it's a single-shot `CREATE TABLE IF NOT EXISTS`
origin schema, applied unconditionally on every boot
(`main.go:65-68`/`applySchema`). Since `CREATE TABLE IF NOT EXISTS` is a
no-op on an existing table, adding columns requires the repo's first-ever
`ALTER TABLE`:

```sql
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS category VARCHAR;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS pending_category VARCHAR;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS pending_priority VARCHAR;
```

Idempotent (`ADD COLUMN IF NOT EXISTS` since Postgres 9.6, well within
`postgres:16-alpine`), all nullable with no default (matches "no
category/pending-suggestion until triage runs" semantics), safe to run
every startup per the existing convention.

## Config, Docker, Compose

- **`config.go`**: add `AnthropicAPIKey`, `TriagePort` (default `3001`),
  `TriageServiceURL` (default `http://localhost:3001`, used by `cmd/api`
  to reach triage), `MainAppURL` (default `http://localhost:3000`, used by
  `cmd/triage` to PATCH results back) — two separate, oppositely-directed
  URLs, not one shared value. Update `.env.example` and the CLAUDE.md
  "Backend env vars" line.
- **`backend/Dockerfile`**: extend the existing multi-stage build to
  compile *both* `./cmd/api` and `./cmd/triage` into the final image
  (`go build -o /out/api ./cmd/api` and `... /out/triage ./cmd/triage`,
  both `COPY --from=build`'d into the alpine runtime stage). One image, two
  binaries — reuses the same base images and `CGO_ENABLED=0` convention
  already in place; a second Dockerfile would just duplicate the
  `go mod download`/`COPY . .` layers for no benefit.
- **`docker-compose.yml`**: new `triage` service (`build: ./backend`,
  `command: ["./triage"]`, `ANTHROPIC_API_KEY`/`TRIAGE_PORT`/`MAIN_APP_URL`
  env, **no** `depends_on: db` — it's stateless). Add `TRIAGE_SERVICE_URL:
  http://triage:3001` to `app`'s environment. Deliberately **no**
  `app: depends_on: triage` — the entire point of fire-and-forget is that
  ticket creation must not depend on triage being up; encoding a boot-order
  dependency would send a mixed signal even though `depends_on` only
  affects startup ordering, not runtime behavior.
- **`Makefile`**: `make backend`/`build-backend`/`all` currently only
  target `cmd/api` — add a `triage` run target and extend the build/CI
  targets so the new binary's compile health is actually checked by local
  `make` workflows, not just inside Docker builds.

## Frontend

- **`types.ts`**: `export type TicketCategory = 'billing' | 'bug' |
  'account_access' | 'feature_request' | 'how_to' | 'other'`; add to the
  `Ticket` interface: `category: TicketCategory | null`,
  `pendingCategory: TicketCategory | null`, `pendingPriority:
  TicketPriority | null`. No `api/client.ts` changes needed for the read
  path — it passes through whatever the backend returns, and every
  existing `request<Ticket>(...)` call site picks up the new fields
  automatically. Add one new method, `reviewTriage(id: string, decision:
  'accept' | 'reject'): Promise<Ticket>`, POSTing to
  `/tickets/{id}/triage/review`.
- **`components/Badge.tsx`**: new `CategoryBadge` (following
  `StatusBadge`/`PriorityBadge`'s exact existing color-map + label
  pattern) and a `NeedsReviewBadge` (a distinct warning-colored badge, no
  category/priority prop needed — just an indicator) shown when
  `pendingCategory` is non-null. Callers guard on `ticket.category &&
  <CategoryBadge .../>` rather than the components accepting `null`
  themselves.
- **`pages/TicketListPage.tsx`**: mirror the existing `PRIORITIES` filter
  const and table column pattern for a `CATEGORIES` filter + `Category`
  column, plus a `NeedsReviewBadge` next to the priority/category badges
  for any ticket with a pending suggestion. Note: the backend
  `Filter`/`findTicketsQuery` doesn't support category (or "needs review")
  filtering yet — if you want dropdowns to filter server-side
  (recommended over client-side filtering, which breaks under real
  pagination), that's a small additional backend change (`Filter.Category`
  + a `category = $N` clause mirroring the existing `Status`/`Priority`
  conditions, and similarly `Filter.NeedsReview bool` → `pending_category
  IS NOT NULL`).
- **`pages/TicketDetailPage.tsx`**: render `<CategoryBadge>` alongside the
  existing `<PriorityBadge>`. When `pendingCategory`/`pendingPriority` are
  set, render a review panel: "AI suggests: `<CategoryBadge
  pendingCategory>` / `<PriorityBadge pendingPriority>`" with **Accept**
  and **Reject** buttons calling the new `reviewTriage(id, decision)`
  client method and refreshing the ticket on success.
- **`pages/TicketCreatePage.tsx`**: the manual priority `<select>` at
  creation may be overwritten by triage shortly after (only for
  medium/high-confidence results — low-confidence ones now correctly leave
  the user's choice alone until a human reviews the suggestion, which
  somewhat reduces the urgency of changing this page). Leaving it as-is is
  fine; optionally relabel it ("Initial priority — may be updated
  automatically") to set expectations. Your call at implementation time,
  not a blocking decision.

## Testing (matches this repo's fake-only, no-DB convention)

- **`triage` package**: `Classifier`/`TicketUpdater` are the seams — fake
  implementations for `service_test.go` (happy path; classifier error
  short-circuits before the updater is called; updater error surfaces,
  isn't swallowed). `buildRequest`/`parseResult` are pure functions, so
  `anthropic_classifier_test.go` gets real coverage with no network call:
  system prompt contains all six/four/three values and the confidence
  guidance, tool schema enums match
  `tickets.AllCategories`/`AllPriorities`/`AllConfidence` exactly (this is
  the drift detector that justifies importing `tickets` rather than
  duplicating the enums), `tool_choice` is forced correctly; `parseResult`
  handles missing tool_use blocks, malformed JSON, and out-of-enum values
  for all three fields. `handler_test.go` follows the existing `httptest`
  pattern used elsewhere in the repo.
- **`tickets` package**: `NewService`'s new third parameter touches 6 call
  sites (`main.go`, `service_test.go`, `handler_test.go` ×3,
  `pagination_test.go`, and `cannedresponses/ac19_integration_test.go`,
  which constructs its own `tickets.Service` and will need its own small
  fake) — add a `fakeTriageNotifier` alongside the existing `fakeAudit`.
  For the goroutine dispatch specifically, use a **channel-based fake**
  (not `time.Sleep`) to assert, without flakiness: (a) `Create` returns
  before `NotifyTicketCreated` completes, and (b) the goroutine is
  dispatched with the right arguments (`select` on a buffered channel with
  a short timeout, failing the test rather than hanging if nothing
  arrives). New `handler_test.go`/`service_test.go` cases:
  - `ApplyTriage` with `confidence: "high"`/`"medium"` → `category`/
    `priority` update directly, `PendingCategory`/`PendingPriority` stay
    nil, `"ticket.triaged"` audit entry recorded.
  - `ApplyTriage` with `confidence: "low"` → `category`/`priority`
    **unchanged**, `PendingCategory`/`PendingPriority` populated,
    `"ticket.triage_needs_review"` audit entry recorded.
  - Invalid category/priority/confidence → 400 with the enum-listing
    message (same shape as the existing priority validation).
  - `ReviewTriage(accept: true)` on a ticket with a pending suggestion →
    `category`/`priority` now match the former pending values, pending
    fields cleared, `"ticket.triage_reviewed"` audit entry with
    `decision: "accept"`.
  - `ReviewTriage(accept: false)` → pending fields cleared,
    `category`/`priority` unchanged, `"ticket.triage_reviewed"` audit
    entry with `decision: "reject"`.
  - `ReviewTriage` on a ticket with no pending suggestion → validation
    error, no repo write, no audit entry.
  - Unknown id on either endpoint → 404 via the existing `NotFoundError`
    path.
- **`backend/internal/docs/openapi.yaml`**: add `PATCH /tickets/{id}/triage`
  (with `confidence` in the request body) and `POST
  /tickets/{id}/triage/review` next to the existing `/tickets/{id}/status`
  entry, and add the nullable `category`/`pendingCategory`/
  `pendingPriority` properties to the `Ticket` schema. Triage's own
  internal `POST /triage` and `/health` don't need entries — not part of
  the public surface this file documents.

## Notable pre-existing gap surfaced by this change

`UpdateTicket` has never persisted `priority` — until now, nothing ever
mutated priority after ticket creation. This isn't a regression you're
introducing, but it's worth calling out explicitly (e.g. in the PR
description) since `ApplyTriage`/`ReviewTriage` will be the very first code
paths to write `priority` post-creation.

## Critical files

- `backend/internal/tickets/service.go` — `TriageNotifier` interface,
  goroutine dispatch in `Create`, new `ApplyTriage`/`ReviewTriage` methods
- `backend/internal/tickets/ticket.go` — new `Category`/`Confidence`
  types, new `Ticket` fields
- `backend/internal/tickets/postgres_repository.go` — column-list
  plumbing, new `SetTriage`/`SetPendingTriage`/`AcceptPendingTriage`/
  `RejectPendingTriage`
- `backend/internal/triage/service.go`, `anthropic_classifier.go` — new
  package core, including the confidence-aware tool schema
- `backend/cmd/api/main.go` — `httpTriageNotifier` adapter, updated
  `NewService` call
- `backend/cmd/triage/main.go` — new binary
- `backend/internal/migrations/schema.sql` — the three `ALTER TABLE`
  statements
- `docker-compose.yml`, `backend/Dockerfile` — new service + two-binary
  build

## Verification

1. `cd backend && go build ./...` — both binaries compile.
2. `make test-backend` — existing suite (with the 6 updated `NewService`
   call sites) plus new tests all green, no real DB or API key needed.
3. `make up` (or `docker compose up --build`) with `ANTHROPIC_API_KEY` set
   in `.env` — confirms the two-binary Dockerfile build and the new
   compose service wire together.
4. End-to-end, high-confidence path: `POST /tickets` with an unambiguous
   subject/description (e.g. "My credit card was charged twice this month"
   → should land in `billing`, `high` confidence). Poll `GET /tickets/{id}`
   a few seconds later; confirm `category` is populated and `priority`
   reflects the AI's assessment, with a `"ticket.triaged"` entry in
   `GET /tickets/{id}/audit`.
5. End-to-end, low-confidence path: `POST /tickets` with deliberately vague
   text (e.g. "something's wrong, please help"). Confirm the ticket's
   `priority`/`category` stay at their original/null values while
   `pendingCategory`/`pendingPriority` get populated, and a
   `"ticket.triage_needs_review"` audit entry appears. Then call
   `POST /tickets/{id}/triage/review` with `{decision: "accept"}` and
   confirm `category`/`priority` now reflect the suggestion, pending
   fields clear, and a `"ticket.triage_reviewed"` entry appears. Repeat
   with `"reject"` on a fresh low-confidence ticket and confirm pending
   fields clear while `category`/`priority` stay untouched.
6. Check the triage service's logs for a deliberately-broken case (e.g.
   temporarily unset `ANTHROPIC_API_KEY` on `cmd/triage` — it should
   `log.Fatal` and refuse to start) and a transient-failure case (stop the
   `triage` container mid-run — ticket creation should still succeed
   immediately, with a logged "triage notify failed" on the `app` side).
