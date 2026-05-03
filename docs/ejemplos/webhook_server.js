/**
 * Servidor Express que recibe webhooks de la pasarela SMS y verifica
 * la firma HMAC-SHA256.
 *
 * Uso:
 *   npm install express
 *   export SMS_WEBHOOK_SECRET="wh_..."
 *   node webhook_server.js
 *
 * IMPORTANTE: Express por defecto re-serializa el body al usar
 * express.json(); eso rompe la firma. Aquí guardamos el raw bytes
 * antes de parsear para verificar contra ese.
 */

const express = require('express')
const crypto = require('crypto')

const SECRET = process.env.SMS_WEBHOOK_SECRET
if (!SECRET) {
  console.error('Falta SMS_WEBHOOK_SECRET en el entorno')
  process.exit(1)
}
const MAX_AGE_S = 300

const app = express()

// Capturamos el rawBody antes del JSON parsing — necesario para HMAC.
app.use(express.json({
  verify: (req, _res, buf) => { req.rawBody = buf },
}))

function verifySignature(secret, rawBody, sigHeader) {
  if (!sigHeader) return false
  const parts = Object.fromEntries(
    sigHeader.split(',').map((p) => p.split('=', 2))
  )
  const t = parseInt(parts.t || '0', 10)
  const v1 = parts.v1 || ''
  if (!t || !v1) return false

  // Replay protection.
  if (Math.abs(Date.now() / 1000 - t) > MAX_AGE_S) return false

  const payload = Buffer.concat([Buffer.from(`${t}.`), rawBody])
  const expected = crypto.createHmac('sha256', secret).update(payload).digest('hex')
  // Compare timing-safe.
  const a = Buffer.from(expected, 'hex')
  const b = Buffer.from(v1, 'hex')
  if (a.length !== b.length) return false
  return crypto.timingSafeEqual(a, b)
}

app.post('/sms-webhook', (req, res) => {
  const sig = req.headers['x-signature']
  const eventId = req.headers['x-event-id']
  const eventType = req.headers['x-event-type']

  if (!verifySignature(SECRET, req.rawBody, sig)) {
    console.warn(`bad signature on event ${eventId}`)
    return res.status(401).end()
  }

  const p = req.body
  console.log(`event ${eventId} ${eventType} message_id=${p.message_id}`)

  // Delegar y responder rápido (no bloquees procesando lógica pesada).
  switch (eventType) {
    case 'sms.delivered':
      console.log(`✅ DELIVERED ${p.message_id} (${p.num_parts} parts)`)
      // db.updateSMSStatus(p.message_id, 'delivered')
      break
    case 'sms.undelivered':
      console.log(`❌ UNDELIVERED ${p.message_id}`)
      break
    case 'sms.rejected':
      console.log(`⛔ REJECTED ${p.message_id}`)
      break
    case 'sms.inbound':
      console.log(`📥 INBOUND from=${p.src} text=${JSON.stringify(p.text)}`)
      break
    default:
      console.warn(`unhandled type ${eventType}`)
  }

  res.status(200).end()
})

const PORT = process.env.PORT || 5000
app.listen(PORT, () => console.log(`webhook server listening on :${PORT}`))
