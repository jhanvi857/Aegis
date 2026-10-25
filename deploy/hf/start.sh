#!/bin/sh
set -eu

if [ -z "${AEGIS_BOOTSTRAP_ADMIN_PASSWORD:-}" ]; then
  echo "AEGIS_BOOTSTRAP_ADMIN_PASSWORD must be provided as a Space secret" >&2
  exit 1
fi

exec /usr/bin/supervisord -c /etc/supervisor/conf.d/aegis.conf
