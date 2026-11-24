-- +goose Up
-- +goose StatementBegin
INSERT INTO app.mail_template (key, name, sort, subject, body) VALUES
('api_key_rotated', '重新產生 Secret', 63, 'API 憑證 Secret 已重新產生：{{.Key}}', $m${{.Name}} 您好：

您已重新產生 API 憑證「{{.Key}}」的 Client Secret，舊的 Secret 已立即失效，使用中的程式需要更新為新的 Secret。操作資訊如下：

• Client ID：{{.ClientID}}
• 產生時間：{{.Time}}
• 操作地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

若非您本人操作，請立即前往憑證管理頁面停用或刪除此組憑證：
{{.Site}}/me/api-key$m$),
('app_received', '投稿已收到', 69, '已收到您的應用投稿：{{.App}}', $m${{.Name}} 您好：

我們已收到您投稿的應用「{{.App}}」，待審核完成後會再寄信通知您結果。

您可隨時至會員中心查看審核狀態：
{{.Site}}/me/showcase$m$)
ON CONFLICT (key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
DELETE FROM app.mail_template WHERE key IN ('api_key_rotated', 'app_received');
