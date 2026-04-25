# LogMonitor

Сервис для мониторинга и контроля целостности журналов событий на удалённых серверах.

Проект умеет:

- подключаться к удалённым серверам по SSH;
- автоматически искать журналы событий;
- собирать новые записи из найденных логов;
- сохранять метаданные, строки логов и контрольные значения в хранилище;
- проверять целостность логов и обнаруживать изменения;
- работать как HTTP-сервис и как отдельная CLI-утилита.

> В текущей версии веб-интерфейс ещё не реализован. Для работы доступны HTTP API, Swagger UI и CLI.

## Содержание

1. [Как это работает](#как-это-работает)
2. [Режимы запуска](#режимы-запуска)
3. [Требования](#требования)
4. [Быстрый старт](#быстрый-старт)
5. [Конфигурация](#конфигурация)
6. [Переменные окружения](#переменные-окружения)
7. [CLI-утилита](#cli-утилита)
8. [HTTP API](#http-api)
9. [Docker и Docker Compose](#docker-и-docker-compose)
10. [Хранение данных и миграции](#хранение-данных-и-миграции)
11. [Сборка бинарников](#сборка-бинарников)
12. [Разработка](#разработка)

## Как это работает

Сервис работает по следующей схеме:

1. В конфиге задаются стартовые серверы, параметры SSH, база данных и поведение фоновых задач.
2. При запуске приложение поднимает общее runtime-окружение: репозиторий, SSH-клиент, сервисный слой и runtime-state.
3. В HTTP-режиме дополнительно поднимаются API, очередь job и scheduler.
4. Для каждого сервера можно:
   - обнаружить журналы;
   - собрать новые строки;
   - запустить проверку целостности.
5. Результаты сохраняются в хранилище.
6. Если сервер недоступен или работает нестабильно, приложение обновляет его health-состояние: `active`, `degraded`, `inactive`, `error`.

## Режимы запуска

Проект поддерживает два режима запуска.

| Режим | Значение `LOGMONITOR_APP_MODE` | Точка входа | Назначение |
| --- | --- | --- | --- |
| HTTP | `HTTP` | `./cmd/server` | Долгоживущий сервис с REST API, Swagger, очередью задач и scheduler |
| CLI | `CLI` | `./cmd/cli` | Локальная консольная утилита для ручных операций |

Переменная `LOGMONITOR_APP_MODE` обязательна. Она читается **до загрузки основного YAML-конфига**. Если она не задана или содержит неверное значение, приложение завершится с ошибкой.

Допустимые значения:

- `HTTP`
- `CLI`

## Требования

- Go `1.26+`
- PostgreSQL `13+` для постоянного хранения данных
- Docker и Docker Compose, если планируется запуск в контейнерах
- SSH-доступ к удалённым серверам

## Быстрый старт

### 1. Выберите конфиг

В проекте уже есть готовые примеры:

- [config.local.yaml](/d:/GolangProjects/учёба/ВКР/Проект1/config.local.yaml) — локальный запуск;
- [config.docker.yaml](/d:/GolangProjects/учёба/ВКР/Проект1/config.docker.yaml) — запуск в Docker;
- [config.yaml.example](/d:/GolangProjects/учёба/ВКР/Проект1/config.yaml.example) — общий пример.

Для первого запуска проще всего использовать `config.local.yaml`.

### 2. Локальный запуск HTTP-сервиса

PowerShell:

```powershell
$env:LOGMONITOR_APP_MODE = "HTTP"
go run ./cmd/server -config config.local.yaml
```

Bash:

```bash
LOGMONITOR_APP_MODE=HTTP go run ./cmd/server -config config.local.yaml
```

После запуска будут доступны:

- `http://localhost:8080/healthz`
- `http://localhost:8080/readyz`
- `http://localhost:8080/swagger`

### 3. Локальный запуск CLI

PowerShell:

```powershell
$env:LOGMONITOR_APP_MODE = "CLI"
go run ./cmd/cli --config config.local.yaml health
```

Примеры:

```powershell
$env:LOGMONITOR_APP_MODE = "CLI"
go run ./cmd/cli --config config.local.yaml config validate
go run ./cmd/cli --config config.local.yaml server list
go run ./cmd/cli --config config.local.yaml dashboard
```

### 4. Что важно знать про локальный старт

Файл [config.local.yaml](/d:/GolangProjects/учёба/ВКР/Проект1/config.local.yaml) по умолчанию не содержит настроек PostgreSQL, поэтому приложение использует **in-memory storage**.

Это удобно для разработки, но у такого режима есть ограничения:

- данные не сохраняются между перезапусками;
- история job и runtime-состояние тоже теряются;
- это не production-режим.

Если нужен постоянный storage, заполните секцию `database` и задайте ключ `security.auth_value_encryption_key`.

## Конфигурация

Основной конфиг хранится в YAML. Ниже кратко описано, что означает каждая секция.

| Секция | Назначение | Обязательна |
| --- | --- | --- |
| `server` | Хост и порт HTTP-сервера | Только для `HTTP` |
| `api` | Токен авторизации для API | Только для `HTTP` |
| `security` | Ключи шифрования и HMAC | Да |
| `database` | Подключение к PostgreSQL | Нет, если устраивает in-memory |
| `ssh` | Таймауты и политика host key | Да |
| `scheduler` | Расписание discovery / collect / integrity | В основном для `HTTP` |
| `collector` | Batch/chunk-параметры сбора логов | Да |
| `health` | Политики degraded/error/backoff | Да |
| `jobs` | Размер очереди и история job | В основном для `HTTP` |
| `runtime` | Служебные режимы, например `dry_run` | Да |
| `workers` | Параллелизм cron-задач | В основном для `HTTP` |
| `servers` | Стартовый список серверов из конфига | Нет |

### Важные особенности

1. `server` и `api` используются только в режиме `HTTP`.
2. Если `database.host`, `database.user` или `database.dbname` не заданы, приложение переключается на in-memory storage.
3. Если `runtime.dry_run=true`, scheduler отключается, а хранилище принудительно становится in-memory.
4. Серверы из секции `servers` автоматически загружаются при старте как `config`-managed.
5. Серверы, созданные через API, помечаются как `api`-managed.

### Пример списка нескольких серверов

```yaml
servers:
  - name: "linux-demo"
    host: "192.168.1.10"
    port: 22
    username: "root"
    auth_type: "password"
    auth_value: "secret"
    os_type: "linux"

  - name: "windows-demo"
    host: "192.168.1.20"
    port: 22
    username: "administrator"
    auth_type: "password"
    auth_value: "secret"
    os_type: "windows"
```

## Переменные окружения

### Общие и режим запуска

| Переменная | Назначение | Обязательна | Пример |
| --- | --- | --- | --- |
| `LOGMONITOR_APP_MODE` | Режим запуска: `HTTP` или `CLI` | Да | `HTTP` |

### HTTP-сервис

| Переменная | Назначение | Обязательна | Пример |
| --- | --- | --- | --- |
| `APP_HOST` | Хост HTTP-сервера | Нет | `0.0.0.0` |
| `APP_PORT` | Порт HTTP-сервера | Нет | `8080` |
| `API_AUTH_TOKEN` | API-токен для `X-API-Key` или `Authorization: Bearer` | Нет, но рекомендуется | `super-secret-token` |

> Если `API_AUTH_TOKEN` пустой, API будет доступен без авторизации. Для production так делать не стоит.

### Безопасность

| Переменная | Назначение | Обязательна | Пример |
| --- | --- | --- | --- |
| `AUTH_VALUE_ENCRYPTION_KEY` | Ключ шифрования `auth_value` в PostgreSQL | Обязательна при PostgreSQL | `change-me-auth-value-encryption-key` |
| `INTEGRITY_HMAC_KEY` | Ключ HMAC для контроля целостности | Да | `change-me-integrity-hmac-key` |

### PostgreSQL

| Переменная | Назначение | Обязательна | Пример |
| --- | --- | --- | --- |
| `DB_HOST` | Хост PostgreSQL для приложения | Для постоянного storage | `postgres` |
| `DB_PORT` | Порт PostgreSQL для приложения | Нет | `5432` |
| `DB_USER` | Пользователь БД для приложения | Для постоянного storage | `logmonitor` |
| `DB_PASSWORD` | Пароль БД для приложения | Для постоянного storage | `logmonitor` |
| `DB_NAME` | Имя БД для приложения | Для постоянного storage | `logmonitor` |
| `DB_SSLMODE` | SSL mode | Нет | `disable` |
| `DB_MAX_CONNS` | Максимум соединений в пуле | Нет | `10` |
| `DB_MIN_CONNS` | Минимум соединений в пуле | Нет | `1` |
| `DB_MIGRATIONS_DIR` | Путь к миграциям goose | Нет | `migrations` |

Дополнительно в Docker Compose используются переменные контейнера PostgreSQL:

- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_PORT`

### SSH

| Переменная | Назначение | Обязательна | Пример |
| --- | --- | --- | --- |
| `SSH_CONNECT_TIMEOUT_SECONDS` | Таймаут подключения | Нет | `10` |
| `SSH_COMMAND_TIMEOUT_SECONDS` | Таймаут выполнения команды | Нет | `30` |
| `SSH_KNOWN_HOSTS_PATH` | Путь к `known_hosts` | Нет | `C:/Users/user/.ssh/known_hosts` |
| `SSH_INSECURE_IGNORE_HOST_KEY` | Игнорировать host key check | Нет | `true` |

### Scheduler, collector, health, jobs и workers

| Переменная | Назначение | Пример |
| --- | --- | --- |
| `DISCOVERY_CRON` | Расписание discovery | `0 */6 * * *` |
| `COLLECTION_CRON` | Расписание collection | `*/5 * * * *` |
| `INTEGRITY_CRON` | Расписание integrity check | `0 * * * *` |
| `COLLECTOR_BATCH_SIZE` | Batch size при вставке логов | `5000` |
| `COLLECTOR_CHUNK_SIZE` | Размер чанка для групповых hash/HMAC-операций | `1000` |
| `COLLECTOR_STORE_RAW_CONTENT` | Сохранять текст строк лога | `true` |
| `COLLECTOR_CHUNK_HASH_ALGO` | Алгоритм hash для чанков | `sha256` |
| `HEALTH_FAILURE_THRESHOLD` | Порог ошибок до реакции health-layer | `1` |
| `HEALTH_BACKOFF_BASE_SECONDS` | Базовый backoff | `60` |
| `HEALTH_BACKOFF_MAX_SECONDS` | Максимальный backoff | `900` |
| `HEALTH_LAST_ERROR_MAX_LENGTH` | Максимальная длина `last_error` | `2048` |
| `JOBS_WORKERS` | Число воркеров очереди job | `2` |
| `JOBS_QUEUE_SIZE` | Размер очереди job | `128` |
| `JOBS_HISTORY_LIMIT` | Максимум job в памяти | `1000` |
| `WORKERS_DISCOVERY_SERVERS` | Параллелизм discovery | `4` |
| `WORKERS_COLLECTION_SERVERS` | Параллелизм collection по серверам | `4` |
| `WORKERS_COLLECTION_LOG_FILES_PER_HOST` | Параллелизм collection по логам на сервер | `2` |
| `WORKERS_INTEGRITY_SERVERS` | Параллелизм integrity по серверам | `2` |
| `WORKERS_INTEGRITY_LOG_FILES_PER_HOST` | Параллелизм integrity по логам на сервер | `1` |
| `WORKERS_PER_SERVER_ISOLATION` | Изоляция конкурентных операций на один сервер | `true` |
| `RUNTIME_DRY_RUN` | Холостой запуск | `false` |

### Где используется `.env`

`.env` автоматически подхватывается:

- `docker compose`;
- `config.docker.yaml`, потому что в нём используются шаблоны `${VAR}`.

Для **локального запуска Go-процесса** `.env` сам по себе не читается. В этом случае:

- либо задайте переменные в shell;
- либо используйте YAML с готовыми значениями;
- либо запускайте через Docker Compose.

## CLI-утилита

CLI работает синхронно и использует тот же сервисный слой, что и HTTP-режим, но не поднимает API.

### Общие флаги

| Флаг | Назначение |
| --- | --- |
| `--config`, `-c` | Путь к YAML-конфигу |
| `--output`, `-o` | Формат вывода: `table` или `json` |

### Основные команды

| Команда | Назначение |
| --- | --- |
| `logmonitor health` | Проверка, что CLI runtime может инициализироваться |
| `logmonitor ready` | Проверка readiness общего runtime |
| `logmonitor config validate` | Показ runtime validation и env resolution |
| `logmonitor server list` | Список серверов |
| `logmonitor server get <id>` | Просмотр сервера |
| `logmonitor server add ...` | Добавление сервера |
| `logmonitor server update <id> ...` | Обновление сервера |
| `logmonitor server delete <id>` | Удаление сервера |
| `logmonitor server retry <id>` | Сброс временного failure/backoff |
| `logmonitor discover [--server-id]` | Поиск логов |
| `logmonitor logfile list [--server-id]` | Список логов |
| `logmonitor collect --server-id ...` | Сбор строк |
| `logmonitor entry list --log-file-id ...` | Просмотр строк |
| `logmonitor check list --log-file-id ...` | История проверок |
| `logmonitor check run --server-id ...` | Запуск проверки целостности |
| `logmonitor problem list` | Агрегированный список проблем |
| `logmonitor dashboard` | Сводная статистика |
| `logmonitor runtime validation` | Снимок runtime-state |

### Примеры CLI

```powershell
$env:LOGMONITOR_APP_MODE = "CLI"

go run ./cmd/cli --config config.local.yaml server list
go run ./cmd/cli --config config.local.yaml server add --name web-1 --host 192.168.1.10 --username root --auth-type password --auth-value secret --os-type linux
go run ./cmd/cli --config config.local.yaml discover --server-id srv_123
go run ./cmd/cli --config config.local.yaml collect --server-id srv_123
go run ./cmd/cli --config config.local.yaml check run --server-id srv_123 --output json
```

## HTTP API

### Базовые URL

| URL | Назначение |
| --- | --- |
| `/healthz` | Liveness probe |
| `/readyz` | Readiness probe |
| `/swagger` | Swagger UI |
| `/swagger/openapi.json` | OpenAPI JSON |

### Авторизация

Если `api.auth_token` не пустой, используйте один из вариантов:

```http
X-API-Key: <token>
```

или

```http
Authorization: Bearer <token>
```

### Синхронные и асинхронные операции

HTTP API смешанного типа:

- **синхронные**: чтение списков, получение сущностей, health/readiness;
- **асинхронные**: discovery, collect, check run.

Асинхронные ручки:

- возвращают `202 Accepted`;
- выставляют `Location: /api/jobs/{id}`;
- сохраняют результат в in-memory истории job;
- поддерживают `X-Idempotency-Key` для идемпотентных повторных вызовов.

### Статусы job

| Статус | Значение |
| --- | --- |
| `queued` | Задача поставлена в очередь |
| `running` | Задача выполняется |
| `succeeded` | Задача завершилась успешно |
| `failed` | Задача завершилась с ошибкой |
| `canceled` | Задача отменена при shutdown |

### Ограничения пагинации

Для list-ручек:

- `offset >= 0`
- `limit > 0`
- `limit <= 1000`

### Ручки API

#### Системные

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/healthz` | Проверка жизни процесса |
| `GET` | `/readyz` | Проверка готовности |
| `GET` | `/swagger` | Swagger UI |
| `GET` | `/swagger/openapi.json` | OpenAPI JSON |

#### Runtime и jobs

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/api/runtime/validation` | Runtime warnings и env resolution |
| `GET` | `/api/jobs` | Список job |
| `GET` | `/api/jobs/{id}` | Получение job по ID |

#### Dashboard и problems

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/api/dashboard` | Сводная статистика |
| `GET` | `/api/problems` | Агрегированный список проблем |

#### Servers

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/api/servers` | Список серверов |
| `GET` | `/api/servers/{id}` | Один сервер |
| `POST` | `/api/servers` | Создание сервера |
| `PUT` | `/api/servers/{id}` | Обновление API-managed сервера |
| `DELETE` | `/api/servers/{id}` | Удаление API-managed сервера |
| `POST` | `/api/servers/{id}/retry` | Сброс backoff/failure state |
| `POST` | `/api/servers/discover` | Асинхронный discovery |

#### Log files

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/api/logfiles` | Список логов |
| `POST` | `/api/logfiles/collect` | Асинхронный collection |

#### Entries

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/api/entries` | Список строк конкретного лога |

#### Checks

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/api/checks` | История проверок конкретного лога |
| `POST` | `/api/checks/run` | Асинхронный integrity check |

### Примеры запросов

#### Создать сервер

```bash
curl -X POST http://localhost:8080/api/servers \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-token" \
  -d '{
    "name": "web-1",
    "host": "192.168.1.10",
    "port": 22,
    "username": "root",
    "auth_type": "password",
    "auth_value": "secret",
    "os_type": "linux"
  }'
```

#### Запустить discovery

```bash
curl -X POST http://localhost:8080/api/servers/discover \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-token" \
  -H "X-Idempotency-Key: discover-web-1" \
  -d '{
    "server_id": "srv_123"
  }'
```

#### Запустить collection

```bash
curl -X POST http://localhost:8080/api/logfiles/collect \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-token" \
  -H "X-Idempotency-Key: collect-web-1" \
  -d '{
    "server_id": "srv_123"
  }'
```

#### Запустить integrity check

```bash
curl -X POST http://localhost:8080/api/checks/run \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-token" \
  -H "X-Idempotency-Key: integrity-web-1" \
  -d '{
    "server_id": "srv_123"
  }'
```

#### Получить статус job

```bash
curl http://localhost:8080/api/jobs/<job-id> \
  -H "X-API-Key: your-token"
```

## Docker и Docker Compose

### Что уже есть в проекте

- [Dockerfile](/d:/GolangProjects/учёба/ВКР/Проект1/Dockerfile) — собирает **HTTP-серверный** бинарник;
- [docker-compose.yaml](/d:/GolangProjects/учёба/ВКР/Проект1/docker-compose.yaml) — поднимает `app` и `postgres`;
- [config.docker.yaml](/d:/GolangProjects/учёба/ВКР/Проект1/config.docker.yaml) — конфиг с `${ENV}`-placeholder’ами;
- [.env.example](/d:/GolangProjects/учёба/ВКР/Проект1/.env.example) — пример переменных.

### Запуск через Docker Compose

1. Скопируйте `.env.example` в `.env`
2. При необходимости задайте реальные секреты
3. Убедитесь, что `LOGMONITOR_APP_MODE=HTTP`
4. Запустите:

```bash
docker compose up --build -d
```

5. Проверьте:

```bash
curl http://localhost:8080/healthz
```

### Важно

Текущий Dockerfile собирает только серверную часть `./cmd/server`. CLI-утилита поставляется отдельно и обычно используется локально.

## Хранение данных и миграции

### PostgreSQL

Если заполнена секция `database`, проект использует PostgreSQL через `pgxpool`.

При старте приложения автоматически выполняются миграции через `goose`.

Каталог миграций:

- [migrations](/d:/GolangProjects/учёба/ВКР/Проект1/migrations)

### In-memory storage

Если база не настроена или включён `runtime.dry_run`, используется in-memory storage.

Плюсы:

- быстро для разработки;
- не нужен PostgreSQL;
- удобно для пробных запусков.

Минусы:

- данные не переживают рестарт;
- история job тоже не сохраняется;
- не подходит для production.

### Что хранится в БД

- серверы;
- найденные лог-файлы;
- строки логов;
- чанки логов;
- результаты проверок.

`auth_value` в PostgreSQL хранится в зашифрованном виде. Для контроля целостности используется HMAC.

## Сборка бинарников

Сервер:

```bash
go build -o bin/logmonitor-server ./cmd/server
```

CLI:

```bash
go build -o bin/logmonitor-cli ./cmd/cli
```

Запуск после сборки:

PowerShell:

```powershell
$env:LOGMONITOR_APP_MODE = "HTTP"
.\bin\logmonitor-server -config config.local.yaml
```

```powershell
$env:LOGMONITOR_APP_MODE = "CLI"
.\bin\logmonitor-cli --config config.local.yaml server list
```

## Разработка

### Генерация Swagger

```bash
go generate ./cmd/server
```

Сгенерированные файлы:

- [docs/swagger.json](/d:/GolangProjects/учёба/ВКР/Проект1/docs/swagger.json)
- [docs/swagger.yaml](/d:/GolangProjects/учёба/ВКР/Проект1/docs/swagger.yaml)

### Полезные команды

```bash
go test ./...
go build ./...
go vet ./...
golangci-lint run
```

### Что учитывать при эксплуатации

1. HTTP async job history хранится в памяти процесса и очищается после рестарта.
2. CLI выполняет операции синхронно и не использует HTTP API.
3. `api.auth_token` лучше всегда задавать явно.
4. Для production лучше использовать PostgreSQL, а не in-memory storage.
5. Если `os_type` пустой, система пытается определить ОС удалённого сервера автоматически.
