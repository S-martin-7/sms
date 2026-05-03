#!/usr/bin/env bash
# Cliente bash para la pasarela SMS — útil para scripts y monitoreo.
#
# Uso:
#   export SMS_API_KEY="sk_live_..."
#   ./sms_cliente.sh +56985917376 "Hola Rodrigo, factura 1234 vence en 5 días"
#
# Argumentos:
#   $1 = destinatario E.164 (+569...)
#   $2 = texto del mensaje
#   $3 = client_ref (opcional, recomendado)
#
# Requiere: curl, jq

set -euo pipefail

BASE_URL="${SMS_BASE_URL:-https://sms.aipanel.cl}"
API_KEY="${SMS_API_KEY:?Falta SMS_API_KEY en el entorno}"

TO="${1:?Falta argumento: destinatario}"
TEXT="${2:?Falta argumento: texto}"
CLIENT_REF="${3:-monitor-$(date +%Y%m%d%H%M%S)}"

# Construimos JSON con jq para no preocuparnos por escapes.
PAYLOAD=$(jq -n \
  --arg sender "Segtelco" \
  --arg to "$TO" \
  --arg text "$TEXT" \
  --arg client_ref "$CLIENT_REF" \
  '{sender: $sender, to: $to, text: $text, client_ref: $client_ref}')

# -i muestra headers en stdout, -w status al final, -s suprime barra de progreso.
RESPONSE=$(curl -sS -i -X POST "$BASE_URL/v1/sms" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD")

# Extraer status code (primera línea).
STATUS=$(echo "$RESPONSE" | head -1 | awk '{print $2}')

# Headers (entre primera línea y línea vacía) y body (después de línea vacía).
HEADERS=$(echo "$RESPONSE" | sed -n '/^HTTP/,/^\r$/p')
BODY=$(echo "$RESPONSE" | sed '1,/^\r$/d')
REQUEST_ID=$(echo "$HEADERS" | grep -i '^X-Request-Id:' | awk '{print $2}' | tr -d '\r')
QUOTA_USED=$(echo "$HEADERS" | grep -i '^X-Daily-Quota-Used:' | awk '{print $2}' | tr -d '\r' || true)
QUOTA_LIMIT=$(echo "$HEADERS" | grep -i '^X-Daily-Quota-Limit:' | awk '{print $2}' | tr -d '\r' || true)

case "$STATUS" in
  202)
    echo "$BODY" | jq -r '"OK id=\(.id) status=\(.status) parts=\(.num_parts)"'
    [[ -n "${QUOTA_USED:-}" ]] && echo "Cuota: ${QUOTA_USED}/${QUOTA_LIMIT}" >&2
    exit 0
    ;;
  409)
    # Duplicate client_ref — idempotencia OK.
    echo "DUPLICATE client_ref=$CLIENT_REF (no es error)" >&2
    exit 0
    ;;
  429)
    CODE=$(echo "$BODY" | jq -r '.error.code')
    RETRY=$(echo "$HEADERS" | grep -i '^Retry-After:' | awk '{print $2}' | tr -d '\r')
    echo "RATE_LIMITED code=$CODE retry_after=${RETRY}s" >&2
    exit 2
    ;;
  *)
    echo "FAILED status=$STATUS request_id=$REQUEST_ID" >&2
    echo "$BODY" >&2
    exit 1
    ;;
esac
