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
source .env

# --- Разворачивание сервисов ---
echo "Инициализация: docker-compose сервисов..."

case "$DEPLOY" in
  postgres)
    echo "- Запускаем PostgreSQL"
    docker-compose up -d db

    echo "Ожидание запуска PostgreSQL..."
    until docker exec db_postgres pg_isready -U "$POSTGRES_USER" >/dev/null 2>&1; do
      sleep 1
    done

    echo "Накатываем Liquibase..."
    LIQUIBASE_CMD="liquibase"  # предполагается, что liquibase доступен в PATH
    CHANGELOG_FILE="liquibase/changelog.yml"
    $LIQUIBASE_CMD \
      --changeLogFile=$CHANGELOG_FILE \
      --url="jdbc:postgresql://localhost:$POSTGRES_PORT/$POSTGRES_DB" \
      --username=$POSTGRES_USER \
      --password=$POSTGRES_PASSWORD \
      update

    echo "Liquibase update завершён."
    ;;

  *)
    # теоретически сюда не попадём
    echo "Неверный параметр DEPLOY: $DEPLOY"
    exit 1
    ;;
esac

echo "Инициализация завершена."
