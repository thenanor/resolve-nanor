---
name: spec-writer
description: Turn a ticket, feature request, or rough idea into an executable specification with numbered acceptance criteria, invariants, constraints and non-goals - then interrogate it for ambiguity before any code is written. Use when the user wants to write a spec, plan a feature, refine requirements or asks what is ambiguous or missing in a ticket.
---

# Spec writer

Convert intent into a contract an agent can execute and a test suite can prove. Two phases: draft, then interogate. **Never write implementation code in this skill.**

## Phase 1 - Draft

Read `Claude.md` and the relevant modules first: a spec that contredicts existing conventions or duplicates an existing endpoint is worse than no spec.

Write to `specs/<slug>.md`:

```markdown
# [ID] - [Feature]

## Context
[2-3 lines: what problem, for whom, why now]

## Acceptance criteria
- **AC-1** - [atomic, observable at an API boundary, assertable]
- **AC-2** - _

## Invariants
- [properties true in every state, after every operation]

## Constraints
- Follow CLAUDE.md conventions
- [dependencies / performance / compatibility / security bounds]

## Non-goals
- [out of scope] - [why, or where it lives instead]

## Open questions
- [deliberate: left to implementer judgement]
- [blocked: needs a human decision - name who]

## Definition of done
- Every AC covered by a test naming its id; invariants tested; suite green
```

Rules for acceptance criteria:

- One behavior per AC: include failure modes, not just the happy path
- Name the observable: status code, field, audit action, state
- Never name a class, method, or file - that is implementation
- If you cannot imagine the assertion, the AC is not finished

## Phase 2 - Interrogate

Now attach the draft as a hostile implementer who will follow it exactly as written and it is not responsible for guessing wee. Find:

1. Ambiguities - quote the line, give both readings
2. Missing edge cases - empty, boundary, repeated, concurrent, out-of-order, already-in-target-state
3. Undefined transitions - for anything with a lifecycle
4. Unstated assumptions - about data, permissions, existing behavior
5. Conflicts - with CLAUDE.md, with existing endpoints, or internal
6. Untestable criteria

Rank by blast radius, then output:

```
BLOCKING DECISIONS (human must decide before imaplementation)
1. [question] - [why blocking] - [2-3 realistic options]

SAFE TO DECIDE DURING IMPLEMENTATION
- [question] - [reasonable default and why]

SUGGESTED ADDITIONS
- [AC or invariant, written in the spec's format]
```

Apply the additions the user approves, then stop. Implementation is a separate request.

