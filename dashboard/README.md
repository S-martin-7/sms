# SMS Gateway Dashboard

React + Vite SPA that talks to the Admin API of the SMS Gateway.

## Local dev

```bash
npm install
npm run dev
# → http://localhost:5173/dashboard/
```

The dev server proxies `/admin/*` and `/v1/*` to `https://sms.aipanel.cl`
(prod) so you can click around without running a local backend.

## Build

```bash
npm run build
# → dist/
```

Outputs a static SPA under `/dashboard/` (subpath, controlled by
`base: '/dashboard/'` in `vite.config.ts`). Deploy `dist/` to
`/root/sms/dashboard/dist/` on the VPS — nginx serves it from there.

## Stack

- Vite 5 + React 18 + TypeScript
- React Router v6 (HashRouter)
- TanStack Query v5 for server state
- axios with Bearer interceptor reading from `localStorage["sms_jwt"]`
- Tailwind v3 with hand-rolled `src/components/ui/*`

## Pages

| Route | Component | Purpose |
|---|---|---|
| `/login` | LoginPage | Email/password → `/admin/login` |
| `/tenants` | TenantsPage | List, create, suspend/activate |
| `/tenants/:id` | TenantDetailPage | Tabs: API keys, Webhooks, Deliveries, Inbound |
| `/messages` | MessagesPage | Cross-tenant search with cursor pagination |
| `/inbound-numbers` | InboundNumbersPage | List + assign + unassign MSISDN routing |
