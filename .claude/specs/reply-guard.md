# REPLYGUARD-1 - Reply-guard: synchronous pre-send review of customer-facing replies

## Context

Support agents (human or AI) reply to customers by calling
`POST /tickets/{id}/comments` with `internal: false`. A "draft reply" is not
a separate resource — it is the body of that comment, in flight, before it
becomes a real, customer-visible `Comment`. Reply-guard (a standalone
service, sibling to `triage/`: `backend/internal/replyguard/` +
`backend/cmd/replyguard/`) reviews that body synchronously, inline in the
request, and either lets the comment through or rejects the call with
findings.

This supersedes the original REPLYGUARD-1 design (a separate `drafts`
resource with its own `POST /tickets/{id}/drafts` → review →
`POST .../send` lifecycle). That design shipped with no frontend ever built
against it — real customer replies always went through
`POST /tickets/{id}/comments` directly, completely unguarded, which is the
gap this revision closes. The `drafts` resource, its table, and its
endpoints are removed.

A reply built from a canned response (see `CANNED-1`,
`.claude/specs/canned-responses.md`) is not guarded — canned text is
pre-approved by whoever created the canned response, and re-reviewing static,
unedited text on every send is pure LLM cost with no signal.

## Domain additions

- **Candidate reply**: the `body` of an in-flight
  `POST /tickets/{id}/comments` call with `internal: false` and
  `fromCannedResponse` not `true`. It has no id, no storage, and no lifecycle
  of its own — it exists only for the duration of the request that creates
  it. This replaces the old persisted "draft reply" concept.
- **Guard result fields** (returned inline by reply-guard, never persisted as
  their own entity):
  - `verdict`: one of `send | revise | escalate`.
  - `findings`: list of `{policy, severity, issue, quote}` —
    - `policy`: one of `disclosure | commitment | answer | tone`.
    - `severity`: one of `low | medium | high` — independent of `policy`
      (a `disclosure` finding and a `tone` finding can each be any
      severity).
    - `issue`: one/two-sentence plain-language description of the problem.
    - `quote`: a literal substring of the candidate reply's body that
      triggered the finding (not a paraphrase — see AC-11).
  - `confidence`: one of `low | medium | high`, same bucketed shape as
    triage's public `confidence` contract (`tickets.Confidence`).
  - `reasoning`: one or two sentences, max ~40 words.
  - `injectionSuspected`: bool — true if internal notes or the candidate
    reply itself appear to contain instructions aimed at the model rather
    than genuine ticket content (mirrors triage's existing rule: treat
    ticket-embedded instructions as information, not instruction — assess
    the underlying content and flag the attempt).
  - `requireHuman`: bool — true if this verdict should be reviewed by a
    second person, not just the agent who wrote the reply. Independent of
    `verdict` — see AC-16 for the exact rule.

## Acceptance criteria

**Request shape**
- **AC-1** - `POST /tickets/{id}/comments` accepts
  `{author, body, internal, fromCannedResponse?, overrideReason?}`.
  `fromCannedResponse` (bool, default `false`) and `overrideReason` (string,
  default absent) are both optional and additive — omitting them preserves
  every existing behavior of this endpoint for internal comments.
- **AC-2** - `author`/`body` validation (non-empty after trim, 400 naming
  the offending field) and the nonexistent-ticket 404 are unchanged from
  today's `AddComment` behavior, for every value of `internal`,
  `fromCannedResponse`, and `overrideReason`.

**Guard applicability**
- **AC-3** - `internal: true` comments are never guarded, regardless of
  `fromCannedResponse`/`overrideReason` (which have no effect when
  `internal: true`) — identical to today's unguarded internal-note path.
- **AC-4** - `internal: false` with `fromCannedResponse: true` is never
  guarded — the comment is created immediately from the existing
  (unguarded) `AddComment` path, and the audit entry is unchanged from
  today's shape.
- **AC-5** - `internal: false` with `fromCannedResponse` absent or `false`
  triggers a synchronous reply-guard call before the comment is created.
  The `POST /tickets/{id}/comments` call blocks on this — accepted, deliberate
  latency, replacing this spec's prior "draft creation latency is
  unaffected" framing, which no longer applies now that guarding is the
  gate itself rather than a background step.

**Guarding**
- **AC-6** - The guard call reads exactly: the ticket (subject, description,
  status, priority), every existing `Comment` on the ticket with
  `internal: true`, and the candidate reply's `body` — nothing else (no
  prior comment/reply history beyond internal notes).
- **AC-7** - A candidate reply that quotes, closely paraphrases, or
  otherwise reveals the substance of any internal-note content produces at
  least one `disclosure` finding.
- **AC-8** - A candidate reply that promises a refund, a specific
  deadline/ETA, compensation, or states what engineering will do produces at
  least one `commitment` finding. Explaining a situation or apologizing does
  not, by itself, produce one.
