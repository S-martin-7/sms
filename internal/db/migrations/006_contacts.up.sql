CREATE TABLE contacts (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    msisdn        TEXT NOT NULL,                         -- E.164 sin '+'
    name          TEXT,
    notes         TEXT,
    opt_out       BOOLEAN NOT NULL DEFAULT false,        -- bajas voluntarias
    opt_out_at    TIMESTAMPTZ,
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,    -- campos custom del cliente
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, msisdn)
);
CREATE INDEX idx_contacts_tenant ON contacts(tenant_id, msisdn);
CREATE INDEX idx_contacts_tenant_optout ON contacts(tenant_id) WHERE opt_out = true;
-- Búsqueda case-insensitive por nombre o número
CREATE INDEX idx_contacts_search ON contacts USING gin (
    to_tsvector('simple', coalesce(name,'') || ' ' || msisdn)
);

CREATE TABLE contact_lists (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX idx_contact_lists_tenant ON contact_lists(tenant_id);

CREATE TABLE contact_list_members (
    list_id       BIGINT NOT NULL REFERENCES contact_lists(id) ON DELETE CASCADE,
    contact_id    BIGINT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (list_id, contact_id)
);
CREATE INDEX idx_clm_contact ON contact_list_members(contact_id);
