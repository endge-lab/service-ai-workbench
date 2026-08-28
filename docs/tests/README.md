# Test Strategy

Сервис содержит unit, contract и PostgreSQL integration coverage для v1 flow.

## Что проверяется сейчас

- package naming;
- базовые architecture boundaries;
- общий error contract;
- Redpanda client wrapper в disabled/enabled режимах;
- наличие `docs/openapi3.yaml`;
- компиляция всех packages;
- deterministic routing evaluation для documentation/domain/mixed intent;
- отсутствие ложных history references на границах слов;
- typed и folder-scoped domain selection;
- fuzzy candidate recall без автоматического выбора;
- compact list context и безопасная compaction больших JSON-документов;
- морфологический ranking документации;
- native Ollama structured-output contract;
- strict response validation и closed citations;
- Interaction/clarification persistence на PostgreSQL.

## Уровни проверки

- `go test ./...` — unit, architecture и contract;
- `AI_WORKBENCH_TEST_DATABASE_URL=... go test ./internal/repo/postgres -run TestInteractionRepositoryIntegration` — PostgreSQL integration в уникальной временной schema;
- локальный stack smoke — реальный backend, Workbench, Keycloak, documentation bundle и Ollama;
- browser evaluation — поддерживаемые documentation, exact, fuzzy, list, folder и clarification flows.

Evaluation fixtures используют только синтетические identity и display name. Успешный unit suite не заменяет PostgreSQL или browser evidence.
