# Готовый промт для описания проекта LogMonitor в ВКР

Ниже находится самодостаточный промт, который можно отправить в чат-модель для генерации текста ВКР. Он описывает назначение проекта, архитектуру, функциональность, технологии, структуру кода, модели данных, ключевые алгоритмы и важные фрагменты реализации.

---

## Промт

Ты пишешь текст выпускной квалификационной работы на русском языке по программному проекту **LogMonitor**. Стиль должен быть академическим, технически точным, но понятным: без маркетинговых формулировок, с объяснением архитектурных решений, причин выбора технологий и описанием алгоритмов. Не придумывай функциональность, которой нет в проекте. Если нужно указать перспективы развития, явно отделяй их от реализованной части.

Нужно подготовить описание проекта для разделов ВКР: постановка задачи, анализ предметной области, проектирование, архитектура, реализация, безопасность, тестирование, эксплуатация и возможные доработки.

## 1. Общая характеристика проекта

**LogMonitor** - это серверное приложение для мониторинга и контроля целостности журналов событий на удалённых серверах. Система подключается к серверам по SSH, автоматически обнаруживает журналы, собирает новые записи, сохраняет метаданные и контрольные значения, а затем проверяет, были ли записи изменены после сбора.

Основная идея проекта: обеспечить централизованный контроль журналов на нескольких удалённых узлах без установки отдельного агента на каждый сервер. Взаимодействие с удалённой машиной выполняется через SSH-команды, а состояние мониторинга хранится в PostgreSQL либо во временном in-memory хранилище для локального запуска и тестирования.

Проект решает следующие задачи:

- регистрация и хранение списка удалённых серверов;
- подключение к серверам по SSH с паролем или приватным ключом;
- автоматическое определение операционной системы, если она не указана в конфигурации;
- обнаружение системных и прикладных журналов на Linux, Windows и macOS;
- синхронизация найденных лог-файлов с хранилищем;
- инкрементальный сбор новых строк журналов;
- вычисление hash/HMAC для каждой строки;
- построение агрегированных hash-чанков для ускорения проверки целостности;
- обнаружение изменения или удаления ранее собранных строк;
- сохранение результатов integrity check;
- ведение health-состояния серверов;
- предоставление HTTP API, Swagger UI и CLI-интерфейса;
- выполнение discovery, collection и integrity check вручную или по расписанию;
- запуск в Docker Compose вместе с PostgreSQL.

В текущей версии веб-интерфейс не реализован. Для работы доступны HTTP API, Swagger UI и CLI.

## 2. Режимы запуска

Проект поддерживает два режима запуска, которые выбираются через переменную окружения `LOGMONITOR_APP_MODE`.

| Режим | Значение | Точка входа | Назначение |
| --- | --- | --- | --- |
| HTTP | `HTTP` | `cmd/server` | Долгоживущий сервис с REST API, Swagger, очередью задач и scheduler |
| CLI | `CLI` | `cmd/cli` | Консольная утилита для ручного управления и диагностики |

Разделение режимов сделано на уровне отдельных бинарников и общей runtime-сборки. HTTP и CLI используют один и тот же сервисный слой, репозитории, модели и SSH-логику. Благодаря этому бизнес-логика не дублируется между API и консольной утилитой.

Проверка режима запуска реализована в пакете `pkg/appmode`: процесс не стартует, если выбранный через окружение режим не соответствует бинарнику. Это снижает вероятность ошибки эксплуатации, например случайного запуска CLI с HTTP-конфигурацией.

## 3. Технологический стек

Основные технологии проекта:

- язык программирования: Go 1.26;
- HTTP framework: Gin;
- CLI framework: Cobra;
- база данных: PostgreSQL;
- драйвер PostgreSQL: pgx/pgxpool;
- миграции: goose;
- SSH: `golang.org/x/crypto/ssh`;
- конфигурация: YAML + env placeholders;
- контейнеризация: Docker, Docker Compose;
- тестирование: Go testing, Allure Go, testcontainers для интеграционных PostgreSQL-тестов;
- криптография: SHA-256, HMAC-SHA256, AES-GCM для хранения SSH-секретов.

## 4. Общая архитектура

Архитектура проекта близка к слоистой архитектуре с элементами hexagonal architecture:

1. **cmd** - точки входа приложения.
2. **internal/app** - сборка runtime-зависимостей и жизненный цикл приложений HTTP/CLI.
3. **internal/transport** - внешние интерфейсы: HTTP handlers и CLI commands.
4. **internal/service** - бизнес-логика: discovery, collection, integrity, health, server/logfile/check/entry operations.
5. **internal/repository** - интерфейсы хранилища.
6. **internal/repository/postgres** - PostgreSQL-реализация.
7. **internal/repository/memory** - in-memory реализация.
8. **internal/ssh** - абстракция и реализация SSH-клиента.
9. **crons** - фоновые runner-ы, scheduler и in-process locks.
10. **models** - доменные модели.
11. **config** - загрузка и валидация конфигурации.
12. **pkg** - переиспользуемые утилиты: appmode, hasher, logger.

