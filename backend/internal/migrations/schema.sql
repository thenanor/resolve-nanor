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
