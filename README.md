# SMS Gateway (Horisen Wrapper)

Multi-tenant SMS gateway built on top of the [Horisen SMS HTTP API](https://developers.horisen.com/en/sms-http-api). Tenants authenticate with API keys, send SMS in bulk, and receive Delivery Reports (DLR) and inbound MO messages via signed outbound webhooks.

## Status

**Live in production** at `https://sms.aipanel.cl`. Customers integrate via `/v1/sms` with an API key.

- Customer-facing API reference → [`docs/api-public.md`](docs/api-public.md)
- Working examples (Python, Node, PHP, Bash, webhook servers) → [`docs/ejemplos/`](docs/ejemplos/)
- Superadmin onboarding guide → [`docs/onboarding-tenant.md`](docs/onboarding-tenant.md)
- Original architecture plan (historical) → [`docs/PLAN.md`](docs/PLAN.md)

## Highlights

- **Stack:** Go 1.24 + PostgreSQL 15
- **Multi-tenancy:** shared Horisen account, logical tenants routed via `custom.tenantId` on DLRs and `inbound_numbers` table on MO
- **Auth:** per-tenant API keys (`sk_live_...`), bcrypt-hashed; JWT for the admin dashboard
- **Event delivery:** signed (HMAC-SHA256) outbound webhooks with exponential backoff + polling API as fallback
- **Dashboard:** React + Vite + shadcn/ui SPA under `dashboard/`
- **Deploy:** single Go binary + systemd + nginx (TLS) on the existing VPS

## Quick links

- [Public API reference](docs/api-public.md) — auth, endpoints, errors, webhooks, examples
- [Examples](docs/ejemplos/) — runnable Python/Node/PHP/Bash clients + webhook servers
- [Tenant onboarding](docs/onboarding-tenant.md) — superadmin runbook
- [Security model](docs/SECURITY.md)
- [Horisen DLR code mapping](docs/horisen-dlr-codes.md)
- [Architecture plan (historical)](docs/PLAN.md)

## Layout (target)

```
cmd/         # server and smsctl binaries
internal/    # tenancy, horisen client, sms service, webhooks, http handlers
dashboard/   # Vite + React admin UI
deploy/      # systemd unit, nginx example, postgres setup
docs/        # plan and API docs
```
