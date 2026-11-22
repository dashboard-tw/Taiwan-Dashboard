# 設定 GitHub 與 Google 登入

網站只提供 GitHub 和 Google 登入，不使用 Email 和密碼。照下面的步驟建立兩個 OAuth 應用程式，再把 Client ID 和 Client Secret 放進環境變數。

Client Secret 請當成密碼保管，不要貼到 Issue 或程式碼裡。外洩時到同一個頁面重新產生。

## 回呼網址

登入後 GitHub 和 Google 會把瀏覽器帶回這個網址，登記的網址要完全一樣。開發和正式環境各登記一個：

| 環境 | GitHub | Google |
|---|---|---|
| 正式 | `https://dashboard.tw/api/v1/auth/github/callback` | `https://dashboard.tw/api/v1/auth/google/callback` |
| 本機開發 | `http://127.0.0.1:5173/api/v1/auth/github/callback` | `http://127.0.0.1:5173/api/v1/auth/google/callback` |

本機開發請用 `http://127.0.0.1:5173` 開網站，不要用 `localhost`。

## GitHub

建議在 GitHub 組織底下建立，使用者授權時看到的會是組織名稱。正式和開發各建一個 App，Secret 分開保管。

1. 到組織的 Settings → Developer settings → OAuth Apps → New OAuth App
2. 填寫：
   - Application name：`dashboard.tw`，開發用的叫 `dashboard.tw（開發）`
   - Homepage URL：`https://dashboard.tw`
   - Authorization callback URL：上表的 GitHub 回呼網址
   - Enable Device Flow：不要勾
3. 按 Register application，畫面上的 Client ID 就是 `GITHUB_CLIENT_ID`
4. 按 Generate a new client secret，複製顯示的值，這是 `GITHUB_CLIENT_SECRET`，只會顯示一次

網站只要求 `read:user` 和 `user:email` 兩個權限，不能讀寫程式碼或倉庫。

GitHub 帳號要建立滿 30 天才能登入。

## Google

1. 到 [Google Cloud Console](https://console.cloud.google.com/) 建立新專案
2. API 和服務 → OAuth 同意畫面：
   - 使用者類型選「外部」
   - 填應用程式名稱、支援信箱、首頁、隱私權政策和服務條款的網址
   - 授權網域加上 `dashboard.tw`
   - 範圍只加 `openid`、`userinfo.email`、`userinfo.profile`，這三個不需要 Google 審核
3. API 和服務 → 憑證 → 建立憑證 → OAuth 用戶端 ID，正式和開發各建一個：
   - 應用程式類型：網頁應用程式
   - 已授權的 JavaScript 來源：開發填 `http://127.0.0.1:5173`，正式填 `https://dashboard.tw`
   - 已授權的重新導向 URI：上表的 Google 回呼網址
4. 建好後的用戶端 ID 是 `GOOGLE_CLIENT_ID`，用戶端密鑰是 `GOOGLE_CLIENT_SECRET`
5. 回到 OAuth 同意畫面，把發布狀態改成「正式版」

## 環境變數

這四個變數給 `dashboardtw serve` 使用。只設定其中一組，就只會出現那一個登入按鈕。

```
GITHUB_CLIENT_ID=
GITHUB_CLIENT_SECRET=
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
```

- 本機開發：寫在 `docker/.env`，用開發用的 App
- 正式環境：寫在 API 的環境變數檔，權限設成 600。正式環境不能設定 `AUTH_DEV_LOGIN`

設定成功時，API 啟動會記錄 `sign-in provider enabled provider=github`。

### EMAIL_KEY

正式環境開啟登入時要設定 `EMAIL_KEY`，用下面的指令產生：

```bash
openssl rand -base64 32
```

請另外備份。遺失後就讀不回已存的 Email。


