# SMS Gateway (Horisen Wrapper)

Multi-tenant SMS gateway built on top of the [Horisen SMS HTTP API](https://developers.horisen.com/en/sms-http-api). Tenants authenticate with API keys, send SMS in bulk, and receive Delivery Reports (DLR) and inbound MO messages via signed outbound webhooks.

## Status

**Planning phase.** See `docs/PLAN.md` for the full architecture and build sequence.

## Highlights

- **Stack:** Go 1.24 + PostgreSQL 15
- **Multi-tenancy:** shared Horisen account, logical tenants routed via `custom.tenantId` on DLRs and `inbound_numbers` table on MO
- **Auth:** per-tenant API keys (`sk_live_...`), bcrypt-hashed; JWT for the admin dashboard
- **Event delivery:** signed (HMAC-SHA256) outbound webhooks with exponential backoff + polling API as fallback
- **Dashboard:** React + Vite + shadcn/ui SPA under `dashboard/`
- **Deploy:** single Go binary + systemd + nginx (TLS) on the existing VPS

## Quick links

- Architecture & plan → `docs/PLAN.md`
- Public API reference → `docs/api-public.md` *(TBD)*
- Webhook payload & HMAC verification → `docs/webhooks.md` *(TBD)*

## Layout (target)

```
cmd/         # server and smsctl binaries
internal/    # tenancy, horisen client, sms service, webhooks, http handlers
dashboard/   # Vite + React admin UI
deploy/      # systemd unit, nginx example, postgres setup
docs/        # plan and API docs
```
