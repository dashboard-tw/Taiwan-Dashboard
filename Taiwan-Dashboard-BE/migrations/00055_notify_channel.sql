-- +goose Up
-- 推播頻道：Discord、Slack、Telegram 或一般 JSON webhook。網址與 token 加密保存。
CREATE TABLE app.notify_channel (
    id           bigserial PRIMARY KEY,
    name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
    kind         text NOT NULL CHECK (kind IN ('discord', 'slack', 'telegram', 'json')),
    target_enc   bytea NOT NULL,
    target_hint  text NOT NULL DEFAULT '' CHECK (char_length(target_hint) <= 80),
    events       text[] NOT NULL DEFAULT '{}',
    enabled      boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.notify_outbox (
    id              bigserial PRIMARY KEY,
    channel_id      bigint NOT NULL REFERENCES app.notify_channel (id) ON DELETE CASCADE,
    event           text NOT NULL,
    message         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'sending', 'sent', 'failed')),
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_at       timestamptz,
    last_error      text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 500),
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);

CREATE INDEX notify_outbox_due_idx ON app.notify_outbox (next_attempt_at) WHERE status = 'pending';
CREATE INDEX notify_outbox_channel_idx ON app.notify_outbox (channel_id, id DESC);

-- +goose Down
DROP TABLE app.notify_outbox;
DROP TABLE app.notify_channel;
