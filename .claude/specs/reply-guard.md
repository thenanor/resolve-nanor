# REPLYGUARD-1 - Reply-guard service: pre-send review of customer-facing replies

## Context

Support agents (human or AI) draft replies to customers on a ticket. Today
nothing checks a reply before it becomes a customer-visible `Comment` — it's
one `POST /tickets/{id}/comments` call with `internal: false` and it's sent.
Reply-guard is a new, standalone service (a sibling to `triage/` —
`backend/internal/replyguard/` + `backend/cmd/replyguard/`) that reviews a
draft reply before a human sends it: it reads the ticket, the ticket's
internal notes, and the draft reply, checks the draft against a fixed reply
policy (disclosure, commitment, answer, tone), and returns a verdict plus
findings a human uses to decide whether to send it, revise it, or escalate
it.

Unlike triage — which fires-and-forgets on ticket creation and never blocks
anything — reply-guard's entire purpose is to be a gate in front of a
customer-facing send. That is a real architectural fork from triage's
"never block" principle, and it is the central open question this spec
raises rather than silently resolves (see Open Questions).

## Domain additions

- **Draft reply**: a reply an agent has written but not yet sent to the
  customer. New concept — does not reuse `Comment` (existing `Comment`s,
  internal or not, are already-sent/already-visible by definition today).
  A draft belongs to exactly one ticket, has a body, an author, a lifecycle
  state, and — once reply-guard has run — a `GuardResult` (see below).
- **Guard result fields** (returned by reply-guard, stored on the draft):
  - `verdict`: one of `send | revise | escalate`.
  - `findings`: list of `{policy, severity, issue, quote}` —
    - `policy`: one of `disclosure | commitment | answer | tone` — which
      reply-policy line this finding is about.
    - `severity`: one of `low | medium | high` — how bad this instance of
      the violation is, independent of which policy line it's under (a
      `disclosure` finding and a `tone` finding can each be any severity;
      policy is *which rule*, severity is *how bad this instance is*).
    - `issue`: one/two-sentence plain-language description of the problem.
    - `quote`: a literal substring of the draft reply that triggered the
      finding (not a paraphrase — see AC-9).
  - `confidence`: one of `low | medium | high` — the guard's confidence in
    its own verdict, same bucketed shape as triage's public `confidence`
    contract (`tickets.Confidence`/`AllConfidence`), for consistency across
    the two AI services' wire contracts. (Internally, same as triage,
    nothing stops the classifier from reasoning about confidence as a
    continuous score before bucketing — that's an implementation detail,
    not part of the contract.)
  - `reasoning`: one or two sentences, max ~40 words, explaining the
    verdict overall (distinct from each finding's own `issue`).
  - `injectionSuspected`: bool — true if the internal notes, the draft
    reply, or prior customer-authored comments on the thread appear to
    contain instructions aimed at the model itself rather than genuine
    ticket content (mirrors triage's existing rule in
    `anthropic_classifier.go`: treat ticket-embedded instructions as
    information, not instruction — classify/verdict on the underlying
    content and flag the attempt).
  - `requireHuman`: bool — true if this verdict must be reviewed by a
    *second* human (e.g. a lead/supervisor), not just the agent who wrote
    the draft. Independent of `verdict`: a `send` verdict can still carry
    `requireHuman: true` (e.g. low `confidence` on an otherwise clean
    draft); see AC-13 for the exact rule.

## Acceptance criteria

**Drafting**
- **AC-1** - `POST /tickets/{id}/drafts` with a non-empty (after trim)
  `author` and non-empty (after trim) `body` returns `201` and the created
  draft: `id`, `ticketId`, `author`, `body`, `status` (`pending_review`),
  `guardResult` (`null`), `createdAt`, `updatedAt`.
- **AC-2** - `POST /tickets/{id}/drafts` with an empty/whitespace `author`
  or `body` returns `400` naming the offending field, mirroring
  `AddComment`'s existing validation (`tickets/service.go`).
- **AC-3** - `POST /tickets/{id}/drafts` on a nonexistent ticket id returns
  `404`.
- **AC-4** - Creating a draft dispatches a fire-and-forget call to
  reply-guard (mirrors `tickets.Service.Create`'s dispatch to triage: its
  own `context.Background()` + timeout, not the request context) and
  returns `201` without waiting for the guard result — draft creation
  latency is unaffected by LLM round-trip time.
- **AC-5** - `GET /tickets/{id}/drafts/{draftId}` returns `200` with the
  draft's current state; while the guard call is in flight, `status` is
  `pending_review` and `guardResult` is `null`.

**Guarding**
- **AC-6** - reply-guard reads exactly: the ticket (subject, description,
  status, priority), every existing `Comment` on the ticket with
  `internal: true`, and the draft reply's `body` — nothing else (see
  Non-goals on prior draft history).
