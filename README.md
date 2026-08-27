# Endge AI Workbench

`service-ai-workbench` — приватный gRPC backend-сервис AI-подсистемы Endge.

## Граница сервиса

Целевой поток запросов:

```text
Configurator -> service-backend -> service-ai-workbench
```

Configurator не должен обращаться к Workbench напрямую. Пользовательскую
аутентификацию, workspace-права и запись версионированных изменений сохраняет
основной backend. Workbench будет отвечать за AI orchestration в рамках явно
авторизованного запуска.

## Возможности v0.5.0

- HTTP на Fiber;
- dependency injection через `fx`;
- config и общие runtime-компоненты из `service-kit-go`;
- structured logging;
- optional OpenTelemetry;
- optional JWT/JWKS middleware;
- PostgreSQL migrations для projections, conversations, messages и runs;
- canonical `workbench.v1` protobuf и standard gRPC Health;
- service-to-service OIDC authentication;
- opaque cursor pagination, atomic reset и один active run на conversation;
- реальный streaming adapter Ollama через native `/api/chat` и hardcoded adapter Anthropic;
- локальный `endge-knowledge/v1` bundle и детерминированный поиск по публичной документации;
- отключённая по умолчанию запись этапов retrieval в `tmp/debug`;
- `/health` и `/version`;
- Swagger/Scalar в non-production окружениях;
- единый JSON-формат ошибок;
- базовые architecture checks из service template.

Canonical contract хранится в `api/proto/endge/ai/workbench/v1/workbench.proto`.
После регенерации server/client stubs обновляются обе копии
`workbench.v1.sha256`, а `../verify-workbench-contract.sh` проверяет отсутствие
расхождения с backend client stubs.

Единственный источник версии сервиса — корневой файл `VERSION`. `make`, Air и
Docker встраивают его значение в бинарник; YAML и environment не владеют
версией приложения. HTTP `/health`, `/version` и gRPC `GetServiceInfo` возвращают
одну и ту же встроенную версию.

## Локальный запуск

Требуется Go версии из `go.mod`.

```bash
cp .env.development.example .env.development
make run
```

По умолчанию HTTP health слушает `8081`, gRPC — `50051`:

```text
GET http://localhost:8081/health
GET http://localhost:8081/version
GET http://localhost:8081/swagger
GET http://localhost:8081/swagger/openapi3.yaml
```

Основной backend использует `localhost:50051` или service DNS внутри compose.

## Docker

```bash
cp .env.development.example .env.development
make up
```

Остановка:

```bash
make down
```

## Конфигурация

`service-kit-go` загружает `.env.*`, затем читает YAML из
`configs/<APP_ENV>.yaml`. Environment variables имеют приоритет над YAML.

PostgreSQL и service identity обязательны для штатного v1 flow. Обязательный CORS allowlist указывает
на зарезервированный `https://cors-disabled.invalid`, поэтому browser-клиенты не
получают прямой доступ к Workbench.

### Локальная документация и retrieval debug

Workbench не читает Git-репозиторий документации напрямую. Сначала в приложении
`egorkozelskij-endge-web-configurator-docs` нужно выполнить:

```bash
pnpm knowledge:build
```

Затем передать путь к созданному каталогу `dist/knowledge`:

```env
AI_KNOWLEDGE_BUNDLE_PATH=/absolute/path/to/dist/knowledge
AI_KNOWLEDGE_MAX_RESULTS=8
AI_DOMAIN_CONTEXT_MAX_RESULTS=20
AI_CONVERSATION_CONTEXT_MESSAGE_LIMIT=10
AI_MODEL_CONTEXT_MAX_CHARS=24000
AI_DEBUG=true
AI_DEBUG_OUTPUT_PATH=tmp/debug
AI_OLLAMA_REQUEST_TIMEOUT=2m
AI_OLLAMA_MAX_RESPONSE_BYTES=8388608
AI_OLLAMA_ALLOW_PRIVATE_NETWORK=false
AI_OLLAMA_ALLOW_INSECURE_HTTP=false
```

При `AI_DEBUG=true` каждый run создаёт каталог
`tmp/debug/<conversation-id>/<UTC-time>_<request-id>/`. Файлы имеют числовые
префиксы `00`–`07`: metadata, prompt, поисковые выражения, документация,
выбранный контекст актуального Workspace, последние сообщения, план бюджета и
точный provider-neutral `ModelRequest`. Текущий prompt не дублируется в истории.
При `AI_DEBUG=false` файловая система debug recorder-ом не изменяется. `tmp/`
целиком исключён из Git.

### Ollama

Для Ollama Cloud connection использует base URL `https://ollama.com`; adapter
сам добавляет `/api/chat` и передаёт сохранённый credential как Bearer token.
Для локального Ollama без credential можно указать, например,
`http://host.docker.internal:11434`, но только при `APP_ENV=development` и двух
явных флагах `AI_OLLAMA_ALLOW_PRIVATE_NETWORK=true` и
`AI_OLLAMA_ALLOW_INSECURE_HTTP=true`. В production эти флаги запрещены.

Credential расшифровывается backend только после проверки доступа и передаётся
внутри одного gRPC `Run`. Workbench не записывает его в PostgreSQL, debug,
telemetry или сообщения об ошибках. В production gRPC server требует TLS;
plaintext transport остаётся только для локального development.

## Архитектура

Правила и будущие business layers описаны в [docs/architecture.md](docs/architecture.md).

Workbench не публикует browser business API. Usecase не импортирует gRPC или concrete PostgreSQL implementation, а gRPC handlers не работают с repository напрямую.

## Проверки

Команды проекта:

```bash
go test ./...
docker compose --env-file .env.development config
```

Запуск проверок выполняется отдельно в соответствии с правилами конкретной
задачи. Наличие команд в README не означает их автоматический запуск.
