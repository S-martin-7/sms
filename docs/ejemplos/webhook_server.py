"""
Servidor Flask que recibe webhooks de la pasarela SMS, verifica la
firma HMAC-SHA256 y procesa los eventos.

Uso:
    pip install flask
    export SMS_WEBHOOK_SECRET="wh_..."   # el secret devuelto por POST /v1/webhooks
    python3 webhook_server.py            # escucha en :5000

Para registrar este endpoint:
    POST https://sms.aipanel.cl/v1/webhooks
      X-API-Key: sk_live_...
      {
        "url": "https://tu-dominio.com/sms-webhook",
        "events": ["sms.delivered","sms.undelivered","sms.rejected","sms.inbound"]
      }
"""

import hmac
import hashlib
import os
import time
import logging

from flask import Flask, request, abort

WEBHOOK_SECRET = os.environ["SMS_WEBHOOK_SECRET"]
MAX_AGE_SECONDS = 300  # eventos > 5 min se rechazan (replay protection)

app = Flask(__name__)
log = logging.getLogger(__name__)


def verify_signature(secret: str, body_bytes: bytes, sig_header: str) -> bool:
    """
    Valida X-Signature: t=<unix>,v1=<hex>.

    Critical: usa el body ORIGINAL (raw bytes), no el JSON re-serializado
    de tu framework — un solo espacio extra rompe la firma.
    """
    try:
        parts = dict(p.split("=", 1) for p in sig_header.split(","))
        t = int(parts["t"])
        v1 = parts["v1"]
    except (KeyError, ValueError):
        return False

    # Replay protection.
    if abs(time.time() - t) > MAX_AGE_SECONDS:
        log.warning("rejecting stale event (age=%ss)", int(time.time() - t))
        return False

    payload = f"{t}.".encode() + body_bytes
    expected = hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, v1)


@app.route("/sms-webhook", methods=["POST"])
def sms_webhook():
    sig = request.headers.get("X-Signature", "")
    event_id = request.headers.get("X-Event-Id", "")
    event_type = request.headers.get("X-Event-Type", "")

    raw = request.get_data()  # ← raw bytes, no .json()
    if not verify_signature(WEBHOOK_SECRET, raw, sig):
        log.warning("bad signature on event %s", event_id)
        abort(401)

    payload = request.get_json(force=True)
    log.info("event %s %s message_id=%s", event_id, event_type,
             payload.get("message_id"))

    # Delegar a handlers — encolar async si tu lógica es pesada.
    handlers = {
        "sms.delivered": handle_delivered,
        "sms.undelivered": handle_undelivered,
        "sms.rejected": handle_rejected,
        "sms.inbound": handle_inbound,
    }
    h = handlers.get(event_type)
    if h:
        h(event_id, payload)
    else:
        log.warning("unhandled event type %s", event_type)

    # Importante: 200 dentro de los 30s aunque proceses async.
    return "", 200


def handle_delivered(event_id: str, p: dict) -> None:
    """SMS confirmado entregado al teléfono. Actualiza tu estado interno."""
    print(f"✅ DELIVERED message_id={p['message_id']} num_parts={p['num_parts']}")
    # tu_db.update_sms_status(p["message_id"], "delivered")


def handle_undelivered(event_id: str, p: dict) -> None:
    """No se pudo entregar (apagado, fuera de cobertura, número inválido)."""
    print(f"❌ UNDELIVERED message_id={p['message_id']}")
    # Considera reintentar más tarde o marcar el contacto como inválido.


def handle_rejected(event_id: str, p: dict) -> None:
    """Rechazado por carrier o validación. No se va a reintentar."""
    print(f"⛔ REJECTED message_id={p['message_id']}")


def handle_inbound(event_id: str, p: dict) -> None:
    """SMS entrante (MO) — usuario respondió a uno tuyo o envió a tu número."""
    print(f"📥 INBOUND from={p['src']} text={p['text']!r}")
    # Si tu número es para opt-out automático, parsea aquí ('STOP', 'BAJA', etc.)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO,
                        format="%(asctime)s %(levelname)s %(message)s")
    # En producción sirve esto detrás de nginx/gunicorn — Flask dev server
    # NO sirve para tráfico real. Importante: NO actives un proxy que
    # modifique el body antes de llegar acá (eso rompe la firma).
    app.run(host="0.0.0.0", port=5000)
