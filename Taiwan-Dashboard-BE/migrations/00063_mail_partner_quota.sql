-- +goose Up
-- +goose StatementBegin
UPDATE app.mail_template SET body = $m${{.Name}} 您好：

恭喜您！您投稿的應用「{{.App}}」已通過審核，現已正式上架至應用展示頁面：
{{.Site}}/showcase

同時，您的帳號 API 每日呼叫額度已自動提升至 3,000 次。若之後您名下已無任何上架中的應用（例如應用遭下架或刪除），額度將自動恢復為每日 600 次。

感謝您的貢獻與分享！$m$, updated_at = now() WHERE key = 'app_approved';
UPDATE app.mail_template SET body = $m${{.Name}} 您好：

通知您，您所投稿的應用「{{.App}}」目前已自應用展示頁面下架，原因說明如下：

• 下架原因：{{.Note}}

若您名下已無其他上架中的應用，API 每日呼叫額度將自動恢復為 600 次。

若您已完成相關調整並希望重新上架，可隨時前往會員中心再次提交審核：
{{.Site}}/me/showcase$m$, updated_at = now() WHERE key = 'app_removed';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE app.mail_template SET body = $m${{.Name}} 您好：

恭喜您！您投稿的應用「{{.App}}」已通過審核，現已正式上架至應用展示頁面：
{{.Site}}/showcase

同時，您的帳號 API 每日呼叫額度已自動提升至 3,000 次。感謝您的貢獻與分享！$m$, updated_at = now() WHERE key = 'app_approved';
UPDATE app.mail_template SET body = $m${{.Name}} 您好：

通知您，您所投稿的應用「{{.App}}」目前已自應用展示頁面下架，原因說明如下：

• 下架原因：{{.Note}}

若您已完成相關調整並希望重新上架，可隨時前往會員中心再次提交審核：
{{.Site}}/me/showcase$m$, updated_at = now() WHERE key = 'app_removed';
-- +goose StatementEnd
