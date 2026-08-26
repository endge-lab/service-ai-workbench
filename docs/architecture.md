# Endge AI Workbench Architecture

## Boundary

```text
Configurator --HTTP/SSE--> service-backend --gRPC/OIDC--> AI Workbench
```

Backend owns user authorization, AI catalog, encrypted credentials and `ExportLive`. Workbench never opens the backend database and never mutates an Endge domain.

## Contract and layers

- Canonical proto: `api/proto/endge/ai/workbench/v1/workbench.proto`.
- gRPC adapter: `internal/api/grpc/v1`.
- Orchestration: `internal/usecase/workbench` through repository and generator ports.
- Persistence: `internal/repo/postgres`.
- Provider adapters: `internal/adapters/llm/{anthropic,ollama}`.

Backend generates client stubs from the canonical proto but does not import this application module.

## Persistence invariants

- `users` and `workspaces` are lazy projections updated by trusted requests.
- There is one active conversation per `actor_id + workspace_id`.
- Conversation stores a model snapshot and has no FK to the backend catalog.
- Message cursor is monotonic and pagination returns at most 50 messages.
- Unique `request_id` and a partial unique index allow one running run per conversation.
- User message is committed at run start. Assistant message is committed only with successful completion; partial output is not history.

## v1 adapters

`anthropic` and `ollama` implement the same generator port. Both return deterministic hardcoded chunks and never use credentials or external APIs in v1.

RAG, embeddings, tools, revisions and domain mutations are explicit non-goals.
