CREATE TABLE messages (
    id              UUID PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id),
    sender          TEXT NOT NULL,
    recipient       TEXT NOT NULL,
    text            TEXT NOT NULL,
    dcs             TEXT NOT NULL,
    num_parts       SMALLINT NOT NULL,
    status          TEXT NOT NULL,               -- queued|sending|sent|delivered|undelivered|rejected|failed
    horisen_msg_id  TEXT,
    error_code      TEXT,
    error_message   TEXT,
    client_ref      TEXT,
    attempts        INT NOT NULL DEFAULT 0,
    claimed_at      TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ,
    final_at        TIMESTAMPTZ,
    UNIQUE (tenant_id, client_ref)
);

CREATE INDEX idx_messages_queue ON messages(next_attempt_at) WHERE status = 'queued';
CREATE INDEX idx_messages_sending ON messages(claimed_at) WHERE status = 'sending';
CREATE INDEX idx_messages_tenant_created ON messages(tenant_id, created_at DESC);
CREATE INDEX idx_messages_horisen ON messages(horisen_msg_id) WHERE horisen_msg_id IS NOT NULL;
