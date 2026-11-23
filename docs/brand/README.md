# Logo

白色的臺灣島，外面圍一圈四色圓餅環，代表臺灣的資料儀表板。

臺灣的外形來自 SVG Repo 的 [Taiwan Taiwan](https://www.svgrepo.com/svg/308354)，CC0 授權。載入中的骨架畫面 `Taiwan-Dashboard-FE/src/components/layout/TaiwanLogo.vue` 也用同一個外形。

## 檔案

| 檔案 | 大小 | 用途 |
|---|---|---|
| `Taiwan-Dashboard-FE/public/logo.svg` | 向量 | 原始檔，圓角方塊 |
| `Taiwan-Dashboard-FE/public/favicon.svg` | 向量 | 瀏覽器分頁圖示，內容同 `logo.svg` |
| `Taiwan-Dashboard-FE/public/logo.png` | 512 × 512 | 一般使用，四角透明 |
| `Taiwan-Dashboard-FE/public/apple-touch-icon.png` | 180 × 180 | iPhone 主畫面圖示，滿版方形 |
| `Taiwan-Dashboard-FE/src/components/layout/BrandMark.vue` | 向量 | 網站頂部列的圖示，沒有底色，臺灣跟著文字顏色 |
| `docs/brand/logo-square.svg` | 向量 | 滿版方形的原始檔 |
| `docs/brand/logo-square-1024.png` | 1024 × 1024 | GitHub 組織頭像與 OAuth App |
| `docs/brand/logo-square-120.png` | 120 × 120 | Google OAuth 同意畫面 |
| `docs/brand/logo-rounded-1024.png` | 1024 × 1024 | 簡報、社群 |
| `docs/brand/logo-light.svg`、`logo-light-1024.png` | 向量、1024 × 1024 | 淺色版：白底、深藍色臺灣，用在淺色的簡報或文件 |
| `docs/brand/logo-mark-white.svg`、`logo-mark-white-1024.png` | 向量、1024 × 1024 | 無背景、白色臺灣，放在深色背景上 |
| `docs/brand/logo-mark-dark.svg`、`logo-mark-dark-1024.png` | 向量、1024 × 1024 | 無背景、深藍色臺灣，放在淺色背景上 |
| `docs/brand/social-preview.jpg` | 2560 × 1280 | GitHub 倉庫的 Social preview，用網站實際畫面製作 |
| `docs/brand/screenshots/` | 2400 × 1500 | 網站實際畫面：首頁、地圖模式、臺北市、電力、天氣、API 文件，用於應用登錄與介紹 |

## 規格

512 × 512 的方格，圓角半徑 112，底色 `#0d1b2e`。

圓餅環外徑 241、粗 52，從正上方順時針分成四段，段與段之間留 9 度：

| 比例 | 顏色 |
|---|---|
| 40% | `#2f7ce0` |
| 25% | `#22b8a6` |
| 20% | `#f5b638` |
| 15% | `#f06a5a` |

臺灣島放大 1.36 倍置中，深色底用 `#ffffff`，淺色底用 `#0d1b2e`。

修改時先改 `logo.svg` 與 `docs/brand` 裡的 SVG，再執行 `node scripts/windows/render-logo.mjs` 重新產生各尺寸 PNG。
