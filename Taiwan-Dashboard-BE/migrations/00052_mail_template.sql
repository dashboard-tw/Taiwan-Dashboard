-- +goose Up
CREATE TABLE app.mail_template (
    key         text PRIMARY KEY CHECK (key ~ '^[a-z_]{1,40}$'),
    name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
    subject     text NOT NULL CHECK (char_length(subject) <= 200),
    body        text NOT NULL CHECK (char_length(body) <= 5000),
    enabled     boolean NOT NULL DEFAULT true,
    sort        integer NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  bigint REFERENCES app.users (id) ON DELETE SET NULL
);

-- +goose StatementBegin
INSERT INTO app.mail_template (key, name, sort, subject, body) VALUES
('signup', '註冊成功', 10, '歡迎加入臺灣儀表板', $$
{{.Name}} 您好：

您已經用 {{.Provider}} 註冊臺灣儀表板。

在會員中心可以申請 API 憑證、投稿應用、管理登入的裝置：
{{.Site}}/me
$$),
('new_device', '新裝置登入', 20, '新裝置登入通知', $$
{{.Name}} 您好：

您的帳號剛剛在一個新裝置上登入。

時間：{{.Time}}
裝置：{{.Device}}
IP：{{.IP}}

如果是您本人，可以不用理會這封信。
如果不是，請到下面的頁面登出這個裝置，並檢查 GitHub 或 Google 帳號的安全：
{{.Site}}/me/devices
$$),
('link', '新增登入方式', 30, '已新增登入方式：{{.Provider}}', $$
{{.Name}} 您好：

您的帳號在 {{.Time}} 新增了 {{.Provider}} 登入。之後用 {{.Provider}} 也能登入同一個帳號。

如果不是您本人操作，請到會員中心移除：
{{.Site}}/me
$$),
('unlink', '移除登入方式', 40, '已移除登入方式：{{.Provider}}', $$
{{.Name}} 您好：

您的帳號在 {{.Time}} 移除了 {{.Provider}} 登入。之後無法再用 {{.Provider}} 登入這個帳號。

如果不是您本人操作，請盡快登入會員中心確認：
{{.Site}}/me
$$),
('account_deleted', '刪除帳號', 50, '帳號已刪除', $$
{{.Name}} 您好：

您的臺灣儀表板帳號已經在 {{.Time}} 刪除。API 憑證、投稿的應用和登入紀錄都一併移除。

謝謝您使用臺灣儀表板。日後想再使用，重新註冊就可以了。
$$),
('api_usage', 'API 用量提醒', 60, 'API 今日用量已超過八成', $$
{{.Name}} 您好：

您的開放資料 API 今天的用量快到上限了。

呼叫次數：{{.Requests}}/{{.RequestLimit}} 次
流量：{{.TrafficMB}}/{{.TrafficLimitMB}} MB

任一項用完後，當天的請求會回傳 429，臺灣時間午夜重新計算。
用量明細可以在會員中心查看：
{{.Site}}/me
$$),
('app_approved', '應用審查通過', 70, '應用審查通過：{{.App}}', $$
{{.Name}} 您好：

您投稿的「{{.App}}」已通過審查，現在會出現在應用活化頁：
{{.Site}}/showcase
$$),
('app_rejected', '應用審查未通過', 80, '應用審查未通過：{{.App}}', $$
{{.Name}} 您好：

您投稿的「{{.App}}」這次沒有通過審查。

原因：{{.Note}}

修改後可以在會員中心重新送出：
{{.Site}}/me/showcase
$$),
('app_removed', '應用下架', 90, '應用已下架：{{.App}}', $$
{{.Name}} 您好：

您投稿的「{{.App}}」已從應用活化頁下架。

原因：{{.Note}}

修改後可以在會員中心重新送出：
{{.Site}}/me/showcase
$$),
('app_submitted', '新投稿（寄給管理員）', 100, '有應用等待審查：{{.App}}', $$
{{.Name}} 您好：

{{.Author}} 投稿了新的應用「{{.App}}」。

到後臺審查：
{{.Site}}/admin/apps
$$),
('app_resubmitted', '投稿修改（寄給管理員）', 110, '應用修改後等待審查：{{.App}}', $$
{{.Name}} 您好：

{{.Author}} 修改了「{{.App}}」，需要重新審查。

到後臺審查：
{{.Site}}/admin/apps
$$),
('footer', '共用頁尾', 900, '', $$
—
臺灣儀表板 {{.Site}}
這封信由系統自動寄出，請不要直接回覆。
$$);
-- +goose StatementEnd

UPDATE app.mail_template SET body = btrim(body, E'\n');

-- +goose Down
DROP TABLE app.mail_template;
