---
description: Cross-check a spec's acceptance criteria against its test suite — ACs with no test, tests that assert shape instead of behavior, and the single highest-value test to write next.
argument-hint: [spec-name]
allowed-tools: Read, Grep, Glob, AskUserQuestion
---

Spec: $ARGUMENTS (optional — see stage 1 if omitted)

Report only. Do not write or edit any test files, even the highest-value one you identify at
the end — describe it, don't implement it.

## 1. Resolve the spec

- If `$ARGUMENTS` was given, treat it as the spec name/slug: resolve it against
  `.claude/specs/*.md` (exact filename match first, then a fuzzy match on title/slug). If it
  doesn't resolve, say so and fall back to the matching step below.
- Otherwise, list `.claude/specs/*.md`. If there's exactly one, use it. If there are several,
  ask the user via AskUserQuestion which one to check — don't guess, since AC ids collide
  across specs (`AC-1` exists in every spec) and picking the wrong one produces a report about
  the wrong feature.
- Read the resolved spec in full: every `AC-<n>` under Acceptance criteria, plus Invariants and
  Constraints (Constraints usually names the package/module this feature lives in, which you'll
  need in step 2).

## 2. Find the matching test suite

- Don't guess file paths. Use the package/module named in the spec's Constraints section (e.g.
  "new `backend/internal/cannedresponses/` package") to locate its `_test.go` files, and Glob/Grep
  for related frontend test files (component or page names matching the feature).
- If the feature isn't implemented yet or you can't find any matching test files, say that
  plainly and stop — an empty report is a valid, useful outcome; don't pad it by reviewing
  unrelated tests.
- Read every matching test file in full, not just test names — you need the actual assertions
  for step 4.

## 3. Map AC ids to tests

- Grep the test suite for each `AC-<n>` (this repo's convention, per the spec-writer skill and
  `pr-desc`, is that tests name the AC id they cover — check subtests/`t.Run` names, `describe`/
  `it` blocks, and comments).
- For each AC, classify:
  - **Tested** — a test explicitly names the id, or its assertions unambiguously exercise
    exactly what the AC describes.
  - **No test** — nothing in the suite references the id or plausibly exercises it.
- Do not mark an AC tested on inference alone ("there's probably a test for this somewhere in
  the 400-response block"). If you can't point to the specific test, it's untested.

## 4. Find tests asserting shape instead of behavior

For every test touching this feature (not just the ones mapped to an AC), check whether it
actually verifies the behavior the AC describes, or only verifies that the code is structured a
particular way. Shape-over-behavior looks like:

- Asserting a mock/spy was called (and with what args) instead of asserting the observable
  outcome of the call.
- Asserting on a SQL query string, an internal struct's field count, or a private
  function/method existing, rather than on stored data or a response.
- Snapshotting an entire response object when the AC only cares about one field — real
  regressions in the field that matters get lost in snapshot noise, and unrelated fields
  breaking the snapshot creates false failures.
- Asserting "no error was returned" / "status was not 4xx" without asserting what the success
  response actually contains.
- A test name that promises behavior (`"rejects empty title"`) but whose only assertion is
  structural (e.g. checks a validation function was invoked, not that the request was rejected
  with 400 and the right message).

List each one with file:line, what it currently asserts, and what behavioral assertion is
missing.

## 5. Identify the single highest-value missing test

From everything gathered in steps 3–4 (untested ACs and shape-only tests), pick exactly one —
not a top-3, one. Rank by blast radius, using the same priorities as this repo's invariant-bearing
code (status transitions, audit trail, id generation, mirrored frontend/backend rules, anything
touching money/authorization-equivalent logic) over routine validation. Justify the pick in one
sentence — why this gap over the others.

Describe the test concretely enough that someone could write it without re-reading the spec:
which AC(s) it covers, what file it belongs in (following this suite's existing test file/
naming pattern), the specific input/state, and the specific assertion that would fail today if
the gap is real (or pass falsely today if it's a shape-only test problem).

## 6. Output

```markdown
## Spec: <name> (<path>)

## ACs with no test
- AC-<n> — <one-line: what the AC requires>
(or: "None — every AC maps to at least one test.")

## Tests asserting shape instead of behavior
- <file:line> — asserts: <what it checks now> — missing: <what behavioral assertion should replace/join it>
(or: "None found.")

## Highest-value missing test
**Covers:** AC-<n> (+ others if relevant)
**Why this one:** <one sentence>
**Where:** <test file>
**Test:** <concrete input/state -> expected observable outcome, specific enough to write directly>
```
