# Onboarding de un tenant nuevo (superadmin)

Guía paso a paso para incorporar un sistema cliente (facturación,
monitoreo, app, etc.) que va a enviar SMS por la pasarela.

Asume que tienes acceso al dashboard `https://sms.aipanel.cl/dashboard/`
con tu cuenta `superadmin`.

---

## TL;DR

```
1. Crear tenant            (Dashboard → Clientes → "Nuevo cliente")
2. Configurar cuota diaria (Resumen del cliente → "Cuota diaria")
3. Configurar sender list  (Resumen del cliente → "Sender IDs autorizados")
4. Emitir API key          (Llaves API → "Generar nueva")
5. Entregar credenciales   (canal seguro: 1Password, GPG, etc.)
6. Cliente registra webhook (opcional pero recomendado)
```

---

## 1. Crear el tenant

**Vía dashboard**: Clientes → "Nuevo cliente". Campos:
- `name`: identificable (e.g. "Segtelco-Facturacion", "Acme-Monitoreo").
- `daily_sms_limit`: dimensiona conservador. Calcula `volumen_esperado_diario × 1.5`. Para un sistema de facturación que envía a ~200 clientes/día → 300. Puedes ajustar después.

**Vía API admin** (para automatización):

```bash
curl -X POST https://sms.aipanel.cl/admin/tenants \
  -H "Authorization: Bearer <jwt_admin>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Segtelco-Facturacion","daily_sms_limit":300}'
```

Devuelve `{"id": N, "name": "...", "status": "active", ...}`.

### Notas sobre la cuota diaria

- La ventana es **midnight America/Santiago a midnight America/Santiago** (no UTC, no rolling 24h). Más fácil de auditar para facturación CLP.
- Solo cuenta SMS que llegaron a `queued`/`sending`/`sent`/`delivered`/`undelivered`. Los `rejected` y `failed` **no consumen** cuota.
- Multi-parte: 1 SMS de 3 partes consume 1 del contador (no 3). El operador sí cobra 3, pero la pasarela solo te limita por mensaje lógico.
- Cuando el tenant cruza 80%, generamos audit log `tenant.quota_warning` y la barra del dashboard se pone ámbar.
- A 100% el cliente recibe `429 daily_quota_exceeded` con `Retry-After` hasta medianoche Santiago.

---

## 2. Configurar la sender allow-list

**Recomendado** para evitar phishing si la API key se filtra. Sin
allow-list, cualquiera con la key puede mandar como `"BancoChile"`.

**Vía dashboard**: Clientes → tu tenant → Resumen → tarjeta "Sender IDs
autorizados" → Editar → ingresa CSV: `Segtelco, FactSegtel`.

**Vía API admin**:

```bash
curl -X PUT https://sms.aipanel.cl/admin/tenants/<id>/allowed-senders \
  -H "Authorization: Bearer <jwt_admin>" \
  -H "Content-Type: application/json" \
  -d '{"allowed_senders":["Segtelco","FactSegtel"]}'
```

Si dejas la lista vacía → cualquier sender pasa. Si la llenas → solo
los listados; cualquier otro recibe `403 sender_not_allowed`.

### ¿Qué senders considerar?

- **Numéricos / shortcodes** (e.g. `2030`): casi siempre aceptados, no requieren registro previo.
- **Alfanuméricos** (e.g. `Segtelco`): hasta 11 chars. **En Chile algunos operadores los exigen pre-registrados** — si tu cliente reporta que sus SMS no llegan o llegan con sender modificado, contacta al carrier antes de descartar.
- Evita parecerse a marcas que no controlas (`SII`, `BancoEstado`...) — además del riesgo legal, los carriers los suelen bloquear.

---

## 3. Emitir la API key

**Vía dashboard**: tu tenant → Llaves API → "Generar nueva". Pon un
nombre descriptivo (`prod-facturacion`, `dev-pruebas`).

**Vía API admin**:

```bash
curl -X POST https://sms.aipanel.cl/admin/tenants/<id>/api-keys \
  -H "Authorization: Bearer <jwt_admin>" \
  -H "Content-Type: application/json" \
  -d '{"name":"prod-facturacion"}'
```

Respuesta:

```json
{"id": 12, "tenant_id": 5, "prefix": "sk_live_vpzOO8t8",
 "token": "sk_live_vpzOO8t8XW0_ABC...",
 "name": "prod-facturacion", "scopes": ["send","read","webhooks"], ...}
```

⚠️ **El campo `token` solo aparece en este create**. Si lo pierdes, revoca y emite otra.

### Entrega segura

- **No** lo pegues en email/Slack abierto.
- Recomendado: 1Password share-link de un solo uso, `gpg --encrypt` con la pública del cliente, o pasaje en una sesión screen-share donde el cliente lo pega directo en su vault.
- El cliente debe guardarla **solo en su vault de secretos** — `.env` con `chmod 600`, AWS Secrets Manager, Vault, etc. Nunca en un repo de código (ni privado).

---

## 4. (Opcional) El cliente registra un webhook

Mejor que pollear `/v1/sms/{id}` o `/v1/events` cuando el cliente
necesita enterarse de DLRs (entregado/no-entregado) en tiempo real.

