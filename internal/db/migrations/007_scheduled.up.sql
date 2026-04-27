-- scheduled_sends: una "programación" puede ser one-shot (recurrence NULL)
-- o recurrente semanal (recurrence='weekly' + recurrence_days = subset de [0..6]
-- domingo=0..sabado=6). Para recurrentes, send_at es la PRÓXIMA fecha de
-- ejecución; el worker la actualiza después de cada disparo.
CREATE TABLE scheduled_sends (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            TEXT,                                  -- etiqueta libre, p.ej. "Recordatorio mensual"
    sender          TEXT NOT NULL,
    text            TEXT NOT NULL,
    -- destinatarios: array literal de msisdns O id de una contact_list (uno de los dos).
    recipients      JSONB,                                  -- array de strings, NULL si usa list_id
    list_id         BIGINT REFERENCES contact_lists(id) ON DELETE SET NULL,
    -- timing
    send_at         TIMESTAMPTZ NOT NULL,                   -- próximo disparo
    recurrence      TEXT,                                   -- NULL=one-shot, 'weekly'
    recurrence_days SMALLINT[],                             -- 0..6 si recurrence='weekly'
    timezone        TEXT NOT NULL DEFAULT 'America/Santiago',
    -- estado
    status          TEXT NOT NULL DEFAULT 'pending',        -- pending|paused|completed|failed
    last_run_at     TIMESTAMPTZ,
    last_batch_id   TEXT,                                   -- batch_id devuelto por bulk send
    total_runs      INT NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_by      BIGINT REFERENCES admin_users(id) ON DELETE SET NULL,
    api_key_id      BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,  -- si fue creado vía API pública
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Pickup index — el worker hace SELECT con FOR UPDATE SKIP LOCKED.
CREATE INDEX idx_scheduled_pickup ON scheduled_sends(send_at) WHERE status = 'pending';
CREATE INDEX idx_scheduled_tenant ON scheduled_sends(tenant_id, created_at DESC);
