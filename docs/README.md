# Documentación

## Para clientes integrando la pasarela (`/v1/*`)

- **[api-public.md](./api-public.md)** — referencia completa de la API
  pública: endpoints, auth, errores, webhooks, headers, ejemplos por
  lenguaje, casos de uso (facturación, monitoreo, OTP, recordatorios).
- **[ejemplos/](./ejemplos/)** — scripts ejecutables listos para copiar:
  - Python / Node.js / PHP / Bash para enviar SMS con retry + idempotencia.
  - Flask / Express para recibir webhooks con verificación HMAC.

## Para superadmins operando la pasarela

- **[onboarding-tenant.md](./onboarding-tenant.md)** — guía paso a paso
  para incorporar un sistema cliente: crear tenant, cuota, sender
  allow-list, emitir keys, monitoreo, suspensión, recuperación 2FA.

## Internas / referencia

- **[SECURITY.md](./SECURITY.md)** — modelo de amenazas, decisiones de
  seguridad, política de incidentes.
- **[horisen-dlr-codes.md](./horisen-dlr-codes.md)** — mapeo de códigos
  de DLR del proveedor (cuándo es retryable vs. permanente).
- **[PLAN.md](./PLAN.md)** — plan original de arquitectura. Documento
  histórico — el estado real del producto está reflejado en los puntos
  de arriba.
