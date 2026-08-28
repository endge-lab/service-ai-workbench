-- +goose Up
ALTER TABLE interactions
    DROP CONSTRAINT interactions_root_message_id_fkey,
    ADD CONSTRAINT interactions_root_message_id_fkey
        FOREIGN KEY (root_message_id) REFERENCES messages(id) ON DELETE CASCADE;

ALTER TABLE clarifications
    DROP CONSTRAINT clarifications_question_message_id_fkey,
    ADD CONSTRAINT clarifications_question_message_id_fkey
        FOREIGN KEY (question_message_id) REFERENCES messages(id) ON DELETE CASCADE,
    DROP CONSTRAINT clarifications_answer_message_id_fkey,
    ADD CONSTRAINT clarifications_answer_message_id_fkey
        FOREIGN KEY (answer_message_id) REFERENCES messages(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE clarifications
    DROP CONSTRAINT clarifications_answer_message_id_fkey,
    ADD CONSTRAINT clarifications_answer_message_id_fkey
        FOREIGN KEY (answer_message_id) REFERENCES messages(id) ON DELETE RESTRICT,
    DROP CONSTRAINT clarifications_question_message_id_fkey,
    ADD CONSTRAINT clarifications_question_message_id_fkey
        FOREIGN KEY (question_message_id) REFERENCES messages(id) ON DELETE RESTRICT;

ALTER TABLE interactions
    DROP CONSTRAINT interactions_root_message_id_fkey,
    ADD CONSTRAINT interactions_root_message_id_fkey
        FOREIGN KEY (root_message_id) REFERENCES messages(id) ON DELETE RESTRICT;
