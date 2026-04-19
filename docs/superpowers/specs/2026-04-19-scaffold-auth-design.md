# Scaffold + Auth Layer — Design Spec

**Fecha:** 2026-04-19
**Módulo:** Fundaciones del gateway SMS (fases 1-2 del `docs/PLAN.md`)
**Stack:** Go 1.24 + PostgreSQL 15 + pgx/v5 + sqlc + chi + zerolog + golang-migrate

## Objetivo

Construir la base del módulo SMS: estructura del repo, conexión a Postgres, migraciones, modelo de tenants y API keys, autenticación (API key para clientes, JWT para admin dashboard), y un CLI `smsctl` para operarlo. Debe terminar con un server HTTP mínimo (`/admin/login` + `/v1/ping`) que prueba el pipeline de autenticación end-to-end.

## Decisiones clave

1. **Migraciones incrementales.** Cada fase trae su propia migración. `001_auth.up.sql` crea solo las tablas necesarias para auth (`tenants`, `api_keys`, `admin_users`, `audit_log`). Fases posteriores añadirán `002_messages`, `003_webhooks`, etc.
2. **HTTP mínimo además del CLI.** El módulo entrega `cmd/server` con 2 endpoints (`POST /admin/login`, `GET /v1/ping`) para cerrar el loop end-to-end y permitir curls reales. CRUD admin completo queda para fase posterior.
3. **Tests contra Postgres real, compartiendo el compose de dev.** `DATABASE_URL_TEST` apunta a una DB `sms_test` en el mismo Postgres. Tests hacen `TRUNCATE ... RESTART IDENTITY CASCADE` entre casos. Nada de mocks de DB.
4. **sqlc para el data access.** Queries escritas en `.sql`, Go tipado generado. Verificación schema-query en tiempo de compilación.
5. **SHA-256 + pepper para API keys, bcrypt solo para passwords admin.** Las API keys son 32 bytes aleatorios (256 bits de entropía) — bcrypt es innecesariamente caro y no aporta seguridad adicional. Patrón estándar en Stripe/GitHub/Slack. Bcrypt (cost 12) se mantiene solo en `admin_users.password_hash` donde el input es elegido por un humano.

## Estructura de directorios

```
C:\codigo\vps\sms\
├── go.mod
├── go.sum
├── Makefile
├── docker-compose.yml            # solo Postgres, para dev y tests
├── sqlc.yaml
├── cmd/
│   ├── server/main.go            # arranca HTTP server
│   └── smsctl/main.go            # CLI admin
├── internal/
│   ├── config/                   # Load() desde env, validación estricta
│   │   └── config.go
│   ├── logger/                   # zerolog global con request-id
│   │   └── logger.go
│   ├── db/
│   │   ├── pool.go               # pgxpool.New() desde DATABASE_URL
│   │   ├── testutil.go           # WithTestDB(t) para integration tests
│   │   ├── migrations/
│   │   │   ├── 001_auth.up.sql
│   │   │   └── 001_auth.down.sql
│   │   └── sqlc/                 # queries.sql + código generado
│   │       ├── queries.sql
│   │       └── generated/        # no editar a mano
│   ├── tenancy/
│   │   ├── tenant.go             # CRUD: Create, GetByID, List, Suspend
│   │   └── apikey.go             # Issue, Verify, Revoke
│   ├── admin/
│   │   └── user.go               # CreateAdmin, VerifyPassword, Login→JWT
│   ├── auth/
│   │   ├── token.go              # IssueJWT, ParseJWT
│   │   ├── apikeyhash.go         # sha256(secret+pepper), ConstantTimeCompare
│   │   └── middleware/
│   │       ├── apikey.go         # X-API-Key → tenant en ctx
│   │       └── adminjwt.go       # Authorization: Bearer → admin en ctx
│   └── httpx/
│       ├── router.go             # mount /admin/login, /v1/ping
│       ├── response.go           # JSON errors estándar {code, message}
│       └── reqid.go              # X-Request-ID middleware
└── docs/
    ├── PLAN.md                   # plan global (ya existe)
    └── superpowers/
        └── specs/
            └── 2026-04-19-scaffold-auth-design.md   # este documento
```

