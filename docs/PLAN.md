# Módulo SMS Multi-Tenant (Horisen Wrapper)

## Context

Necesitas un **módulo nuevo, independiente** (sin código base previo) que actúe como gateway multi-tenant sobre la **Horisen SMS HTTP API**. Clientes (tenants) se autentican con una API key que tú emites, envían SMS a través de tu API, y reciben Delivery Reports (DLR) y mensajes entrantes (MO) vía webhook outbound firmado. Una sola cuenta Horisen detrás (tú controlas el crédito); tus clientes son tenants lógicos.

**Decisiones tomadas:**
- Stack: **Go + PostgreSQL**
- Endpoints públicos: **Bulk send**, **query status/history**, **balance**
- Entrega de eventos: **Webhooks outbound firmados** + **polling API** como fallback
- Tenancy: **Una cuenta Horisen compartida**; enrutamiento de DLRs por `custom.tenantId`
- Gestión: **API admin + dashboard moderno (React/Vue)**
- MO inbound: **Sí en v1** (tabla de routing número destino → tenant)
- Deploy: **Mismo VPS** (38.54.20.110), systemd separado, puerto distinto al 7200, HTTPS vía nginx + Let's Encrypt

---

## Arquitectura (vista alta)

```
 ┌──────────┐    API key    ┌────────────────────┐  Horisen auth   ┌─────────┐
 │ Tenant A │ ─────────────▶│ SMS Gateway (Go)   │────────────────▶│ Horisen │
 │ Tenant B │ ─────────────▶│  - HTTP API        │                 │   API   │
 └──────────┘               │  - Admin API       │                 └────┬────┘
                            │  - Send workers    │                      │ DLR/MO
 ┌──────────┐               │  - Webhook workers │◀─────────────────────┘
 │ Dashboard│ ── admin JWT ▶│  - Postgres DB     │
 │  (SPA)   │               └─────────┬──────────┘
 └──────────┘                         │ webhook outbound (HMAC-SHA256)
                                      ▼
                                 ┌─────────┐
                                 │ Tenant  │
                                 │ callback│
                                 └─────────┘
```

**Flujos principales:**
1. **Envío MT**: Tenant → `POST /v1/sms/bulk` → cola `outbox` → worker envía a Horisen con `custom={tenantId, internalId}` → msgId se persiste.
2. **DLR**: Horisen → `POST /v1/horisen/dlr` → localizamos mensaje por `msgId`, actualizamos estado → enqueue evento → webhook worker entrega al tenant.
3. **MO inbound**: Horisen → `POST /v1/horisen/mo` → resolvemos tenant por número destino (`inbound_numbers`) → persistir → enqueue evento → webhook al tenant.
4. **Polling**: Tenant puede `GET /v1/sms/{id}` o `GET /v1/events?cursor=...` como fallback.

---

## Stack y dependencias

**Runtime:** Go 1.24+, PostgreSQL 15+.

**Librerías (mínimas, prefer stdlib):**
- `net/http` + `github.com/go-chi/chi/v5` — router ligero con middleware
- `github.com/jackc/pgx/v5` + `pgx/v5/pgxpool` — driver y pool Postgres
- `github.com/golang-migrate/migrate/v4` — migraciones SQL versionadas
- `github.com/golang-jwt/jwt/v5` — JWT para sesiones admin del dashboard
- `golang.org/x/crypto/bcrypt` — hash de contraseñas admin
- `github.com/rs/zerolog` — logging estructurado JSON

**Frontend (dashboard):**
- Vite + React 18 + TypeScript
- TanStack Query (server state) + React Router
- shadcn/ui + Tailwind (estético moderno, rápido de construir)
- Recharts para gráficos de volumen/entrega

**Infra local:**
- `docker-compose.yml` solo para dev (Postgres + Mailpit opcional)
- En producción: Postgres nativo en el VPS, app como binario systemd

---

## Estructura de directorios (a crear)

