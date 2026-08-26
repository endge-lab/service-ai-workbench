# Test Strategy

Сервис пока содержит только инфраструктурный smoke-level набор тестов.

## Что проверяется сейчас

- package naming;
- базовые architecture boundaries;
- общий error contract;
- Redpanda client wrapper в disabled/enabled режимах;
- наличие `docs/openapi3.yaml`;
- компиляция всех packages.

## Что добавляется вместе с бизнес-логикой

- unit tests для domain/usecase;
- repository integration tests;
- HTTP contract tests;
- e2e tests для основных user flows;
- migration tests для реальных бизнес-таблиц.
