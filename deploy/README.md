# Deploy (portable)

Everything needed to run sms-server on a fresh Linux VPS (Ubuntu 22+/Debian 12+, Postgres 14+, Go 1.24+).

## One-time bootstrap on a new VPS

```bash
# 1. Install tooling
apt install -y postgresql nginx certbot python3-certbot-nginx git
# Install Go 1.24 to /usr/local/go (out of scope — use official tarball).

# 2. Clone the code (bare-repo + post-receive hook pattern)
mkdir -p /root/repos /root/sms-src /root/sms
git init --bare /root/repos/sms.git
cp deploy/post-receive.sample /root/repos/sms.git/hooks/post-receive   # or write by hand
chmod +x /root/repos/sms.git/hooks/post-receive
# On dev: git remote add vps root@<ip>:/root/repos/sms.git && git push vps main

# 3. Create Postgres role + databases
SMS_DB_PASSWORD="$(openssl rand -hex 24)" bash deploy/postgres-init.sh

# 4. Create /root/sms/.env from the template and fill DATABASE_URL + secrets
cp .env.example /root/sms/.env
openssl rand -hex 32   # paste into JWT_SECRET
openssl rand -hex 32   # paste into API_KEY_PEPPER
chmod 600 /root/sms/.env

# 5. Install the systemd unit and start
cp deploy/systemd/sms-server.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now sms-server.service

# 6. nginx vhost (after DNS for the subdomain resolves to the VPS)
cp deploy/nginx/sms.conf.example /etc/nginx/sites-available/sms
sed -i "s/__DOMAIN__/sms.example.com/g" /etc/nginx/sites-available/sms
ln -s /etc/nginx/sites-available/sms /etc/nginx/sites-enabled/sms
certbot --nginx -d sms.example.com    # issues cert + reloads nginx
```

## Redeploy after code changes

```bash
# On dev laptop:
git push vps main     # auto-checkout into /root/sms-src/

# On VPS:
bash /root/sms-src/deploy/deploy.sh
```

## Files

| File | Purpose |
|------|---------|
| `postgres-init.sh` | Create `sms` role + `sms` / `sms_test` DBs. Idempotent. |
| `systemd/sms-server.service` | systemd unit. Logs to journald. |
| `nginx/sms.conf.example` | Reverse-proxy template with TLS + rate-limit. |
| `deploy.sh` | Build binaries + migrate + restart. Atomic swap. |
