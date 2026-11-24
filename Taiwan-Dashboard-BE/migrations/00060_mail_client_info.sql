-- +goose Up
-- +goose StatementBegin
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您的帳號剛剛在一個新裝置上登入。

時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果是您本人，可以不用理會這封信。
如果不是，請到下面的頁面登出這個裝置，並檢查 GitHub 或 Google 帳號的安全：
{{.Site}}/me/devices
$$, updated_at = now() WHERE key = 'new_device';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您的帳號新增了 {{.Provider}} 登入。之後用 {{.Provider}} 也能登入同一個帳號。

時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果不是您本人操作，請到會員中心移除：
{{.Site}}/me
$$, updated_at = now() WHERE key = 'link';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您的帳號移除了 {{.Provider}} 登入。之後無法再用 {{.Provider}} 登入這個帳號。

時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果不是您本人操作，請盡快登入會員中心確認：
{{.Site}}/me
$$, updated_at = now() WHERE key = 'unlink';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您建立了 API 憑證「{{.Key}}」。

Client ID：{{.ClientID}}
時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果不是您本人操作，請到會員中心刪除這組憑證：
{{.Site}}/me/api-key
$$, updated_at = now() WHERE key = 'api_key_created';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您停用了 API 憑證「{{.Key}}」，使用這組憑證的程式會無法呼叫 API。

Client ID：{{.ClientID}}
時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

要恢復使用，可以到會員中心重新啟用：
{{.Site}}/me/api-key
$$, updated_at = now() WHERE key = 'api_key_disabled';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您刪除了 API 憑證「{{.Key}}」，這組憑證無法再使用。

Client ID：{{.ClientID}}
時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

需要的話，可以到會員中心建立新的憑證：
{{.Site}}/me/api-key
$$, updated_at = now() WHERE key = 'api_key_deleted';
UPDATE app.mail_template SET body = btrim(body, E'\n')
 WHERE key IN ('new_device', 'link', 'unlink', 'api_key_created', 'api_key_disabled', 'api_key_deleted');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您的帳號剛剛在一個新裝置上登入。

時間：{{.Time}}
裝置：{{.Device}}
IP：{{.IP}}

如果是您本人，可以不用理會這封信。
如果不是，請到下面的頁面登出這個裝置，並檢查 GitHub 或 Google 帳號的安全：
{{.Site}}/me/devices
$$, updated_at = now() WHERE key = 'new_device';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您的帳號在 {{.Time}} 新增了 {{.Provider}} 登入。之後用 {{.Provider}} 也能登入同一個帳號。

如果不是您本人操作，請到會員中心移除：
{{.Site}}/me
$$, updated_at = now() WHERE key = 'link';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您的帳號在 {{.Time}} 移除了 {{.Provider}} 登入。之後無法再用 {{.Provider}} 登入這個帳號。

如果不是您本人操作，請盡快登入會員中心確認：
{{.Site}}/me
$$, updated_at = now() WHERE key = 'unlink';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您在 {{.Time}} 建立了 API 憑證「{{.Key}}」。

Client ID：{{.ClientID}}

如果不是您本人操作，請到會員中心刪除這組憑證：
{{.Site}}/me/api-key
$$, updated_at = now() WHERE key = 'api_key_created';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您在 {{.Time}} 停用了 API 憑證「{{.Key}}」，使用這組憑證的程式會無法呼叫 API。

要恢復使用，可以到會員中心重新啟用：
{{.Site}}/me/api-key
$$, updated_at = now() WHERE key = 'api_key_disabled';
UPDATE app.mail_template SET body = $$
{{.Name}} 您好：

您在 {{.Time}} 刪除了 API 憑證「{{.Key}}」，這組憑證無法再使用。

需要的話，可以到會員中心建立新的憑證：
{{.Site}}/me/api-key
$$, updated_at = now() WHERE key = 'api_key_deleted';
UPDATE app.mail_template SET body = btrim(body, E'\n')
 WHERE key IN ('new_device', 'link', 'unlink', 'api_key_created', 'api_key_disabled', 'api_key_deleted');
-- +goose StatementEnd
