# Reopening Tickets — Branch Comparison

All three branches modify the same three files (`transitions.go`, `service.go`, `service_test.go` on the backend; `transitions.ts`/`.test.ts` on the frontend), reached via [thenanor/resolve-nanor](https://github.com/thenanor/resolve-nanor) branches `var-1`, `var-2`, `var-3`. None touch the HTTP handler — all reuse the existing generic `POST /{id}/status` endpoint.

| | **var-1** | **var-2** | **var-3** |
|---|---|---|---|
| **Reopening allowed from which states?** | Only `resolved` | Only `resolved` | Only `resolved` |
| **New status or reuse open?** | Reuses `open` (`resolved → open`) | Reuses `in_progress` (`resolved → in_progress`), no new status | Reuses `open` (`resolved → open`) |
| **Audit action name?** | Reused `ticket.status_changed` (no dedicated `reopened` action) | Reused `ticket.status_changed` | Reused `ticket.status_changed` |
| **Window limit invented?** | No — no time-based restriction added | No | No |
| **API shape?** | Same generic `POST /{id}/status {to}` endpoint; only difference is a UI label — button text shows "reopen" instead of "open" when ticket is resolved (`TicketDetailPage.tsx`) | Same generic `POST /{id}/status {to}` endpoint; no UI label change | Same generic `POST /{id}/status {to}` endpoint; no UI label change |
| **Test depth?** | 2 new Go tests (reopen happy-path + re-resolve, reject-reopen-from-open) + 1 trivial FE test. Doesn't explicitly test rejection from a genuinely mid-flow state (e.g. `in_progress`) | 2 new Go tests (reopen happy-path, re-resolve sets new `resolvedAt`) + 1 trivial FE test. No explicit rejection test for reopening from non-resolved states beyond the pre-existing "closed" test | **Deepest**: 4 new Go tests — reopen happy-path w/ round-trip through `closed`, explicit regression test that closing *without* reopening preserves `resolvedAt`, rejection test from a genuine non-resolved state (`in_progress`), and a dedicated audit-trail test — + 1 trivial FE test |

## Notable details

- **`resolvedAt` handling** differs subtly in implementation style but is functionally equivalent in var-1/var-3 (`resolvedAt` cleared when leaving `resolved` to anywhere but `closed`); var-2's condition is narrower (`== in_progress` explicitly) since `in_progress` is its only reopen target.
- **var-1** is the only branch that also updates the README.md state diagram and adds a "reopen" label in the UI — the only variant with a user-facing distinction between "open" (fresh ticket) and "reopen" (resurrected ticket), even though under the hood it's still the same `open` status.
- **var-3** is the only branch with a regression test proving normal `resolved → closed` still preserves `resolvedAt` (guards against a regression the other two don't explicitly test for).
- **var-2** is functionally distinct in behavior, not just code style: reopened tickets land in `in_progress` (skipping `open`), meaning a reopened ticket can't be filtered/found via an "open tickets" status filter the way var-1/var-3's do.
