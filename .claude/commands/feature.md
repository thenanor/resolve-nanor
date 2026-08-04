---
description: Build a feature end-to-end via explore → plan → implement → test → summary
argument-hint: <feature description>
---

Feature request: $ARGUMENTS

Work through this feature using the following loop, in order. Do not skip a stage or collapse
stages together — each one produces something the next stage depends on.

## 1. Explore

Before writing any plan, understand the relevant parts of the codebase:

- Find the files, modules, and patterns relevant to this feature (use the Explore agent for
  broad searches; use direct Read/Grep for targeted lookups).
- Identify existing conventions to follow (naming, file layout, testing style, error handling).
- Identify related code that could be affected or that this feature should be consistent with.
- Note any open questions or ambiguities in the request.

If the request is ambiguous or there's a real fork in approach (e.g. two valid architectures,
missing requirements), ask the user via AskUserQuestion before proceeding — don't guess on
something consequential.

## 2. Plan

Once you understand the codebase context, produce a concrete implementation plan:

- Use EnterPlanMode for this stage if the feature is non-trivial, so the user can review and
  approve the approach before code changes happen.
- The plan should name specific files to change/add, the shape of the change in each, and the
  order of operations.
- Call out trade-offs only where a real decision was made — skip exhaustive alternatives.
- Keep the plan scoped to what the feature actually requires. No speculative abstractions or
  unrelated cleanup.

Wait for plan approval before moving to implementation.

## 3. Implement

- Follow the approved plan. If reality diverges from the plan in a meaningful way (a file
  doesn't exist, an assumption was wrong), adjust and briefly say why, don't silently improvise.
- Match existing code style and conventions found during exploration.
- Keep changes scoped to the feature — no drive-by refactors.
- Use TodoWrite to track multi-step implementation progress.

## 4. Test

- Run the project's existing test suite / linter / type-checker relevant to the changed code.
- Add or update tests that cover the new feature's behavior, following existing test patterns.
- For UI/frontend features, actually run the app and exercise the feature in a browser rather
  than relying on type-checks alone.
- If something can't be verified in this environment, say so explicitly instead of claiming it
  works.

## 5. Summary

End with a concise summary (a few sentences, not a report):

- What was built and where (file:line references for key pieces).
- What was tested and how, and anything that could not be verified.
- Any follow-ups or known gaps worth flagging.
