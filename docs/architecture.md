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

## Knowledge retrieval debug

- Configurator documentation owns the immutable `endge-knowledge/v1` bundle.
- Workbench loads a local bundle path and never reads the documentation Git repository.
- Initial retrieval is deterministic lexical ranking without embeddings or a vector database.
- The same normalized query selects matching documents from the trusted `ExportLive` snapshot and then adds one-hop documents that reference a selected identity.
- Domain context always carries sanitized Workspace metadata and installed integrations; credential-like fields are redacted before debug output.
- Conversation context takes the latest messages whose monotonic sequence is lower than the current user message sequence, so the current prompt cannot be duplicated.
- A deterministic assembler classifies only an intent hint, removes exact duplicate blocks and allocates a character budget between recent complete conversation turns, domain and documentation.
- The assembler produces one provider-neutral `ModelRequest`: a fixed safety/system prompt, selected history and a structured final user message containing trusted context plus the current request.
- Character count is the enforced deterministic budget. Token count in debug output is explicitly an approximation until provider-specific tokenizers are introduced.
- Both hardcoded adapters receive the assembled request but still do not perform external model calls.
- Debug artifacts are best-effort, disabled by default and stored only under a Git-ignored runtime directory.

Vector RAG, tools, revisions and domain mutations remain explicit non-goals.
