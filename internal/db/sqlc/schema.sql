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
