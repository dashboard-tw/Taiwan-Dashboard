-- +goose Up
-- +goose StatementBegin
UPDATE app.mail_template SET subject = $m$歡迎使用臺灣儀表板$m$, body = $m${{.Name}} 您好：

您已成功透過 {{.Provider}} 完成帳號建立。

您現在可以前往會員中心申請專屬 API 憑證，或投稿您開發的應用作品與社群分享：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'signup';
UPDATE app.mail_template SET subject = $m$新裝置登入通知$m$, body = $m${{.Name}} 您好：

系統偵測到您的帳號剛透過一台新裝置登入，詳細資訊如下：

• 登入時間：{{.Time}}
• 登入地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

若此為您本人操作，請忽略此封信件。
若非您本人操作，為確保帳號安全，請立即前往以下連結登出該裝置並變更安全設定：
{{.Site}}/me/devices$m$, updated_at = now() WHERE key = 'new_device';
UPDATE app.mail_template SET subject = $m$已綁定 {{.Provider}} 登入$m$, body = $m${{.Name}} 您好：

您的帳號已成功新增綁定 {{.Provider}} 登入方式，操作資訊如下：

• 操作時間：{{.Time}}
• 操作地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

若非您本人操作，請立即前往會員中心解除此登入綁定：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'link';
UPDATE app.mail_template SET subject = $m$已解除 {{.Provider}} 登入$m$, body = $m${{.Name}} 您好：

您的帳號已解除 {{.Provider}} 登入綁定，往後將無法再透過該方式登入系統。操作資訊如下：

• 操作時間：{{.Time}}
• 操作地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

若非您本人操作，請立即登入會員中心檢視帳號安全狀態：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'unlink';
UPDATE app.mail_template SET subject = $m$帳號刪除確認通知$m$, body = $m${{.Name}} 您好：

您的「臺灣儀表板」帳號已於 {{.Time}} 完成刪除程序。與該帳號關聯的所有 API 憑證、AI 工具授權及已投稿之應用皆已一併移除，且資料將無法復原。

感謝您過去對臺灣儀表板的支持與使用。$m$, updated_at = now() WHERE key = 'account_deleted';
UPDATE app.mail_template SET subject = $m$API 用量提醒：今日額度已達 80%$m$, body = $m${{.Name}} 您好：

提醒您，您今日的 API 使用量已超過每日上限的 80%，目前用量統計如下：

• 呼叫次數：{{.Requests}} / {{.RequestLimit}} 次
• 傳輸流量：{{.TrafficMB}} / {{.TrafficLimitMB}} MB

當用量達到上限時，系統將回傳 HTTP 429 狀態碼並暫停服務，額度將於每日午夜 00:00 重置。您可隨時至憑證管理頁面檢視詳細狀態：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_usage';
UPDATE app.mail_template SET subject = $m$已建立新的 API 憑證：{{.Key}}$m$, body = $m${{.Name}} 您好：

您的帳號已成功建立一組新的 API 憑證「{{.Key}}」，詳細資訊如下：

• Client ID：{{.ClientID}}
• 建立時間：{{.Time}}
• 操作地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

請妥善保管您的憑證金鑰。若非您本人操作，請立即前往憑證管理頁面刪除該組憑證：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_key_created';
UPDATE app.mail_template SET subject = $m$API 憑證停用通知：{{.Key}}$m$, body = $m${{.Name}} 您好：

您已停用 API 憑證「{{.Key}}」，使用此憑證的應用程式目前將無法呼叫 API 服務。操作資訊如下：

• Client ID：{{.ClientID}}
• 停用時間：{{.Time}}
• 操作地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

若需恢復服務，可隨時前往憑證管理頁面重新啟用：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_key_disabled';
UPDATE app.mail_template SET subject = $m$API 憑證刪除通知：{{.Key}}$m$, body = $m${{.Name}} 您好：

您已永久刪除 API 憑證「{{.Key}}」，該組憑證已失效且無法再用於 API 驗證。操作資訊如下：

• Client ID：{{.ClientID}}
• 刪除時間：{{.Time}}
• 操作地點：{{.Location}}
• IP 位址：{{.IP}}
• 裝置類型：{{.DeviceType}}
• 作業系統：{{.OS}}
• 瀏覽器：{{.Browser}}

若非您本人操作，請立即前往憑證管理頁面檢視帳號安全：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_key_deleted';
UPDATE app.mail_template SET subject = $m$重要通知：您的 API 憑證已被系統管理員停用$m$, body = $m${{.Name}} 您好：