- **AC-7** - A draft whose body quotes, closely paraphrases, or otherwise
  reveals the substance of any internal-note content produces at least one
  `disclosure` finding.
- **AC-8** - A draft that promises a refund, a specific deadline/ETA,
  compensation, or states what engineering will do produces at least one
  `commitment` finding. Explaining a situation or apologizing does not,
  by itself, produce a `commitment` finding.
- **AC-9** - Every finding's `quote` field is a literal, contiguous
  substring of the draft reply's `body` (byte-for-byte, not a paraphrase or
  summary of the offending text).
- **AC-10** - A draft that does not address the customer's most recent
  non-internal question/request on the ticket produces at least one
  `answer` finding.
- **AC-11** - A draft that is defensive, dismissive, or blames the customer
  produces at least one `tone` finding. A merely neutral (non-warm) but
  professional draft produces no `tone` finding — warmth is not the bar.
- **AC-12** - Grammar, spelling, word choice, and style are never findings,
  regardless of severity — reply-guard only evaluates policy compliance
  (disclosure/commitment/answer/tone), not copyediting.
- **AC-13** - `requireHuman` is `true` whenever `verdict` is `escalate`,
  whenever any finding has `severity: high`, or whenever `confidence` is
  `low`; `requireHuman` is `false` only when `verdict` is `send` and no
  finding is `high` severity and `confidence` is not `low`.
- **AC-14** - `verdict` is `send` only when `findings` is empty or contains
  only `low`-severity findings.
- **AC-15** - `verdict` is `escalate` whenever any finding has `severity:
  high`, or whenever `injectionSuspected` is `true`.
- **AC-16** - `verdict` is `revise` for everything else (at least one
  `medium`-severity finding, none reaching `high`, and no injection
  suspected).
- **AC-17** - When the guard call fails (classifier error, malformed model
  output, network error), the draft's `status` becomes `guard_failed`
  (distinct from `pending_review`), `guardResult` stays `null`, and the
  failure is logged — mirrors triage's best-effort failure handling
  (`triage/service.go`), but unlike triage, a failed guard call must not
  silently leave the draft looking sendable (see Invariants).

**Result write-back**
- **AC-18** - On a successful guard call, reply-guard writes the full
  `GuardResult` back onto the draft (mirrors triage's `HTTPTicketUpdater`:
  `X-Actor: reply-guard-service` header threads through the main app's
  existing `httpx.Actor(r)` → audit-trail path) and the draft's `status`
  becomes `guarded`.
- **AC-19** - Writing a guard result back records an audit entry
  `draft.guarded` on the ticket with `draftId`, `verdict`, `findingCount`,
  `confidence`, and `injectionSuspected` in `details`.

**Sending**
- **AC-20** - `POST /tickets/{id}/drafts/{draftId}/send` on a draft whose
  `status` is `guarded` and `guardResult.verdict` is `send` returns `200`,
  creates a real customer-visible `Comment` (`internal: false`, `body` =
  the draft's `body`, `author` = the draft's `author`) via the same path
  `AddComment` already uses, and marks the draft `status: sent`.
- **AC-21** - `POST /tickets/{id}/drafts/{draftId}/send` on a draft whose
  `guardResult.verdict` is `revise` or `escalate`, and no valid override
  applies (AC-23), returns `409` with the current findings in the response
  body, and does not create a `Comment`.