Упрощённая схема зависимостей:

```text
cmd/server или cmd/cli
        |
        v
internal/app/http или internal/app/cli
        |
        v
internal/app/general Runtime
        |
        +--> services
        |       +--> discovery / collector / integrity / health
        |
        +--> repository interface
        |       +--> postgres.Storage или memory.Storage
        |
        +--> ssh.ClientFactory
        |
        +--> runtimeinfo.State
```

Ключевой принцип: транспортный слой не работает напрямую с базой данных или SSH. HTTP handlers и CLI commands вызывают application services, а сервисы используют интерфейсы репозиториев и SSH-клиента. Это упрощает тестирование и замену инфраструктурных компонентов.

## 5. Структура проекта

Основные директории:

```text
cmd/
  cli/                  CLI-точка входа
  server/               HTTP server точка входа и Swagger генерация

config/
  config.go             структура YAML-конфига
  runtime_loader.go     загрузка env placeholders, defaults и validation

crons/
  scheduler/            interval scheduler
  locks/                in-process per-server locks
  discovery/            scheduled discovery runner
  collection/           scheduled collection runner
  integrity/            scheduled integrity runner

internal/app/
  general/              общая runtime-сборка
  http/                 HTTP lifecycle, jobs, scheduler
  cli/                  CLI runtime loading

internal/service/
  discovery/            поиск логов и определение ОС
  collector/            чтение логов, hash, chunks, сохранение
  integrity/            проверка целостности
  health/               lifecycle статусы серверов и backoff
  server/               операции над серверами, dashboard, problems
  logfile/              операции над лог-файлами и collection
  check/                история и запуск integrity check
  entry/                чтение сохранённых строк

internal/repository/
  *.go                  интерфейсы хранилища
  memory/               in-memory реализация
  postgres/             PostgreSQL реализация

internal/transport/
  http/                 Gin server, handlers, middleware
  cli/                  Cobra commands

internal/ssh/           SSH client abstraction and executor
internal/security/      AES-GCM шифрование секретов
internal/jobs/          in-memory async job queue
internal/runtimeinfo/   runtime validation/readiness state

models/                 доменные модели
migrations/             SQL-миграции goose
docs/                   Swagger и вспомогательная документация
```

## 6. Доменные сущности

Основные модели:

- `Server` - удалённый сервер, за журналами которого ведётся наблюдение.
- `LogFile` - обнаруженный журнал на удалённом сервере.
- `LogEntry` - сохранённая строка журнала с номером строки и hash.
- `LogChunk` - агрегированный hash группы строк.
- `CheckResult` - результат проверки целостности.
- `TamperedEntry` - конкретная изменённая или отсутствующая строка.
- `Job` - асинхронная операция HTTP API.
- `SystemProblem` - агрегированная проблема для оператора.

### Server

Сервер содержит SSH-параметры, тип ОС, статус, источник управления и health-поля:

- `ID`, `Name`, `Host`, `Port`;
- `Username`, `AuthType`, `AuthValue`;
- `OSType`: `linux`, `windows`, `macos` или пусто для автоопределения;
- `Status`: `active`, `degraded`, `inactive`, `error`;
- `ManagedBy`: `config` или `api`;
- `SuccessCount`, `FailureCount`, `LastError`, `LastSeenAt`, `BackoffUntil`;
- timestamps создания и обновления.

`ManagedBy` нужен для разделения серверов, загруженных из конфигурации, и серверов, созданных через API. API не позволяет изменять config-managed серверы, чтобы не было конфликта между файлом конфигурации и runtime-изменениями.

### LogFile

`LogFile` хранит путь к журналу, тип лога, активность, идентичность файла и состояние последнего сканирования.

`FileIdentity` используется для обнаружения ротации или замены файла:

- для Linux/macOS: `device_id`, `inode`, `size_bytes`, `mod_time_unix`;
- для Windows: `file_id`, `volume_id`, размер и время изменения;
- для Windows Event Log: `event_log`.

### LogEntry

`LogEntry` хранит:

- номер строки;
- исходное содержимое строки, если `collector.store_raw_content=true`;
- hash строки;
- время сбора.

По умолчанию raw content выключен, потому что строки логов могут содержать чувствительные данные. Даже при выключенном хранении содержимого hash всё равно вычисляется по реальному тексту строки.

### LogChunk

`LogChunk` содержит агрегированный hash группы строк. Чанки нужны, чтобы при integrity check быстро понять, какая группа строк изменилась, и не сравнивать каждую строку там, где агрегированный hash совпал.

### CheckResult

Результат проверки имеет статус:

- `ok` - все сохранённые строки совпали с текущим состоянием лога;
- `tampered` - обнаружены изменённые или отсутствующие строки;
- `error` - проверка не может быть корректно выполнена, например лог был ротирован и требуется повторный collection.

## 7. Конфигурация

