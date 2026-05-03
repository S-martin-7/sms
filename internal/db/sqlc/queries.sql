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

-- name: GetAdminUserByID :one
SELECT * FROM admin_users WHERE id = $1;

-- name: ListAdminUsers :many
SELECT * FROM admin_users ORDER BY id;

-- name: CountAdminUsers :one
SELECT COUNT(*) FROM admin_users;

-- BumpAdminFailedAttempts increments the bad-login counter and locks the
-- account for 15 minutes once we hit 5 in a row. Single statement so the
-- check + update are atomic under credential-stuffing concurrency.
--
-- name: BumpAdminFailedAttempts :one
UPDATE admin_users
SET failed_attempts = failed_attempts + 1,
    locked_until    = CASE
        WHEN failed_attempts + 1 >= 5
        THEN now() + interval '15 minutes'
        ELSE locked_until
    END
WHERE id = $1
RETURNING failed_attempts, locked_until;

-- name: ResetAdminLoginState :exec
UPDATE admin_users
SET failed_attempts = 0,
    locked_until    = NULL,
    last_login_at   = now()
WHERE id = $1;

-- SetAdminTOTPSecret stores a freshly generated secret without flipping
-- totp_enabled — the user must prove the secret with a valid code first.
--
-- name: SetAdminTOTPSecret :exec
UPDATE admin_users
SET totp_secret = $2
WHERE id = $1;

-- SetAdminTOTPEnabled flips totp_enabled. When disabling, the secret is
-- also cleared so a re-enroll must start fresh.
--
-- name: SetAdminTOTPEnabled :exec
UPDATE admin_users
SET totp_enabled = $2,
    totp_secret  = CASE WHEN $2 THEN totp_secret ELSE NULL END
WHERE id = $1;

-- GetAPIKeyWithTenantStatus joins api_keys × tenants so the auth path can
-- both verify the key AND short-circuit on suspended tenants in a single
-- round trip.
--
-- name: GetAPIKeyWithTenantStatus :one
SELECT k.id, k.tenant_id, k.hash, k.revoked_at, t.status AS tenant_status
FROM api_keys k
JOIN tenants t ON t.id = k.tenant_id
WHERE k.prefix = $1;

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

-- ApplyDLR updates a message based on a Horisen DLR. We only touch messages
-- in non-final states (sent, queued, sending) so a late or duplicate DLR
-- can't clobber an existing terminal state. RETURNING tells the caller
-- whether the update actually happened.
--
-- name: ApplyDLR :one
UPDATE messages
SET status = $2,
    error_code = COALESCE($3, error_code),
    error_message = COALESCE($4, error_message),
    horisen_msg_id = COALESCE(horisen_msg_id, $5),
    final_at = CASE WHEN $2 IN ('delivered','undelivered','rejected','failed')
                    THEN now() ELSE final_at END
WHERE id = $1
  AND status NOT IN ('delivered','undelivered','rejected','failed')
RETURNING id, tenant_id, status;

-- GetMessageByHorisenMsgID is the fallback lookup for DLRs that carry the
-- Horisen msgId but no custom.msgId — for example DLRs generated by jobs
-- submitted before custom.msgId was being set.
--
-- name: GetMessageByHorisenMsgID :one
SELECT id, tenant_id, status FROM messages
WHERE horisen_msg_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- ===== webhook_endpoints =====

-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (tenant_id, url, secret, events, active)
VALUES ($1, $2, $3, $4, true)
RETURNING *;

-- name: GetWebhookEndpoint :one
SELECT * FROM webhook_endpoints WHERE id = $1 AND tenant_id = $2;

-- name: ListWebhookEndpointsByTenant :many
SELECT * FROM webhook_endpoints WHERE tenant_id = $1 ORDER BY id DESC;

-- name: ListActiveEndpointsForEvent :many
-- Returns all active endpoints for a tenant that have subscribed to the
-- given event type. Used by the DLR/MO fan-out path.
SELECT * FROM webhook_endpoints
WHERE tenant_id = sqlc.arg(tenant_id)
  AND active
  AND sqlc.arg(event_type)::text = ANY(events);

-- name: SetWebhookEndpointActive :exec
UPDATE webhook_endpoints SET active = $3 WHERE id = $1 AND tenant_id = $2;

-- name: DeleteWebhookEndpoint :exec
DELETE FROM webhook_endpoints WHERE id = $1 AND tenant_id = $2;

-- ===== webhook_deliveries =====

-- name: EnqueueWebhookDelivery :one
INSERT INTO webhook_deliveries (
    endpoint_id, tenant_id, event_id, event_type, payload, status, next_attempt_at
) VALUES (
    $1, $2, $3, $4, $5, 'pending', now()
)
RETURNING *;

