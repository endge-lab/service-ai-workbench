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

## Возможности v0.2.0

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
- hardcoded streaming adapters `anthropic` и `ollama` без внешних API-вызовов;
- `/health` и `/version`;
- Swagger/Scalar в non-production окружениях;
- единый JSON-формат ошибок;
- базовые architecture checks из service template.

Canonical contract хранится в `api/proto/endge/ai/workbench/v1/workbench.proto`.
После регенерации server/client stubs обновляются обе копии
`workbench.v1.sha256`, а `../verify-workbench-contract.sh` проверяет отсутствие
расхождения с backend client stubs.

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