Конфигурация хранится в YAML. Поддерживаются placeholders вида `${VAR}` и `${VAR:-default}`. При загрузке конфигурации приложение:

1. читает YAML-файл;
2. заменяет env placeholders;
3. сохраняет сведения о том, какие переменные были provided/defaulted/missing;
4. применяет defaults;
5. выполняет mode-aware validation.

Важные секции:

- `server` - host/port HTTP-сервера;
- `api` - HTTP API token и явное разрешение unauthenticated режима;
- `security` - ключи шифрования и HMAC;
- `database` - PostgreSQL;
- `ssh` - таймауты и known_hosts policy;
- `scheduler` - интервалы discovery/collection/integrity;
- `collector` - batch size, chunk size, хранение raw content;
- `health` - threshold/backoff;
- `jobs` - очередь HTTP jobs;
- `workers` - параллелизм фоновых задач;
- `servers` - стартовый список серверов.

В HTTP-режиме `api.auth_token` обязателен, если явно не задано `api.allow_unauthenticated: true`. Это защищает от случайного запуска production API без авторизации. Локальный `config.local.yaml` может явно отключать auth для разработки.

## 8. Runtime и сборка зависимостей

Центральная точка сборки приложения - `internal/app/general.Runtime`. Она создаёт:

- logger;
- runtime state;
- repository backend;
- SSH factory;
- service layer;
- health service;
- lock manager;
- server/logfile/entry/check application services;
- seed серверов из конфигурации.

Важный фрагмент:

```go
func NewRuntime(cfg *config.Config) (*Runtime, error) {
    log := logger.New("info")
    runtimeState := buildRuntimeState(cfg)

    store, _, err := buildRepository(cfg)
    if err != nil {
        return nil, err
    }

    sshFactory, err := sshclient.NewClientFactoryWithOptions(sshclient.Options{
        ConnectTimeout:        time.Duration(cfg.SSH.ConnectTimeoutSeconds) * time.Second,
        CommandTimeout:        time.Duration(cfg.SSH.CommandTimeoutSeconds) * time.Second,
        KnownHostsPath:        cfg.SSH.KnownHostsPath,
        InsecureIgnoreHostKey: *cfg.SSH.InsecureIgnoreHostKey,
    })
    if err != nil {
        return nil, fmt.Errorf("app: create ssh client factory: %w", err)
    }

    discoveryService := discoveryservice.NewServiceWithServerRepository(sshFactory, store, store, nil)
    collectorService := collectservice.NewServiceWithOptions(sshFactory, store, store, store, collectservice.Options{
        BatchSize:        cfg.Collector.BatchSize,
        ChunkSize:        cfg.Collector.ChunkSize,
        StoreRawContent:  *cfg.Collector.StoreRawContent,
        ChunkHashAlgo:    cfg.Collector.ChunkHashAlgo,
        IntegrityHMACKey: cfg.Security.IntegrityHMACKey,
    })
    integrityService := integrityservice.NewServiceWithOptions(sshFactory, store, store, store, integrityservice.Options{
        IntegrityHMACKey: cfg.Security.IntegrityHMACKey,
    })
}
```

Repository выбирается автоматически:

- если включён `runtime.dry_run`, используется memory storage;
- если PostgreSQL-настройки не заполнены, используется memory storage;
- если database настроена, создаётся PostgreSQL pool, выполняются миграции и используется postgres storage.

## 9. Репозиторный слой

Репозиторный слой описан интерфейсами в `internal/repository`. Общий интерфейс объединяет работу с серверами, лог-файлами, строками, чанками и результатами проверок.

Ключевой фрагмент:

```go
type Repository interface {
    ServerRepository
    LogFileRepository
    LogEntryRepository
    LogChunkRepository
    LogBatchRepository
    CheckResultRepository

    Ping(ctx context.Context) error
    Close() error
}
```

Такой подход позволяет бизнес-логике зависеть от абстракций, а не от конкретной базы данных. Поэтому сервисы одинаково работают с PostgreSQL и in-memory storage.

### PostgreSQL storage

PostgreSQL-реализация:

- использует `pgxpool`;
- при старте выполняет миграции через goose;
- хранит `auth_value` в зашифрованном виде;
- использует `CopyFrom` для пакетной вставки логов и чанков;
- поддерживает транзакционное сохранение entries + chunks;
- задаёт уникальные индексы для серверов и лог-файлов;
- использует каскадное удаление зависимых данных.

### In-memory storage

In-memory storage используется для:

- локальной разработки;
- dry-run режима;
- unit-тестов.

Он реализован на map-структурах под `sync.RWMutex`, возвращает clone-модели наружу и поддерживает те же интерфейсы, что PostgreSQL.

## 10. Discovery: обнаружение журналов

Discovery отвечает за поиск журналов на удалённом сервере.

Алгоритм:

1. Создать SSH-клиент.
2. Подключиться к серверу.
3. Если ОС не указана, попытаться определить её:
   - `uname -s` для Linux/macOS;
   - PowerShell или `cmd /c ver` для Windows.
