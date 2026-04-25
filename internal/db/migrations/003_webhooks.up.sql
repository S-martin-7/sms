CREATE TABLE webhook_endpoints (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL,                            -- HMAC key shared with tenant
    events      TEXT[] NOT NULL,                          -- e.g. {sms.delivered, sms.undelivered, sms.rejected, sms.inbound}
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_endpoints_tenant_active ON webhook_endpoints(tenant_id) WHERE active;

CREATE TABLE webhook_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    endpoint_id     BIGINT NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id        UUID NOT NULL,                         -- idempotency key surfaced as X-Event-Id
    event_type      TEXT NOT NULL,                         -- sms.delivered, sms.inbound, etc.
    payload         JSONB NOT NULL,                        -- the body POSTed to the tenant
    status          TEXT NOT NULL DEFAULT 'pending',       -- pending|in_flight|success|failed|dead
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status     INT,                                   -- last HTTP status from tenant (NULL if no response)
    last_error      TEXT,                                  -- last error string (timeouts, dial fail, etc.)
    last_response   TEXT,                                  -- truncated body of last response (for debugging)
    claimed_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at    TIMESTAMPTZ
);
CREATE INDEX idx_wd_pickup ON webhook_deliveries(next_attempt_at)
    WHERE status IN ('pending','failed');
CREATE INDEX idx_wd_in_flight ON webhook_deliveries(claimed_at) WHERE status = 'in_flight';
CREATE INDEX idx_wd_tenant_created ON webhook_deliveries(tenant_id, created_at DESC);