```
C:\codigo\vps\sms\
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── cmd/
│   ├── server/main.go          # binario principal HTTP
│   └── smsctl/main.go          # CLI admin (seed, rotate-key, migrate)
├── internal/
│   ├── config/                 # env vars (Horisen creds, DB DSN, JWT secret, HMAC pepper)
│   ├── db/
│   │   ├── pool.go             # pgxpool init
│   │   ├── migrations/         # *.up.sql / *.down.sql
│   │   └── queries/            # funciones sqlc-style (opcional) o repos manuales
│   ├── tenancy/
│   │   ├── tenant.go           # Tenant struct + CRUD
│   │   └── apikey.go           # generación (32 bytes base64url), hash bcrypt, prefijo público "sk_live_xxxx"
│   ├── horisen/
│   │   ├── client.go           # HorisenClient: SendSMS, GetBalance (OAuth2 token cache)
│   │   ├── encoding.go         # detectar GSM-7 vs UCS-2, contar partes
│   │   └── errors.go           # mapeo de codes 101-116 a tipos retryables/no-retryables
│   ├── sms/
│   │   ├── model.go            # Message, MessagePart, DLR, InboundSMS
│   │   ├── outbox.go           # enqueue + worker pool (N goroutines)
│   │   ├── ratelimit.go        # limiter global por TPS configurable
│   │   └── service.go          # lógica de SendBulk, GetStatus, ListMessages
│   ├── webhooks/
│   │   ├── model.go            # Endpoint, Delivery (pending/success/failed)
│   │   ├── signer.go           # HMAC-SHA256 sobre timestamp+body
│   │   ├── dispatcher.go       # worker con backoff exponencial (ej: 1m, 5m, 30m, 2h, 8h, 24h)
│   │   └── service.go
│   ├── events/
│   │   └── cursor.go           # paginación por cursor para GET /v1/events
│   ├── http/
│   │   ├── router.go           # registro de rutas
│   │   ├── middleware/
│   │   │   ├── apikey.go       # extrae X-API-Key, busca hash, setea tenant en ctx
│   │   │   ├── adminauth.go    # JWT admin dashboard
│   │   │   ├── reqid.go        # X-Request-ID
│   │   │   └── ratelimit.go    # per-tenant token bucket
│   │   ├── handlers_public/    # /v1/sms, /v1/sms/bulk, /v1/sms/{id}, /v1/events, /v1/balance, /v1/webhooks
│   │   ├── handlers_admin/     # /admin/tenants, /admin/api-keys, /admin/messages, /admin/webhook-deliveries
│   │   └── handlers_horisen/   # /v1/horisen/dlr, /v1/horisen/mo (protegidos por IP allowlist + shared secret en query)
│   └── logger/                 # wrapper zerolog + correlación de request-id
├── dashboard/                  # app Vite/React/TS separada
│   ├── package.json
│   ├── vite.config.ts
│   └── src/
│       ├── pages/              # Login, Tenants, TenantDetail, Messages, Webhooks, Balance, Settings
│       ├── components/         # layouts, tables, forms (shadcn/ui)
│       ├── api/                # cliente tipado del admin API
│       └── hooks/
├── deploy/
│   ├── sms-gateway.service     # unit systemd
│   ├── nginx.conf.example      # reverse proxy + TLS + rate-limit edge
│   └── postgres-setup.sql      # crear DB y usuario
└── docs/
    ├── api-public.md           # OpenAPI 3 + ejemplos curl
    └── webhooks.md             # formato del payload y verificación HMAC
```

---

## Esquema de base de datos (PostgreSQL)

**Migraciones versionadas en `internal/db/migrations/`.** Claves surrogate `BIGSERIAL`, timestamps `TIMESTAMPTZ`, soft-delete donde aplique.