-- name: ClaimPendingDelivery :one
-- Atomically pick one ready delivery and mark it in_flight so other workers
-- skip it. attempts is bumped here so even crashes leave a trail.
UPDATE webhook_deliveries
SET status = 'in_flight',
    claimed_at = now(),
    attempts = attempts + 1
WHERE id = (
    SELECT id FROM webhook_deliveries
    WHERE status IN ('pending','failed') AND next_attempt_at <= now()
    ORDER BY next_attempt_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: MarkDeliverySuccess :exec
UPDATE webhook_deliveries
SET status = 'success',
    delivered_at = now(),
    last_status = $2,
    last_error = NULL,
    last_response = $3,
    claimed_at = NULL
WHERE id = $1;

-- name: MarkDeliveryFailed :exec
-- Used for both "retry later" (status=failed) and terminal "dead".
UPDATE webhook_deliveries
SET status = $2,
    next_attempt_at = $3,
    last_status = $4,
    last_error = $5,
    last_response = $6,
    claimed_at = NULL
WHERE id = $1;

-- name: RecoverStaleWebhookDeliveries :exec
-- Reset rows stuck in_flight beyond the cutoff so a crashed worker doesn't
-- pin them forever.
UPDATE webhook_deliveries
SET status = 'pending', claimed_at = NULL
WHERE status = 'in_flight' AND claimed_at < $1;

-- name: ListDeliveriesForEndpoint :many
SELECT * FROM webhook_deliveries
WHERE endpoint_id = $1 AND tenant_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- ===== inbound_numbers =====

-- name: CreateInboundNumber :one
INSERT INTO inbound_numbers (msisdn, tenant_id, label)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetInboundNumber :one
SELECT * FROM inbound_numbers WHERE msisdn = $1;

-- name: ListInboundNumbersByTenant :many
SELECT * FROM inbound_numbers WHERE tenant_id = $1 ORDER BY msisdn;

-- name: ListInboundNumbersAll :many
SELECT * FROM inbound_numbers ORDER BY tenant_id, msisdn;

-- name: DeleteInboundNumber :exec
DELETE FROM inbound_numbers WHERE msisdn = $1;

-- ===== inbound_messages =====

-- name: CreateInboundMessage :one
-- ON CONFLICT on horisen_id makes the insert idempotent: a duplicate MO
-- callback returns the existing row instead of failing.
INSERT INTO inbound_messages (id, tenant_id, horisen_id, src, dst, text, dcs, received_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (horisen_id) DO UPDATE SET horisen_id = EXCLUDED.horisen_id
RETURNING *;

-- name: GetInboundMessage :one
SELECT * FROM inbound_messages WHERE id = $1 AND tenant_id = $2;

-- name: ListInboundMessagesByTenant :many
SELECT * FROM inbound_messages
WHERE tenant_id = $1
ORDER BY received_at DESC
LIMIT $2;

-- ===== events (polling feed) =====

-- name: CreateEvent :one
INSERT INTO events (tenant_id, type, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListEventsByTenant :many
-- cursor_id = 0 means "from newest"; types NULL/empty array means no type filter.
SELECT * FROM events
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(cursor_id)::bigint = 0 OR id < sqlc.arg(cursor_id)::bigint)
  AND (sqlc.narg(types)::text[] IS NULL OR type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR created_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz   IS NULL OR created_at <  sqlc.narg(to_time)::timestamptz)
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: ListMessagesFiltered :many
-- Cursor is (created_at, id) tuple — id is UUID v4 so not monotonic on its own.
-- nullable cursor_created_at (and matching cursor_id) means "from newest".
SELECT * FROM messages
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
  AND (sqlc.narg(status)::text     IS NULL OR status     = sqlc.narg(status)::text)
  AND (sqlc.narg(recipient)::text  IS NULL OR recipient  = sqlc.narg(recipient)::text)
  AND (sqlc.narg(client_ref)::text IS NULL OR client_ref = sqlc.narg(client_ref)::text)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR created_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz   IS NULL OR created_at <  sqlc.narg(to_time)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: ListMessagesAdminFiltered :many
-- Same as ListMessagesFiltered but tenant_id is OPTIONAL — used by the
-- admin /admin/messages endpoint to search across tenants.
SELECT * FROM messages
WHERE (sqlc.narg(tenant_id)::bigint IS NULL OR tenant_id = sqlc.narg(tenant_id)::bigint)
  AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
  AND (sqlc.narg(status)::text     IS NULL OR status     = sqlc.narg(status)::text)
  AND (sqlc.narg(recipient)::text  IS NULL OR recipient  = sqlc.narg(recipient)::text)
  AND (sqlc.narg(client_ref)::text IS NULL OR client_ref = sqlc.narg(client_ref)::text)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR created_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz   IS NULL OR created_at <  sqlc.narg(to_time)::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim);

-- name: ListWebhookDeliveriesByTenant :many
-- Cursor is the BIGSERIAL id (DESC). 0 = from newest.
SELECT * FROM webhook_deliveries
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(cursor_id)::bigint = 0 OR id < sqlc.arg(cursor_id)::bigint)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: GetWebhookDelivery :one
-- Used by the retry endpoint; tenant_id required so admin URLs include it
-- defensively even though an admin can read everything.
SELECT * FROM webhook_deliveries WHERE id = $1;

