-- +goose Up
ALTER TABLE app.user_identity DROP CONSTRAINT user_identity_provider_check;
ALTER TABLE app.user_identity ADD CONSTRAINT user_identity_provider_check
    CHECK (provider IN ('dev', 'apple', 'gov', 'github', 'google'));

CREATE TABLE app.oauth_login (
    state_hash     bytea PRIMARY KEY CHECK (length(state_hash) = 32),
    browser_hash   bytea NOT NULL CHECK (length(browser_hash) = 32),
    provider       text NOT NULL,
    code_verifier  text NOT NULL,
    redirect_uri   text NOT NULL,                   -- 送給提供者的回呼網址，換 token 時必須相同
    return_to      text NOT NULL,                   -- 登入後回到本站的路徑
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL
);
CREATE INDEX oauth_login_expires_idx ON app.oauth_login (expires_at);

-- +goose Down
DROP TABLE app.oauth_login;
DELETE FROM app.user_identity WHERE provider IN ('github', 'google');
ALTER TABLE app.user_identity DROP CONSTRAINT user_identity_provider_check;
ALTER TABLE app.user_identity ADD CONSTRAINT user_identity_provider_check
    CHECK (provider IN ('dev', 'apple', 'gov'));