- **AC-22** - `POST /tickets/{id}/drafts/{draftId}/send` on a draft whose
  `status` is `pending_review` (guard hasn't finished) or `guard_failed`
  returns `409` and does not create a `Comment`, regardless of
  `overrideReason` — override only ever waives a `revise` verdict, never a
  missing/failed guard result.
- **AC-23** - `POST /tickets/{id}/drafts/{draftId}/send` accepts an
  optional `overrideReason` (non-empty string) that allows sending despite
  a `revise` verdict only. When used, the created `Comment`'s audit trail
  and a new `draft.send_overridden` audit entry both record
  `overrideReason` and the actor who overrode it. Omitting `overrideReason`
  on a `revise` verdict is what triggers AC-21's `409`.
- **AC-23a** - `POST /tickets/{id}/drafts/{draftId}/send` on a draft whose
  `guardResult.verdict` is `escalate` always returns `409` and never
  creates a `Comment`, regardless of `overrideReason` — escalate is a hard
  block. The only path from `escalate` back to a sendable state is editing
  the draft (AC-26), which clears `guardResult` and re-runs the guard.
- **AC-24** - Sending a draft (with or without override) is idempotent
  against replay of the exact same `draftId`: a second `send` call on an
  already-`sent` draft returns `409` and creates no second `Comment`.
- **AC-25** - `POST /tickets/{id}/drafts/{draftId}/send` on a nonexistent
  `draftId`, or a `draftId` that belongs to a different ticket than `{id}`,
  returns `404`.

**Revising**
- **AC-26** - `PUT /tickets/{id}/drafts/{draftId}` (edit body, resubmit)
  is only allowed while `status` is `guarded` with a `revise` verdict, or
  `guard_failed`; it updates `body`, resets `status` to `pending_review`,
  clears `guardResult`, and re-dispatches the guard call (same as AC-4).
  Attempting to edit a `sent` draft returns `409`.

## Invariants

- A `Comment` with `internal: false` is only ever created via the send path
  (AC-20/AC-23) or the pre-existing direct `POST /tickets/{id}/comments`
  path used for internal notes and any reply flow that deliberately bypasses
  drafting — reply-guard does not change `AddComment`'s existing contract or
  validation (see Non-goals).
- A draft never reaches `status: sent` without either `guardResult.verdict
  == "send"` or a recorded `overrideReason` on a `revise`-verdict draft —
  there is no code path that sends a `guarded`/`revise` draft silently, and
  no code path at all (override or otherwise) sends an `escalate`-verdict
  draft without it first being edited and re-guarded to a non-`escalate`
  verdict.
- `guardResult` is only ever set once per guard run and is fully replaced
  (never partially merged) on re-guard after an edit (AC-26).
- Every finding's `quote` is always found verbatim in the draft body it was
  produced from (AC-9) — a finding whose quote can't be located in the
  current `body` is a bug, not a valid state.
- Guard failures never leave a draft indistinguishable from
  "reviewed and clean" — `guard_failed` is a distinct `status` from
  `guarded`, and only `guarded` + `verdict: send` (or an explicit override)
  authorizes sending.

## Constraints

- Follow CLAUDE.md conventions: `backend/internal/replyguard/` package
  mirrors `triage/`'s shape exactly — a `Result`/local-interface file, a
  synchronous `Service`, an `anthropic_classifier.go` (forced single-tool
  call, JSON Schema enums, Claude Haiku), an `http_*` write-back adapter
  using `X-Actor`, a thin internal `handler.go` (not part of the public
  OpenAPI surface). `backend/cmd/replyguard/main.go` mirrors
  `cmd/triage/main.go`: stateless, `log.Fatal`s if `ANTHROPIC_API_KEY` is
  unset, own configured port.
- New config: `REPLYGUARD_PORT` (default `3002`), `REPLYGUARD_SERVICE_URL`
  (default `http://localhost:3002`, used by `cmd/api` to reach
  reply-guard), reuses the existing `MAIN_APP_URL` and
  `ANTHROPIC_API_KEY` — same pattern as `TriagePort`/`TriageServiceURL` in
  `backend/internal/config/config.go`.
- New `drafts` table/columns in `backend/internal/migrations/schema.sql`,
  applied the same idempotent way as the triage columns
  (`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, or a new `CREATE TABLE IF
  NOT EXISTS drafts`).
- Typed `*ValidationError`/`*NotFoundError` (and a new conflict-style error
  for AC-21/AC-22/AC-24/AC-26, mapped to `409` in `respondIfError`) —
  matching `tickets/errors.go`'s existing pattern rather than raw
  `fmt.Errorf`.
- `backend/internal/docs/openapi.yaml` updated with the new
  `/tickets/{id}/drafts` endpoints and schemas.
- No new authentication/authorization model — same `X-Actor`-header-only
  model as the rest of the app; `requireHuman`/override is a UI/workflow
  signal, not an enforced permission check (there is no concept of
  "supervisor" role in this app today — see Non-goals).

## Non-goals

- Enforcing *who* is allowed to override or approve an `escalate`/`revise`
  verdict — this app has no roles/permissions system; `requireHuman` and
  `overrideReason` are advisory/logged, not access-controlled. A future
  RBAC feature could tighten this.
- Multi-draft-per-ticket history/versioning beyond the current edit-in-place
  flow (AC-26) — editing a draft mutates it, it does not create a new
  draft row or preserve prior guard results for the same `draftId`. A full
  version history is a separate feature if needed.
- Guarding internal-note content itself, or any comment posted with
  `internal: true` — internal notes are only ever *read* by reply-guard as
  context (AC-6), never evaluated against the reply policy themselves.
- Guarding replies sent through the pre-existing
  `POST /tickets/{id}/comments` endpoint directly — that endpoint's
  behavior/contract is completely unchanged by this feature (mirrors
  canned-responses' AC-22 precedent of leaving a pre-existing endpoint
  alone). Whether that endpoint should eventually be locked down in favor
  of drafts-only replying is a follow-up product decision, not this spec.
- Real-time/streaming guard status (e.g. websocket push when a guard
  result lands) — clients poll `GET /tickets/{id}/drafts/{draftId}`
  (AC-5), consistent with this app having no realtime infrastructure
  today.
- A UI for the drafting/review flow — this spec covers the backend
  contract only, matching how `plan-triage-agent.md` scoped triage's
  backend first.

## Open questions

Both previously blocked questions were resolved by the feature owner:
reply-guard uses a new "draft reply" resource with an async, fire-and-forget
guard call and an explicit `/send` step (not a synchronous gate on the
existing comments endpoint); and `escalate` is a hard block with no
override, while `revise` may be overridden with a logged reason. Reflected
in the ACs and Invariants above.

- **Deliberate** - Exact wording/length limits for `reasoning` and `issue`
  are left to the implementer, mirroring how triage's system prompt pins
  `reasoning` to "one sentence, max 25 words" without that being spec'd as
  an AC.
- **Deliberate** - `drafts` table shape (own table vs. reusing/extending
  the `Comment`/`ticket_comments` shape with a `status` column) is left to
  the implementer, same as canned-responses left tag storage shape open —
  either satisfies the ACs above.
- **Deliberate** - Whether `GET /tickets/{id}` includes drafts inline
  (alongside `comments`) or drafts are fetched only via the dedicated
  `/drafts` endpoints is left to the implementer; no AC above depends on
  ticket-response shape changing.

## Definition of done

- Every AC (AC-1 through AC-26, plus AC-23a) covered by a test naming its
  id.
- Every Invariant covered by a test (in particular: a `guard_failed` draft
  cannot be sent; a `sent` draft cannot be sent or edited again; an
  `escalate`-verdict draft cannot be sent even with `overrideReason`; a
  finding quote is always a substring of its draft body).
- `make ci` green (vet, lint, backend + frontend tests, typecheck, build).
- `backend/internal/docs/openapi.yaml` updated and consistent with the
  implemented endpoints.
