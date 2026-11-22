# ImageAnalyzerS3

## Локальный запуск API

Понадобятся Docker с Compose, Goose и OpenSSL. Команды выполняются из корня проекта.

1. Создайте `.env` из примера.

   ```sh
   cp -n .env.example .env
   ```

   Замените `CHANGE_ME` в `.env`. Для пароля PostgreSQL и секрета Garage можно использовать `openssl rand -hex 32`, а для ID ключа Garage — `printf 'GK%s\n' "$(openssl rand -hex 16)"`.

2. Если `garage.toml` ещё нет, создайте его.

   ```sh
   cat > garage.toml <<EOF
   metadata_dir = "/tmp/meta"
   data_dir = "/tmp/data"
   db_engine = "sqlite"
   replication_factor = 1
   rpc_bind_addr = "[::]:3901"
   rpc_public_addr = "127.0.0.1:3901"
   rpc_secret = "$(openssl rand -hex 32)"

   [s3_api]
   s3_region = "garage"
   api_bind_addr = "[::]:3900"
   EOF
   ```

3. Запустите PostgreSQL и Garage, примените миграции и соберите API.

   ```sh
   docker compose up -d --wait postgres garage
   goose up
   docker compose up -d --build --wait api
   ```

API доступен на `http://localhost:8080` по умолчанию. Порт хоста задаёт `APP_PORT` в `.env`. Compose подключает `config/example.yaml` и передаёт `POSTGRES_DSN`, `GARAGE_BASE_ENDPOINT` и `GARAGE_BUCKET` для контейнерной сети. [Контракт загрузки](docs/architecture.md#post-apianalyze).

### Запуск на хосте

Понадобится Go 1.27.4 или новее. После запуска PostgreSQL, Garage и применения миграций скопируйте `config/example.yaml` в `config/local.yaml` и заполните `postgres_dsn` и параметры бакета по настройкам из `.env`.

```sh
(
  . ./.env
  AWS_ACCESS_KEY_ID="$GARAGE_DEFAULT_ACCESS_KEY" \
    AWS_SECRET_ACCESS_KEY="$GARAGE_DEFAULT_SECRET_KEY" \
    go run ./services/api/cmd -config config/local.yaml
)
```

Для GoLand задайте те же переменные окружения и аргумент `-config config/local.yaml` в конфигурации запуска.
