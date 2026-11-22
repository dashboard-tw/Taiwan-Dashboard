export type Method = 'GET' | 'POST'

export interface Param {
  name: string
  in: 'path' | 'query' | 'form'
  type: string
  required?: boolean
  text: string
}

export interface Status {
  code: number
  name: string
  text: string
}

export interface Endpoint {
  slug: string
  method: Method
  path: string
  title: string
  summary: string
  auth: boolean
  params: Param[]
  example: (base: string, origin: string) => Record<string, string>
  response: string
  statuses: Status[]
}

const TOKEN = '-H "Authorization: Bearer $DASHBOARD_TW_TOKEN"'

const ID: Param = {
  name: 'id',
  in: 'path',
  type: 'string',
  required: true,
  text: '資料集代碼，例如 aqi_obs。完整清單見資料集目錄。',
}
const COUNTY: Param = {
  name: 'county',
  in: 'query',
  type: 'string',
  text: '縣市代碼，例如 63000 是臺北市。只有部分資料集支援。',
}
const SITE: Param = {
  name: 'site',
  in: 'query',
  type: 'string',
  text: '觀測點代碼，可從 sites 端點查詢。',
}
const CURSOR: Param = {
  name: 'cursor',
  in: 'query',
  type: 'string',
  text: '上一頁回應的 meta.next_cursor，取得下一頁。',
}
const FORMAT: Param = {
  name: 'format',
  in: 'query',
  type: 'string',
  text: '輸出格式：json（預設）或 csv。',
}

const AUTH_ERRORS: Status[] = [
  { code: 401, name: 'invalid_token', text: '缺少 Access Token，或 token 無效、已過期' },
  { code: 429, name: 'rate_limited', text: '每分鐘的請求太多' },
  { code: 429, name: 'quota_exceeded', text: '今天的次數已用完' },
  { code: 429, name: 'traffic_exceeded', text: '今天的流量已用完' },
]
const NOT_FOUND: Status = { code: 404, name: 'dataset_not_found', text: '找不到這個資料集' }

const META = `  "meta": {
    "dataset": "aqi_obs",
    "source": {
      "agency": "環境部",
      "license": "政府資料開放授權條款－第 1 版",
      "url": "https://data.moenv.gov.tw/..."
    },
    "status": "ok",
    "last_observed_at": "2026-11-20T21:00:00+08:00"
  }`

