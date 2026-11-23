-- +goose Up
-- +goose StatementBegin
INSERT INTO app.mail_template (key, name, sort, subject, body) VALUES
('api_key_created', '建立 API 憑證', 62, '已建立 API 憑證：{{.Key}}', $$
{{.Name}} 您好：

您在 {{.Time}} 建立了 API 憑證「{{.Key}}」。

Client ID：{{.ClientID}}

如果不是您本人操作，請到會員中心刪除這組憑證：
{{.Site}}/me/api-key
$$),
('api_key_disabled', '停用 API 憑證', 64, '已停用 API 憑證：{{.Key}}', $$
{{.Name}} 您好：

您在 {{.Time}} 停用了 API 憑證「{{.Key}}」，使用這組憑證的程式會無法呼叫 API。

要恢復使用，可以到會員中心重新啟用：
{{.Site}}/me/api-key
$$),
('api_key_deleted', '刪除 API 憑證', 66, '已刪除 API 憑證：{{.Key}}', $$
{{.Name}} 您好：

您在 {{.Time}} 刪除了 API 憑證「{{.Key}}」，這組憑證無法再使用。

需要的話，可以到會員中心建立新的憑證：
{{.Site}}/me/api-key
$$),
('api_key_revoked', 'API 憑證被停用', 68, '您的 API 憑證已被停用', $$
{{.Name}} 您好：

管理員在 {{.Time}} 停用了您的 API 憑證「{{.Key}}」，這組憑證無法再使用。

如有疑問，請來信 support@dashboard.tw。
$$);
-- +goose StatementEnd

UPDATE app.mail_template SET body = btrim(body, E'\n') WHERE key LIKE 'api_key_%';

-- +goose Down
DELETE FROM app.mail_template WHERE key IN ('api_key_created', 'api_key_disabled', 'api_key_deleted', 'api_key_revoked');