## Esquema de base de datos

Migración **`001_auth.up.sql`** — 4 tablas. El schema del `PLAN.md` con **una diferencia**: `api_keys.hash` guarda `hex(sha256(secret + pepper))` en vez de bcrypt, y se añade `hash_algo` para permitir rotación futura de algoritmo.

```sql
-- 001_auth.up.sql

CREATE TABLE tenants (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',      -- active|suspended
  daily_sms_limit INT,                                 -- NULL = sin límite
  monthly_budget  NUMERIC(12,4),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  prefix          TEXT NOT NULL UNIQUE,                -- primeros 12 chars del token visible
  hash            TEXT NOT NULL,                       -- hex(sha256(secret + pepper))
  hash_algo       TEXT NOT NULL DEFAULT 'sha256-v1',
  name            TEXT,                                -- label dado por el admin
  scopes          TEXT[] NOT NULL DEFAULT ARRAY['send','read','webhooks']::TEXT[],
  last_used_at    TIMESTAMPTZ,
  revoked_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_api_keys_tenant_active ON api_keys(tenant_id) WHERE revoked_at IS NULL;

CREATE TABLE admin_users (
  id              BIGSERIAL PRIMARY KEY,
  email           TEXT UNIQUE NOT NULL,
  password_hash   TEXT NOT NULL,                       -- bcrypt cost 12
  role            TEXT NOT NULL,                       -- superadmin|operator
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id              BIGSERIAL PRIMARY KEY,
  actor_id        BIGINT REFERENCES admin_users(id),
  action          TEXT NOT NULL,
  target_type     TEXT,
  target_id       TEXT,
  metadata        JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_actor ON audit_log(actor_id, created_at DESC);
```

**`001_auth.down.sql`** — `DROP TABLE` inverso en orden (audit_log, admin_users, api_keys, tenants).

## API key format

- **Generación:** 32 bytes `crypto/rand` → `base64.RawURLEncoding` (43 chars sin padding) → token visible = `sk_live_<encoded>`.
- **prefix** persistido = primeros 12 chars del token completo (`sk_live_ab12`). Sirve como lookup key y para mostrarlo en el dashboard (la primera parte es pública).
- **hash** persistido = `hex(sha256(token_bytes + API_KEY_PEPPER_bytes))`. Longitud 64 chars.
- **Verificación** (middleware `apikey.go`):
  1. Lee `X-API-Key` header. Falta o vacío → 401.
  2. Valida formato: empieza con `sk_live_` y longitud total = 51 (`sk_live_` + 43 chars base64url). Inválido → 401.
  3. `SELECT tenant_id, hash, revoked_at FROM api_keys WHERE prefix = $1`. No encontrado → 401.
  4. Si `revoked_at IS NOT NULL` → 401.
  5. Compara con `subtle.ConstantTimeCompare(storedHash, computedHash)`. Mismatch → 401.
  6. Ok → `ctx = context.WithValue(ctx, tenantKey, tenantID)` y handler continúa.
  7. Fire-and-forget: `UPDATE api_keys SET last_used_at = now() WHERE id = $1` en goroutine, con timeout 2s para no bloquear.
- **El token completo se muestra UNA sola vez** al emitirlo (`smsctl key issue` imprime en stdout; futuro dashboard lo mostrará en un modal no copiable dos veces).

## HTTP surface

### `POST /admin/login`

```http
POST /admin/login
Content-Type: application/json

{ "email": "you@domain.com", "password": "..." }
```

Respuesta `200`:
```json
{ "token": "eyJhbGciOiJIUzI1NiJ9...", "expires_at": "2026-04-20T10:00:00Z", "role": "superadmin" }
```

