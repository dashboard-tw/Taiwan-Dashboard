# 參與開發

## 回報問題

到 [Issues](https://github.com/dashboard-tw/Taiwan-Dashboard/issues) 建立新的 issue。

- 資料有誤：附上網頁連結、看到的數值和官方來源
- 想新增資料：附上開放資料平臺的連結和更新頻率
- 資安問題：不要公開回報，請見 [SECURITY.md](SECURITY.md)

也可以到 [Discord](https://discord.gg/gU5gndueZH) 討論。

## 送出修改

1. Fork 這個倉庫，從 `main` 開新分支
2. 照 [README](README.md) 的快速開始在本機啟動
3. 送出前先跑過檢查：

```bash
# 在 Taiwan-Dashboard-BE/
gofmt -l .
go vet ./...
go test ./...

# 在 Taiwan-Dashboard-FE/
npx vue-tsc --noEmit -p tsconfig.app.json
npx eslint src
npx prettier --check src
npx vitest run
```

有些測試需要資料庫。設定 `TEST_DATABASE_URL` 指到有 PostGIS 的 PostgreSQL，沒設定時會略過。

4. 開 Pull Request，寫下改了什麼

## 慣例

- 網站文字用繁體中文，句子簡短
- 網站上用「臺」，機關和公司名稱照原名
- 新增資料集時，一起加上 migration、資料管線、測試和 API 欄位說明
- 不要把金鑰、密碼或主機位址放進程式或文件

送出的程式碼會以 [AGPL-3.0](LICENSE) 授權。
