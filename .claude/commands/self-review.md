---
description: Hostile-reviewer pass over your uncommitted diff — conventions, invented behavior, weak tests, missing edge cases. Report only, fixes nothing.
argument-hint: [optional focus, e.g. "just the backend" or "tests only"]
allowed-tools: Bash(git status:*), Bash(git diff:*), Bash(git log:*), Read, Grep, Glob, ReportFindings
---

Focus: $ARGUMENTS (optional — if empty, review the full uncommitted diff)

You are reviewing this diff as a hostile reviewer, not a helpful pair programmer. Assume the
author is trying to sneak something past you: an untested branch, a rule that only half-matches
its counterpart, a test that asserts the easy thing instead of the real thing. Your job is to
find what would embarrass them in review — not to rewrite their code, and not to be nice about
it. Do not fix anything, even something trivial to fix. Report only.

## 1. Gather the uncommitted diff

- `git status --porcelain` (never `-uall`) to see everything modified, staged, and untracked.
- `git diff HEAD` for the full tracked diff (staged + unstaged together).
- For any `??` untracked files in status, read them in full — they won't appear in `git diff`
  but are still uncommitted work in scope for this review.
- If there is nothing uncommitted, say so and stop rather than reviewing old history.
- If `$ARGUMENTS` names a focus area, narrow to the matching files/hunks, but still run the full
  gather first so you're not missing context (e.g. a frontend change that should have a backend
  counterpart).

## 2. Establish ground truth before judging

Don't invent conventions to hold the diff to. Before flagging something as "wrong," confirm
what "right" actually looks like in this repo:

- Check `CLAUDE.md` for documented conventions (package shape, error-handling pattern via typed
  errors, sync points like `transitions.go`/`transitions.ts`, `openapi.yaml`, the audit trail).
- Look at sibling files in the same package/module for the pattern this diff should be
  following (naming, test structure, how errors are returned, how similar endpoints are shaped).
- If a spec exists for this work (`.claude/specs/*.md`), read it — it's the source of truth for
  what was actually asked for, which matters for step 3b below.

## 3. Review across four lenses

Work through the diff systematically, not just skimming for anything that looks off. For each
finding, note the file:line and be concrete about the failure mode — "this could be cleaner"
is not a finding, "X breaks when Y" is.

**a. Convention violations** — deviations from the established pattern found in step 2: naming,
file layout, error-handling shape, a mirrored pair (frontend/backend transition map, spec docs,
openapi.yaml) that was updated on one side and not the other, service logic leaking into a
handler, etc.

**b. Invented behavior** — anything the diff does beyond what was asked. New parameters or
endpoints nobody requested, silent fallback/default logic, speculative abstraction for a case
that doesn't exist yet, a comment asserting behavior the code doesn't actually implement,
scope creep past the spec or task at hand.

**c. Weak tests** — tests that would pass even if the logic were wrong: assertions on the wrong
value, missing assertions on the interesting output, tautological tests, mocking something that
should be exercised for real, only testing the happy path, a test name that promises more than
it checks.

**d. Missing edge cases** — inputs or states the diff doesn't handle: boundary values, empty/nil,
pagination limit/offset extremes, invalid status transitions, unauthorized or missing actor,
concurrent mutation, malformed input the handler should reject but doesn't.

## 4. Verify before reporting

For each candidate finding, re-check it against the actual file content — don't report something
you inferred without confirming it in the code. If you can't confirm a finding by reading the
relevant file, drop it rather than reporting a guess. Prefer fewer, confirmed findings over a
long list padded with maybes.

## 5. Report

Call `ReportFindings` with the verified findings, most severe first (blast radius over line
count — a silently-diverged status-transition map outranks a naming nitpick). Do not also print
the findings as prose. Do not edit, fix, or stage anything — this command reports only.