```sql
-- 001_init.up.sql

CREATE TABLE tenants (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',  -- active|suspended
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- cuotas
  daily_sms_limit INT,                              -- NULL = sin límite
  monthly_budget  NUMERIC(12,4)
);

CREATE TABLE api_keys (
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  prefix          TEXT NOT NULL,                    -- primeros 8 chars visibles, ej "sk_live_ab12cd34"
  hash            TEXT NOT NULL,                    -- bcrypt del token completo
  name            TEXT,                             -- label dado por el admin
  scopes          TEXT[] NOT NULL DEFAULT '{send,read,webhooks}',
  last_used_at    TIMESTAMPTZ,
  revoked_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (prefix)
);
CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id) WHERE revoked_at IS NULL;

CREATE TABLE inbound_numbers (                      -- MO routing: nº destino → tenant
  msisdn          TEXT PRIMARY KEY,                 -- E.164 sin '+'
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE messages (                             -- unidad lógica (una petición del tenant)
  id              UUID PRIMARY KEY,                 -- nuestro id interno
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id),
  sender          TEXT NOT NULL,
  recipient       TEXT NOT NULL,
  text            TEXT NOT NULL,
  dcs             TEXT NOT NULL,                    -- GSM|UCS
  num_parts       SMALLINT NOT NULL,
  status          TEXT NOT NULL,                    -- queued|sent|delivered|undelivered|rejected|failed
  horisen_msg_id  TEXT,                             -- UUID devuelto por Horisen
  error_code      TEXT,
  error_message   TEXT,
  client_ref      TEXT,                             -- referencia libre del cliente (idempotency helper)
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at         TIMESTAMPTZ,
  final_at        TIMESTAMPTZ,
  UNIQUE (tenant_id, client_ref)                    -- idempotencia a nivel tenant
);
CREATE INDEX idx_messages_tenant_created ON messages(tenant_id, created_at DESC);
CREATE INDEX idx_messages_horisen ON messages(horisen_msg_id);

CREATE TABLE message_parts (                        -- 1 row por parte concatenada
  id              BIGSERIAL PRIMARY KEY,
  message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  part_num        SMALLINT NOT NULL,
  status          TEXT NOT NULL,
  received_at     TIMESTAMPTZ,
  UNIQUE (message_id, part_num)
);

CREATE TABLE inbound_messages (                     -- MO SMS
  id              UUID PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id),
  horisen_id      TEXT UNIQUE,
  src             TEXT NOT NULL,
  dst             TEXT NOT NULL,
  text            TEXT NOT NULL,
  received_at     TIMESTAMPTZ NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_endpoints (
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  url             TEXT NOT NULL,
  secret          TEXT NOT NULL,                    -- para firmar HMAC, compartido con el cliente
  events          TEXT[] NOT NULL,                  -- sms.delivered, sms.undelivered, sms.rejected, sms.inbound
  active          BOOLEAN NOT NULL DEFAULT true,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (                   -- cola persistente de entregas
  id              BIGSERIAL PRIMARY KEY,
  endpoint_id     BIGINT NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
  tenant_id       BIGINT NOT NULL,
  event_type      TEXT NOT NULL,
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL,                    -- pending|success|failed|dead
  attempts        INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_response   JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at    TIMESTAMPTZ
);
CREATE INDEX idx_wd_pickup ON webhook_deliveries(status, next_attempt_at)
  WHERE status IN ('pending','failed');

CREATE TABLE events (                               -- feed de polling para tenants
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id),
  type            TEXT NOT NULL,
  payload         JSONB NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_tenant_id ON events(tenant_id, id);

CREATE TABLE admin_users (                          -- para login al dashboard
  id              BIGSERIAL PRIMARY KEY,
  email           TEXT UNIQUE NOT NULL,
  password_hash   TEXT NOT NULL,                    -- bcrypt
  role            TEXT NOT NULL,                    -- superadmin|operator
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (                            -- acciones admin sensibles
  id              BIGSERIAL PRIMARY KEY,
  actor_id        BIGINT REFERENCES admin_users(id),
  action          TEXT NOT NULL,
  target_type     TEXT,
  target_id       TEXT,
  metadata        JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Cola de outbox MT** (se puede modelar como tabla `messages` con `status='queued'` + índice parcial `WHERE status='queued'`). Workers hacen `SELECT ... FOR UPDATE SKIP LOCKED LIMIT N` para garantizar que un mensaje solo lo procesa un worker.

---

## Horisen client (internal/horisen)

**Endpoint de envío:** `POST https://<horisen-domain>/bulk/sendsms` con payload:
```json
{
  "type": "text",
  "auth": {"username": "<env>", "password": "<env>"},
  "sender": "...", "receiver": "...", "dcs": "GSM|UCS",
  "text": "...",
  "dlrMask": 19,
  "dlrUrl": "https://sms.tudominio.com/v1/horisen/dlr?sig=<shared-secret>",
  "custom": {"tenantId": 42, "msgId": "<nuestro uuid>"}
}
```