您的 API 憑證「{{.Key}}」已於 {{.Time}} 由系統管理員停用，相關 API 請求將無法繼續執行。

若您對此處置有任何疑問，或需進一步了解原因，請透過下方聯絡方式與我們聯繫。$m$, updated_at = now() WHERE key = 'api_key_revoked';
UPDATE app.mail_template SET subject = $m$應用審查通過通知：{{.App}}$m$, body = $m${{.Name}} 您好：

恭喜您！您投稿的應用「{{.App}}」已通過審核，現已正式上架至應用展示頁面：
{{.Site}}/showcase

同時，您的帳號 API 每日呼叫額度已自動提升至 3,000 次。感謝您的貢獻與分享！$m$, updated_at = now() WHERE key = 'app_approved';
UPDATE app.mail_template SET subject = $m$應用審查結果通知：{{.App}}$m$, body = $m${{.Name}} 您好：

感謝您投稿應用「{{.App}}」。經審核團隊評估後，此作品目前尚未符合上架標準，未通過原因如下：

• 審核意見：{{.Note}}

您可依據上述建議調整後，前往會員中心重新提交審核：
{{.Site}}/me/showcase$m$, updated_at = now() WHERE key = 'app_rejected';
UPDATE app.mail_template SET subject = $m$應用下架通知：{{.App}}$m$, body = $m${{.Name}} 您好：

通知您，您所投稿的應用「{{.App}}」目前已自應用展示頁面下架，原因說明如下：

• 下架原因：{{.Note}}

若您已完成相關調整並希望重新上架，可隨時前往會員中心再次提交審核：
{{.Site}}/me/showcase$m$, updated_at = now() WHERE key = 'app_removed';
UPDATE app.mail_template SET subject = $m$【管理員通知】新應用投稿待審核：{{.App}}$m$, body = $m$管理員您好：

開發者 {{.Author}} 已提交新應用「{{.App}}」，目前正等待審核。

請前往管理後台進行檢視與審核作業：
{{.Site}}/admin/apps$m$, updated_at = now() WHERE key = 'app_submitted';
UPDATE app.mail_template SET subject = $m$【管理員通知】應用內容已更新，待重新審核：{{.App}}$m$, body = $m$管理員您好：

開發者 {{.Author}} 已更新應用「{{.App}}」的提交內容，請重新進行審核作業。

請前往管理後台檢視變更項目：
{{.Site}}/admin/apps$m$, updated_at = now() WHERE key = 'app_resubmitted';
UPDATE app.mail_template SET subject = $m$$m$, body = $m$--
臺灣儀表板 | {{.Site}}
此信件由系統自動發送，請勿直接回覆。
若有任何疑問或需要協助，歡迎透過以下方式聯繫我們：
客服信箱：support@dashboard.tw
Discord 社群：https://discord.gg/gU5gndueZH$m$, updated_at = now() WHERE key = 'footer';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE app.mail_template SET subject = $m$歡迎使用臺灣儀表板$m$, body = $m${{.Name}} 您好：

您已經用 {{.Provider}} 註冊臺灣儀表板。

在會員中心可以申請 API 憑證、投稿應用、管理登入的裝置：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'signup';
UPDATE app.mail_template SET subject = $m$新裝置登入通知$m$, body = $m${{.Name}} 您好：

您的帳號剛剛在一個新裝置上登入。

時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果是您本人，可以不用理會這封信。
如果不是，請到下面的頁面登出這個裝置，並檢查 GitHub 或 Google 帳號的安全：
{{.Site}}/me/devices$m$, updated_at = now() WHERE key = 'new_device';
UPDATE app.mail_template SET subject = $m$已新增登入方式：{{.Provider}}$m$, body = $m${{.Name}} 您好：

您的帳號新增了 {{.Provider}} 登入。之後用 {{.Provider}} 也能登入同一個帳號。

時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果不是您本人操作，請到會員中心移除：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'link';
UPDATE app.mail_template SET subject = $m$已移除登入方式：{{.Provider}}$m$, body = $m${{.Name}} 您好：

您的帳號移除了 {{.Provider}} 登入。之後無法再用 {{.Provider}} 登入這個帳號。

時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果不是您本人操作，請盡快登入會員中心確認：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'unlink';
UPDATE app.mail_template SET subject = $m$帳號已刪除$m$, body = $m${{.Name}} 您好：