- JWT HS256, secreto `JWT_SECRET`, TTL configurable vía `JWT_TTL_HOURS` (default 12), claims `{sub:<admin_id>, role, iat, exp}`.
- Contra `admin_users`: bcrypt-compare.
- 401 en cualquier fallo sin diferenciar "usuario no existe" vs "password mala" (evita enumeration).
- Rate limiting edge (fuera de alcance de este módulo, lo hará nginx o middleware de fase posterior).

### `GET /v1/ping`

```http
GET /v1/ping
X-API-Key: sk_live_...
```

Respuesta `200`:
```json
{ "ok": true, "tenant_id": 42, "at": "2026-04-19T21:00:00Z" }
```

- Endpoint trivial para verificar que todo el pipeline auth funciona.
- 401 si falta/inválida/revocada la key.

### Formato de errores estándar

```json
{ "error": { "code": "unauthorized", "message": "invalid API key" } }
```

Códigos usados en este módulo: `unauthorized`, `bad_request`, `internal`.

## CLI `smsctl`

Usando `flag` stdlib con subcomandos manuales (cero deps extra).

```
smsctl migrate up                           # aplica migraciones pendientes
smsctl migrate down                         # revierte la última
smsctl migrate version                      # imprime versión actual

smsctl admin create --email X --password Y [--role superadmin|operator]
                                            # role default: superadmin si no hay admins aún, operator en el resto
smsctl admin list

smsctl tenant create --name "Acme" [--daily-limit N] [--monthly-budget 100.50]
smsctl tenant list
smsctl tenant suspend --id N
smsctl tenant activate --id N

smsctl key issue --tenant-id N [--name "label"]
                                            # imprime el token UNA sola vez a stdout, luego lo hashea y guarda
smsctl key list --tenant-id N
smsctl key revoke --id N
```

Todos los comandos usan los mismos paquetes (`internal/tenancy`, `internal/admin`) que el HTTP server — cero duplicación.

**Idempotencia:** `admin create` con email existente → error claro; `tenant create` con mismo name → permitido (no es UNIQUE). `migrate up` es idempotente por naturaleza (golang-migrate lleva su tabla `schema_migrations`).

## Testing

### Estrategia

- **Unit tests:** funciones puras (generación de key, hash, JWT issue/parse, parsing de headers, validación de format). Sin DB.
- **Integration tests:** contra Postgres real vía `DATABASE_URL_TEST`. Cada test usa `WithTestDB(t)` que hace `TRUNCATE tenants, api_keys, admin_users, audit_log RESTART IDENTITY CASCADE` al iniciar y recibe un `*pgxpool.Pool` listo.

### Casos integrales mínimos

1. `TestAPIKeyLifecycle` — Issue key → Verify con hash correcto OK → Revoke → Verify rechaza.
2. `TestAdminLogin` — CreateAdmin → Login con password correcta → JWT válido y parseable → Login con password mala → 401.
3. `TestPingEndpoint` — Levanta server con `httptest`, curl con key válida → 200, sin header → 401, con key revocada → 401.
4. `TestMigrations` — `migrate up` → tablas existen → `migrate down` → tablas desaparecen.

### CI

Fuera de alcance de este módulo. Nota en el spec para montarlo después con GitHub Actions + service Postgres.

## Configuración (env vars)

```
DATABASE_URL=postgres://sms:sms@localhost:5432/sms?sslmode=disable
DATABASE_URL_TEST=postgres://sms:sms@localhost:5432/sms_test?sslmode=disable
BIND_ADDR=127.0.0.1:7300
JWT_SECRET=<64 bytes hex>            # required, fail-fast si falta
JWT_TTL_HOURS=12
API_KEY_PEPPER=<32 bytes hex>        # required, fail-fast si falta
BCRYPT_COST=12
LOG_LEVEL=info                       # debug|info|warn|error
ENV=dev                              # dev|prod; en prod rechaza secrets de ejemplo
```