**Puntos clave:**
- **No hay bulk nativo**: nuestro endpoint `/v1/sms/bulk` acepta N destinatarios y el worker hace N POSTs secuenciales respetando rate limit.
- **Mapeo de errores** en `horisen/errors.go`:
  - Retryables: `105` (throttling, esperar 1s + jitter), HTTP 500 (60s).
  - No retryables: `102, 103, 104, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116` → marcar mensaje como `rejected` con `error_code`.
- **OAuth2 para balance**: client_credentials, token cacheado en memoria con expiración (604800s). Refrescar proactivamente al 80% de vida.
- **Detección de encoding**: inspeccionar texto; si contiene caracteres fuera del alfabeto GSM-7 → `dcs=UCS`. Calcular `num_parts` antes de enviar (útil para logging/cuotas).
- **Timeouts**: `http.Client{Timeout: 15s}` por request; contexto con deadline propagado desde el handler.

---

## Workers y concurrencia

**Outbox worker pool** (`internal/sms/outbox.go`):
- N goroutines (config `SMS_SENDER_WORKERS=8`) tomando de `messages` con `FOR UPDATE SKIP LOCKED`.
- Rate limiter global: `golang.org/x/time/rate` configurable por env (`HORISEN_TPS=10`) hasta confirmar TPS real con soporte Horisen.
- En éxito → `UPDATE messages SET status='sent', horisen_msg_id=..., sent_at=now()`.
- En fallo retryable → reinserta con backoff; en fallo permanente → `status='rejected'`, enqueue evento `sms.rejected`.

**Webhook dispatcher** (`internal/webhooks/dispatcher.go`):
- Poll cada 5s a `webhook_deliveries` con `status IN ('pending','failed') AND next_attempt_at <= now()`.
- Intentos: **1, +1m, +5m, +30m, +2h, +8h, +24h** (7 intentos). Tras el 7º → `status='dead'` (el cliente puede reenviar via polling API).
- Firma: `X-Signature: t=<unixts>,v1=<hex(hmac_sha256(secret, t+"."+body))>` (estilo Stripe). Header `X-Event-Id` para idempotencia del lado cliente.
- Timeout por intento: 10s.

**Scheduler simple**: una goroutine `time.Ticker` por worker type. Sin cron externo.

---

## API pública (tenant-facing)

Autenticación: header `X-API-Key: sk_live_<prefix>_<secret>`. El middleware parsea `prefix`, busca `api_keys.prefix`, compara bcrypt contra la parte secreta, carga `tenant_id` al `context.Context`.

### Envío bulk
```
POST /v1/sms/bulk
Content-Type: application/json
X-API-Key: sk_live_...
Idempotency-Key: <opcional, UUID del cliente>

{
  "default_sender": "MiMarca",
  "messages": [
    {"to": "4179123456", "text": "Hola", "client_ref": "order-123"},
    {"to": "4179876543", "text": "Hello", "sender": "OtroSender"}
  ]
}
```
Response `202 Accepted`:
```json
{
  "batch_id": "b_7f2...",
  "accepted": 2,
  "rejected": 0,
  "messages": [
    {"id": "msg_01H...", "to": "4179123456", "status": "queued", "client_ref": "order-123"},
    {"id": "msg_01H...", "to": "4179876543", "status": "queued"}
  ]
}
```

Single-send es un caso particular de bulk con 1 item, o alias `POST /v1/sms` que internamente llama a la misma lógica.

### Consulta
- `GET /v1/sms/{id}` → detalle del mensaje + últimos DLRs por parte.
- `GET /v1/sms?status=...&from=...&to=...&cursor=...&limit=100` → paginación por cursor (id descendente).
- `GET /v1/events?cursor=...&types=sms.delivered,sms.inbound&limit=100` → feed de eventos para polling fallback.

### Balance
- `GET /v1/balance` → proxy al OAuth2 Balance API. Cacheable 60s por tenant.

### Webhooks (gestión)
- `GET/POST/DELETE /v1/webhooks` — CRUD de endpoints del tenant y selección de eventos suscritos.
- `POST /v1/webhooks/{id}/test` — envía un evento sintético para verificar la URL.

---

## API admin (dashboard-facing)

