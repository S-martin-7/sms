-- Endurecimiento del flujo admin: bloqueo por intentos fallidos + 2FA TOTP.
ALTER TABLE admin_users
    ADD COLUMN failed_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN locked_until    TIMESTAMPTZ,
    ADD COLUMN totp_secret     TEXT,
    ADD COLUMN totp_enabled    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN last_login_at   TIMESTAMPTZ;
