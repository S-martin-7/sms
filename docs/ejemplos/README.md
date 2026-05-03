# Ejemplos ejecutables

Scripts listos para copiar y adaptar a tu sistema.

## Cliente — enviar SMS

| Archivo | Lenguaje | Caso |
|---|---|---|
| [`sms_cliente.py`](./sms_cliente.py) | Python (requests) | Sistema de facturación con idempotencia + retry en 429 |
| [`sms_cliente.js`](./sms_cliente.js) | Node.js (fetch nativo, sin deps) | Idem, ESM-friendly |
| [`sms_cliente.php`](./sms_cliente.php) | PHP (curl) | Idem |
| [`sms_cliente.sh`](./sms_cliente.sh) | Bash + curl + jq | Útil para scripts de monitoreo / cron |

Todos esperan `SMS_API_KEY` en el entorno y muestran el patrón
recomendado: `client_ref` para idempotencia + reintento automático en
429 respetando `Retry-After`.

## Servidor — recibir webhooks

| Archivo | Stack | Notas |
|---|---|---|
| [`webhook_server.py`](./webhook_server.py) | Flask | Verifica HMAC contra raw bytes del body |
| [`webhook_server.js`](./webhook_server.js) | Express | Captura `rawBody` antes de `express.json()` parsing |

Ambos esperan `SMS_WEBHOOK_SECRET` en el entorno (el secret devuelto
por `POST /v1/webhooks` cuando registraste el endpoint).

## Probar localmente sin webhook público

Cuando aún no tienes URL HTTPS pública, [ngrok](https://ngrok.com/) es
lo más rápido:

```bash
# Terminal A: tu servidor de webhook
SMS_WEBHOOK_SECRET="wh_..." python3 webhook_server.py

# Terminal B: tunel público
ngrok http 5000

# Registra el endpoint con la URL de ngrok:
curl -X POST https://sms.aipanel.cl/v1/webhooks \
  -H "X-API-Key: $SMS_API_KEY" \
  -d '{"url":"https://abc123.ngrok.io/sms-webhook","events":["sms.delivered"]}'

# Manda un SMS y observa el webhook llegar firmado.
```

Cuando termines, **revoca el webhook** (no dejes el ngrok URL apuntado
a algo que ya no controlas):

```bash
curl -X DELETE https://sms.aipanel.cl/v1/webhooks/<id> \
  -H "X-API-Key: $SMS_API_KEY"
```