Autenticación: JWT emitido por `POST /admin/login` (email+password de `admin_users`). Header `Authorization: Bearer <jwt>`.

- `GET/POST /admin/tenants` — crear/listar tenants, editar cuotas.
- `GET/POST/DELETE /admin/tenants/{id}/api-keys` — emitir key (secret se muestra **una sola vez**), revocar.
- `GET /admin/messages` — búsqueda global con filtros tenant/status/fecha.
- `GET /admin/webhook-deliveries` — inspeccionar fallos, reintentar manual (`POST .../retry`).
- `GET /admin/inbound-numbers` — asignar MSISDN → tenant.
- `GET /admin/stats` — volumen diario, tasa de entrega, top senders.

---

## Endpoints hacia Horisen (inbound callbacks)

**Protección:** shared secret en query (`?sig=...`) + opcionalmente IP allowlist (pendiente confirmar con Horisen).

- `POST /v1/horisen/dlr` — parsea payload, `UPDATE messages` por `horisen_msg_id`, crea evento `sms.delivered|undelivered|rejected`, enqueue webhook delivery para cada endpoint del tenant suscrito.
- `POST /v1/horisen/mo` — resuelve `tenant_id = inbound_numbers.tenant_id WHERE msisdn = payload.dst`; si no hay match → log + 200 OK (no dejar que Horisen reintente indefinidamente); si hay match → insertar en `inbound_messages`, crear evento `sms.inbound`, enqueue webhook.

Ambos responden **200 OK rápido** (< 1s) después de persistir; el envío de webhook va por la cola, no síncrono.

---

## Dashboard (frontend)

**Separado** en `dashboard/` como SPA estática. En producción: `npm run build` → `dashboard/dist/` servido por nginx (misma origen que API vía `/dashboard/`).

**Páginas clave:**
- **Login** (email/password).
- **Tenants** — tabla con búsqueda, crear/editar, ver uso.
- **Tenant detail** — API keys, webhooks, números inbound asignados, mensajes del tenant.
- **Messages** — tabla filtrable; click abre drawer con DLR timeline.
- **Webhook deliveries** — estado (pending/success/failed/dead), respuesta del cliente, botón "reintentar".
- **Balance** — saldo Horisen actual + gráfico de gasto diario.
- **Settings** — rotar shared secret de callbacks Horisen, gestionar admin users.

**Estética:** shadcn/ui con Tailwind, sidebar + topbar, dark mode. Recharts para gráficos.

---

## Seguridad

- **API keys**: 32 bytes random → base64url → formato `sk_live_<prefix>_<secret>`. Almacenamos solo bcrypt(secret). El admin ve el token completo **solo al crearlo**.
- **Bcrypt cost** 12 para api_keys y admin_users.
- **HMAC webhooks**: SHA-256 con `timestamp.body`, tolerancia de 5 min en el lado cliente para prevenir replay.
- **Rate limit per-tenant** en el edge (token bucket en memoria, sync periódico a DB para cuotas diarias).
- **IP allowlist** (opcional) en `/v1/horisen/*` — confirmar IPs de Horisen antes de prod.
- **TLS obligatorio** — nginx con cert Let's Encrypt, HSTS.
- **Audit log** de acciones admin (crear tenant, revocar key, reintentar webhook).
- **Secrets** en variables de entorno (systemd EnvironmentFile con permisos 0600).

---

## Configuración (env vars)

```
DATABASE_URL=postgres://sms:***@localhost:5432/sms
HORISEN_BASE_URL=https://sms.<domain>
HORISEN_USERNAME=***
HORISEN_PASSWORD=***
HORISEN_OAUTH_CLIENT_ID=***
HORISEN_OAUTH_CLIENT_SECRET=***
HORISEN_OAUTH_TOKEN_URL=https://accounts.<domain>/oauth2/access-token
HORISEN_BALANCE_URL=https://api.<domain>/finance/sit/v1/balances/biz-partners/customers
HORISEN_CALLBACK_SECRET=<aleatorio 32b>
HORISEN_TPS=10
PUBLIC_BASE_URL=https://sms.tudominio.com
SMS_SENDER_WORKERS=8
WEBHOOK_WORKERS=4
JWT_SECRET=<aleatorio 64b>
BIND_ADDR=127.0.0.1:7300
LOG_LEVEL=info
```

