-- Per-tenant allowed sender IDs. Empty array (default) = no restriction
-- (backwards compatible with existing tenants). Non-empty = sender on
-- /v1/sms must match one of the entries verbatim.
ALTER TABLE tenants
    ADD COLUMN allowed_senders TEXT[] NOT NULL DEFAULT '{}';
