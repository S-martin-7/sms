CREATE TABLE inbound_numbers (
    msisdn      TEXT PRIMARY KEY,                              -- E.164 without leading '+'
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    label       TEXT,                                          -- e.g. "shortcode 12345 — marketing"
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_inbound_numbers_tenant ON inbound_numbers(tenant_id);

CREATE TABLE inbound_messages (
    id          UUID PRIMARY KEY,                              -- our internal id surfaced via webhook
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    horisen_id  TEXT UNIQUE,                                   -- Horisen's MO id, used for idempotency
    src         TEXT NOT NULL,                                 -- sender MSISDN
    dst         TEXT NOT NULL,                                 -- our number that received it
    text        TEXT NOT NULL,
    dcs         TEXT,                                          -- 'GSM' | 'UCS' (Horisen tells us)
    received_at TIMESTAMPTZ NOT NULL,                          -- timestamp from Horisen payload
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_inbound_messages_tenant_received ON inbound_messages(tenant_id, received_at DESC);
