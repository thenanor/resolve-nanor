# CANNED-1 - Canned responses with tags for ticket comments

## Context
Agents currently write every ticket comment from scratch (`POST /tickets/{id}/comments`,
free-text `body`). Canned responses let an agent save reusable comment text, tag it for
retrieval, and insert it into the comment form before submitting — without changing how
comments themselves are created or stored.

## Acceptance criteria

**Create**
- **AC-1** - `POST /canned-responses` with a non-empty (after trim) `title` and non-empty
  (after trim) `body` returns `201` and the created resource: `id`, `title`, `body`, `tags`
  (array, possibly empty), `createdAt`, `updatedAt`.
- **AC-2** - `POST /canned-responses` with an empty or whitespace-only `title` returns `400`
  with a message naming `title` as the offending field.
- **AC-3** - `POST /canned-responses` with an empty or whitespace-only `body` returns `400`
  with a message naming `body` as the offending field.
- **AC-4** - `POST /canned-responses` with `tags` omitted defaults to an empty array; the
  request is not rejected for lacking tags.
- **AC-5** - Each tag in `tags` is stored trimmed and lower-cased, so `"Billing"` and
  `" billing "` submitted on two different canned responses are stored as the identical
  string `"billing"`.
- **AC-6** - `POST /canned-responses` with a `tags` array containing an empty/whitespace-only
  entry returns `400`.
- **AC-7** - The generated `id` matches the existing id-format convention (a short prefix
  followed by 8 hex characters, e.g. `cr_a1b2c3d4`), consistent with ticket ids.

**Read**
- **AC-8** - `GET /canned-responses` returns `200` with all canned responses, newest-first
  is not guaranteed unless specified — order is by `title` ascending (see Open Questions if
  this is wrong).
- **AC-9** - `GET /canned-responses?tag=billing` returns only canned responses whose `tags`
  contain the normalized value `"billing"`; matching is case-insensitive on the query param.
- **AC-10** - `GET /canned-responses?tag=` (empty value) is treated as no filter and returns
  the full list, matching how absent-vs-empty filters behave elsewhere in this API.
- **AC-11** - `GET /canned-responses/{id}` returns `200` and the resource when it exists.
- **AC-12** - `GET /canned-responses/{id}` returns `404` when no canned response with that id
  exists.

**Update**
- **AC-13** - `PUT /canned-responses/{id}` with valid `title`/`body`/`tags` returns `200` with
  the updated resource and a changed `updatedAt`.
- **AC-14** - `PUT /canned-responses/{id}` on a nonexistent id returns `404`.
- **AC-15** - `PUT /canned-responses/{id}` with an empty/whitespace `title` or `body` returns
  `400`, same rule as create (AC-2/AC-3).
- **AC-16** - `PUT /canned-responses/{id}` re-normalizes `tags` the same way create does
  (AC-5/AC-6).
- **AC-25** - The edit action in the canned-response UI presents a confirmation prompt before
  issuing the `PUT` request; canceling the prompt issues no request and the canned response's
  stored fields are unchanged.

**Delete**
- **AC-17** - `DELETE /canned-responses/{id}` on an existing id returns `204` and the
  resource is no longer returned by `GET /canned-responses` or `GET /canned-responses/{id}`.
- **AC-18** - `DELETE /canned-responses/{id}` on a nonexistent id returns `404`.
- **AC-19** - Deleting a canned response does not alter any ticket comment previously created
  by inserting its text — comments are plain stored text with no live reference back to the
  canned response (see Invariants).
- **AC-24** - The delete action in the canned-response UI presents a confirmation prompt
  identifying the canned response (at minimum by title) before issuing the `DELETE` request;
  canceling the prompt issues no request and the canned response remains listed.

**Using a canned response in a comment**
- **AC-20** - Inserting a canned response into the comment composer populates the comment
  body field with the canned response's `body` text; it does not call
  `POST /tickets/{id}/comments` itself and does not submit the comment.
- **AC-21** - After insertion, the comment body field remains editable and the existing
  `author`/`internal` fields on the comment form are untouched by the insertion.
- **AC-22** - `POST /tickets/{id}/comments` never references a canned response id — inserting
  a canned response is a client-side convenience only (see Invariants and Non-goals). Per
  `REPLYGUARD-1`, the request additionally accepts an optional `fromCannedResponse` flag the
  composer sets when the submitted body is unedited from the inserted canned response (see
  `REPLYGUARD-1` AC-23); that flag is never persisted and is not itself a reference to the
  canned response's `id`, so it does not weaken this AC's "no reference" guarantee.
