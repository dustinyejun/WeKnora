#!/bin/sh
# Tunnels ONLY the share gateway, never the full WeKnora site.
set -e
if [ -z "$CPOLAR_AUTHTOKEN" ]; then
  echo "CPOLAR_AUTHTOKEN 未设置：请在 .env 中填写 cpolar 认证令牌" >&2
  sleep 3600; exit 1
fi
cpolar authtoken "$CPOLAR_AUTHTOKEN" >/dev/null
# CPOLAR_SUBDOMAIN needs a paid reserved subdomain; free plans get a random one.
if [ -n "$CPOLAR_SUBDOMAIN" ]; then
  exec cpolar http -subdomain="$CPOLAR_SUBDOMAIN" -region="${CPOLAR_REGION:-cn}" -log=stdout -log-level=INFO http://share-gateway:80
fi
exec cpolar http -region="${CPOLAR_REGION:-cn}" -log=stdout -log-level=INFO http://share-gateway:80
