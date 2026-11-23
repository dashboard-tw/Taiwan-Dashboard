-- +goose Up
CREATE TABLE app.mail_queue (
    id              bigserial PRIMARY KEY,
    template        text NOT NULL,
    user_id         bigint NOT NULL,
    email_enc       bytea,
    data            jsonb NOT NULL DEFAULT '{}',
    merge_key       text,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'skipped')),
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_at       timestamptz,
    last_error      text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 500),
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);

CREATE INDEX mail_queue_due_idx ON app.mail_queue (next_attempt_at) WHERE status = 'pending';
CREATE UNIQUE INDEX mail_queue_merge_idx ON app.mail_queue (merge_key) WHERE status = 'pending' AND merge_key IS NOT NULL;
CREATE INDEX mail_queue_sent_idx ON app.mail_queue (sent_at) WHERE status = 'sent';

ALTER TABLE app.mail_queue ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.mail_queue FORCE ROW LEVEL SECURITY;
CREATE POLICY owner_only ON app.mail_queue
    USING (app.rls_bypass() OR user_id = app.current_user_id());

-- +goose Down
DROP TABLE app.mail_queue;
