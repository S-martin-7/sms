# Pasarela SMS — API pública

Base URL: `https://sms.aipanel.cl`

Esta es la API que usan los sistemas externos (facturación, monitoreo,
cualquier app que necesite enviar SMS) para integrarse con la pasarela.

---

## Autenticación

Todas las rutas bajo `/v1/*` requieren el header:

```
X-API-Key: sk_live_<43 caracteres>
```

La key se emite una sola vez al crear un tenant. **Guárdala en un vault** —
si la pierdes hay que emitir otra y revocar la anterior. Si sospechas que
se filtró, escribe a soporte para revocarla inmediatamente.

Errores de auth:

| Código HTTP | `error.code`         | Cuándo |
|---|---|---|
| 401 | `unauthorized`       | Falta el header o la key no existe / fue revocada |
| 403 | `tenant_suspended`   | El tenant está suspendido — contacta soporte |
| 429 | `rate_limited`       | Demasiadas requests; revisa el header `Retry-After` |

---

## POST /v1/sms — enviar un SMS

```bash
curl -X POST https://sms.aipanel.cl/v1/sms \
  -H "X-API-Key: sk_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "sender": "Segtelco",
    "to": "+56985917376",
    "text": "Estimado Rodrigo Quiroga: su factura 123 vence en 5 días.",
    "client_ref": "factura-123-recordatorio-1"
  }'
```

### Request

| Campo | Tipo | Requerido | Descripción |
|---|---|---|---|
| `sender` | string | sí | Sender ID. Si tu tenant tiene allow-list configurado, debe ser uno de los autorizados. |
| `to` | string | sí | Número en formato E.164 (`+569...`). |
| `text` | string | sí | Cuerpo del mensaje. Tu sistema arma el texto final con los parámetros que necesite. |
| `client_ref` | string | no | **Idempotency key**. Si reintentas con el mismo `client_ref`, recibes `409` sin duplicar el SMS. **Recomendado para sistemas con reintentos**. |
| `send_at` | string (RFC3339) | no | Si está presente, programa el envío en lugar de enviarlo ahora. |

### Respuesta `202 Accepted`

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
  "horisen_msg_id": null,
  "client_ref": "factura-123-recordatorio-1",
  "attempts": 0,
  "created_at": "2026-05-04T07:00:11.337+08:00"
}
```

`status` empieza como `queued`. Pasa a `sending` → `sent` → `delivered` (o
`undelivered`/`rejected`) en cuestión de segundos.

### Errores

| HTTP | `error.code` | Significado |
|---|---|---|
| 400 | `bad_request` | Faltan campos o JSON inválido. |
| 403 | `sender_not_allowed` | El `sender` no está en el allow-list del tenant. |
| 409 | `duplicate_client_ref` | Ya enviaste un SMS con ese `client_ref` para este tenant. |
| 429 | `rate_limited` | Excediste el rate limit (5 SMS/segundo, burst 10). |
| 429 | `daily_quota_exceeded` | Ya alcanzaste tu cuota diaria. `Retry-After` indica los segundos hasta medianoche America/Santiago. |

---

## POST /v1/sms/bulk — enviar varios en una sola request

Hasta 1000 mensajes por batch. Aceptación parcial: las filas que fallan
se reportan en la respuesta sin bloquear las que pasan.

```bash
curl -X POST https://sms.aipanel.cl/v1/sms/bulk \
  -H "X-API-Key: sk_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "default_sender": "Segtelco",
    "messages": [
      {"to":"+56911111111","text":"Hola Juan...", "client_ref":"factura-1"},
      {"to":"+56922222222","text":"Hola María...","client_ref":"factura-2"}
    ]
  }'
```

Respuesta:

```json
{
  "batch_id": "b_<uuid>",
  "accepted": 2,
  "rejected": 0,
  "messages": [
    {"index":0, "id":"...", "to":"+56911111111", "status":"queued", "client_ref":"factura-1"},
    {"index":1, "id":"...", "to":"+56922222222", "status":"queued", "client_ref":"factura-2"}
  ]
}
```

Filas rechazadas vienen con `status: "rejected"`, `error`, `error_code`
(mismo set de códigos que `POST /v1/sms`).

---

## GET /v1/sms/{id} — consultar estado de un SMS

```bash
curl https://sms.aipanel.cl/v1/sms/c847a2aa-9437-4138-b1bd-acb98f6c5eb0 \
  -H "X-API-Key: sk_live_..."
```

Mismo schema que la respuesta de `POST /v1/sms`, con `sent_at` y `final_at`
poblados según estado:

| `status` | Significado |
|---|---|
| `queued` | En cola; el outbox lo va a tomar pronto. |
| `sending` | Enviado al carrier, esperando confirmación. |
| `sent` | Aceptado por Horisen, esperando DLR. |
| `delivered` | Confirmado entregado al destino. |
| `undelivered` | Carrier reportó no-entrega (apagado, fuera de cobertura). |
| `rejected` | Rechazado por carrier o validación pre-envío (sender bloqueado, etc). |
| `failed` | Error tras agotar reintentos. |

---

## GET /v1/sms — listar SMS recientes

Filtros opcionales: `status`, `recipient`, `client_ref`, `from`, `to`,
`limit` (max 200, default 50), `cursor` (formato `<rfc3339>_<uuid>`).

```bash
curl "https://sms.aipanel.cl/v1/sms?status=delivered&limit=20" \
  -H "X-API-Key: sk_live_..."
