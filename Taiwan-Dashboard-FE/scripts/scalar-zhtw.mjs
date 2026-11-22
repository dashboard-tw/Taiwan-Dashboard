import { writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import path from 'node:path'

const [frontend, openccDir, out] = process.argv.slice(2)
const require = createRequire(path.join(openccDir, 'package.json'))
const OpenCC = require('opencc-js')
const convert = OpenCC.Converter({ from: 'cn', to: 'twp' })

const mod = (p) => import(pathToFileURL(path.join(frontend, 'node_modules/@scalar', p)).href)
const { zhCn: reference } = await mod('api-reference/dist/features/localization/locales/zh-cn.js')
const { zhCn: client } = await mod('api-client/dist/v2/features/localization/locales/zh-cn.js')

const fixes = [
  ['示例', '範例'],
  ['默認', '預設'],
  ['響應', '回應'],
  ['請求頭', '請求標頭'],
  ['響應頭', '回應標頭'],
  ['密鑰', '金鑰'],
  ['菜單', '選單'],
  ['數據', '資料'],
  ['鏈接', '連結'],
  ['服務器', '伺服器'],
  ['搜索', '搜尋'],
  ['引數', '參數'],
  ['專案', '項目'],
  ['流項目', '串流項目'],
  ['客戶端庫', '用戶端程式庫'],
  ['客戶端', '用戶端'],
  ['請求體', '請求內容'],
  ['回應體', '回應內容'],
  ['協議', '協定'],
  ['標籤組', '標籤群組'],
  ['分組', '群組'],
  ['導航', '瀏覽'],
  ['回撥', '回呼'],
  ['生成', '產生'],
  ['常量', '常數'],
  ['只讀', '唯讀'],
  ['只寫', '唯寫'],
  ['配置', '設定'],
  ['佈局', '版面'],
  ['自定義', '自訂'],
  ['域名', '網域'],
  ['訪問令牌', '存取權杖'],
  ['令牌', '權杖'],
  ['重定向', '重新導向'],
  ['回應頭', '回應標頭'],
  ['頭部', '標頭'],
  ['全域性', '全域'],
  ['二進位制', '二進位'],
  ['遞迴引用', '遞迴參照'],
  ['文本', '文字'],
  ['當前', '目前'],
  ['許可權', '權限'],
  ['獲取', '取得'],
  ['憑據', '憑證'],
  ['證書', '憑證'],
  ['通過', '透過'],
  ['按回車', '按 Enter '],
  ['工具欄', '工具列'],
  ['檢視瀏覽器控制台', '查看瀏覽器主控台'],
  ['7 天后', '7 天後'],
]
const tw = (s) => fixes.reduce((t, [a, b]) => t.split(a).join(b), convert(s))

const walk = (v) => {
  if (typeof v === 'string') return tw(v)
  if (Array.isArray(v)) return v.map(walk)
  if (v && typeof v === 'object')
    return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, walk(x)]))
  return v
}

const translations = { ...walk(reference), apiClient: walk(client) }
writeFileSync(
  out,
  `// Scalar API 文件介面的臺灣繁體中文文字。由 Scalar ${process.env.SCALAR_VERSION} 的簡體中文（zh-CN）以 OpenCC
// （s2twp，臺灣用語）轉換後再人工校對；升級 Scalar 時重新產生，新增的文字會先顯示英文。
import type { ApiReferenceConfiguration } from '@scalar/api-reference'

type Localization = NonNullable<Partial<ApiReferenceConfiguration>['localization']>

export const scalarZhTw: NonNullable<Localization['translations']> = ${JSON.stringify(translations, null, 2)}
`,
)
console.log('wrote', out)
