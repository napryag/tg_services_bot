#!/bin/bash
set -e

usage() {
  cat <<EOF
Использование: $0 [-d postgres|redis]

  -d postgres   Развернуть PostgreSQL и применить Liquibase (по умолчанию)
  -h            Показать это сообщение
EOF
  exit 1
}

# --- Парсинг аргументов ---
DEPLOY="postgres"  # значение по умолчанию

while getopts ":d:h" opt; do
  case $opt in
    d)
      case "${OPTARG}" in
        postgres)
          DEPLOY="${OPTARG}"
          ;;
        *)
          echo "Неизвестный аргумент для -d: ${OPTARG}"
          usage
          ;;
      esac
      ;;
    h|*)
      usage
      ;;
  esac
done

if [ ! -f .env ]; then
  echo "Файл .env не найден. Пожалуйста, создайте его в корне проекта."
  exit 1
fi

# Подгружаем переменные окружения
set -a
source .env
set +a

# --- Разворачивание сервисов ---
# Поддержим и docker compose v2, и старый docker-compose
if command -v docker &>/dev/null && docker compose version &>/dev/null; then
  DC="docker compose"
else
  DC="docker-compose"
fi

trap 'echo "❌ Ошибка на строке $LINENO"; exit 1' ERR

case "$DEPLOY" in
  postgres)
    echo "▶️  Запускаем PostgreSQL через $DC ..."
    $DC up -d db

    echo "⏳ Ожидание готовности Postgres..."
    # Попытка №1: если есть healthcheck — ждём его
    CID="$($DC ps -q db)"
    if [[ -n "$CID" ]]; then
      # если health отсутствует, inspect вернёт пусто — перейдём к pg_isready
      for _ in {1..60}; do
        status="$(docker inspect -f '{{.State.Health.Status}}' "$CID" 2>/dev/null || true)"
        if [[ "$status" == "healthy" ]]; then
          echo "✅ Postgres healthy"; break
        fi
        # если статуса нет — проверим pg_isready внутри контейнера
        if docker exec "$CID" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
          echo "✅ Postgres готов (pg_isready)"; break
        fi
        sleep 1
      done
    fi

    echo "▶️  Применяем Liquibase миграции..."

    if command -v liquibase >/dev/null 2>&1; then
      liquibase \
        --changeLogFile="$CHANGELOG_FILE" \
        --url="jdbc:postgresql://$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable" \
        --username="$POSTGRES_USER" \
        --password="$POSTGRES_PASSWORD" \
        update
    else
      echo "ℹ️  liquibase не найден в PATH. Установите CLI или запустите через Docker образ:"
      echo "    docker run --rm \\"
      echo "      -v \"\$PWD/db/liquibase:/liquibase/changelog\" \\"
      echo "      --network host \\"
      echo "      liquibase/liquibase \\"
      echo "      --changeLogFile=/liquibase/changelog/changelog-root.yaml \\"
      echo "      --url=\"jdbc:postgresql://$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable\" \\"
      echo "      --username=\"$POSTGRES_USER\" --password=\"$POSTGRES_PASSWORD\" \\"
      echo "      update"
      exit 1
    fi

    echo "✅ Liquibase update завершён."      
    ;;

  *)
    # теоретически сюда не попадём
    echo "Неверный параметр DEPLOY: $DEPLOY"
    exit 1
    ;;
esac

echo "Инициализация завершена."