-- name: RequeueWebhookDelivery :exec
-- Reset a failed/dead/success delivery to pending and ready-now. Useful for
-- admin manual retry. Does NOT clear last_status/last_error so the prior
-- attempt's diagnostics are preserved until the next attempt overwrites.
UPDATE webhook_deliveries
SET status = 'pending',
    next_attempt_at = now(),
    claimed_at = NULL
WHERE id = $1;

-- ===== admin stats =====

-- name: AdminStatsTotals :one
-- Counts per status in the last N hours (passed as a timestamp cutoff).
SELECT
    COUNT(*) FILTER (WHERE status IN ('queued','sending'))                AS queued,
    COUNT(*) FILTER (WHERE status = 'sent')                                AS sent,
    COUNT(*) FILTER (WHERE status = 'delivered')                           AS delivered,
    COUNT(*) FILTER (WHERE status = 'undelivered')                         AS undelivered,
    COUNT(*) FILTER (WHERE status IN ('rejected','failed'))                AS rejected,
    COUNT(*)                                                               AS total
FROM messages
WHERE created_at >= $1;

-- name: AdminStatsByTenant :many
-- Volume per tenant in the window, joined with tenant name; useful for
-- "top customers" widgets.
SELECT t.id, t.name, COUNT(m.*) AS total,
       COUNT(*) FILTER (WHERE m.status = 'delivered')                  AS delivered,
       COUNT(*) FILTER (WHERE m.status IN ('rejected','failed'))       AS rejected
FROM messages m
JOIN tenants t ON t.id = m.tenant_id
WHERE m.created_at >= $1
GROUP BY t.id, t.name
ORDER BY total DESC
LIMIT $2;

-- name: AdminStatsRecentFailures :many
-- Recent message failures or webhook deliveries stuck failed/dead — the
-- "things to look at" list. Two queries kept separate for clarity; this
-- one is for messages.
SELECT id, tenant_id, recipient, status, error_code, error_message, created_at
FROM messages
WHERE status IN ('rejected','failed','undelivered')
  AND created_at >= $1
ORDER BY created_at DESC
LIMIT $2;

-- name: AdminStatsStuckDeliveries :many
SELECT id, tenant_id, endpoint_id, event_type, status, attempts, last_status, last_error, created_at
FROM webhook_deliveries
WHERE status IN ('failed','dead')
  AND created_at >= $1
ORDER BY created_at DESC
LIMIT $2;

-- ===== contacts =====

-- name: CreateContact :one
INSERT INTO contacts (tenant_id, msisdn, name, notes, metadata)
VALUES ($1, $2, $3, $4, COALESCE($5, '{}')::jsonb)
RETURNING *;

-- name: UpsertContact :one
INSERT INTO contacts (tenant_id, msisdn, name, notes, metadata)
VALUES ($1, $2, $3, $4, COALESCE($5, '{}')::jsonb)
ON CONFLICT (tenant_id, msisdn) DO UPDATE
SET name      = COALESCE(EXCLUDED.name, contacts.name),
    notes     = COALESCE(EXCLUDED.notes, contacts.notes),
    updated_at = now()
RETURNING *;

-- name: GetContact :one
SELECT * FROM contacts WHERE id = $1 AND tenant_id = $2;

-- name: ListContacts :many
SELECT * FROM contacts
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(cursor_id)::bigint = 0 OR id < sqlc.arg(cursor_id)::bigint)
  AND (sqlc.narg(q)::text IS NULL
       OR msisdn ILIKE '%' || sqlc.narg(q)::text || '%'
       OR coalesce(name,'') ILIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(opt_out)::boolean IS NULL OR opt_out = sqlc.narg(opt_out)::boolean)
  AND (sqlc.narg(list_id)::bigint IS NULL
       OR id IN (SELECT contact_id FROM contact_list_members WHERE list_id = sqlc.narg(list_id)::bigint))
ORDER BY id DESC
LIMIT sqlc.arg(lim);

