-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id TEXT NOT NULL REFERENCES users(id),
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    model_profile_id UUID NOT NULL,
    connection_id UUID NOT NULL,
    adapter TEXT NOT NULL CHECK (adapter IN ('anthropic', 'ollama')),
    provider_model_id TEXT NOT NULL,
    model_display_name TEXT NOT NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX conversations_one_active_idx
    ON conversations(actor_id, workspace_id)
    WHERE archived_at IS NULL;
CREATE INDEX conversations_history_idx
    ON conversations(actor_id, workspace_id, updated_at DESC, id DESC);

CREATE TABLE messages (
    sequence BIGSERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX messages_conversation_sequence_idx
    ON messages(conversation_id, sequence DESC);

CREATE TABLE runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL UNIQUE,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id),
    adapter TEXT NOT NULL CHECK (adapter IN ('anthropic', 'ollama')),
    model_profile_id UUID NOT NULL,
    provider_model_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'cancelled')),
    snapshot_generation TEXT NOT NULL,
    snapshot_head_revision_id TEXT NOT NULL DEFAULT '',
    snapshot_sha256 TEXT NOT NULL,
    snapshot_size BIGINT NOT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX runs_one_active_idx
    ON runs(conversation_id)
    WHERE status = 'running';

-- +goose Down
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS users;
