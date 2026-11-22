#!/bin/sh
set -e

until pg_isready -d "$DATABASE_URL" -q; do
  echo "等待資料庫…"
  sleep 2
done

if [ "$1" = "serve" ]; then
  dashboardtw migrate up
  components=$(psql "$DATABASE_URL" -Atc "SELECT count(*) FROM app.component")
  if [ "$components" = "0" ]; then
    dashboardtw admin import
  fi

  if [ "${LOAD_SAMPLE_DATA:-true}" = "true" ] && [ -f /sample/sample.sql.gz ]; then
    loaded=$(psql "$DATABASE_URL" -Atc "SELECT count(*) FROM ref.aqi_site")
    if [ "$loaded" = "0" ]; then
      echo "載入範例資料…"
      gunzip -c /sample/sample.sql.gz | psql "$DATABASE_URL" -q -v ON_ERROR_STOP=1 >/dev/null
    fi
  fi
fi

exec dashboardtw "$@"