4. Выбрать Discoverer из registry по типу ОС.
5. Выполнить ОС-специфичную команду поиска логов.
6. Нормализовать и классифицировать найденные пути.
7. Синхронизировать результат с repository:
   - новые пути создать;
   - существующие обновить;
   - исчезнувшие пометить `is_active=false`.

Поддерживаемые ОС:

- Linux: поиск в `/var/log`;
- macOS: поиск в `/var/log` и `/Library/Logs`;
- Windows: Windows Event Log (`Application`, `System`, `Security`) и `.log` файлы в типичных директориях.

Классификация типа журнала выполняется по пути: `syslog`, `auth`, `nginx`, `apache`, `eventlog`, `kernel`, `app`, `unknown`.

Важный фрагмент:

```go
func (s *Service) Discover(ctx context.Context, serverModel *models.Server) ([]DiscoveredLog, error) {
    client := s.clientFactory.NewClient()
    if err := client.Connect(serverModel); err != nil {
        return nil, fmt.Errorf("discovery: connect to server %q: %w", serverModel.Name, err)
    }
    defer client.Close()

    osType := serverModel.OSType
    if osType == "" {
        detected, err := DetectOSType(ctx, client)
        if err != nil {
            return nil, err
        }
        serverModel.OSType = detected
        osType = detected
        if s.servers != nil {
            if err := s.servers.UpdateServer(ctx, serverModel); err != nil {
                return nil, fmt.Errorf("discovery: persist detected os for server %q: %w", serverModel.Name, err)
            }
        }
    }

    discoverer, ok := s.registry.Get(osType)
    if !ok {
        return nil, fmt.Errorf("discovery: unsupported os %q", osType)
    }

    return discoverer.Discover(client)
}
```

## 11. Collection: сбор журналов

Collection отвечает за инкрементальное чтение новых строк из найденных логов.

Алгоритм:

1. Подключиться к серверу по SSH.
2. Получить текущую идентичность источника лога.
3. Сравнить её с сохранённой идентичностью.
4. Если файл был ротирован или заменён, очистить старые entries/chunks и начать сбор заново.
5. Получить максимальный сохранённый номер строки.
6. Выполнить команду чтения строк после этого номера.
7. Для каждой новой строки:
   - сохранить номер строки;
   - вычислить hash или HMAC;
   - при необходимости сохранить raw content;
   - добавить запись в batch.
8. Сформировать агрегированные chunks.
9. Сохранить entries и chunks, по возможности атомарно.
10. Обновить metadata лог-файла: last scanned, last line, file identity.

Для Linux/macOS используется `awk`, для Windows файлов - PowerShell `Get-Content`, для Windows Event Log - `Get-WinEvent`.

Ключевой фрагмент:

```go
func (s *Service) CollectLogFile(ctx context.Context, serverModel *models.Server, logFile *models.LogFile) (int, error) {
    client := s.clientFactory.NewClient()
    if err := client.Connect(serverModel); err != nil {
        return 0, fmt.Errorf("collector: connect to %q: %w", serverModel.Name, err)
    }
    defer client.Close()

    identity, meta, identityErr := InspectLogFileIdentity(ctx, client, serverModel, logFile)
    shouldReset := identityErr == nil && shouldResetCollection(logFile, identity)

    maxLine := int64(0)
    if !shouldReset {
        maxLine, err = s.entries.GetMaxLineNumber(ctx, logFile.ID)
        if err != nil {
            return 0, fmt.Errorf("collector: get max line for %q: %w", logFile.Path, err)
        }
    }

    lines, err := ReadLogLinesAfter(ctx, client, logFile, maxLine)
    if err != nil {
        return 0, err
    }

    if shouldReset {
        if err := s.resetLogFileState(ctx, logFile); err != nil {
            return 0, err
        }
    }

    for _, line := range lines {
        content := line.Content
        if !s.options.StoreRawContent {
            content = ""
        }
        newEntries = append(newEntries, &models.LogEntry{
            LogFileID:  logFile.ID,
            LineNumber: line.Number,
            Content:    content,
            Hash:       hasher.HashString(line.Content, s.options.IntegrityHMACKey),
        })
    }
}
```

Особенность: даже если raw content не сохраняется, hash вычисляется от настоящего содержимого строки. Это позволяет проверять целостность без хранения потенциально чувствительных логов.

## 12. Chunk hashing

Для ускорения integrity check collector группирует entries в чанки. Каждый entry имеет собственный hash. Hash чанка считается как SHA-256 от последовательности entry-hash значений.

Фрагмент:

```go
func hashEntryBatch(entries []*models.LogEntry) string {
    builder := strings.Builder{}
    for _, entry := range entries {
        builder.WriteString(entry.Hash)
        builder.WriteByte('\n')
    }
    return hasher.SHA256String(builder.String())
}
```

Если при integrity check hash чанка совпадает, детальная проверка строк внутри этого чанка не требуется. Если hash не совпадает, сервис сравнивает строки внутри диапазона чанка.