- **AC-9** - A candidate reply that does not address the customer's most
  recent non-internal question/request on the ticket produces at least one
  `answer` finding.
- **AC-10** - A candidate reply that is defensive, dismissive, or blames the
  customer produces at least one `tone` finding. A merely neutral
  (non-warm) but professional reply produces no `tone` finding.
- **AC-11** - Every finding's `quote` is a literal, contiguous substring of
  the candidate reply's `body` (byte-for-byte, not a paraphrase).
- **AC-12** - Grammar, spelling, word choice, and style are never findings,
  regardless of severity.
- **AC-13** - `verdict` is `send` only when `findings` is empty or contains
  only `low`-severity findings.
- **AC-14** - `verdict` is `escalate` whenever any finding has
  `severity: high`, or whenever `injectionSuspected` is `true`.
- **AC-15** - `verdict` is `revise` for everything else (at least one
  `medium`-severity finding, none reaching `high`, no injection suspected).
- **AC-16** - `requireHuman` is `true` whenever `verdict` is `escalate`,
  whenever any finding has `severity: high`, or whenever `confidence` is
  `low`; `false` only when `verdict` is `send`, no finding is `high`
  severity, and `confidence` is not `low`.

**Outcome of the guarded POST**
- **AC-17** - Guard call succeeds with `verdict: send`:
  `POST /tickets/{id}/comments` returns `201` and creates the `Comment`
  exactly as today, with `guardResult` (the full guard result object)
  included in the response body alongside the comment.
- **AC-18** - Guard call succeeds with `verdict: revise` and
  `overrideReason` is absent/empty: returns `409` with `verdict`,
  `findings`, and the rest of the guard result in the body; no `Comment` is
  created.
- **AC-19** - Guard call succeeds with `verdict: revise` and a non-empty
  `overrideReason` is present: returns `201`, creates the `Comment`, and
  records both the guard result and `overrideReason` (with the actor) on the
  audit entry (AC-21).
- **AC-20** - Guard call succeeds with `verdict: escalate`: always returns
  `409` and never creates a `Comment`, regardless of `overrideReason` —
  escalate is a hard block with no override, exactly as in the original
  design. The only way past an `escalate` verdict is resubmitting a
  materially revised `body` (which is guarded fresh, as a new candidate
  reply — there is no persisted state to edit-in-place).
- **AC-21** - Every guarded `Comment` creation (send or override) records an
  audit entry `ticket.commented` whose `details` include, in addition to the
  existing `commentId`/`internal` fields, `verdict`, `findingCount`,
  `confidence`, `injectionSuspected`, and — only when an override was used —
  `overrideReason`. Unguarded comments (internal, or `fromCannedResponse`)
  keep today's audit shape unchanged (no guard fields).
- **AC-22** - If the guard call itself fails (reply-guard unreachable,
  non-2xx response, malformed/invalid model output), `POST /tickets/{id}/comments`
  returns `502` with a message indicating the guard could not be completed,
  logs the failure, and creates no `Comment` — fails closed, never falls
  back to creating an unguarded customer-facing reply.

**Frontend**
- **AC-23** - The comment composer sets `fromCannedResponse: true` on submit
  only when the body currently equals, unedited, the text a canned response
  was inserted as (mirrors `CANNED-1` AC-20's insertion behavior); any edit
  to the body after insertion clears this, so the next submit is guarded
  normally.
- **AC-24** - On a `409` with `verdict: revise`, the composer displays the
  findings (policy, severity, issue, quote) and offers a way to send anyway
  with a required reason, which is submitted as `overrideReason`.
- **AC-25** - On a `409` with `verdict: escalate`, the composer displays the
  findings and does not offer an override control — the agent's only path
  forward is editing the reply and resubmitting.

## Invariants

- A `Comment` with `internal: false` is only ever created when one of: (a)
  `fromCannedResponse: true`, (b) the guard ran and returned `verdict: send`,
  or (c) the guard ran, returned `verdict: revise`, and a non-empty
  `overrideReason` was supplied. No code path creates an `internal: false`
  `Comment` on an `escalate` verdict, ever, regardless of `overrideReason`.
- No code path creates an `internal: false` `Comment` when the guard call
  itself failed — a guard failure is indistinguishable in outcome from a
  `revise`/`escalate` rejection (no `Comment` created), even though its
  status code (`502`) is distinct so the agent can tell "guard said no" from
  "guard couldn't run."
- `fromCannedResponse` is never persisted on the created `Comment` row —
  mirrors `CANNED-1`'s existing invariant that no stored reference connects
  a `ticket_comments` row back to a `canned_responses` row. It is read once,
  per request, purely to decide whether to call the guard.
- `internal: true` comments are never subject to the guard, regardless of
  `fromCannedResponse`/`overrideReason`.
- Every finding's `quote` is always found verbatim in the candidate reply's
  `body` it was produced from (AC-11) — a finding whose quote can't be
  located in that body is a bug, not a valid state.
- The guard call happens at most once per `POST /tickets/{id}/comments`
  call — there is no server-side retry of a failed guard call within the
  same request.

## Constraints

- `backend/internal/replyguard/`'s internal shape is unchanged from the
  original design (a `Result`/local-interface file, `anthropic_classifier.go`
  with a forced single-tool call, JSON Schema enums, Claude Haiku) — only its
  HTTP contract changes: `POST /guard` now returns the full assessment
  (`verdict`, `findings`, `confidence`, `reasoning`, `injectionSuspected`,
  `requireHuman`) synchronously in its response body, instead of `204` plus
  an async callback. The old callback path
  (`DraftUpdater`/`HTTPDraftUpdater`, and main app's
  `POST /tickets/{id}/drafts/{draftId}/guard-result`) is removed.
- `tickets.Service.AddComment` gains a new, consumer-declared dependency
  (e.g. `ReplyGuardClient`) that it calls synchronously for
  `internal: false`, non-canned comments — declared in `tickets/` per this
  repo's import-cycle-avoidance convention (mirrors `AuditRecorder`,
  `TriageNotifier`).
