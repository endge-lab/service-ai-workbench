-- +goose Up
CREATE TABLE interactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    root_message_id UUID NOT NULL UNIQUE REFERENCES messages(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN (
        'planning', 'resolving', 'awaiting_clarification', 'ready', 'generating',
        'completed', 'failed', 'cancelled', 'superseded'
    )),
    plan JSONB NOT NULL DEFAULT '{}'::jsonb,
    plan_version INTEGER NOT NULL DEFAULT 1 CHECK (plan_version > 0),
    workspace_generation TEXT NOT NULL DEFAULT '',
    workspace_snapshot_sha256 TEXT NOT NULL DEFAULT '',
    documentation_version TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX interactions_one_active_idx
    ON interactions(conversation_id)
    WHERE status IN ('planning', 'resolving', 'awaiting_clarification', 'ready', 'generating');

CREATE TABLE clarifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interaction_id UUID NOT NULL REFERENCES interactions(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    slot TEXT NOT NULL,
    question_message_id UUID NOT NULL UNIQUE REFERENCES messages(id) ON DELETE RESTRICT,
    answer_message_id UUID UNIQUE REFERENCES messages(id) ON DELETE RESTRICT,
    candidate_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('open', 'answered', 'cancelled', 'superseded')),
    plan_version INTEGER NOT NULL CHECK (plan_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX clarifications_one_open_idx
    ON clarifications(interaction_id)
    WHERE status = 'open';

ALTER TABLE runs
    ADD COLUMN interaction_id UUID REFERENCES interactions(id) ON DELETE SET NULL;

ALTER TABLE runs DROP CONSTRAINT runs_status_check;
ALTER TABLE runs ADD CONSTRAINT runs_status_check
    CHECK (status IN ('running', 'completed', 'clarification_required', 'failed', 'cancelled'));

-- +goose Down
ALTER TABLE runs DROP CONSTRAINT runs_status_check;
ALTER TABLE runs ADD CONSTRAINT runs_status_check
    CHECK (status IN ('running', 'completed', 'failed', 'cancelled'));
ALTER TABLE runs DROP COLUMN interaction_id;
DROP TABLE clarifications;
DROP TABLE interactions;
