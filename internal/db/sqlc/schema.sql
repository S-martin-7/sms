CREATE TABLE tenants (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',
  daily_sms_limit INT,
  monthly_budget  NUMERIC(12,4),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  prefix          TEXT NOT NULL UNIQUE,
  hash            TEXT NOT NULL,
  hash_algo       TEXT NOT NULL DEFAULT 'sha256-v1',
  name            TEXT,
  scopes          TEXT[] NOT NULL DEFAULT ARRAY['send','read','webhooks']::TEXT[],
  last_used_at    TIMESTAMPTZ,
  revoked_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_api_keys_tenant_active ON api_keys(tenant_id) WHERE revoked_at IS NULL;

CREATE TABLE admin_users (
  id              BIGSERIAL PRIMARY KEY,
  email           TEXT UNIQUE NOT NULL,
  password_hash   TEXT NOT NULL,
  role            TEXT NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id              BIGSERIAL PRIMARY KEY,
  actor_id        BIGINT REFERENCES admin_users(id),
  action          TEXT NOT NULL,
  target_type     TEXT,
  target_id       TEXT,
  metadata        JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_actor ON audit_log(actor_id, created_at DESC);

CREATE TABLE messages (
    id              UUID PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id),
    sender          TEXT NOT NULL,
    recipient       TEXT NOT NULL,
    text            TEXT NOT NULL,
    dcs             TEXT NOT NULL,
    num_parts       SMALLINT NOT NULL,
    status          TEXT NOT NULL,
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

CREATE TABLE webhook_endpoints (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL,
    events      TEXT[] NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_webhook_endpoints_tenant_active ON webhook_endpoints(tenant_id) WHERE active;

CREATE TABLE webhook_deliveries (
    id              BIGSERIAL PRIMARY KEY,
    endpoint_id     BIGINT NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id        UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status     INT,
    last_error      TEXT,
    last_response   TEXT,
    claimed_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at    TIMESTAMPTZ
);
CREATE INDEX idx_wd_pickup ON webhook_deliveries(next_attempt_at)
    WHERE status IN ('pending','failed');
CREATE INDEX idx_wd_in_flight ON webhook_deliveries(claimed_at) WHERE status = 'in_flight';
CREATE INDEX idx_wd_tenant_created ON webhook_deliveries(tenant_id, created_at DESC);

CREATE TABLE inbound_numbers (
    msisdn      TEXT PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    label       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_inbound_numbers_tenant ON inbound_numbers(tenant_id);

CREATE TABLE inbound_messages (
    id          UUID PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    horisen_id  TEXT UNIQUE,
    src         TEXT NOT NULL,
    dst         TEXT NOT NULL,
    text        TEXT NOT NULL,
    dcs         TEXT,
    received_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_inbound_messages_tenant_received ON inbound_messages(tenant_id, received_at DESC);

CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    payload     JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_tenant_id ON events(tenant_id, id DESC);
CREATE INDEX idx_events_tenant_type_id ON events(tenant_id, type, id DESC);