**El cliente** llama:

```bash
curl -X POST https://sms.aipanel.cl/v1/webhooks \
  -H "X-API-Key: sk_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://tu-app.com/webhooks/sms",
    "events": ["sms.delivered","sms.undelivered","sms.rejected","sms.inbound"]
  }'
```

Respuesta incluye un `secret` (también one-time) que el cliente usa
para verificar HMAC de cada webhook entrante. Ver
[`docs/api-public.md#webhooks-cómo-recibir-y-verificar`](./api-public.md#webhooks-cómo-recibir-y-verificar).

---

## 5. Smoke test E2E con el cliente

Antes de declararlo "vivo en producción", hacer un envío real al
celular del cliente o tuyo:

```bash
API_KEY="sk_live_..."
curl -X POST https://sms.aipanel.cl/v1/sms \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"sender":"Segtelco","to":"+5698XXXXXXX","text":"Smoke test pasarela"}'
```

Esperado: `202 Accepted`, ~3 segundos después el SMS llega al teléfono
y `GET /v1/sms/<id>` muestra `status: "delivered"`.

---

## Operación día a día

### Monitorear cuotas

**Dashboard**: en cada tenant, la tarjeta "Cuota diaria · hoy" se
actualiza cada 30s. Ámbar a partir de 80%, rojo a 100%.

**Vista global**: `GET /admin/stats/quota` lista todos los tenants
con cuota, ordenados por % usado DESC. Útil para alertas a tu equipo.

### Suspender un tenant (incidente / impago)

```bash
curl -X POST https://sms.aipanel.cl/admin/tenants/<id>/suspend \
  -H "Authorization: Bearer <jwt_admin>"
```

Efecto inmediato: todas las requests del tenant a `/v1/*` reciben
`403 tenant_suspended`. Reactivar:

```bash
curl -X POST https://sms.aipanel.cl/admin/tenants/<id>/activate \
  -H "Authorization: Bearer <jwt_admin>"
```

### Revocar una API key filtrada

```bash
# 1. Listar keys del tenant para encontrar el id
curl https://sms.aipanel.cl/admin/tenants/<id>/api-keys \
  -H "Authorization: Bearer <jwt_admin>"

# 2. Revocar
curl -X POST https://sms.aipanel.cl/admin/api-keys/<key-id>/revoke \
  -H "Authorization: Bearer <jwt_admin>"

# 3. Emitir reemplazo y entregar al cliente
curl -X POST https://sms.aipanel.cl/admin/tenants/<id>/api-keys \
  -H "Authorization: Bearer <jwt_admin>" \
  -d '{"name":"prod-facturacion-v2"}'
```

La revocación es inmediata — el siguiente uso recibe 401.

### Cambiar la cuota / allow-list de un tenant ya en producción

Cualquier momento, sin downtime:

```bash
# Subir cuota
curl -X PATCH https://sms.aipanel.cl/admin/tenants/<id> \
  -d '{"daily_sms_limit": 1000}'

# Agregar sender
curl -X PUT https://sms.aipanel.cl/admin/tenants/<id>/allowed-senders \
  -d '{"allowed_senders":["Segtelco","FactSegtel","NuevaMarca"]}'
```

Las nuevas reglas aplican en la siguiente request entrante (no hay
caché por más de 1s).

### Recuperación de 2FA de otro admin

Si un admin pierde su autenticador (cambió de teléfono, etc.):

```bash
# 1. Listar para encontrar el id
curl https://sms.aipanel.cl/admin/users \
  -H "Authorization: Bearer <jwt_admin>"

# 2. Resetear
curl -X POST https://sms.aipanel.cl/admin/users/<id>/totp/reset \
  -H "Authorization: Bearer <jwt_admin>"
```

Solo `superadmin` puede ejecutar esto, y nunca contra sí mismo (para
self-disable hay otro endpoint que valida un código actual).

Tras el reset, el admin víctima loggea solo con password, va a `/cuenta`
y re-enrola TOTP escaneando el QR.

---

## Checklist pre-producción

Antes de soltarle el sistema a un cliente real:

- [ ] Tenant creado con `daily_sms_limit` razonable (1.5× el volumen esperado).
- [ ] Sender allow-list configurado (no dejar vacío salvo razón concreta).
- [ ] API key emitida y entregada por canal seguro.
- [ ] Smoke test exitoso (1 SMS real → `delivered`).
- [ ] Cliente sabe cómo:
  - usar `client_ref` (idempotencia)
  - manejar 429 (rate_limited y daily_quota_exceeded)
  - leer headers `X-Daily-Quota-*` para alertar cerca del tope
- [ ] (Si aplica) webhook registrado y firma verificada con un evento real.
- [ ] Dashboard del cliente añadido a tu lista de "tenants a monitorear".

---

## Archivos relacionados

- [`api-public.md`](./api-public.md) — referencia que entregas al cliente
- [`ejemplos/`](./ejemplos) — scripts de cliente listos para copiar
- [`SECURITY.md`](./SECURITY.md) — modelo de amenazas + decisiones de seguridad