`config.Load()` valida todos en arranque. Si `ENV=prod` y un secreto tiene longitud < mínimo o valor obvio de ejemplo, falla arranque.

## Error handling

- **Errores sentinel** en cada paquete de dominio:
  - `tenancy.ErrTenantNotFound`, `tenancy.ErrAPIKeyNotFound`, `tenancy.ErrAPIKeyRevoked`, `tenancy.ErrAPIKeyInvalid`
  - `admin.ErrInvalidCredentials`, `admin.ErrAdminExists`
- **Envoltura** con `fmt.Errorf("verifying key: %w", err)` para preservar el sentinel y añadir contexto.
- **Middleware traduce** a HTTP:
  - `ErrAPIKey*` → 401 `unauthorized`
  - `pgx.ErrNoRows` (filtrado antes de llegar al middleware) → nunca se propaga crudo
  - Cualquier otro → 500 `internal`, loguea el error con stack.
- **Logger** adjunta `request_id`, `tenant_id` (si está en ctx), `route`, `status`, `duration_ms` en cada request log line.

## `docker-compose.yml`

```yaml
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: sms
      POSTGRES_PASSWORD: sms
      POSTGRES_DB: sms
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./deploy/postgres-init.sql:/docker-entrypoint-initdb.d/init.sql:ro

volumes:
  postgres-data:
```

`deploy/postgres-init.sql` adicionalmente crea la DB `sms_test`. La app **no** corre en compose — `go run ./cmd/server` local.

## Makefile

Targets mínimos:
```
make up           # docker compose up -d postgres
make down         # docker compose down
make migrate      # smsctl migrate up contra DATABASE_URL
make migrate-test # smsctl migrate up contra DATABASE_URL_TEST
make sqlc         # regenera código sqlc
make test         # go test ./... (requiere migrate-test previo)
make run          # go run ./cmd/server
make build        # go build ./cmd/server ./cmd/smsctl
```

## Criterios de aceptación

1. `docker compose up -d postgres` → Postgres escucha en :5432 con DBs `sms` y `sms_test`.
2. `smsctl migrate up` → 4 tablas creadas en `sms`.
3. `smsctl admin create --email a@b --password p1234567` → admin creado, id impreso.
4. `smsctl tenant create --name Acme` → tenant id impreso.
5. `smsctl key issue --tenant-id 1` → imprime `sk_live_...` una única vez.
6. `go run ./cmd/server` → escucha en 127.0.0.1:7300, loguea arranque.
7. `curl -H "X-API-Key: <token>" http://127.0.0.1:7300/v1/ping` → 200 + `{tenant_id:1, ok:true}`.
8. `curl -H "X-API-Key: bad" .../v1/ping` → 401.
9. `smsctl key revoke --id 1` → siguiente curl con el token → 401.
10. `curl -X POST -d '{"email":"a@b","password":"p1234567"}' .../admin/login` → 200 + JWT.
11. `go test ./...` en verde (requiere `make migrate-test` previo).

## Fuera de alcance explícito

- CRUD admin HTTP completo (`/admin/tenants/*`, `/admin/api-keys/*`) — fase posterior.
- Rate limiting per-tenant — fase posterior (edge nginx + middleware).
- Cliente Horisen, workers de outbox, webhooks, DLR/MO — fases 3-7 del `PLAN.md`.
- Dashboard React — fase 11.
- Deploy a producción (systemd, nginx, certbot) — fase 12.
- CI en GitHub Actions — nota para después.

## Riesgos / decisiones a revisitar

- **sqlc:** primera vez en este proyecto. Si el onboarding duele, podemos retroceder a repos manuales sin perder compatibilidad (el schema no cambia).
- **JWT TTL 12h sin refresh:** simple para v1. Si el dashboard futuro necesita sesiones más largas, añadimos refresh tokens en una migración posterior.
- **Fire-and-forget `last_used_at`:** si el pool está agotado, se pierden algunos updates. Aceptable para este campo (no es crítico para auth).