您的臺灣儀表板帳號已經在 {{.Time}} 刪除。API 憑證、投稿的應用和登入紀錄都一併移除。

謝謝您使用臺灣儀表板。日後想再使用，重新註冊就可以了。$m$, updated_at = now() WHERE key = 'account_deleted';
UPDATE app.mail_template SET subject = $m$API 今日用量已超過八成$m$, body = $m${{.Name}} 您好：

您的開放資料 API 今天的用量快到上限了。

呼叫次數：{{.Requests}}/{{.RequestLimit}} 次
流量：{{.TrafficMB}}/{{.TrafficLimitMB}} MB

任一項用完後，當天的請求會回傳 429，臺灣時間午夜重新計算。
用量明細可以在會員中心查看：
{{.Site}}/me$m$, updated_at = now() WHERE key = 'api_usage';
UPDATE app.mail_template SET subject = $m$已建立 API 憑證：{{.Key}}$m$, body = $m${{.Name}} 您好：

您建立了 API 憑證「{{.Key}}」。

Client ID：{{.ClientID}}
時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

如果不是您本人操作，請到會員中心刪除這組憑證：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_key_created';
UPDATE app.mail_template SET subject = $m$已停用 API 憑證：{{.Key}}$m$, body = $m${{.Name}} 您好：

您停用了 API 憑證「{{.Key}}」，使用這組憑證的程式會無法呼叫 API。

Client ID：{{.ClientID}}
時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

要恢復使用，可以到會員中心重新啟用：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_key_disabled';
UPDATE app.mail_template SET subject = $m$已刪除 API 憑證：{{.Key}}$m$, body = $m${{.Name}} 您好：

您刪除了 API 憑證「{{.Key}}」，這組憑證無法再使用。

Client ID：{{.ClientID}}
時間：{{.Time}}
地點：{{.Location}}
IP：{{.IP}}
裝置：{{.DeviceType}}
作業系統：{{.OS}}
瀏覽器：{{.Browser}}

需要的話，可以到會員中心建立新的憑證：
{{.Site}}/me/api-key$m$, updated_at = now() WHERE key = 'api_key_deleted';
UPDATE app.mail_template SET subject = $m$您的 API 憑證已被停用$m$, body = $m${{.Name}} 您好：

管理員在 {{.Time}} 停用了您的 API 憑證「{{.Key}}」，這組憑證無法再使用。

如有疑問，請來信 support@dashboard.tw。$m$, updated_at = now() WHERE key = 'api_key_revoked';
UPDATE app.mail_template SET subject = $m$應用審查通過：{{.App}}$m$, body = $m${{.Name}} 您好：

您投稿的「{{.App}}」已通過審查，現在會出現在應用活化頁：
{{.Site}}/showcase$m$, updated_at = now() WHERE key = 'app_approved';
UPDATE app.mail_template SET subject = $m$應用審查未通過：{{.App}}$m$, body = $m${{.Name}} 您好：

您投稿的「{{.App}}」這次沒有通過審查。

原因：{{.Note}}

修改後可以在會員中心重新送出：
{{.Site}}/me/showcase$m$, updated_at = now() WHERE key = 'app_rejected';
UPDATE app.mail_template SET subject = $m$應用已下架：{{.App}}$m$, body = $m${{.Name}} 您好：

您投稿的「{{.App}}」已從應用活化頁下架。

原因：{{.Note}}

修改後可以在會員中心重新送出：
{{.Site}}/me/showcase$m$, updated_at = now() WHERE key = 'app_removed';
UPDATE app.mail_template SET subject = $m$有應用等待審查：{{.App}}$m$, body = $m${{.Name}} 您好：

{{.Author}} 投稿了新的應用「{{.App}}」。

到後臺審查：
{{.Site}}/admin/apps$m$, updated_at = now() WHERE key = 'app_submitted';
UPDATE app.mail_template SET subject = $m$應用修改後等待審查：{{.App}}$m$, body = $m${{.Name}} 您好：

{{.Author}} 修改了「{{.App}}」，需要重新審查。

到後臺審查：
{{.Site}}/admin/apps$m$, updated_at = now() WHERE key = 'app_resubmitted';
UPDATE app.mail_template SET subject = $m$$m$, body = $m$—
臺灣儀表板 {{.Site}}
這封信由系統自動寄出，請不要直接回覆。$m$, updated_at = now() WHERE key = 'footer';
-- +goose StatementEnd
