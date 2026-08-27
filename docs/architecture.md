# Endge AI Workbench Architecture

## Boundary

```text
Configurator --HTTP/SSE--> service-backend --gRPC/OIDC--> AI Workbench
```

Backend owns user authorization, AI catalog, encrypted credentials and `ExportLive`. Workbench never opens the backend database and never mutates an Endge domain.

For an authorized run, backend decrypts the selected connection credential and sends ephemeral provider access over the authenticated gRPC call. Workbench keeps it in memory only and excludes it from persistence, debug artifacts, telemetry and client-visible errors.

Production gRPC transport must use TLS in addition to service identity authentication. Plaintext gRPC is a development-only mode.

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

`anthropic` and `ollama` implement the same streaming generator port. Anthropic remains deterministic and hardcoded. Ollama calls the native streaming `/api/chat` endpoint, forwards a non-empty credential as a Bearer token and emits only assistant content chunks.

Ollama outbound requests do not follow redirects, require HTTPS by default and reject targets resolving to private or reserved networks. Plain HTTP and private-network targets require explicit development-only settings.

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
- Both adapters receive the assembled provider-neutral request; only Ollama performs an external model call at this stage.
- Debug artifacts are best-effort, disabled by default and stored only under a Git-ignored runtime directory.

Vector RAG, tools, revisions and domain mutations remain explicit non-goals.
