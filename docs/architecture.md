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
- Application entrypoint: `internal/usecase/workbench`; conversation queries, run lifecycle and streaming orchestration остаются файлами одного package.
- Logical request lifecycle: `internal/usecase/interaction`.
- Gated preparation pipeline and response validation: `internal/usecase/preparation`.
- Prompt catalog: strict embedded templates in `internal/adapters/prompts/embedded`, resolved by typed stable IDs.
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
- One active Interaction and one open clarification are allowed per conversation. Clarification answer and plan patch are persisted atomically with optimistic plan version validation.

## v1 adapters

`anthropic` and `ollama` implement the same streaming generator port. Anthropic remains deterministic and hardcoded. Ollama calls the native streaming `/api/chat` endpoint, forwards a non-empty credential as a Bearer token and emits only assistant content chunks.

Ollama outbound requests do not follow redirects, require HTTPS by default and reject targets resolving to private or reserved networks. Plain HTTP and private-network targets require explicit development-only settings.

## Preparation pipeline

- Configurator documentation owns the immutable `endge-knowledge/v1` bundle.
- Workbench loads a local bundle path and never reads the documentation Git repository.
- Initial retrieval is deterministic lexical ranking without embeddings or a vector database.
- The same normalized query selects matching documents from the trusted `ExportLive` snapshot and then adds one-hop documents that reference a selected identity.
- Domain context always carries sanitized Workspace metadata and installed integrations; credential-like fields are redacted before debug output.
- Conversation context takes the latest messages whose monotonic sequence is lower than the current user message sequence, so the current prompt cannot be duplicated.
- The implemented intent registry contains `explain_documentation`, `find_entity`, `inspect_entity` and `list_entities`, with a dedicated folder-scope dependency.
- Planner, Query Expander, Reranker and Clarification Classifier use the selected conversation model only at their gates and share a three-call per-turn budget.
- Exact identity and normalized display-name matches are deterministic. Non-exact candidates are closed, capped at five and require a validated Reranker decision with sufficient confidence.
- Context Adequacy Gate runs before the assembler and verifies mandatory context against the character budget.
- The assembler produces one provider-neutral structured `ModelRequest` from resolved context and embedded prompts.
- Final provider output is buffered. Only a schema-valid response with confirmed identity/documentation citations and no mutation claim becomes SSE deltas and an assistant message.
- Character count is the enforced deterministic budget until provider-specific tokenizers are introduced.
- Both adapters receive the assembled provider-neutral request; only Ollama performs an external model call at this stage.
- Debug artifacts `00`–`12` are best-effort, disabled by default and stored only under a Git-ignored runtime directory. Skipped stages are explicit; prompt metadata contains ID/hash but no credential.

Vector RAG, tools, revisions and domain mutations remain explicit non-goals.
