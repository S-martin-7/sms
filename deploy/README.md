# Deploy scripts

One-time bootstrap and ongoing deploy helpers for the VPS (Ubuntu 22.04, Postgres 14 native — no Docker).

## `postgres-init.sh`

Creates the `sms` Postgres role and two databases (`sms`, `sms_test`). Idempotent.

```bash
# On the VPS, from /root/sms-src/:
export SMS_DB_PASSWORD='...'     # from /root/sms-src/.env
bash deploy/postgres-init.sh
```
