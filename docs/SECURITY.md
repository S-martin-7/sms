# Postura de seguridad — Pasarela SMS

Documento de referencia operacional. Lista los controles ya en producción
y los pendientes recomendados para hardening adicional.

## Resumen

| Control | Estado |
|---|---|
| TLS extremo a extremo (Let's Encrypt + nginx) | ✅ |
| Aislamiento por tenant (todas las queries filtran por `tenant_id`) | ✅ |
| Bcrypt (cost=12) para passwords admin | ✅ |
| Hash con pepper (sha256-v1) para API keys | ✅ |
| HMAC-SHA256 firmando webhooks salientes | ✅ |
| Rate-limit de login (5/min/IP, sliding window) | ✅ |
| Security headers (CSP, XFO, XCTO, Referrer-Policy, HSTS, Permissions) | ✅ |
| Auditoría de acciones admin (`audit_log` con actor, target, metadata) | ✅ |
| JWT con expiración (default 12h, configurable) | ✅ |
| Postgres con `prepare`d statements vía sqlc (anti-SQLi) | ✅ |
| Body limit en endpoints de import (CSV 5MB, XLSX 10MB) | ✅ |
| Constant-time compare en Basic Auth y firmas | ✅ |
| 2FA admin | ⏳ pendiente |
| Rotación periódica de JWT secret | ⏳ pendiente (manual) |
| Edge rate-limit por IP en `/v1/` (nginx) | ✅ |
| Edge rate-limit per-tenant (token bucket) | ⏳ pendiente |
| Bloqueo de cuenta tras N fallos de login | ⏳ pendiente |
| Hasta-`pwned-password` check | ⏳ pendiente |
| Escaneo periódico con govulncheck | ⏳ pendiente |

## Modelo de amenazas

### Activos protegidos

1. **Crédito Horisen**: el atacante con acceso podría enviar SMS gratis.
2. **Datos de tenants**: contactos, mensajes, números entrantes.
3. **API keys de tenants**: si fugan, alguien puede mandar SMS como ese cliente.
4. **Llaves admin (JWT secret, BCRYPT pepper)**: comprometen toda la app.
5. **Reputación del remitente** (sender ID): spam saliente daña el SamuelOTP.

### Vectores considerados

- **Brute force de admin login** → mitigado con rate-limit por IP.
- **Credential stuffing** → mismo mitigante; pendiente bloqueo por cuenta.
- **SQL injection** → no aplicable, todas las queries van por `pgx` con
  parámetros tipados (sqlc-generated).
- **XSS en dashboard** → React escapa por default; CSP bloquea inline.
- **CSRF en /admin/*** → no aplicable: JWT en `Authorization: Bearer`,
  no cookies.
- **Fuga de webhook secret** → solo se devuelve en el POST de creación,
  nunca en GETs subsecuentes; almacenado en plano (necesario para HMAC).
- **Replay de webhook** → tenant valida `t=<unix>` en `X-Signature` con
  ventana de 5 min.
- **Replay de DLR/MO** → handler dropea malformados con 200 (Horisen no
  reintenta en 4xx) — adecuado, evita amplificación.
- **MITM** → TLS 1.2+ en nginx, HSTS 1 año, certificados Let's Encrypt
  rotando automáticamente.
- **Path traversal en uploads** → no escribimos a disco; solo parseamos.
- **DOS por XLSX gigante** → cap de 10MB en body, parser excelize lazy.
- **Robo de admin con tenant suspendido** → `/v1/sms` valida estado
  (TODO: actualmente no chequea suspendido — ver §pendientes).

## Detalles operacionales

### Rotación de secretos

Los secretos viven en `/root/sms/.env` con `chmod 600`:
- `JWT_SECRET` — rotación implica re-login de todos los admins.
- `API_KEY_PEPPER` — rotación invalidaría todas las llaves emitidas;
  evitar a menos que se sospeche fuga del pepper en sí.
- `HORISEN_PASSWORD`, `HORISEN_OAUTH_CLIENT_SECRET`, `HORISEN_CALLBACK_*`:
  rotar en coordinación con Horisen.

Procedimiento de rotación de JWT_SECRET:
1. Generar nuevo: `openssl rand -hex 32`.
2. Reemplazar en `.env`.
3. `systemctl restart sms-server.service`.
4. Notificar a admins: re-login obligatorio.

### Audit log

Cada acción admin escribe a `audit_log`:
- `actor_id` — admin user id (NULL si vía CLI)
- `action` — verbo dot-notation, p.ej. `tenant.create`, `api_key.revoke`
- `target_type` / `target_id` — entidad afectada
- `metadata` — payload JSON (no incluye secretos en plano)

Consulta histórico:

```bash
ssh root@vps 'set -a; source /root/sms/.env; set +a; psql $DATABASE_URL \
  -c "SELECT created_at, actor_id, action, target_type, target_id FROM audit_log ORDER BY id DESC LIMIT 50"'
```

### Backups

PostgreSQL backups dependen de la política del VPS — verificar con
`/etc/cron.daily/`. Para producción real, se recomienda:
- `pg_dump` diario a almacenamiento offsite (S3/Backblaze).
- Retención 30 días.
- Test de restore mensual.

## Pendientes recomendados

Por orden de impacto, no de esfuerzo:

1. **Bloqueo temporal de cuenta tras N fallos** además del rate-limit por IP.
   El IP rate-limit no protege contra distribuir fuerza bruta entre múltiples
   IPs (botnet). Implementar contador en `admin_users.failed_attempts` con
   reset cada hora.

2. **2FA TOTP** para admins. Usar `github.com/pquerna/otp` y agregar
   `admin_users.totp_secret`. Forzar para `superadmin`.

3. **Validar tenant suspendido en /v1/sms**. Hoy solo suspende el envío vía
   dashboard. Si tienen una llave activa pueden seguir enviando. Verificar
   `tenants.status='active'` en el middleware APIKey.

4. **Per-tenant rate limit en /v1/sms**. Si un tenant se vuelve loco, puede
   saturar nuestra cola. Token bucket en memoria es suficiente para el
   volumen actual.

5. **Webhook secret rotation**. Permitir rotar el secret de un endpoint
   sin re-crearlo, con período de gracia donde ambos firman.

6. **Have-I-Been-Pwned check** vía k-anon API durante creación de admin.
   Bloquea passwords que aparecen en breaches conocidos.

7. **`govulncheck` en CI**. Cuando montemos CI, agregarlo al pipeline
   junto con `go vet` y `staticcheck`.

8. **Vista del audit_log en dashboard** (`/admin/audit-log`). Ya tenemos
   los datos; falta el endpoint + página. Útil para investigar incidentes.

9. **Logs estructurados a SIEM** (Loki/Datadog). Hoy van a journald —
   ok para forensics manual, no para alertas en tiempo real.

10. **DLR amplification check**. Validar que el `custom.msgId` en el DLR
    realmente nos pertenece (UUID generado por nosotros) antes de
    actualizar `messages.status`. Hoy aceptamos cualquier UUID que matchee.

## Contacto y reporte de vulnerabilidades

Para reportar una vulnerabilidad en este sistema, escribir a
`security@aipanel.cl` con detalle reproducible. No publicar antes de 90
días post-fix.