```

Respuesta: `{"messages": [...], "next_cursor": "..."}`.

---

## Webhooks — recibir push de eventos

Mejor que pollear: registra una URL pública y recibes un POST firmado por
cada evento.

### Registrar

```bash
curl -X POST https://sms.aipanel.cl/v1/webhooks \
  -H "X-API-Key: sk_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://tu-app.com/sms-events",
    "events": ["sms.delivered","sms.undelivered","sms.rejected","sms.inbound"]
  }'
```

Respuesta:

```json
{"id": 7, "url": "...", "events": [...], "active": true, "secret": "wh_..."}
```

⚠️ El `secret` se devuelve **solo en el create**. Guárdalo, lo necesitas
para validar las firmas.

### Validar firma del webhook

Cada POST a tu URL trae:

```
X-Signature: t=1777839284,v1=<hex_hmac_sha256>
X-Event-Id: <uuid>
X-Event-Type: sms.delivered
Content-Type: application/json
```

Para validar:

```python
import hmac, hashlib, time

def verify(secret, body_bytes, header):
    parts = dict(p.split("=", 1) for p in header.split(","))
    t, v1 = int(parts["t"]), parts["v1"]
    if abs(time.time() - t) > 300:
        return False  # rechaza eventos > 5 min de antigüedad
    payload = f"{t}.".encode() + body_bytes
    expected = hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, v1)
```

Body del evento (mismo shape que `/v1/events`):

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

### Reintentos

Si tu endpoint no responde 2xx, reintentamos: 1m → 5m → 30m → 2h → 8h →
24h. Después marcamos el delivery como `dead`. Asegúrate de responder
rápido (< 5s) y devolver 2xx aunque proceses async.

---

## GET /v1/events — feed polling (alternativa a webhooks)

Si prefieres no exponer un endpoint para webhooks, puedes pollear esta
ruta cada N segundos.

```bash
curl "https://sms.aipanel.cl/v1/events?types=sms.delivered,sms.undelivered&limit=100" \
  -H "X-API-Key: sk_live_..."
```

Filtros: `types` (CSV), `from`, `to` (RFC3339), `limit` (max 500). Usa
`next_cursor` para paginar — guarda el último cursor que procesaste y
mándalo en la siguiente request con `&cursor=<value>` para no duplicar.

---

## Ejemplo Python — sistema de facturación

```python
import requests
from typing import Optional

API_KEY = "sk_live_..."  # vault, no en código

def enviar_sms(sender: str, to: str, text: str,
               client_ref: Optional[str] = None) -> dict:
    r = requests.post(
        "https://sms.aipanel.cl/v1/sms",
        headers={"X-API-Key": API_KEY},
        json={
            "sender": sender, "to": to, "text": text,
            "client_ref": client_ref,
        },
        timeout=10,
    )
    if r.status_code == 202:
        return r.json()
    if r.status_code == 409:
        # ya enviado antes con ese client_ref; idempotente.
        return {"status": "duplicate", "client_ref": client_ref}
    if r.status_code == 429:
        # rate-limited o quota; respeta Retry-After.
        retry = int(r.headers.get("Retry-After", "60"))
        raise RuntimeError(f"rate limited, retry in {retry}s")
    r.raise_for_status()
    return r.json()

# uso
r = enviar_sms(
    sender="Segtelco",
    to="+56985917376",
    text=f"Estimado {nombre}: su factura {factura} emitida el {fecha} "
         f"por ${monto:,} vence en {dias} días.",
    client_ref=f"factura-{factura}-recordatorio-1",
)
print(r["id"], r["status"])
```

---

## Buenas prácticas

- **Siempre usa `client_ref`**: si tu sistema reintenta (timeout, fallo
  parcial), el segundo intento NO duplica el SMS. Patrón:
  `{tipo}-{id-recurso}-{intento}` (e.g. `factura-123-recordatorio-1`).
- **Captura `429 daily_quota_exceeded`** y alerta a tu equipo de monitoreo
  — significa que tu sistema está enviando más de lo previsto (o un loop).
- **Sender allow-list**: configurada por tu superadmin. Si tu app necesita
  cambiar de sender ID, pídeselo en lugar de hard-codear varios.
- **Revoca la key inmediatamente** si sospechas de filtración. La pasarela
  no avisa cuando una key se usa desde una IP nueva — es tu responsabilidad
  rotarla.
- **Usa webhooks para eventos**, no polling continuo: te ahorra latencia
  y carga.