export const ENDPOINTS: Endpoint[] = [
  {
    slug: 'token',
    method: 'POST',
    path: '/auth/token',
    title: '取得 Access Token',
    summary:
      '用 Client Id 與 Client Secret 換取 Access Token，有效 24 小時。請重複使用同一個 token，過期再重新取得。這個端點不計入額度。',
    auth: false,
    params: [
      {
        name: 'grant_type',
        in: 'form',
        type: 'string',
        required: true,
        text: '固定為 client_credentials。',
      },
      {
        name: 'client_id',
        in: 'form',
        type: 'string',
        required: true,
        text: '在「我的 API 憑證」建立的 Client Id。也可以用 HTTP Basic 傳送。',
      },
      {
        name: 'client_secret',
        in: 'form',
        type: 'string',
        required: true,
        text: '建立憑證時顯示的 Client Secret。',
      },
    ],
    example: (base) => ({
      curl: `curl -X POST "${base}/auth/token" \\
  -d grant_type=client_credentials \\
  -d client_id="$DASHBOARD_TW_CLIENT_ID" \\
  -d client_secret="$DASHBOARD_TW_CLIENT_SECRET"`,
      Python: `import os, requests

res = requests.post(
    "${base}/auth/token",
    data={
        "grant_type": "client_credentials",
        "client_id": os.environ["DASHBOARD_TW_CLIENT_ID"],
        "client_secret": os.environ["DASHBOARD_TW_CLIENT_SECRET"],
    },
)
token = res.json()["access_token"]`,
      JavaScript: `const res = await fetch("${base}/auth/token", {
  method: "POST",
  body: new URLSearchParams({
    grant_type: "client_credentials",
    client_id: process.env.DASHBOARD_TW_CLIENT_ID,
    client_secret: process.env.DASHBOARD_TW_CLIENT_SECRET,
  }),
})
const { access_token } = await res.json()`,
    }),
    response: `{
  "access_token": "Yp3kQ...",
  "token_type": "Bearer",
  "expires_in": 86400
}`,
    statuses: [
      { code: 200, name: 'OK', text: '成功' },
      { code: 400, name: 'invalid_request', text: '格式錯誤，請以表單格式送出' },
      { code: 400, name: 'unsupported_grant_type', text: 'grant_type 不是 client_credentials' },
      { code: 401, name: 'invalid_client', text: 'Client Id 或 Secret 錯誤，或憑證已停用' },
      { code: 429, name: 'rate_limited', text: '請求太頻繁，請稍後再試' },
    ],
  },
  {
    slug: 'datasets',
    method: 'GET',
    path: '/datasets',
    title: '資料集清單',
    summary: '列出所有資料集，包含名稱、來源機關、更新頻率與最新資料時間。',
    auth: true,
    params: [],
    example: (base) => ({ curl: `curl "${base}/datasets" \\\n  ${TOKEN}` }),
    response: `{
  "data": [
    {
      "id": "aqi_obs",
      "name": "空氣品質監測",
      "agency": "環境部",
      "status": "ok",
      "update_freq_seconds": 3600,
      "last_observed_at": "2026-11-20T21:00:00+08:00"
    }
  ]
}`,
    statuses: [{ code: 200, name: 'OK', text: '成功' }, ...AUTH_ERRORS],
  },
  {
    slug: 'dataset',
    method: 'GET',
    path: '/datasets/{id}',
    title: '資料集說明',
    summary: '單一資料集的說明、欄位定義，以及是否提供歷史資料與大量下載。',
    auth: true,
    params: [ID],
    example: (base) => ({ curl: `curl "${base}/datasets/aqi_obs" \\\n  ${TOKEN}` }),
    response: `{
  "data": {
    "dataset": { "id": "aqi_obs", "name": "空氣品質監測", "...": "..." },
    "fields": [
      { "name": "aqi", "type": "integer", "description": "空氣品質指標" }
    ],
    "latest_fields": [ "..." ],
    "site_fields": [ "..." ],
    "has_history": true,
    "history": { "page_size": 1000, "max_range_days": 31 },
    "downloads": "/downloads/aqi_obs/index.json"
  }
}`,
    statuses: [{ code: 200, name: 'OK', text: '成功' }, NOT_FOUND, ...AUTH_ERRORS],
  },
  {
    slug: 'latest',
    method: 'GET',
    path: '/datasets/{id}/latest',
    title: '最新資料',
    summary:
      '每個觀測點最新的一筆資料。名冊類的資料集會回傳完整清單並分頁。回應帶有 ETag，帶上 If-None-Match 時資料沒變會回應 304。',
    auth: true,
    params: [ID, COUNTY, SITE, CURSOR, FORMAT],
    example: (base) => ({
      curl: `curl "${base}/datasets/aqi_obs/latest?county=63000" \\\n  ${TOKEN}`,
      Python: `import requests

res = requests.get(
    "${base}/datasets/aqi_obs/latest",
    params={"county": "63000"},
    headers={"Authorization": f"Bearer {token}"},
)
body = res.json()`,
      JavaScript: `const res = await fetch("${base}/datasets/aqi_obs/latest?county=63000", {
  headers: { Authorization: \`Bearer \${token}\` },
})
const { data, meta } = await res.json()`,
    }),
    response: `{
  "data": [
    {
      "site_id": 12,
      "observed_at": "2026-11-20T21:00:00+08:00",
      "aqi": 38,
      "status": "良好",
      "pm25": 9.0
    }
  ],
${META}
}`,
    statuses: [
      { code: 200, name: 'OK', text: '成功' },
      { code: 304, name: 'Not Modified', text: '資料沒變，不帶內容。仍算一次' },
      { code: 400, name: 'unsupported_parameter', text: '這個資料集不支援該參數' },
      NOT_FOUND,
      ...AUTH_ERRORS,
    ],
  },
  {
    slug: 'history',
    method: 'GET',
    path: '/datasets/{id}/history',
    title: '歷史資料',
    summary:
      '指定期間內的歷史資料，每次最多 31 天，每頁最多 1000 筆。更長的期間請改用每月的大量下載。',
    auth: true,
    params: [
      ID,
      {
        name: 'from',
        in: 'query',
        type: 'date',
        text: '開始日期或時間，例如 2026-11-01。預設是 to 的前 24 小時。',
      },
      {
        name: 'to',
        in: 'query',
        type: 'date',
        text: '結束日期或時間，不含。預設是現在。',
      },
      COUNTY,
      SITE,
      CURSOR,
      FORMAT,
    ],
    example: (base) => ({
      curl: `curl "${base}/datasets/rain_obs/history?from=2026-11-01&to=2026-11-08" \\\n  ${TOKEN}`,
      下一頁: `curl "${base}/datasets/rain_obs/history?from=2026-11-01&to=2026-11-08&cursor=eyJ0Ijoi..." \\\n  ${TOKEN}`,
    }),
    response: `{
  "data": [
    {
      "site_id": "C0A520",
      "observed_at": "2026-11-01T00:10:00+08:00",
      "rain_1h": 0.5
    }
  ],
  "meta": {
    "dataset": "rain_obs",
    "next_cursor": "eyJ0Ijoi...",
    "...": "..."
  }
}`,
    statuses: [
      { code: 200, name: 'OK', text: '成功' },
      { code: 400, name: 'invalid_time', text: 'from 或 to 的格式錯誤' },
      { code: 400, name: 'range_too_long', text: '期間超過 31 天' },
      { code: 404, name: 'no_history', text: '這個資料集沒有歷史資料' },
      NOT_FOUND,
      ...AUTH_ERRORS,
    ],
  },
  {
    slug: 'sites',
    method: 'GET',
    path: '/datasets/{id}/sites',
    title: '觀測點清單',
    summary: '測站、水庫等觀測點的代碼、名稱與位置。用 site 參數查詢特定觀測點時會用到。',
    auth: true,
    params: [ID, COUNTY, FORMAT],
    example: (base) => ({ curl: `curl "${base}/datasets/aqi_obs/sites" \\\n  ${TOKEN}` }),
    response: `{
  "data": [
    {
      "site_id": 12,
      "name": "中山",
      "county": "63000",
      "lat": 25.0623,
      "lon": 121.5265
    }
  ],
${META}
}`,
    statuses: [
      { code: 200, name: 'OK', text: '成功' },
      { code: 404, name: 'no_sites', text: '這個資料集沒有觀測點' },
      ...AUTH_ERRORS,
    ],
  },
  {
    slug: 'download-index',
    method: 'GET',
    path: '/downloads/{id}/index.json',
    title: '可下載的月份',
    summary: '列出可以下載的月份，以及每個檔案的筆數、大小與 SHA-256。每天 03:30 更新。',
    auth: true,
    params: [ID],
    example: (_, origin) => ({
      curl: `curl "${origin}/downloads/aqi_obs/index.json" \\\n  ${TOKEN}`,
    }),
    response: `{
  "dataset": "aqi_obs",
  "license": "政府資料開放授權條款－第 1 版",
  "fields": [ "..." ],
  "files": [
    {
      "month": "2026-10",
      "file": "2026-10.csv.gz",
      "rows": 412345,
      "bytes": 5123456,
      "sha256": "9f2c...",
      "generated_at": "2026-11-01T03:30:00+08:00"
    }
  ]
}`,
    statuses: [{ code: 200, name: 'OK', text: '成功' }, NOT_FOUND, ...AUTH_ERRORS],
  },
  {
    slug: 'download-file',
    method: 'GET',
    path: '/downloads/{id}/{month}.csv.gz',
    title: '下載月份檔案',
    summary: '下載一整個月的 CSV，gzip 壓縮、UTF-8 編碼，Excel 可以直接開啟。每個檔案算一次。',
    auth: true,
    params: [
      ID,
      { name: 'month', in: 'path', type: 'string', required: true, text: '月份，例如 2026-10。' },
    ],
    example: (_, origin) => ({
      curl: `curl "${origin}/downloads/aqi_obs/2026-10.csv.gz" \\\n  ${TOKEN} -o aqi_obs-2026-10.csv.gz`,
    }),
    response: `site_id,observed_at,aqi,status,pm25
12,2026-10-01T00:00:00+08:00,41,良好,10.0
...`,
    statuses: [
      { code: 200, name: 'OK', text: '成功' },
      { code: 404, name: 'not_found', text: '沒有這個月份的檔案' },
      ...AUTH_ERRORS,
    ],
  },
]

export function endpointBySlug(slug: string): Endpoint | undefined {
  return ENDPOINTS.find((e) => e.slug === slug)
}
