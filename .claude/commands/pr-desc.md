---
description: Generate a PR description from the current diff and its matching spec — summary, AC coverage, reviewer risks, deliberate omissions
argument-hint: [spec-name]
allowed-tools: Bash(git log:*), Bash(git diff:*), Bash(git status:*), Read, Grep, Glob, AskUserQuestion
---

Spec: $ARGUMENTS (optional — see stage 2 if omitted)

Work through this in order — each stage feeds the next, don't skip to the output format early.

## 1. Gather the diff

- Base branch is always `main`.
- `git log main..HEAD --oneline` for the commit list, `git diff main...HEAD` for the full diff,
  `git diff main...HEAD --stat` for a shape overview.
- If there's no diff against `main`, say so and stop rather than inventing content.

## 2. Find the matching spec

- If `$ARGUMENTS` was given, treat it as the spec name/slug: resolve it against
  `.claude/specs/*.md` (exact filename match first, then a fuzzy match on title/slug). If it
  doesn't resolve to a file, say so and fall back to the matching step below rather than
  silently dropping it.
- Otherwise, list `.claude/specs/*.md` and match a spec to the diff by content, not just
  filename: compare the spec's `Context` and package/file references against what actually
  changed (e.g. a diff touching `backend/internal/cannedresponses/` matches
  `canned-responses.md`).
- If exactly one spec is resolved (via `$ARGUMENTS` or content match), read it in full
  (Acceptance criteria, Invariants, Constraints, Non-goals, Open questions).
- If multiple specs plausibly match, or none do, ask the user via AskUserQuestion which spec
  applies (or confirm there isn't one) rather than guessing — a wrong AC list is worse than
  asking.
- No matching spec is a valid outcome: proceed without an AC-coverage section, and note that
  explicitly in the output rather than fabricating criteria.

## 3. Determine acceptance-criteria coverage

For each `AC-<n>` in the matched spec:

- Grep the diff and changed test files for the id (this repo's specs require tests to name
  their AC id — see each spec's "Definition of done").
- Classify each one:
  - **Covered** — a test explicitly names the id.
  - **Likely covered, unverified** — code changed in a way that plausibly satisfies the AC, but
    no test references the id directly. Say so as a caveat, don't upgrade it to "Covered."
  - **Not covered** — no evidence in the diff.
- Do not mark an AC covered on inference alone. If you can't confirm it, say you can't.

## 4. Identify risks a reviewer should look at first

Prioritize by blast radius, not line count. Specifically flag, when present in the diff:

- Schema/migration changes (`backend/internal/migrations/schema.sql`).
- Changes to invariant-bearing logic — the status-transition map
  (`transitions.go`/`transitions.ts`), validation rules, id generation.
- Anything audit-adjacent (even if the protect-audit hook didn't block it, mutation-logging
  changes deserve a second look).
- Auth/actor handling (`X-Actor`, `httpx.Actor`).
- Shared contract files that are easy to forget: `backend/internal/docs/openapi.yaml`,
  frontend/backend copies of the same rule.
- Changed code paths with no corresponding test change.

## 5. What was deliberately not done

- Pull directly from the matched spec's `Non-goals` and any `Deliberate` entries in
  `Open questions` — these are already-decided scope cuts, not gaps to invent.
- Cross-check: if the diff actually touches something the spec listed as a non-goal, flag that
  as a discrepancy between spec and implementation instead of silently listing it as a non-goal.
- If no spec was found, base this section only on what the diff visibly leaves out (e.g. a
  TODO, an obvious counterpart change not made) — don't speculate beyond that.

## 6. Output

Produce the PR description in this shape:

```markdown
## Summary
[2-4 sentences: what changed and why, in plain terms]

## Acceptance criteria covered
- AC-1 — [covered / likely covered, unverified / not covered] — [one line]
- AC-2 — ...

(Omit this section, with a one-line note why, if no spec matched.)

## Risks — review these first
- [highest blast-radius item first, with file:line]

## Deliberately not done
- [item] — [why, referencing the spec's Non-goals/Open questions where applicable]
```

After producing it, ask the user whether to open it as an actual PR (`gh pr create --body ...`)
— don't run that without confirmation, since creating a PR is visible to others.
