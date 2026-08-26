# Endge AI Workbench

`service-ai-workbench` — приватный backend-сервис для будущей AI-подсистемы Endge.

Сейчас репозиторий содержит только инфраструктурный Go-skeleton на основе
`service-template-go` и `service-kit-go`. LLM, RAG, agent runs, инструменты и
бизнесовые HTTP endpoints пока не реализованы.

## Граница сервиса

Целевой поток запросов:

```text
Configurator -> service-backend -> service-ai-workbench
```

Configurator не должен обращаться к Workbench напрямую. Пользовательскую
аутентификацию, workspace-права и запись версионированных изменений сохраняет
основной backend. Workbench будет отвечать за AI orchestration в рамках явно
авторизованного запуска.

## Что уже есть

- HTTP на Fiber;
- dependency injection через `fx`;
- config и общие runtime-компоненты из `service-kit-go`;
- structured logging;
- optional OpenTelemetry;
- optional JWT/JWKS middleware;
- optional PostgreSQL и Redpanda configuration;
- `/health` и `/version`;
- Swagger/Scalar в non-production окружениях;
- единый JSON-формат ошибок;
- базовые architecture checks из service template.

## Локальный запуск

Требуется Go версии из `go.mod`.

```bash
cp .env.development.example .env.development
make run
```

По умолчанию сервис слушает `http://localhost:8081`:

```text
GET http://localhost:8081/health
GET http://localhost:8081/version
GET http://localhost:8081/swagger
GET http://localhost:8081/swagger/openapi3.yaml
```

Основной backend может использовать внутренний адрес Workbench, например:

```text
http://localhost:8081
```

Точный backend contract будет добавлен вместе с первой read-only AI-функцией.

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

PostgreSQL, auth, telemetry и Redpanda выключены или не используются, пока у
сервиса нет соответствующего business flow. Обязательный CORS allowlist указывает
на зарезервированный `https://cors-disabled.invalid`, поэтому browser-клиенты не
получают прямой доступ к Workbench.

## Архитектура

Правила и будущие business layers описаны в [docs/architecture.md](docs/architecture.md).

Бизнесовые endpoints добавляются под `/api/v1`. Usecase не должен импортировать
HTTP или concrete PostgreSQL implementation, а HTTP handlers не должны работать
с repository напрямую.

## Проверки

Команды проекта:

```bash
go test ./...
docker compose --env-file .env.development config
```

Запуск проверок выполняется отдельно в соответствии с правилами конкретной
задачи. Наличие команд в README не означает их автоматический запуск.
