-- name: CreateTenant :one
INSERT INTO tenants (name, daily_sms_limit, monthly_budget)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY id;

-- name: SetTenantStatus :exec
UPDATE tenants SET status = $2, updated_at = now() WHERE id = $1;

-- name: CreateAPIKey :one
INSERT INTO api_keys (tenant_id, prefix, hash, name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys WHERE prefix = $1;

-- name: ListAPIKeysByTenant :many
SELECT * FROM api_keys WHERE tenant_id = $1 ORDER BY id DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: CreateAdminUser :one
INSERT INTO admin_users (email, password_hash, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAdminUserByEmail :one
SELECT * FROM admin_users WHERE email = $1;

-- name: ListAdminUsers :many
SELECT * FROM admin_users ORDER BY id;

-- name: CountAdminUsers :one
SELECT COUNT(*) FROM admin_users;

-- name: AppendAuditLog :exec
INSERT INTO audit_log (actor_id, action, target_type, target_id, metadata)
VALUES ($1, $2, $3, $4, $5);

-- name: CreateMessage :one
INSERT INTO messages (
    id, tenant_id, sender, recipient, text, dcs, num_parts, status, client_ref
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'queued', $8
)
RETURNING *;

-- name: GetMessageForTenant :one
SELECT * FROM messages WHERE id = $1 AND tenant_id = $2;

-- name: ListMessagesByTenant :many
SELECT * FROM messages
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ClaimQueuedMessage :one
UPDATE messages
SET status = 'sending',
    claimed_at = now(),
    attempts = attempts + 1
WHERE id = (
    SELECT id FROM messages
    WHERE status = 'queued' AND next_attempt_at <= now()
    ORDER BY next_attempt_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: MarkMessageSent :exec
UPDATE messages
SET status = 'sent',
    horisen_msg_id = $2,
    sent_at = now(),
    claimed_at = NULL
WHERE id = $1;

-- name: MarkMessageRejected :exec
UPDATE messages
SET status = 'rejected',
    error_code = $2,
    error_message = $3,
    final_at = now(),
    claimed_at = NULL
WHERE id = $1;

-- name: BumpMessageRetry :exec
UPDATE messages
SET status = 'queued',
    next_attempt_at = $2,
    error_code = $3,
    error_message = $4,
    claimed_at = NULL
WHERE id = $1;

-- name: RecoverStaleSending :exec
UPDATE messages
SET status = 'queued', claimed_at = NULL
WHERE status = 'sending' AND claimed_at < $1;
