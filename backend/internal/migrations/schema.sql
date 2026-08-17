CREATE TABLE IF NOT EXISTS tickets (
    id             VARCHAR PRIMARY KEY,
    subject        VARCHAR NOT NULL,
    description    TEXT NOT NULL,
    customer_email VARCHAR NOT NULL,
    priority       VARCHAR NOT NULL,
    status         VARCHAR NOT NULL,
    created_at     VARCHAR NOT NULL,
    updated_at     VARCHAR NOT NULL,
    resolved_at    VARCHAR
);

-- Added after the initial schema; CREATE TABLE IF NOT EXISTS above is a
-- no-op on an already-existing tickets table, so new columns need an
-- explicit, idempotent ALTER TABLE. Nullable / no default: a ticket starts
-- untriaged, and pending_* only populate for a low-confidence suggestion
-- awaiting human review.
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS category VARCHAR;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS pending_category VARCHAR;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS pending_priority VARCHAR;

CREATE TABLE IF NOT EXISTS ticket_comments (
    seq        BIGSERIAL PRIMARY KEY,
    id         VARCHAR UNIQUE NOT NULL,
    ticket_id  VARCHAR NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author     VARCHAR NOT NULL,
    body       TEXT NOT NULL,
    internal   BOOLEAN NOT NULL DEFAULT FALSE,
    at         VARCHAR NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ticket_comments_ticket_id ON ticket_comments(ticket_id);

CREATE TABLE IF NOT EXISTS audit_entries (
    seq        BIGSERIAL PRIMARY KEY,
    actor      VARCHAR NOT NULL,
    action     VARCHAR NOT NULL,
    ticket_id  VARCHAR NOT NULL,
    details    JSONB NOT NULL DEFAULT '{}'::jsonb,
    at         VARCHAR NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_entries_ticket_id ON audit_entries(ticket_id);

CREATE TABLE IF NOT EXISTS canned_responses (
    id         VARCHAR PRIMARY KEY,
    title      VARCHAR NOT NULL,
    body       TEXT NOT NULL,
    tags       TEXT[] NOT NULL DEFAULT '{}',
    created_at VARCHAR NOT NULL,
    updated_at VARCHAR NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_canned_responses_tags ON canned_responses USING GIN(tags);

-- A reply an agent has written but not yet sent to the customer. Reviewed
-- by the reply-guard service before it may become a real ticket_comments
-- row (see backend/internal/drafts). guard_result is one JSONB blob
-- (verdict/findings/confidence/reasoning/injectionSuspected/requireHuman
-- together) rather than normalized columns, matching audit_entries.details
-- above — it's written and read as a single unit, never queried by its
-- sub-fields.
CREATE TABLE IF NOT EXISTS drafts (
    id           VARCHAR PRIMARY KEY,
    ticket_id    VARCHAR NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author       VARCHAR NOT NULL,
    body         TEXT NOT NULL,
    status       VARCHAR NOT NULL,
    guard_result JSONB,
    created_at   VARCHAR NOT NULL,
    updated_at   VARCHAR NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_drafts_ticket_id ON drafts(ticket_id);
