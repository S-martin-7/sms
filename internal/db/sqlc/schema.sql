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
