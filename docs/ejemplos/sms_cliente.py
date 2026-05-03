"""
Cliente Python para la pasarela SMS — caso real: sistema de facturación
que envía recordatorios de vencimiento.

Uso:
    export SMS_API_KEY="sk_live_..."
    python3 sms_cliente.py

Requiere: requests (pip install requests)

El módulo expone una función `enviar_sms()` y un `__main__` con un
ejemplo concreto. Copia las dos primeras secciones a tu app.
"""

import os
import time
import logging
import requests
from typing import Optional

# ──────────────────────────────────────────────────────────────────────
# Configuración
# ──────────────────────────────────────────────────────────────────────

BASE_URL = os.environ.get("SMS_BASE_URL", "https://sms.aipanel.cl")
API_KEY = os.environ["SMS_API_KEY"]  # falla rápido si no está en env

DEFAULT_TIMEOUT = 10  # segundos para HTTP
MAX_RETRIES = 3       # reintentos cuando es razonable

log = logging.getLogger("sms")


# ──────────────────────────────────────────────────────────────────────
# Cliente — copia esto a tu app
# ──────────────────────────────────────────────────────────────────────

class SMSError(Exception):
    """Error no recuperable del envío. El campo .code mapea al
    error.code del response (e.g. 'sender_not_allowed')."""
    def __init__(self, code: str, message: str, request_id: str = ""):
        self.code = code
        self.request_id = request_id
        super().__init__(f"[{code}] {message}" + (f" (req={request_id})" if request_id else ""))


def enviar_sms(
    sender: str,
    to: str,
    text: str,
    client_ref: Optional[str] = None,
) -> dict:
    """
    Envía un SMS y devuelve el cuerpo de la respuesta. Reintenta
    automáticamente en 429 respetando Retry-After. Eleva SMSError en
    fallos no recuperables.

    Idempotencia: pasa siempre `client_ref`. Si el segundo intento llega
    con el mismo, esta función devuelve {"status": "duplicate", ...}
    sin propagar excepción — el primer envío ya fue procesado.
    """
    payload = {"sender": sender, "to": to, "text": text}
    if client_ref:
        payload["client_ref"] = client_ref

    for attempt in range(1, MAX_RETRIES + 1):
        r = requests.post(
            f"{BASE_URL}/v1/sms",
            headers={"X-API-Key": API_KEY, "Content-Type": "application/json"},
            json=payload,
            timeout=DEFAULT_TIMEOUT,
        )
        request_id = r.headers.get("X-Request-Id", "")

        # Cuota disponible (logueable; el cliente puede alertar a 80%).
        used = r.headers.get("X-Daily-Quota-Used")
        limit = r.headers.get("X-Daily-Quota-Limit")
        if used and limit:
            log.debug("quota %s/%s", used, limit)

        if r.status_code == 202:
            return r.json()

        if r.status_code == 409:
            # Idempotencia funcionando — no es error.
            return {"status": "duplicate", "client_ref": client_ref, "id": None}

        if r.status_code == 429:
            wait = int(r.headers.get("Retry-After", "5"))
            wait = min(wait, 60)  # cap; en quota_exceeded podría ser horas
            err = r.json().get("error", {})
            if err.get("code") == "daily_quota_exceeded":
                # No tiene sentido reintentar: vence al día siguiente.
                raise SMSError("daily_quota_exceeded", err.get("message", ""), request_id)
            log.warning("rate_limited; sleeping %ss (attempt %d/%d)", wait, attempt, MAX_RETRIES)
            time.sleep(wait)
            continue

        # Errores no recuperables (400, 401, 403, etc.)
        try:
            err = r.json().get("error", {})
            raise SMSError(err.get("code", "unknown"), err.get("message", r.text), request_id)
        except ValueError:
            raise SMSError("unknown", r.text, request_id)

    raise SMSError("max_retries_exceeded", f"failed after {MAX_RETRIES} attempts")


def consultar_estado(message_id: str) -> dict:
    """GET /v1/sms/{id} — útil para polling cuando no usas webhooks."""
    r = requests.get(
        f"{BASE_URL}/v1/sms/{message_id}",
        headers={"X-API-Key": API_KEY},
        timeout=DEFAULT_TIMEOUT,
    )
    r.raise_for_status()
    return r.json()


# ──────────────────────────────────────────────────────────────────────
# Ejemplo de uso real: notificar facturas que vencen
# ──────────────────────────────────────────────────────────────────────

def notificar_factura(cliente_nombre: str, cliente_celular: str,
                      factura_numero: str, fecha_emision: str,
                      monto: int, dias_para_vencer: int,
                      recordatorio_n: int = 1) -> dict:
    """
    Caso de uso real: facturación enviando recordatorio de vencimiento.

    `recordatorio_n` permite mandar 3 recordatorios por factura sin
    duplicar — cada uno con un client_ref distinto:
        factura-1234-r1 (3 días antes)
        factura-1234-r2 (1 día antes)
        factura-1234-r3 (día del vencimiento)
    """
    texto = (
        f"Estimado {cliente_nombre}: su factura {factura_numero} "
        f"emitida el {fecha_emision} por ${monto:,} "
        f"vence en {dias_para_vencer} días."
    )
    return enviar_sms(
        sender="Segtelco",
        to=cliente_celular,
        text=texto,
        client_ref=f"factura-{factura_numero}-r{recordatorio_n}",
    )


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")

    # Caso 1: notificar una factura
    res = notificar_factura(
        cliente_nombre="Rodrigo Quiroga",
        cliente_celular="+56985917376",
        factura_numero="1234",
        fecha_emision="22-01-2026",
        monto=200000,
        dias_para_vencer=5,
        recordatorio_n=1,
    )
    print("Enviado:", res)

    if res.get("id"):
        print("\nPolling estado durante 30s...")
        for _ in range(15):
            s = consultar_estado(res["id"])
            print(f"  status={s['status']} attempts={s.get('attempts',0)}")
            if s["status"] in ("delivered", "undelivered", "rejected", "failed"):
                break
            time.sleep(2)

    # Caso 2: idempotencia — reintentar el mismo envío no duplica
    res2 = notificar_factura(
        cliente_nombre="Rodrigo Quiroga",
        cliente_celular="+56985917376",
        factura_numero="1234",
        fecha_emision="22-01-2026",
        monto=200000,
        dias_para_vencer=5,
        recordatorio_n=1,  # mismo recordatorio_n → mismo client_ref
    )
    print("\nReintento (idempotente):", res2)  # → {"status": "duplicate", ...}