-- name: CountContactsByTenant :one
SELECT
    COUNT(*)                              AS total,
    COUNT(*) FILTER (WHERE opt_out)       AS opted_out
FROM contacts
WHERE tenant_id = $1;

-- name: SetContactOptOut :exec
UPDATE contacts SET opt_out = $3, opt_out_at = CASE WHEN $3 THEN now() ELSE NULL END,
                    updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteContact :exec
DELETE FROM contacts WHERE id = $1 AND tenant_id = $2;

-- ===== contact_lists =====

-- name: CreateContactList :one
INSERT INTO contact_lists (tenant_id, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListContactLists :many
SELECT cl.*, COUNT(clm.contact_id)::bigint AS member_count
FROM contact_lists cl
LEFT JOIN contact_list_members clm ON clm.list_id = cl.id
WHERE cl.tenant_id = $1
GROUP BY cl.id
ORDER BY cl.name;

-- name: DeleteContactList :exec
DELETE FROM contact_lists WHERE id = $1 AND tenant_id = $2;

-- name: AddContactsToList :exec
INSERT INTO contact_list_members (list_id, contact_id)
SELECT $1::bigint, c.id
FROM contacts c
WHERE c.tenant_id = $2 AND c.id = ANY(sqlc.arg(contact_ids)::bigint[])
ON CONFLICT DO NOTHING;

-- name: RemoveContactFromList :exec
DELETE FROM contact_list_members
WHERE list_id = $1 AND contact_id = $2;

-- ===== reports =====

-- name: AdminMessagesTimeBucketed :many
SELECT
    (date_trunc(sqlc.arg(bucket)::text, created_at))::timestamptz AS ts,
    COUNT(*)                                       AS total,
    COUNT(*) FILTER (WHERE status = 'delivered')   AS delivered,
    COUNT(*) FILTER (WHERE status IN ('rejected','failed','undelivered')) AS failed
FROM messages
WHERE (sqlc.narg(tenant_id)::bigint IS NULL OR tenant_id = sqlc.narg(tenant_id)::bigint)
  AND created_at >= sqlc.arg(from_time)
  AND created_at <  sqlc.arg(to_time)
GROUP BY ts
ORDER BY ts;

-- name: AdminTopRecipients :many
SELECT recipient,
       COUNT(*)                                       AS total,
       COUNT(*) FILTER (WHERE status = 'delivered')   AS delivered
FROM messages
WHERE tenant_id = sqlc.arg(tenant_id)
  AND created_at >= sqlc.arg(from_time)
  AND created_at <  sqlc.arg(to_time)
GROUP BY recipient
ORDER BY total DESC
LIMIT sqlc.arg(lim);

-- ===== scheduled_sends =====

-- name: CreateScheduledSend :one
INSERT INTO scheduled_sends (
    tenant_id, name, sender, text, recipients, list_id,
    send_at, recurrence, recurrence_days, timezone,
    created_by, api_key_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, COALESCE($10, 'America/Santiago'),
    $11, $12
)
RETURNING *;

-- name: GetScheduledSend :one
SELECT * FROM scheduled_sends WHERE id = $1 AND tenant_id = $2;

-- name: ListScheduledSends :many
SELECT * FROM scheduled_sends
WHERE tenant_id = $1
ORDER BY
  CASE WHEN status = 'pending' THEN 0 ELSE 1 END,
  send_at ASC,
  id DESC
LIMIT $2;

-- name: SetScheduledSendStatus :exec
UPDATE scheduled_sends SET status = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteScheduledSend :exec
DELETE FROM scheduled_sends WHERE id = $1 AND tenant_id = $2;

-- name: ClaimDueScheduledSend :one
UPDATE scheduled_sends
SET status = 'running', updated_at = now()
WHERE id = (
    SELECT id FROM scheduled_sends
    WHERE status = 'pending' AND send_at <= now()
    ORDER BY send_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: MarkScheduledSendFired :exec
UPDATE scheduled_sends
SET status        = $2,
    send_at       = COALESCE($3, send_at),
    last_run_at   = now(),
    last_batch_id = $4,
    total_runs    = total_runs + 1,
    last_error    = NULL,
    updated_at    = now()
WHERE id = $1;

-- name: MarkScheduledSendFailed :exec
UPDATE scheduled_sends
SET status     = 'failed',
    last_error = $2,
    updated_at = now()
WHERE id = $1;

-- name: GetContactListMSISDNs :many
SELECT c.msisdn
FROM contact_list_members clm
JOIN contacts c ON c.id = clm.contact_id
WHERE clm.list_id = $1 AND c.tenant_id = $2 AND c.opt_out = false
ORDER BY c.id;
