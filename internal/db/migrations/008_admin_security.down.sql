ALTER TABLE admin_users
    DROP COLUMN IF EXISTS last_login_at,
    DROP COLUMN IF EXISTS totp_enabled,
    DROP COLUMN IF EXISTS totp_secret,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_attempts;
