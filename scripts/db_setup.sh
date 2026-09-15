#!/bin/bash
# Создание таблиц режимов работы мотора и их наполнение.
# Использование: ./scripts/db_setup.sh          (миграция + наполнение)
#                ./scripts/db_setup.sh --reset  (сначала удалить таблицы)
set -e

CONTAINER="ebike_postgres"
PSQL="docker exec -i $CONTAINER psql -v ON_ERROR_STOP=1 -U ebike -d ebike_battery"

if [ "$1" = "--reset" ]; then
    $PSQL -c "DROP TABLE IF EXISTS motor_mode_likes, motor_modes, riders;"
fi

go run ./cmd/migrate
$PSQL < db/seed.sql
$PSQL -c "SELECT id, mode_name, status, consumption_wh_per_km FROM motor_modes ORDER BY id;"