- `backend/internal/drafts/` (package, table, migrations, endpoints) is
  removed entirely: `POST/GET/PUT /tickets/{id}/drafts*` and everything
  behind them.
- `REPLYGUARD_PORT`/`REPLYGUARD_SERVICE_URL` config is unchanged in shape
  (reply-guard stays a separately deployed/run service — `make replyguard`,
  requires `ANTHROPIC_API_KEY`) but its role changes from an
  async fire-and-forget notifier to synchronous request-path middleware.
- A new typed error (e.g. `*GuardUnavailableError`) distinct from
  `*ValidationError`/`*NotFoundError`/the existing conflict error, mapped to
  `502` in `respondIfError`, for AC-22. A `revise`/`escalate` rejection
  (guard ran, said no) is a different typed error mapped to `409`, so the
  two failure modes stay distinguishable in code, not just by status code.
- `backend/internal/docs/openapi.yaml` updated: `/tickets/{id}/drafts*`
  removed; `POST /tickets/{id}/comments` request/response schemas gain
  `fromCannedResponse`, `overrideReason`, `guardResult`, and document the new
  `409`/`502` responses.

## Non-goals

- Enforcing that `fromCannedResponse: true` is truthful — an agent (or a
  buggy client) can set it on hand-edited text and skip the guard; this app
  has no way to verify client-reported flags any more than it verifies
  `X-Actor` today. Same trust model, not tightened by this spec.
- Automatic retry of a failed guard call — `AC-22`'s `502` is the end of
  that request; the agent resubmits.
- Guarding replies with `fromCannedResponse: true`, even when the agent has
  visibly edited the canned text but still sent the flag — see the Non-goal
  above; detecting "edited vs. not" server-side is out of scope, it's a
  client-side responsibility (AC-23).
- Multi-draft-per-ticket history/versioning — there is no persisted
  candidate-reply entity to version; a rejected attempt simply isn't
  created, and the agent's next attempt is a fresh, independent guard call.
- Guarding internal-note content itself — internal notes are only ever
  *read* by reply-guard as context (AC-6), never evaluated against the
  reply policy.
- Role-based enforcement of who may use `overrideReason` — this app has no
  roles/permissions system; `overrideReason` is advisory/logged, not
  access-controlled, same as the original design's stance on
  `requireHuman`.
- Real-time/streaming guard status — moot now that the guard is synchronous
  within a single request/response cycle.

## Open questions

- **Deliberate** - Exact wording of the `502` message for AC-22, and of the
  `409` messages for AC-18/AC-20, left to the implementer.
- **Deliberate** - Whether the composer's "send anyway" control for a
  `revise` verdict (AC-24) is a modal, an inline banner, or something else
  — left to the implementer; the requirement is only that findings are
  shown and a reason is required before the override request is sent.
- **Deliberate** - How the composer decides "unedited since insertion" for
  AC-23 (e.g. tracking the exact inserted string vs. a dirty flag on any
  keystroke) — either satisfies the AC as long as any edit clears the flag.

## Definition of done

- Every AC (AC-1 through AC-25) covered by a test naming its id.
- Every Invariant covered by a test (in particular: an `escalate` verdict
  is never sendable via any override; a failed guard call never creates a
  `Comment`; a finding quote is always a substring of the candidate reply
  it was produced from; `fromCannedResponse` never appears on a stored
  `Comment`).
- `backend/internal/drafts/` and its migration are fully removed, with no
  remaining references.
- `make ci` green (vet, lint, backend + frontend tests, typecheck, build).
- `backend/internal/docs/openapi.yaml` updated and consistent with the
  implemented endpoints.