---

## Deployment (VPS 38.54.20.110)

1. **Postgres**: instalar Postgres 15, crear DB `sms` + usuario, ejecutar `deploy/postgres-setup.sql`. Configurar `pg_hba.conf` solo local.
2. **Subdominio**: apuntar `sms.tudominio.com` al VPS. Solicitar cert Let's Encrypt con certbot.
3. **nginx**: reverse proxy `443 → 127.0.0.1:7300`. Servir `/dashboard/` desde `dashboard/dist/`. Rate-limit edge `limit_req_zone` para `/v1/`.
4. **systemd**: unit `sms-gateway.service` con `After=postgresql.service`, `EnvironmentFile=/etc/sms-gateway.env`, `Restart=on-failure`.
5. **Migraciones**: `smsctl migrate up` en deploy.
6. **Seed inicial**: `smsctl admin create --email you@domain --password ***` para crear primer superadmin.
7. **Horisen**: registrar `https://sms.tudominio.com/v1/horisen/dlr?sig=<HORISEN_CALLBACK_SECRET>` como DLR URL por defecto de la cuenta, y el análogo `/v1/horisen/mo` como MO URL.

---

## Fases de construcción (orden sugerido)

1. **Scaffold**: `go mod init`, estructura de directorios, `docker-compose.yml` con Postgres, migración `001_init`, configuración, logging.
2. **Auth layer**: tenants + api_keys + admin_users, middleware `apikey` y `adminauth`, CLI `smsctl` con `migrate`, `admin create`, `tenant create`, `key issue`.
3. **Horisen client**: `SendSMS` + tests contra sandbox (o mock HTTP). Detección de encoding y mapeo de errores.
4. **Outbox + workers**: handler `POST /v1/sms/bulk`, worker pool con rate limit, estado `queued→sent`.
5. **DLR inbound**: `POST /v1/horisen/dlr`, update de mensajes, creación de eventos.
6. **MO inbound**: tabla `inbound_numbers`, `POST /v1/horisen/mo`, evento `sms.inbound`.
7. **Webhooks outbound**: CRUD de endpoints, dispatcher con backoff, firma HMAC.
8. **Polling API**: `GET /v1/sms/{id}`, `GET /v1/sms`, `GET /v1/events` con cursor.
9. **Balance**: OAuth2 token cache + `GET /v1/balance`.
10. **Admin API**: handlers admin, audit log.
11. **Dashboard**: scaffold Vite + auth flow, páginas en el orden Tenants → Messages → Webhooks → Balance → Settings.
12. **Deploy**: nginx + systemd + certbot + seed inicial + smoke test E2E (enviar SMS real a tu número y verificar DLR).

---

## Verificación (criterios de aceptación)

- `go test ./...` en verde; tests unitarios mínimos para: encoding GSM/UCS, firma HMAC, clasificación retry/no-retry de errores Horisen, parser de DLR.
- **Smoke test E2E**: crear tenant via `smsctl` → emitir key → `curl POST /v1/sms/bulk` a tu móvil → verificar recepción física → verificar que el webhook del tenant (usar https://webhook.site) recibe el evento `sms.delivered` firmado → verificar que la firma HMAC valida.
- **MO**: enviar SMS desde tu móvil al número asignado → verificar `inbound_messages` + webhook `sms.inbound` en webhook.site.
- **Balance**: `GET /v1/balance` responde con el saldo actual de Horisen.
- **Reintentos**: apagar webhook.site temporalmente → verificar que `webhook_deliveries` progresa por los intentos con `next_attempt_at` correcto.
- **Dashboard**: login, crear tenant, emitir API key (ver una sola vez), ver mensajes y deliveries.
- **Seguridad**: API key inválida → 401; key de otro tenant → 401; `/admin/*` sin JWT → 401.

---

## Gaps conocidos a confirmar con Horisen antes de prod

- TPS/RPS máximo permitido (ajustar `HORISEN_TPS`).
- Si los webhooks DLR/MO vienen firmados (hoy asumimos shared secret en query).
- IPs de origen de Horisen para allowlist.
- Política exacta de reintentos del lado Horisen sobre nuestro endpoint DLR/MO (para idempotencia).
