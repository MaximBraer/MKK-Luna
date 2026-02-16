#!/usr/bin/env sh
set -e

if [ -z "${CONFIG_PATH}" ]; then
  export CONFIG_PATH=/app/config/local.yaml
fi

/app/migrator -cmd=up
exec /app/api
