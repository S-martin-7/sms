#!/usr/bin/env bash
# Rebuild the sms-server + smsctl binaries and restart the systemd unit.
# Run this on the VPS after a `git push vps main`. Portable: no paths
# hardcoded outside $SMS_ROOT / $SMS_SRC.

set -euo pipefail

SMS_SRC="${SMS_SRC:-/root/sms-src}"
SMS_ROOT="${SMS_ROOT:-/root/sms}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
UNIT="${UNIT:-sms-server.service}"

cd "$SMS_SRC"

echo ">> Building sms-server + smsctl"
"$GO_BIN" build -o "$SMS_ROOT/sms-server.new" ./cmd/server
"$GO_BIN" build -o "$SMS_ROOT/smsctl.new"     ./cmd/smsctl

# Atomic swap so a failed build doesn't leave a broken binary.
mv "$SMS_ROOT/sms-server.new" "$SMS_ROOT/sms-server"
mv "$SMS_ROOT/smsctl.new"     "$SMS_ROOT/smsctl"

echo ">> Applying any new migrations"
( cd "$SMS_SRC" && set -a && . "$SMS_ROOT/.env" && set +a && "$SMS_ROOT/smsctl" migrate up )

echo ">> Restarting $UNIT"
systemctl restart "$UNIT"
systemctl --no-pager status "$UNIT" | head -10

echo ">> Done."