- **AC-23** - The comment composer can filter the canned-response picker by tag and/or by a
  case-insensitive substring match on `title`.

## Invariants
- A canned response's `id` is immutable and unique for the lifetime of the record.
- `tags` on a stored canned response are always trimmed, lower-cased, and non-empty strings;
  the array may itself be empty but never contains an empty-string element.
- Creating, editing, or deleting a canned response never mutates any existing ticket or
  ticket comment. Canned responses and comments are related only at the moment of insertion,
  client-side — no foreign key or stored reference connects a `ticket_comments` row back to a
  `canned_responses` row.
- `POST /tickets/{id}/comments` behavior, request shape, and response shape are unchanged by
  this feature, aside from the optional `fromCannedResponse` flag added by `REPLYGUARD-1`
  (AC-22) — that flag exists to let the guard step introduced there skip already-approved
  canned text; it carries no canned-response `id` and is never stored.

## Constraints
- Follow CLAUDE.md conventions: new `backend/internal/cannedresponses/` package following the
  existing entity → `Repository` interface → `PostgresRepository` → `Service` → `Handler`
  shape (see `tickets/`); typed `*ValidationError` / `*NotFoundError` returned from the
  service and mapped to HTTP status in the handler, matching `tickets/errors.go`.
- Schema change goes into `backend/internal/migrations/schema.sql` (no migration tool, applied
  at startup) — a new `canned_responses` table (+ tag storage, shape left to implementer per
  Open Questions).
- `backend/internal/docs/openapi.yaml` updated with the new endpoints/schemas.
- No new authentication/authorization model is introduced — same `X-Actor`-header-only model
  tickets use today.

## Non-goals
- Variable/placeholder substitution (e.g. `{{customer_name}}`) in canned response bodies —
  static text only for this spec; a substitution engine is a separate feature.
- Audit-trail linkage recording *which* canned response seeded a given comment — out of scope
  since `POST /tickets/{id}/comments` is explicitly unchanged (AC-22). Revisit as a follow-up
  spec if usage analytics become a requirement.
- An "internal-only" flag on canned responses (paralleling `Comment.Internal`) — a canned
  response is just reusable text; whether the resulting comment is internal is controlled by
  the existing comment form field, untouched by this feature (AC-21).
- Per-user or per-team ownership/permissions on canned responses — all canned responses are
  global and mutable by any actor, consistent with the rest of this app having no RBAC.
- Pagination on `GET /canned-responses` — confirmed out of scope; `GET /canned-responses`
  returns a flat, unpaginated array. Revisit with a page envelope only if this list later
  grows the way ticket lists did (see `docs/pagination-verdict.md` for that precedent).
- Title uniqueness — confirmed out of scope; duplicate `title` values across canned responses
  are allowed and not validated against.

## Open questions
- **Deliberate** - Tag storage shape (`TEXT[]` column vs. a normalized join table) is left to
  the implementer; either satisfies AC-5/AC-6/AC-9, a join table only matters if tag
  autocomplete-across-responses becomes a later requirement.
- **Deliberate** - Default sort order for `GET /canned-responses` (AC-8) is set to
  title-ascending as a reasonable default; no product requirement was given for this.
- **Deliberate** - Comment-composer insertion behavior when the body field is non-empty
  (replace vs. append vs. insert-at-cursor) is left to the implementer; AC-20 only requires
  that the field ends up populated with the canned response's body.
- **Deliberate** - Exact confirmation-prompt copy/UX for AC-24/AC-25 (modal vs. inline,
  wording) is left to the implementer; the requirement is only that a confirmation step
  exists and cancellation is a true no-op.

All previously blocked questions (title uniqueness, delete/edit confirmation, pagination)
were resolved by the feature owner: no title uniqueness constraint, confirmation required on
both delete and edit, no pagination. Reflected in the ACs, Non-goals, and Constraints above.

## Definition of done
- Every AC (AC-1 through AC-25) covered by a test naming its id.
- Both Invariants covered by tests (e.g. deleting a canned response after it was used to seed
  a comment leaves that comment's stored `body` unchanged; `POST /tickets/{id}/comments`
  request/response shape has no new fields).
- `make ci` green (vet, lint, backend + frontend tests, typecheck, build).
- `backend/internal/docs/openapi.yaml` updated and consistent with the implemented endpoints.
