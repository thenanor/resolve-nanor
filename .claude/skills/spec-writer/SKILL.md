---
name: spec-writer
description: Turn a ticket, feature request, or rough idea into an executable specification with numbered acceptance criteria, invariants, constraints and non-goals - then interrogate it for ambiguity before any code is written. Use when the user wants to write a spec, plan a feature, refine requirements or asks what is ambiguous or missing in a ticket.
---

# Spec writer

Convert intent into a contract an agent can execute and a test suite can prove. Two phases: draft, then interrogate. **Never write implementation code in this skill.**

## Phase 1 - Draft

Read `CLAUDE.md` and the relevant modules first: a spec that contradicts existing conventions or duplicates an existing endpoint is worse than no spec.

Write to `.claude/specs/<slug>.md`:

```markdown
# [ID] - [Feature]

## Context
[2–3 lines: what problem, for whom, why now. This prevents the agent
from "helpfully" reinterpreting the feature into something else.]

## Acceptance criteria

<!-- Numbered and atomic — see "Writing AC that agents can execute" below. -->

- **AC-1** - [Given/when/then, or a plain checkable statement]
- **AC-2** — …
- **AC-3** — …

## Invariants

<!-- Not scenarios: properties that hold in EVERY state, after every
     operation. These become property tests or repeated assertions. -->

- [e.g. "total refunded never exceeds captured amount"]
- [e.g. "every status change has exactly one audit entry"]

## Constraints

- Follow CLAUDE.md conventions
- [No new dependencies / perf bound / security requirement / API compatibility rule]

## Non-goals

<!-- Explicit, with reasons. Agents are enthusiastic; this is the fence. -->

- [Thing that sounds related but is out of scope] — [why / where it lives]

## Open questions

<!-- What you DELIBERATELY leave to implementation judgment, and what is
     still blocked on a human decision. Anything blocked is not ready to
     implement. -->

- [Deliberate: "response shape of the list endpoint — implementer's call"]
- [Blocked: "does reopening reset SLA? — needs support lead"]

## Definition of done

- Every AC covered by at least one test whose name cites the AC id,
  e.g. `it('AC-4: rejects reopen after 7 days', ...)`
- Invariants covered by tests
- Full suite green
- PR description lists the AC ids implemented
- [Docs/README updated if the public API changed]
```

## Writing AC that agents can execute

| Weak | Executable |
|---|---|
| "Reopening should work smoothly" | **AC-1** — `POST /tickets/:id/reopen` on a `resolved` ticket returns 200 and sets status to `open` |
| "Don't allow old tickets to reopen" | **AC-4** — reopening a ticket resolved more than 7 days ago returns 400 with a message naming the window |
| "Track who did it" | **AC-6** — every reopen writes an audit entry `ticket.reopened` with the `X-Actor` value and the previous status |
| "Handle errors" | **AC-7** — reopening a ticket in any status other than `resolved` returns 400 listing the allowed source states |

Rules for acceptance criteria:

- One behavior per AC: include failure modes, not just the happy path
- Name the observable: status code, field, audit action, state
- Never name a class, method, or file - that is implementation
- If you cannot imagine the assertion, the AC is not finished

## Phase 2 - Interrogate

Now read the draft as a hostile implementer: someone who will follow it exactly as written and is not responsible for guessing what you meant. Find:

1. Ambiguities - quote the line, give both readings
2. Missing edge cases - empty, boundary, repeated, concurrent, out-of-order, already-in-target-state
3. Undefined transitions - for anything with a lifecycle
4. Unstated assumptions - about data, permissions, existing behavior
5. Conflicts - with CLAUDE.md, with existing endpoints, or internal
6. Untestable criteria

Rank by blast radius, then output:

```
BLOCKING DECISIONS (human must decide before implementation)
1. [question] - [why blocking] - [2-3 realistic options]

SAFE TO DECIDE DURING IMPLEMENTATION
- [question] - [reasonable default and why]

SUGGESTED ADDITIONS
- [AC or invariant, written in the spec's format]
```

Apply the additions the user approves, then stop. Implementation is a separate request.

Cite AC ids consistently in test names and PR descriptions (see Definition
of done above) so coverage of requirements becomes a grep, not a meeting.