## 13. Integrity check: проверка целостности

Integrity check сравнивает сохранённые hash-значения с текущим состоянием лога на удалённом сервере.

Алгоритм:

1. Подключиться к серверу.
2. Проверить текущую идентичность источника.
3. Если источник изменился, не считать это tampering, а сохранить статус `error` с рекомендацией выполнить collection заново.
4. Прочитать текущие строки лога.
5. Построить map `line number -> content`.
6. Посчитать количество сохранённых entries.
7. Если есть chunks:
   - отсортировать chunks;
   - для каждого chunk пересчитать aggregate hash по текущим строкам;
   - если chunk совпал, пропустить его;
   - если не совпал, сравнить строки внутри chunk;
   - отдельно проверить entries, которые не покрыты chunks.
8. Если chunks нет, сравнить все entries.
9. Сохранить `CheckResult` со статусом `ok` или `tampered`.
10. Вернуть список `TamperedEntry`.

Важный фрагмент:

```go
func (s *Service) CheckLogFile(ctx context.Context, serverModel *models.Server, logFile *models.LogFile) (*models.CheckResult, []models.TamperedEntry, error) {
    client := s.clientFactory.NewClient()
    if err := client.Connect(serverModel); err != nil {
        result := s.storeErrorResult(ctx, logFile.ID, fmt.Sprintf("connect to %s: %v", serverModel.Name, err))
        return result, nil, fmt.Errorf("integrity: connect to %q: %w", serverModel.Name, err)
    }
    defer client.Close()

    currentIdentity, _, identityErr := collectservice.InspectLogFileIdentity(ctx, client, serverModel, logFile)
    if identityErr == nil && collectservice.RequiresRecollection(logFile, currentIdentity) {
        message := fmt.Sprintf("log source %q changed identity since last collection; run collection again before integrity check", logFile.Path)
        return s.storeErrorResult(ctx, logFile.ID, message), nil, nil
    }

    currentLines, err := collectservice.ReadLogLines(ctx, client, logFile)
    if err != nil {
        result := s.storeErrorResult(ctx, logFile.ID, err.Error())
        return result, nil, err
    }

    tampered, err := s.findTamperedEntries(ctx, logFile.ID, currentByLine)
    if err != nil {
        return nil, nil, fmt.Errorf("integrity: compare entries for %q: %w", logFile.Path, err)
    }
}
```

Фрагмент сравнения entries:

```go
func (s *Service) compareStoredEntries(storedEntries []*models.LogEntry, currentByLine map[int64]string) []models.TamperedEntry {
    tampered := make([]models.TamperedEntry, 0)
    for _, entry := range storedEntries {
        currentContent, ok := currentByLine[entry.LineNumber]
        currentHash := ""
        if ok {
            currentHash = hasher.HashString(currentContent, s.integrityKey)
        }

        if !ok || currentHash != entry.Hash {
            tampered = append(tampered, models.TamperedEntry{
                LineNumber:     entry.LineNumber,
                StoredHash:     entry.Hash,
                CurrentHash:    currentHash,
                CurrentContent: currentContent,
            })
        }
    }

    return tampered
}
```

Integrity check обнаруживает два основных типа нарушения:

- строка была изменена: текущий hash отличается от сохранённого;
- строка отсутствует: сохранённая line number не найдена в текущем логе.

## 14. Health layer и backoff

Health service ведёт состояние доступности сервера. После успешной операции сервер становится `active`, счётчик ошибок сбрасывается, обновляется `last_seen_at`. После ошибки сервер получает статус `error`, увеличивается `failure_count`, сохраняется `last_error`, при необходимости рассчитывается `backoff_until`.

Если операция частично успешна, например часть логов собрана, а часть нет, сервер помечается как `degraded`.

Backoff нужен, чтобы scheduler не пытался постоянно работать с недоступным сервером. Оператор может сбросить backoff через API или CLI командой retry.

## 15. HTTP API

HTTP API построен на Gin. Публичные health endpoints доступны без авторизации:

- `GET /healthz`;
- `GET /readyz`;
- `GET /swagger`;
- `GET /swagger/openapi.json`.

Основные API endpoints находятся под `/api` и защищены API token:

- `GET /api/dashboard`;
- `GET /api/problems`;
- `GET /api/runtime/validation`;
- `GET /api/jobs`;
- `GET /api/jobs/{id}`;
- `GET /api/servers`;
- `GET /api/servers/{id}`;
- `POST /api/servers`;
- `PUT /api/servers/{id}`;
- `DELETE /api/servers/{id}`;
- `POST /api/servers/{id}/retry`;
- `POST /api/servers/discover`;
- `GET /api/logfiles`;
- `POST /api/logfiles/collect`;
- `GET /api/entries`;
- `GET /api/checks`;
- `POST /api/checks/run`.

Важный фрагмент маршрутизации:

```go
apiGroup := engine.Group("/api", middleware.APIKeyAuth(authToken))
{
    apiGroup.GET("/dashboard", serverHandler.Dashboard)
    apiGroup.GET("/problems", serverHandler.ListProblems)
    apiGroup.GET("/runtime/validation", runtimeHandler.Validation)
    apiGroup.GET("/jobs", jobHandler.List)
    apiGroup.GET("/jobs/:id", jobHandler.Get)

    apiGroup.GET("/servers", serverHandler.List)
    apiGroup.POST("/servers", serverHandler.Create)
    apiGroup.POST("/servers/discover", serverHandler.Discover)

    apiGroup.GET("/logfiles", logFileHandler.List)
    apiGroup.POST("/logfiles/collect", logFileHandler.Collect)

    apiGroup.GET("/entries", entryHandler.List)
    apiGroup.GET("/checks", checkHandler.List)
    apiGroup.POST("/checks/run", checkHandler.Run)
}
```

Авторизация поддерживает два способа передачи токена:

```http
X-API-Key: <token>
```

или:

```http
Authorization: Bearer <token>
```

Для защиты от медленных клиентов HTTP server настроен с `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout` и `MaxHeaderBytes`.

## 16. Асинхронные HTTP jobs

Операции discovery, collection и integrity check могут быть длительными, поэтому HTTP API не выполняет их синхронно в handler-ах. Вместо этого handler создаёт job и возвращает `202 Accepted` + `Location: /api/jobs/{id}`.

Очередь jobs:

- хранится в памяти процесса;
- имеет ограниченный размер;
- выполняется worker goroutine-ами;
- хранит историю ограниченной длины;
- поддерживает статусы `queued`, `running`, `succeeded`, `failed`, `canceled`;
- поддерживает `X-Idempotency-Key`;
- использует fingerprint для дедупликации одинаковых активных операций.

Фрагмент submit:

```go
func (m *Manager) Submit(spec TaskSpec) (*models.Job, bool, error) {
    spec.IdempotencyKey = strings.TrimSpace(spec.IdempotencyKey)
    spec.Fingerprint = strings.TrimSpace(spec.Fingerprint)

    if spec.IdempotencyKey != "" {
        if jobID, ok := m.idempotencyKeys[spec.IdempotencyKey]; ok {
            job, exists := m.jobs[jobID]
            if exists {
                return cloneJob(job), true, nil
            }
        }
    }

    if spec.Fingerprint != "" {
        if jobID, ok := m.activeFingerprints[spec.Fingerprint]; ok {
            job, exists := m.jobs[jobID]
            if exists {
                return cloneJob(job), true, nil
            }
        }
    }

    job := &models.Job{
        ID:        uuid.NewString(),
        Type:      spec.Type,
        Status:    models.JobStatusQueued,
        CreatedAt: time.Now().UTC(),
    }
}
```

Ограничение текущей реализации: job history не сохраняется в PostgreSQL и очищается после рестарта процесса.

## 17. Scheduler и фоновые задачи

В HTTP-режиме приложение запускает scheduler, который выполняет три типа фоновых задач:

- discovery;
- collection;
- integrity.

Расписания задаются в конфигурации. Scheduler поддерживает ограниченный набор cron-like выражений и `@every`.

Runner-ы используют bounded concurrency:

- параллелизм по серверам;
- параллелизм по лог-файлам на одном сервере;
- in-process per-server locks, чтобы ручные и фоновые операции не конфликтовали.

Пример: collection runner сначала получает список серверов, пропускает inactive/backoff серверы, берёт lock на сервер, получает активные log files и запускает bounded worker pool по лог-файлам.

Ограничение: lock manager работает внутри одного процесса. Для multi-replica deployment нужны distributed locks, например PostgreSQL advisory locks.

## 18. CLI

CLI построен на Cobra и использует тот же runtime, что HTTP-сервис. Операции выполняются синхронно.

Основные команды:

- `health`;
- `ready`;
- `config validate`;
- `server list/get/add/update/delete/retry`;
- `discover`;
- `logfile list`;
- `collect`;
- `entry list`;
- `check list`;
- `check run`;
- `problem list`;
- `dashboard`;
- `runtime validation`.

CLI поддерживает вывод в `table` и `json`, что удобно как для ручной работы, так и для автоматизации.

## 19. Безопасность

В проекте реализованы следующие меры:

1. HTTP API token.
2. Явная защита от случайного запуска HTTP API без токена.
3. SSH host key policy:
   - строгий режим через `known_hosts`;
   - insecure mode только при явном включении.
4. Таймауты SSH-подключения и выполнения команд.
5. Шифрование `auth_value` в PostgreSQL через AES-GCM.
6. HMAC-SHA256 для hash строк при наличии `integrity_hmac_key`.
7. Возможность не хранить raw content логов.
8. Readiness и runtime validation для диагностики небезопасных режимов.

Фрагмент HMAC:

```go
func HashString(value, key string) string {
    if key == "" {
        return SHA256String(value)
    }
    return HMACSHA256String(value, key)
}
```

