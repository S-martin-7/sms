CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,                -- sms.delivered, sms.undelivered, sms.rejected, sms.inbound
    payload     JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_tenant_id ON events(tenant_id, id DESC);
CREATE INDEX idx_events_tenant_type_id ON events(tenant_id, type, id DESC);
