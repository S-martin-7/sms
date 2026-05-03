# Pasarela SMS — Documentación pública

API HTTP para enviar SMS desde tu aplicación. Diseñada para ser un puente
delgado: tú armas el texto del mensaje (interpolando los datos que
quieras: nombre, factura, monto, etc.) y nosotros lo entregamos al
operador. Sin manejar contactos en nuestro lado, sin plantillas
server-side.

- **Base URL**: `https://sms.aipanel.cl`
- **Formato**: JSON sobre HTTPS
- **Auth**: header `X-API-Key`
- **Soporte**: cuando algo falla, `X-Request-Id` en la respuesta te da el
  identificador para reportar.

---

## Tabla de contenidos

- [Quick start (3 minutos)](#quick-start-3-minutos)
- [Autenticación](#autenticación)
- [Endpoints](#endpoints)
  - [POST /v1/sms — enviar uno](#post-v1sms--enviar-uno)
  - [POST /v1/sms/bulk — enviar varios](#post-v1smsbulk--enviar-varios)
  - [GET /v1/sms/{id} — consultar estado](#get-v1smsid--consultar-estado)
  - [GET /v1/sms — listar](#get-v1sms--listar)
  - [POST /v1/webhooks — registrar webhook](#post-v1webhooks--registrar-webhook)
  - [GET /v1/events — feed polling](#get-v1events--feed-polling)
  - [GET /v1/balance, /v1/ping](#otros)
- [Estados del SMS](#estados-del-sms)
- [Errores](#errores)
- [Headers que devolvemos](#headers-que-devolvemos)
- [Webhooks: cómo recibir y verificar](#webhooks-cómo-recibir-y-verificar)
- [Buenas prácticas](#buenas-prácticas)
- [Ejemplos por lenguaje](#ejemplos-por-lenguaje)
- [Casos de uso reales](#casos-de-uso-reales)
- [FAQ](#faq)

---

## Quick start (3 minutos)

```bash
# 1. Tu superadmin te dio una API key. Por ejemplo:
API_KEY="sk_live_vpzOO8t8XW0_..."

# 2. Envía un SMS:
curl -X POST https://sms.aipanel.cl/v1/sms \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "sender": "Segtelco",
    "to": "+56985917376",
    "text": "Hola Rodrigo, tu factura 1234 vence en 5 días.",
    "client_ref": "factura-1234-r1"
  }'

# Respuesta (HTTP 202):
# {"id":"c847a2aa-...","status":"queued","sender":"Segtelco","to":"+56985917376", ...}

# 3. Consulta el estado (queued → sending → sent → delivered):
curl https://sms.aipanel.cl/v1/sms/c847a2aa-... \
  -H "X-API-Key: $API_KEY"
```

Eso es todo. En la práctica te toma menos de 3 segundos llegar de
`queued` a `delivered` (verificado en producción).

---

## Autenticación

Todas las rutas bajo `/v1/*` requieren el header:

```
X-API-Key: sk_live_<43 caracteres>
```

La key se emite **una sola vez** al crear tu tenant. Guárdala en un
vault (1Password, Vault de HashiCorp, AWS Secrets Manager, archivo
`.env` con permisos restringidos). Si sospechas filtración, escribe a
soporte para revocarla y emitir otra.

### Errores de auth

| HTTP | `error.code`         | Cuándo |
|---|---|---|
| 401 | `unauthorized`       | Falta el header o la key no existe / fue revocada |
| 403 | `tenant_suspended`   | Tu tenant está suspendido — contacta soporte |
| 403 | `sender_not_allowed` | El `sender` no está en tu allow-list (si tu tenant la tiene configurada) |
| 429 | `rate_limited`       | Demasiadas requests — respeta `Retry-After` |
| 429 | `daily_quota_exceeded` | Alcanzaste tu cuota diaria — respeta `Retry-After` |

---

## Endpoints

### POST /v1/sms — enviar uno

```bash
curl -X POST https://sms.aipanel.cl/v1/sms \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "sender": "Segtelco",
    "to": "+56985917376",
    "text": "Estimado Rodrigo Quiroga: su factura 1234 emitida el 22-01-2026 por $200.000 vence en 5 días.",
    "client_ref": "factura-1234-r1"
  }'
```

**Request**

| Campo | Tipo | Requerido | Descripción |
|---|---|---|---|
| `sender` | string | sí | Sender ID. Si tu tenant tiene allow-list, debe ser uno autorizado. Sender numérico (shortcode) o alfanumérico (≤ 11 chars). |
| `to` | string | sí | Destinatario en E.164 (`+569...`). |
| `text` | string | sí | Cuerpo del mensaje. Tu app arma el texto final con los datos del cliente. |
| `client_ref` | string | no | **Idempotency key**. Reintenta con el mismo valor → 409 sin duplicar SMS. |
| `send_at` | string RFC3339 | no | Si está presente, programa el envío en lugar de enviarlo ahora. |

**Respuesta `202 Accepted`**

```json
{
  "id": "c847a2aa-9437-4138-b1bd-acb98f6c5eb0",
  "tenant_id": 4,
  "sender": "Segtelco",
  "to": "+56985917376",
  "text": "...",
  "dcs": "GSM",
  "num_parts": 1,
  "status": "queued",
  "client_ref": "factura-1234-r1",
  "attempts": 0,
  "created_at": "2026-05-04T07:00:11.337+08:00"
}
```

**Headers de respuesta** (cuando hay cuota configurada):

```
X-Daily-Quota-Limit: 500
X-Daily-Quota-Used: 12
X-Daily-Quota-Remaining: 488
```

Úsalos para alertar cuando estás cerca del tope sin tener que pollear
`/admin/stats/quota`.

### POST /v1/sms/bulk — enviar varios

Hasta **1000 mensajes** por request. Aceptación parcial: las filas que
fallan no bloquean las que pasan.

```bash
curl -X POST https://sms.aipanel.cl/v1/sms/bulk \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "default_sender": "Segtelco",
    "messages": [
      {"to":"+56911111111","text":"Hola Juan","client_ref":"factura-1"},
      {"to":"+56922222222","text":"Hola María","client_ref":"factura-2"}
    ]
  }'
```

**Respuesta `202`**

```json
{
  "batch_id": "b_<uuid>",
  "accepted": 2,
  "rejected": 0,
  "messages": [
    {"index":0,"id":"...","to":"+56911111111","status":"queued","client_ref":"factura-1"},
    {"index":1,"id":"...","to":"+56922222222","status":"queued","client_ref":"factura-2"}
  ]
}
```

Filas rechazadas vienen con `status: "rejected"`, `error`, `error_code`.
Tipos de `error_code`: `bad_request`, `duplicate_client_ref`,
`daily_quota_exceeded`, `sender_not_allowed`.

### GET /v1/sms/{id} — consultar estado

```bash
curl https://sms.aipanel.cl/v1/sms/c847a2aa-... \
  -H "X-API-Key: $API_KEY"
```

Devuelve el mismo schema que `POST /v1/sms`, con `sent_at` y `final_at`
poblados según el estado.

### GET /v1/sms — listar

```bash
curl "https://sms.aipanel.cl/v1/sms?status=delivered&limit=20" \
  -H "X-API-Key: $API_KEY"
```

**Filtros opcionales**: `status`, `recipient`, `client_ref`, `from`
(RFC3339), `to` (RFC3339), `limit` (max 200, default 50), `cursor`.

**Respuesta**: `{"messages": [...], "next_cursor": "<rfc3339>_<uuid>"}`.

Para paginar: usa el último `next_cursor` en la siguiente request con
`&cursor=<value>`.

### POST /v1/webhooks — registrar webhook

```bash
curl -X POST https://sms.aipanel.cl/v1/webhooks \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://tu-app.com/webhooks/sms",
    "events": ["sms.delivered","sms.undelivered","sms.rejected","sms.inbound"]
  }'
```

**Respuesta**

```json
{
  "id": 7,
  "url": "https://tu-app.com/webhooks/sms",
  "events": ["sms.delivered","sms.undelivered","sms.rejected","sms.inbound"],
  "active": true,
  "created_at": "2026-05-04T07:30:00Z",
  "secret": "wh_..."
}
```

⚠️ El `secret` se devuelve **solo en el create**. Guárdalo: lo necesitas
para validar las firmas de los webhooks entrantes (ver
[sección webhooks](#webhooks-cómo-recibir-y-verificar)).

### GET /v1/events — feed polling

Alternativa a webhooks si no puedes/quieres exponer un endpoint público.

```bash
curl "https://sms.aipanel.cl/v1/events?types=sms.delivered,sms.undelivered&limit=100" \
  -H "X-API-Key: $API_KEY"
```

**Filtros**: `types` (CSV), `from`, `to` (RFC3339), `limit` (max 500),
`cursor`.

**Respuesta**:

```json
{
  "events": [
    {
      "id": 192,
      "type": "sms.delivered",
      "created_at": "2026-05-04T07:00:13.967+08:00",
      "data": {
        "type": "sms.delivered",
        "tenant_id": 4,
        "message_id": "c847a2aa-...",
        "horisen_msg_id": "14b2b268-...",
        "status": "delivered",
        "num_parts": 1,
        "timestamp": "2026-05-03T23:00:13.967Z"
      }
    }
  ],
  "next_cursor": null
}
```

### Otros

- **`GET /v1/ping`** — health check con tu API key. Devuelve
  `{"ok":true,"tenant_id":N,"at":"..."}`.
- **`GET /v1/balance`** — saldo del proveedor (informativo). Puede
  responder `503` si el endpoint no está configurado en tu instancia.

---

## Estados del SMS

| `status` | Descripción |
|---|---|
| `queued` | En cola interna; el outbox lo va a tomar pronto. |
| `sending` | Enviando al carrier, esperando ack. |
| `sent` | Aceptado por el carrier, esperando DLR del operador. |
| `delivered` | ✅ Confirmado entregado al teléfono destino. |
| `undelivered` | ❌ Carrier reportó no-entrega (apagado, fuera de cobertura, número inválido). |
| `rejected` | ❌ Rechazado pre-envío por validación o por carrier. |
| `failed` | ❌ Error tras agotar reintentos internos. |

**Estados terminales**: `delivered`, `undelivered`, `rejected`, `failed`.
`final_at` se setea cuando entra a uno de estos.

---

## Errores

Formato común:

```json
{"error":{"code":"<machine_code>","message":"<human description>"}}
```

| HTTP | `error.code` | Significado | Qué hacer |
|---|---|---|---|
| 400 | `bad_request` | JSON inválido o campos faltantes | Arregla el payload |
| 401 | `unauthorized` | Key inválida / revocada / falta header | Verifica `X-API-Key` |
| 403 | `tenant_suspended` | Tu tenant está suspendido | Contacta a soporte |
| 403 | `sender_not_allowed` | `sender` no está en tu allow-list | Usa un sender autorizado o pide ampliar la lista |
| 404 | `not_found` | El `id` consultado no existe (o no es tuyo) | Verifica el id |
| 409 | `duplicate_client_ref` | Ya enviaste un SMS con ese `client_ref` | Es idempotente — ya quedó procesado, no reintentes |
| 429 | `rate_limited` | Demasiadas requests/seg | Espera `Retry-After` segundos |
| 429 | `daily_quota_exceeded` | Cuota diaria alcanzada | Espera `Retry-After` (segundos hasta medianoche America/Santiago) |
| 500 | `internal` | Algo nuestro falló | Reporta con `X-Request-Id` |

### Comportamiento de los rate limiters

Tienes **dos** capas:

1. **Edge (nginx)**: 20 req/s/IP, burst 40 — ayuda contra DDoS.
2. **Per-tenant**: 5 SMS/s, burst 10 (por defecto, configurable). Aplica
   solo a `POST /v1/sms` y `POST /v1/sms/bulk`. GETs no se throttlean.

Excederlos → `429 rate_limited`. Excederlos no rompe nada — espera unos
segundos y vuelve.

---

## Headers que devolvemos

| Header | Cuándo | Significado |
|---|---|---|
| `X-Request-Id` | Siempre | Identificador para reportar a soporte |
| `Retry-After` | En 429 | Segundos a esperar antes de reintentar |
| `X-Daily-Quota-Limit` | En `POST /v1/sms` cuando hay cuota | Tope diario configurado |
| `X-Daily-Quota-Used` | Idem | SMS consumidos hoy (no cuenta `rejected`/`failed`) |
| `X-Daily-Quota-Remaining` | Idem | Lo que queda |

Recomendamos loguear/alertar cuando `X-Daily-Quota-Used >= 0.8 * Limit`
en tu monitoreo.

---

## Webhooks: cómo recibir y verificar

Cuando un SMS cambia de estado (`delivered`, `undelivered`, `rejected`)
o llega un MO, te enviamos un POST a tu URL registrada con headers:

```
POST /tu-endpoint HTTP/1.1
Content-Type: application/json
X-Event-Id: <uuid>
X-Event-Type: sms.delivered
X-Signature: t=1777839284,v1=<hex_hmac_sha256>
```

**Body** (mismo schema que `/v1/events`):

```json
{
  "type": "sms.delivered",
  "tenant_id": 4,
  "message_id": "c847a2aa-...",
  "horisen_msg_id": "14b2b268-...",
  "status": "delivered",
  "num_parts": 1,
  "timestamp": "2026-05-03T23:00:13.967Z"
}
```

### Verificación de firma (Python)

```python
import hmac, hashlib, time

def verify_webhook(secret: str, body_bytes: bytes, signature_header: str) -> bool:
    """Verifica una firma X-Signature: t=<unix>,v1=<hex>."""
    parts = dict(p.split("=", 1) for p in signature_header.split(","))
    t, v1 = int(parts["t"]), parts["v1"]
    # Rechaza eventos con más de 5 minutos (replay protection)
    if abs(time.time() - t) > 300:
        return False
    payload = f"{t}.".encode() + body_bytes
    expected = hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, v1)
```

### Reintentos

Si tu endpoint **no responde 2xx** (o tarda >30s), reintentamos con
backoff: 1m → 5m → 30m → 2h → 8h → 24h. Tras 6 fallos lo marcamos como
`dead`.

**Recomendación**: responde `200` lo antes posible (encola async en tu
lado). No proceses la lógica pesada mientras nos tienes en espera.

---

## Buenas prácticas

### 1. Siempre usa `client_ref` (idempotencia)

Si tu sistema reintenta por timeout o un crash parcial, sin
`client_ref` te arriesgas a duplicar SMS. Patrón sugerido:

```
{tipo}-{id_recurso}-{intento}
factura-1234-r1
monitor-cpu-202605040730
otp-usuario-456-1717843200
```

Si el segundo POST llega con el mismo `client_ref`:
- HTTP 409 `duplicate_client_ref`
- Significa "ya está; el primer intento se procesó"
- **No es error** — es la idempotencia funcionando

### 2. Captura `429 daily_quota_exceeded` y alerta

Cruzar tu cuota es señal de:
- Loop en tu código (envío en bucle), o
- Uso legítimo creciendo más rápido de lo previsto

Loguéalo a tu sistema de monitoreo. La pasarela también dispara un
audit log `tenant.quota_warning` al cruzar 80% (visible en el dashboard
admin), pero tu propia alerta es más útil para tu equipo.

### 3. No hardcodees el `sender`

Si tu tenant tiene allow-list configurado, recibirás `403
sender_not_allowed` si pones uno equivocado. **No** hardcodees varios
posibles senders y "probar cuál funciona": pide al admin agregar el que
necesitas a la lista.

### 4. Maneja los reintentos del rate limit

```python
import time
def send_with_backoff(payload, max_retries=3):
    for attempt in range(max_retries):
        r = requests.post(URL, headers={"X-API-Key": API_KEY}, json=payload, timeout=10)
        if r.status_code == 202:
            return r.json()
        if r.status_code == 429:
            wait = int(r.headers.get("Retry-After", "5"))
            time.sleep(min(wait, 60))   # cap por si Retry-After es muy alto
            continue
        if r.status_code == 409:
            return {"status": "duplicate"}  # idempotencia ya cubierta
        r.raise_for_status()
    raise RuntimeError("max retries exceeded")
```

### 5. Webhook + polling como respaldo

Webhooks pueden fallar (caída de tu servidor, problema DNS). Si
necesitas garantía absoluta de entrega de eventos, registra el webhook
**Y** además poll `/v1/events?cursor=<último>` cada 30 minutos como
respaldo.

### 6. SMS con caracteres especiales = más caro

- **GSM7 (caracteres ASCII)**: 160 chars en 1 parte; 153 chars/parte en
  multi-parte.
- **UCS-2 (cualquier emoji, tildes en mayúscula, símbolos no-GSM)**:
  70 chars en 1 parte; 67 chars/parte en multi-parte.

La respuesta incluye `dcs` (`GSM` o `UCS`) y `num_parts`. Cada parte
cuenta como 1 SMS para tu cuota y para el costo del operador.

**Tip**: si te importa el costo, evita emojis y normaliza tildes
(`Ñ`/`Ñ`, `Á`/`A`...) — pero **no** lo hagas si afecta legibilidad,
porque eso degrada la experiencia del cliente final más que ahorra.

---

## Ejemplos por lenguaje

Scripts ejecutables completos en [`docs/ejemplos/`](./ejemplos):

- [`sms_cliente.py`](./ejemplos/sms_cliente.py) — Python, caso facturación
- [`sms_cliente.js`](./ejemplos/sms_cliente.js) — Node.js
- [`sms_cliente.php`](./ejemplos/sms_cliente.php) — PHP
- [`sms_cliente.sh`](./ejemplos/sms_cliente.sh) — Bash + curl
- [`webhook_server.py`](./ejemplos/webhook_server.py) — recibir webhooks (Flask)
- [`webhook_server.js`](./ejemplos/webhook_server.js) — recibir webhooks (Express)

---

## Casos de uso reales

### A. Sistema de facturación

```python
# Tu código, una vez por factura
from sms_cliente import enviar_sms

def notificar_factura(cliente, factura):
    texto = (
        f"Estimado {cliente.nombre}: su factura {factura.numero} "
        f"emitida el {factura.fecha:%d-%m-%Y} por ${factura.monto:,} "
        f"vence en {factura.dias_para_vencer} días."
    )
    return enviar_sms(
        sender="Segtelco",
        to=cliente.celular,
        text=texto,
        client_ref=f"factura-{factura.id}-r{factura.recordatorio_n}",
    )
```

### B. Sistema de monitoreo (alertas técnicas)

```python
# Cron cada 5 min, alerta solo cuando cruza umbral
def alertar_si_caido(host):
    if host.estado_actual == "offline" and host.estado_anterior == "online":
        enviar_sms(
            sender="MonitorOPS",
            to=oncall.celular,
            text=f"⚠ {host.nombre} ({host.ip}) DOWN desde {host.caido_desde:%H:%M}.",
            client_ref=f"monitor-{host.id}-{host.caido_desde:%Y%m%d%H%M}",
        )
```

El `client_ref` con timestamp evita reenvío si el cron reintenta el
mismo evento.

### C. Recordatorio de cita médica

```python
def recordar_cita(paciente, cita):
    texto = (
        f"Hola {paciente.nombre}, recordamos su cita con "
        f"Dr. {cita.doctor} el {cita.fecha:%d-%m %H:%M} en {cita.lugar}. "
        f"Para reagendar llame al {clinica.telefono}."
    )
    return enviar_sms(
        sender="ClinicaSur",
        to=paciente.celular,
        text=texto,
        client_ref=f"cita-{cita.id}-recordatorio-24h",
    )
```

### D. Código de verificación (OTP)

```python
def enviar_otp(usuario, codigo):
    return enviar_sms(
        sender="Segtelco",
        to=usuario.celular,
        # Texto corto y claro para que no se confunda con phishing
        text=f"Tu código de verificación es {codigo}. Vence en 5 minutos.",
        # client_ref único por OTP — si el usuario pide reenvío, manda otro
        client_ref=f"otp-{usuario.id}-{int(time.time())}",
    )
```

---

## FAQ

### ¿Puedo enviar a números fuera de Chile?

Sí. Cualquier número en formato E.164 (`+<código_pais><número>`). El
costo y aceptación dependen del operador destino.

### ¿Cuántos caracteres puedo poner?

Sin límite duro. El SMS se parte en múltiples si excede la longitud de
1 parte (160 chars GSM, 70 chars UCS). `num_parts` en la respuesta
indica en cuántas se partió.

### ¿Cómo cambio mi sender allow-list?

Llama a tu superadmin. Vía dashboard: **Clientes → tu tenant →
Resumen → Sender IDs autorizados → Editar**. Vía API admin:
`PUT /admin/tenants/{id}/allowed-senders`.

### ¿Cómo cambio mi cuota diaria?

También solo el superadmin (vía dashboard o API admin). No hay endpoint
para que el tenant suba su propia cuota.

### Mi `sender_id` quedó "BancoAcme" pero la operadora no lo acepta. ¿Qué hago?

Sender IDs alfanuméricos requieren registro previo con el operador
(según país). Pide a soporte verificar si tu sender está pre-registrado;
si no, registrarlo o usar un shortcode numérico mientras tanto.

### Recibí un webhook con firma inválida — ¿qué pasó?

Tres causas comunes:
1. Estás usando el secret de **otro** webhook (no el que devolvió el
   create del que estás recibiendo).
2. Tu framework está modificando el body antes de que tu handler lo
   reciba (e.g. parsing JSON re-serializa con espacios distintos).
   Verifica con el body **raw bytes**, no el JSON re-serializado.
3. Reloj fuera de sincronía: el chequeo `abs(now - t) > 300` rechaza
   eventos viejos. Sincroniza tu servidor con NTP.

### ¿Puedo borrar un SMS encolado antes de que se envíe?

No (todavía). Los SMS pasan a `sending` rápido (~1s). Si necesitas
cancelar, cambia el flujo: programa con `send_at` futuro y bórralo
antes de la hora vía endpoint admin de programados.

### ¿Cuánto tiempo se guardan los mensajes y eventos?

Sin retención automática hoy. Si necesitas exportar antes de migración
o limpieza, usa `GET /v1/sms?from=...&to=...` con paginación.

### ¿Soporta MMS / imágenes?

No. Solo SMS de texto.

---

## Soporte

Cuando reportes un problema incluye siempre:
- `X-Request-Id` (de la respuesta o tu log)
- `id` del mensaje (el UUID que devolvió el `POST /v1/sms`)
- Hora aproximada (con timezone)
- Cuál es el comportamiento esperado vs. el que ves