Фрагмент шифрования секретов:

```go
func (c *StringCipher) Encrypt(value string) (string, error) {
    if c == nil || value == "" || strings.HasPrefix(value, encryptedStringPrefix) {
        return value, nil
    }

    nonce := make([]byte, c.gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    ciphertext := c.gcm.Seal(nil, nonce, []byte(value), nil)

    payload := append(nonce, ciphertext...)
    return encryptedStringPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}
```

## 20. Хранилище и миграции

PostgreSQL schema включает таблицы:

- `servers`;
- `log_files`;
- `log_entries`;
- `log_chunks`;
- `check_results`.

Связи:

- `log_files.server_id` ссылается на `servers.id`;
- `log_entries.log_file_id` ссылается на `log_files.id`;
- `log_chunks.log_file_id` ссылается на `log_files.id`;
- `check_results.log_file_id` ссылается на `log_files.id`;
- зависимости удаляются каскадно.

Индексы:

- уникальный индекс на `LOWER(name)` для серверов;
- уникальный индекс на `LOWER(host)` для серверов;
- уникальный индекс на `(server_id, path)` для лог-файлов;
- уникальный индекс на `(log_file_id, line_number)` для entries;
- уникальный индекс на `(log_file_id, chunk_number)` для chunks;
- индексы для active log files, ranges, latest checks и backoff.

## 21. Типовые сценарии работы

### Сценарий 1: старт HTTP-сервиса

1. Пользователь задаёт `LOGMONITOR_APP_MODE=HTTP`.
2. `cmd/server` проверяет режим.
3. Загружается YAML-конфигурация.
4. Применяются env placeholders и defaults.
5. Выполняется validation.
6. Собирается runtime.
7. Создаётся repository.
8. Запускаются migrations, если используется PostgreSQL.
9. Создаются сервисы и SSH factory.
10. Сидируются config-managed серверы.
11. Создаётся jobs manager.
12. Регистрируются scheduler jobs.
13. Стартуют workers и HTTP server.

### Сценарий 2: discovery

1. Оператор вызывает API/CLI или scheduler запускает задачу.
2. Сервис выбирает сервер или все серверы.
3. Проверяется backoff/inactive статус.
4. Берётся per-server lock.
5. Через SSH определяется ОС.
6. Запускается ОС-специфичный discoverer.
7. Найденные пути классифицируются.
8. Repository синхронизируется с новым списком.
9. Health status обновляется.

### Сценарий 3: collection

1. Сервис выбирает server и log file(s).
2. Подключается по SSH.
3. Проверяет identity источника.
4. При ротации очищает старые entries/chunks.
5. Читает строки после последнего сохранённого номера.
6. Вычисляет hash/HMAC.
7. Сохраняет entries и chunks.
8. Обновляет metadata лог-файла.
9. Обновляет health status.

### Сценарий 4: integrity check

1. Сервис читает текущий лог по SSH.
2. Проверяет, не изменилась ли identity файла.
3. Сравнивает текущие hash со stored hash.
4. Использует chunks для ускорения проверки.
5. Формирует список tampered entries.
6. Сохраняет CheckResult.
7. Обновляет health/degraded status.

## 22. Тестирование

В проекте есть unit-тесты сервисного слоя, cron runner-ов, locks, config validation, job behavior и repository logic. Для PostgreSQL предусмотрены integration tests через testcontainers.

Тестируемые сценарии включают:

- сохранение entries и chunks collector-ом;
- пропуск уже сохранённых строк;
- reset после ротации файла;
- обработку ошибок SSH/read;
- integrity OK;
- обнаружение tampering;
- ускоренную проверку через chunks;
- проверку строк вне покрытия chunks;
- работу с разреженными line numbers;
- ошибку integrity при изменении source identity;
- server CRUD и unique constraints;
- health backoff;
- async jobs и idempotency;
- runtime config validation.

Команды проверки:

```bash
go test ./...
go build ./...
go vet ./...
golangci-lint run
```

## 23. Docker и эксплуатация

Проект содержит:

- `Dockerfile` для сборки HTTP server binary;
- `docker-compose.yaml` для запуска app + PostgreSQL;
- `config.docker.yaml` с env placeholders;
- `.env.example` с основными переменными.

Docker image собирается как multi-stage:

1. builder на `golang:1.26.2-alpine`;
2. runtime на `alpine`;
3. установка `openssh-client`;
4. запуск от отдельного пользователя `logmonitor`;
5. копирование binary, config и migrations.

## 24. Нефункциональные характеристики

Проект учитывает:

- расширяемость: новые ОС можно добавить через новый `Discoverer`;
- тестируемость: сервисы зависят от interfaces;
- надёжность: health/backoff, readiness, timeouts;
- безопасность: token auth, HMAC, AES-GCM, known_hosts policy;
- производительность: batch insert, CopyFrom, chunks, bounded concurrency;
- сопровождаемость: явное разделение transport/service/repository;
- эксплуатационность: Docker Compose, migrations, runtime validation, Swagger.

