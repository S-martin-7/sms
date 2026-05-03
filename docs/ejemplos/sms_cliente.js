/**
 * Cliente Node.js para la pasarela SMS — caso real: notificación de
 * facturas. Usa fetch nativo (Node ≥ 18). Sin dependencias.
 *
 * Uso:
 *   export SMS_API_KEY="sk_live_..."
 *   node sms_cliente.js
 */

const BASE_URL = process.env.SMS_BASE_URL || 'https://sms.aipanel.cl'
const API_KEY = process.env.SMS_API_KEY
if (!API_KEY) {
  console.error('Falta SMS_API_KEY en el entorno')
  process.exit(1)
}

const TIMEOUT_MS = 10_000
const MAX_RETRIES = 3

class SMSError extends Error {
  constructor(code, message, requestId = '') {
    super(`[${code}] ${message}${requestId ? ` (req=${requestId})` : ''}`)
    this.code = code
    this.requestId = requestId
  }
}

/**
 * Envía un SMS. Reintenta automáticamente en 429 (rate_limited)
 * respetando Retry-After. Devuelve el cuerpo JSON del 202.
 *
 * `clientRef` activa idempotencia — si reintentas con el mismo, el
 * segundo intento devuelve {status:"duplicate"} en vez de duplicar.
 */
async function enviarSMS({ sender, to, text, clientRef }) {
  const payload = { sender, to, text }
  if (clientRef) payload.client_ref = clientRef

  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    const ctrl = new AbortController()
    const t = setTimeout(() => ctrl.abort(), TIMEOUT_MS)
    let res
    try {
      res = await fetch(`${BASE_URL}/v1/sms`, {
        method: 'POST',
        headers: {
          'X-API-Key': API_KEY,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
        signal: ctrl.signal,
      })
    } finally {
      clearTimeout(t)
    }
    const requestId = res.headers.get('x-request-id') || ''
    const used = res.headers.get('x-daily-quota-used')
    const limit = res.headers.get('x-daily-quota-limit')
    if (used && limit) console.debug(`quota ${used}/${limit}`)

    if (res.status === 202) return res.json()
    if (res.status === 409) {
      return { status: 'duplicate', client_ref: clientRef, id: null }
    }
    if (res.status === 429) {
      const body = await res.json().catch(() => ({}))
      const code = body?.error?.code
      if (code === 'daily_quota_exceeded') {
        // No reintentes — la cuota se libera al día siguiente.
        throw new SMSError('daily_quota_exceeded', body.error?.message || '', requestId)
      }
      const wait = Math.min(parseInt(res.headers.get('retry-after') || '5', 10), 60)
      console.warn(`rate_limited; sleeping ${wait}s (attempt ${attempt}/${MAX_RETRIES})`)
      await new Promise((r) => setTimeout(r, wait * 1000))
      continue
    }

    // No recuperable.
    let body
    try { body = await res.json() } catch { body = { error: { code: 'unknown', message: await res.text() } } }
    throw new SMSError(body.error?.code || 'unknown', body.error?.message || '', requestId)
  }
  throw new SMSError('max_retries_exceeded', `failed after ${MAX_RETRIES} attempts`)
}

async function consultarEstado(id) {
  const res = await fetch(`${BASE_URL}/v1/sms/${id}`, {
    headers: { 'X-API-Key': API_KEY },
  })
  if (!res.ok) throw new Error(`status ${res.status}`)
  return res.json()
}

// ──────────────────────────────────────────────────────────────────────
// Caso de uso real: notificar factura que vence
// ──────────────────────────────────────────────────────────────────────

async function notificarFactura({
  clienteNombre,
  clienteCelular,
  facturaNumero,
  fechaEmision,
  monto,
  diasParaVencer,
  recordatorioN = 1,
}) {
  const texto =
    `Estimado ${clienteNombre}: su factura ${facturaNumero} ` +
    `emitida el ${fechaEmision} por $${monto.toLocaleString('es-CL')} ` +
    `vence en ${diasParaVencer} días.`

  return enviarSMS({
    sender: 'Segtelco',
    to: clienteCelular,
    text: texto,
    clientRef: `factura-${facturaNumero}-r${recordatorioN}`,
  })
}

// ──────────────────────────────────────────────────────────────────────
// Entry point
// ──────────────────────────────────────────────────────────────────────

;(async () => {
  const res = await notificarFactura({
    clienteNombre: 'Rodrigo Quiroga',
    clienteCelular: '+56985917376',
    facturaNumero: '1234',
    fechaEmision: '22-01-2026',
    monto: 200000,
    diasParaVencer: 5,
  })
  console.log('Enviado:', res)

  if (res.id) {
    console.log('\nPolling estado...')
    for (let i = 0; i < 15; i++) {
      const s = await consultarEstado(res.id)
      console.log(`  status=${s.status}`)
      if (['delivered','undelivered','rejected','failed'].includes(s.status)) break
      await new Promise((r) => setTimeout(r, 2000))
    }
  }
})().catch((e) => {
  console.error('FAILED:', e)
  process.exit(1)
})