## 25. Ограничения текущей реализации

Реализованные ограничения, которые можно честно указать в ВКР:

- веб-интерфейс пока отсутствует;
- job history хранится в памяти процесса и очищается после рестарта;
- in-process locks не защищают от конфликтов при запуске нескольких replica;
- scheduler поддерживает ограниченный набор cron-like выражений;
- raw log content не шифруется отдельно, поэтому по умолчанию его хранение выключено;
- alerting/уведомления не реализованы;
- полноценная ролевая модель доступа не реализована, используется API token;
- нет distributed tracing/metrics;
- collection reset и обновление metadata можно дополнительно усилить общей транзакцией на уровне repository.

## 26. Возможные направления развития

Для дальнейшего развития можно предложить:

- веб-интерфейс для dashboard, problems, jobs, servers и log files;
- persisted job history в PostgreSQL;
- distributed locks для multi-instance deployment;
- RBAC и пользователи вместо одного API token;
- TLS/mTLS на уровне HTTP API;
- Prometheus metrics;
- audit log действий оператора;
- alerting в Telegram/email/webhook при tampering;
- расширение discoverers и поддержка пользовательских путей логов;
- более гибкий cron parser;
- отдельное шифрование raw log content;
- экспорт отчётов по integrity checks.

## 27. Что важно подчеркнуть в тексте ВКР

При написании ВКР подчеркни следующие идеи:

1. Проект предназначен для контроля целостности журналов на удалённых серверах без установки агента.
2. Архитектура разделяет транспорт, бизнес-логику и storage.
3. SSH вынесен за интерфейс, что упрощает тестирование.
4. Repository abstraction позволяет переключаться между PostgreSQL и in-memory storage.
5. Integrity реализован через hash/HMAC отдельных строк и aggregate chunks.
6. Ротация логов обрабатывается через file identity.
7. HTTP API использует async jobs для долгих операций.
8. Scheduler выполняет регулярные проверки с ограниченным параллелизмом.
9. Health/backoff защищает систему от постоянных повторов на недоступных серверах.
10. Security defaults ориентированы на production: token auth обязателен, raw logs не сохраняются по умолчанию.

## 28. Предложенная структура глав ВКР

Можно построить текст ВКР так:

1. **Введение**
   - актуальность контроля журналов;
   - проблема изменения/удаления логов;
   - цель и задачи работы;
   - объект и предмет исследования;
   - практическая значимость.

2. **Анализ предметной области**
   - журналы событий и их роль;
   - угрозы целостности логов;
   - подходы к мониторингу;
   - требования к системе;
   - обоснование agentless-подхода через SSH.

3. **Проектирование системы**
   - функциональные требования;
   - нефункциональные требования;
   - архитектура;
   - модель данных;
   - сценарии использования;
   - алгоритмы discovery, collection, integrity.

4. **Реализация**
   - структура проекта;
   - runtime;
   - service layer;
   - repository layer;
   - HTTP API;
   - CLI;
   - scheduler;
   - безопасность.

5. **Тестирование и эксплуатация**
   - unit-тесты;
   - интеграционные тесты PostgreSQL;
   - Docker Compose;
   - проверки качества;
   - ограничения и будущие улучшения.

6. **Заключение**
   - достигнутые результаты;
   - соответствие поставленным задачам;
   - направления развития.

## 29. Пример формулировки цели и задач

Цель работы: разработать серверное приложение для централизованного мониторинга и контроля целостности журналов событий на удалённых серверах с использованием SSH, хэширования и периодических проверок.

Задачи:

- проанализировать способы мониторинга и контроля целостности журналов;
- разработать архитектуру приложения с разделением транспортного, сервисного и инфраструктурного слоёв;
- реализовать механизм подключения к удалённым серверам по SSH;
- реализовать обнаружение журналов для Linux, Windows и macOS;
- реализовать инкрементальный сбор строк журналов;
- реализовать хранение метаданных, hash-значений и результатов проверок;
- реализовать алгоритм проверки целостности на основе hash/HMAC и агрегированных chunks;
- реализовать HTTP API, CLI и фоновые задачи;
- обеспечить базовые механизмы безопасности и диагностики;
- провести тестирование разработанной системы.

## 30. Краткое резюме проекта для аннотации

В рамках работы разработано приложение LogMonitor, предназначенное для централизованного мониторинга и контроля целостности журналов событий на удалённых серверах. Система подключается к серверам по SSH, обнаруживает журналы, выполняет инкрементальный сбор новых строк, сохраняет их контрольные значения и периодически проверяет, были ли ранее собранные записи изменены или удалены. Приложение поддерживает HTTP API, CLI-интерфейс, фоновые задачи по расписанию, PostgreSQL-хранилище, in-memory режим для разработки, а также механизмы защиты секретов и диагностики состояния. Архитектура проекта построена с разделением на транспортный, сервисный и репозиторный слои, что упрощает сопровождение, тестирование и дальнейшее развитие системы.

