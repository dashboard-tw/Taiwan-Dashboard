<script setup lang="ts">
import { computed, ref } from 'vue'
import { usePageTitle } from '../composables/usePageTitle'

usePageTitle('關於')

const sources = [
  {
    name: '空氣品質指標（AQI）',
    agency: '環境部',
    url: 'https://data.gov.tw/dataset/40448',
    freq: '每小時',
  },
  {
    name: '紫外線即時監測資料',
    agency: '環境部（整合交通部中央氣象署測站）',
    url: 'https://data.moenv.gov.tw/dataset/detail/UV_S_01',
    freq: '每小時',
  },
  {
    name: '空氣品質預報資料',
    agency: '環境部',
    url: 'https://data.moenv.gov.tw/dataset/detail/AQF_P_01',
    freq: '每天三次',
  },
  {
    name: '水庫即時水情、水庫基本資料與每日營運狀況',
    agency: '經濟部水利署',
    url: 'https://data.gov.tw/dataset/45501',
    freq: '每小時／每天',
  },
  {
    name: '即時水位資料、河川水位測站站況（含警戒水位）',
    agency: '經濟部水利署',
    url: 'https://data.gov.tw/dataset/25768',
    freq: '每 10 分鐘／每天',
  },
  {
    name: '停水資訊',
    agency: '台灣自來水公司',
    url: 'https://data.gov.tw/dataset/6050',
    freq: '隨公告更新',
  },
  {
    name: '臺北市停水資訊（大臺北地區）',
    agency: '臺北自來水事業處',
    url: 'https://data.gov.tw/dataset/128236',
    freq: '隨公告更新',
  },
  {
    name: '電力維生管線災情資訊',
    agency: '台灣電力公司',
    url: 'https://data.gov.tw/dataset/44799',
    freq: '災害期間每小時',
  },
  {
    name: '自動氣象站與現在天氣觀測（氣溫、濕度、風）、今明 36 小時天氣預報',
    agency: '交通部中央氣象署',
    url: 'https://opendata.cwa.gov.tw/dataset/observation/O-A0001-001',
    freq: '每 10 分鐘至每 6 小時',
  },
  {
    name: 'WRF 數值預報（地圖上的風場流動）',
    agency: '交通部中央氣象署',
    url: 'https://opendata.cwa.gov.tw/dataset/mathematics/M-A0061-000',
    freq: '每 6 小時',
  },
  {
    name: '顯著有感地震報告、小區域有感地震報告',
    agency: '交通部中央氣象署',
    url: 'https://data.gov.tw/dataset/6068',
    freq: '有地震時',
  },
  {
    name: '颱風路徑、颱風警報、天氣警特報、自動雨量站雨量、雷達回波、衛星雲圖',
    agency: '交通部中央氣象署',
    url: 'https://opendata.cwa.gov.tw/dataset/warning/W-C0034-005',
    freq: '每 10 分鐘至每 3 小時',
  },
  {
    name: '向日葵衛星紅外線雲圖（Band 13，地圖上的動態雲圖）',
    agency: 'NASA GIBS（日本氣象廳向日葵衛星）',
    url: 'https://nasa-gibs.github.io/gibs-api-docs/',
    freq: '每 10 分鐘（約晚 1 小時）',
  },
  {
    name: '民生示警（土石流、淹水、淹水感測、道路封閉、水庫放流）',
    agency: '國家災害防救科技中心（彙整農村發展及水土保持署、水利署、公路局等）',
    url: 'https://alerts.ncdr.nat.gov.tw/',
    freq: '隨時',
  },
  {
    name: '天然災害停止上班及上課情形',
    agency: '行政院人事行政總處（資料來源：各縣市政府）',
    url: 'https://www.dgpa.gov.tw/typh/daily/nds.html',
    freq: '颱風期間隨時',
  },
  {
    name: '高速公路路段與即時路況',
    agency: '交通部高速公路局',
    url: 'https://tisvcloud.freeway.gov.tw/',
    freq: '每分鐘',
  },
  {
    name: 'A1 類道路交通事故',
    agency: '內政部警政署',
    url: 'https://data.gov.tw/dataset/12818',
    freq: '約每月',
  },
  {
    name: '健保特約醫事機構（醫院、診所、藥局）',
    agency: '衛生福利部中央健康保險署',
    url: 'https://data.gov.tw/dataset/39283',
    freq: '每天',
  },
  {
    name: '長照服務據點（ABC 據點）',
    agency: '衛生福利部',
    url: 'https://data.gov.tw/dataset/88270',
    freq: '每月',
  },
  {
    name: '避難收容處所、收容所開設通報',
    agency: '內政部消防署',
    url: 'https://data.gov.tw/dataset/77133',
    freq: '災害期間每 10 分鐘',
  },
  {
    name: '村里戶數、人口數按戶別及性別',
    agency: '內政部戶政司',
    url: 'https://data.gov.tw/dataset/14299',
    freq: '每年',
  },
  {
    name: '觀光景點、旅館民宿',
    agency: '交通部觀光署',
    url: 'https://data.gov.tw/dataset/7777',
    freq: '每天',
  },
  {
    name: '河川水質監測資料（河川污染指數）',
    agency: '環境部',
    url: 'https://data.moenv.gov.tw/dataset/detail/WQX_P_01',
    freq: '每月',
  },
  {
    name: '再生能源發電量',
    agency: '經濟部能源署',
    url: 'https://www.moeaea.gov.tw/',
    freq: '每月',
  },
  {
    name: '國民小學、國民中學、高級中等學校、大專校院名錄',
    agency: '教育部統計處',
    url: 'https://data.gov.tw/dataset/6087',
    freq: '每學年',
  },
  {
    name: '臺鐵與高鐵營運通阻、臺鐵列車即時位置、公共自行車（YouBike）站點與即時車位',
    agency: '交通部（TDX 運輸資料流通服務）',
    url: 'https://tdx.transportdata.tw/',
    freq: '每 5 至 20 分鐘',
  },
  {
    name: '各村(里)戶籍人口統計月報表（出生、死亡、結婚、離婚）',
    agency: '內政部戶政司',
    url: 'https://data.gov.tw/dataset/77140',
    freq: '每月',
  },
  {
    name: '社會福利機構（托嬰中心）',
    agency: '衛生福利部',
    url: 'https://data.gov.tw/dataset/165355',
    freq: '每月',
  },
  {
    name: '各縣市住宅、服務業及機關用電統計',
    agency: '台灣電力公司',
    url: 'https://data.gov.tw/dataset/29935',
    freq: '每月',
  },
  {
    name: '執行機關一般廢棄物產生量（垃圾與資源回收）',
    agency: '環境部',
    url: 'https://data.gov.tw/dataset/89040',
    freq: '每月',
  },
  {
    name: '農產品交易行情（果菜、花卉批發市場）',
    agency: '農業部',
    url: 'https://data.gov.tw/dataset/8066',
    freq: '每天',
  },
  {
    name: '邊境查驗不合格食品',
    agency: '衛生福利部食品藥物管理署',
    url: 'https://data.fda.gov.tw/',
    freq: '每天',
  },
  {
    name: '火災事件資料',
    agency: '內政部消防署',
    url: 'https://data.gov.tw/dataset/13764',
    freq: '每年',
  },
  {
    name: '不動產實價登錄（買賣、預售屋、租賃）',
    agency: '內政部',
    url: 'https://plvr.land.moi.gov.tw/DownloadOpenData',
    freq: '每季（每月 1、11、21 日補登錄）',
  },
  {
    name: '加油站服務資訊',
    agency: '台灣中油',
    url: 'https://data.gov.tw/dataset/6065',
    freq: '每天',
  },
  {
    name: '登革熱每日確定病例、健保門急診就診人次（類流感、腸病毒、新冠）',
    agency: '衛生福利部疾病管制署',
    url: 'https://data.cdc.gov.tw/',
    freq: '每天、每週',
  },
  {
    name: '消費者物價指數、人力資源調查失業率',
    agency: '行政院主計總處',
    url: 'https://data.gov.tw/dataset/6019',
    freq: '每月',
  },
  {
    name: '景氣指標及燈號',
    agency: '國家發展委員會',
    url: 'https://data.gov.tw/dataset/6099',
    freq: '每月',
  },
  {
    name: '綜稅綜合所得總額各縣市鄉鎮村里統計',
    agency: '財政部財政資訊中心',
    url: 'https://data.gov.tw/dataset/103066',
    freq: '每年',
  },
  {
    name: '動物認領養、公立動物收容所收容處理情形統計',
    agency: '農業部',
    url: 'https://data.gov.tw/dataset/85903',
    freq: '每天、每月',
  },
  {
    name: '土石流及大規模崩塌警戒',
    agency: '農業部農村發展及水土保持署',
    url: 'https://data.gov.tw/dataset/148402',
    freq: '每 10 分鐘',
  },
  {
    name: '淹水感測器（民生公共物聯網）',
    agency: '經濟部水利署',
    url: 'https://colife.org.tw/',
    freq: '每 10 分鐘',
  },
  {
    name: '家禽交易行情（雞蛋、白肉雞）、毛豬交易行情',
    agency: '農業部畜牧司',
    url: 'https://data.gov.tw/dataset/7536',
    freq: '每天',
  },
  {
    name: '電動車充電站與即時狀態',
    agency: '交通部（TDX 運輸資料流通服務）',
    url: 'https://tdx.transportdata.tw/',
    freq: '每天、每 30 分鐘',
  },
  {
    name: '機場航班動態',
    agency: '交通部（TDX 運輸資料流通服務）',
    url: 'https://tdx.transportdata.tw/',
    freq: '每 10 分鐘',
  },
  {
    name: '汽柴油參考牌價',
    agency: '台灣中油',
    url: 'https://vipmbr.cpc.com.tw/cpcstn/listpricewebservice.asmx',
    freq: '每週',
  },
  {
    name: '各機組發電量即時資訊',
    agency: '台灣電力公司',
    url: 'https://data.gov.tw/dataset/8931',
    freq: '每 10 分鐘',
  },
  {
    name: '今日區域別用電情況',
    agency: '台灣電力公司',
    url: 'https://data.gov.tw/dataset/162596',
    freq: '每 10 分鐘',
  },
  {
    name: '今日系統供需狀況',
    agency: '台灣電力公司',
    url: 'https://data.gov.tw/dataset/162595',
    freq: '每 10 分鐘',
  },
  {
    name: '過去電力供需資訊、未來二個月電力供需預估',
    agency: '台灣電力公司',
    url: 'https://data.gov.tw/dataset/19995',
    freq: '每月／約每 10 天',
  },
]

const MINISTRIES = [
  '交通部',
  '經濟部',
  '環境部',
  '衛生福利部',
  '內政部',
  '農業部',
  '教育部',
  '財政部',
  '行政院',
]
const STATE_OWNED = ['台灣電力公司', '台灣中油', '台灣自來水公司', '臺北自來水事業處']
const agencyOf = (a: string) => {
  const m = MINISTRIES.find((p) => a.startsWith(p))
  if (m) return m
  if (STATE_OWNED.some((p) => a.startsWith(p))) return '公營事業'
  return '其他機關'
}
const agencies = computed(() => {
  const count = new Map<string, number>()
  for (const s of sources) {
    const a = agencyOf(s.agency)
    count.set(a, (count.get(a) ?? 0) + 1)
  }
  const last = (n: string) => (n === '其他機關' ? 1 : 0)
  return [...count.entries()]
    .sort((a, b) => last(a[0]) - last(b[0]) || b[1] - a[1])
    .map(([name, n]) => ({ name, n }))
})
const query = ref('')
const agency = ref('')
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return sources.filter(
    (s) =>
      (!agency.value || agencyOf(s.agency) === agency.value) &&
      (!q || `${s.name} ${s.agency}`.toLowerCase().includes(q)),
  )
})
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">info</span>關於</h1>

    <div class="doc">
      <p class="doc-alert">
        本站是雲際科技工作室發起與維護的公益專案，與政府機關無關。資料可能延遲或有誤，重要決定請以各機關公告為準。
      </p>

      <h2>關於本站</h2>
      <p>
        dashboard.tw
        把散落各處的政府開放資料，整理成一目了然的儀表板，並長期保存歷史資料。官方只提供即時數值的資料，本站也會保留下來。
      </p>
      <p>
        圖表可以下載與嵌入，資料也能透過
        <RouterLink to="/developers">開放資料 API</RouterLink> 免費取得。
      </p>

      <h2 id="sources">資料來源與授權</h2>
      <p>
        政府開放資料依<a href="https://data.gov.tw/license" rel="noopener"
          >政府資料開放授權條款第 1 版</a
        >使用。地圖上的動態雲圖來自 NASA GIBS
        提供的向日葵衛星影像，縣市界線來自內政部國土測繪中心。每張圖表下方都有標示來源機關。
      </p>
    </div>

    <section class="card sources" aria-label="資料來源清單">
      <div class="tools">
        <input
          v-model="query"
          class="input"
          type="search"
          placeholder="搜尋資料或機關"
          aria-label="搜尋資料來源"
        />
        <select v-model="agency" class="select" aria-label="依機關篩選">
          <option value="">全部機關（{{ sources.length }}）</option>
          <option v-for="a in agencies" :key="a.name" :value="a.name">
            {{ a.name }}（{{ a.n }}）
          </option>
        </select>
      </div>
      <div class="table-wrap">
        <table class="doc-table">
          <thead>
            <tr>
              <th>資料</th>
              <th>提供機關</th>
              <th>更新頻率</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in shown" :key="s.url + s.name">
              <td>
                <a :href="s.url" rel="noopener" target="_blank">{{ s.name }}</a>
                <span class="meta">{{ s.agency }}・{{ s.freq }}</span>
              </td>
              <td>{{ s.agency }}</td>
              <td class="freq">{{ s.freq }}</td>
            </tr>
            <tr v-if="!shown.length">
              <td colspan="3" class="empty">找不到符合的資料來源</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div class="doc">
      <h2>原始碼</h2>
      <p>
        原始碼以
        <a href="https://www.gnu.org/licenses/agpl-3.0.html" rel="noopener">AGPL-3.0</a> 授權公開於
        <a href="https://github.com/dashboard-tw" rel="noopener">GitHub</a>，架構參考<a
          href="https://github.com/tpe-doit/Taipei-City-Dashboard"
          rel="noopener"
          >臺北城市儀表板</a
        >。
      </p>

      <h2>聯絡我們</h2>
      <p>
        一般聯絡請寄
        <a href="mailto:support@dashboard.tw">support@dashboard.tw</a
        >。資料有誤或有任何建議，也可以到
        <a href="https://github.com/dashboard-tw/Taiwan-Dashboard/issues" rel="noopener"
          >GitHub Issues</a
        >
        回報。
      </p>
      <p>
        想交流或即時討論，歡迎加入我們的
        <a href="https://discord.gg/gU5gndueZH" rel="noopener">Discord</a>。
      </p>
    </div>
  </div>
</template>

<style scoped>
#sources {
  scroll-margin-top: calc(var(--header-h) + 1rem);
}
.sources {
  max-width: 62.5rem;
  margin: 0.25rem 0 0.5rem;
  padding: var(--font-m);
}
.tools {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 0.5rem;
}
.input {
  flex: 1 1 14rem;
  max-width: 20rem;
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
  font-size: var(--font-ms);
}
.table-wrap {
  overflow-x: auto;
}
.doc-table td a {
  color: var(--text-primary);
  text-decoration: none;
}
.doc-table td a:hover {
  color: var(--accent);
  text-decoration: underline;
}
.doc-table td {
  color: var(--text-secondary);
}
.freq {
  white-space: nowrap;
}
.meta {
  display: none;
}
@media (max-width: 600px) {
  .doc-table th:not(:first-child),
  .doc-table td:not(:first-child) {
    display: none;
  }
  .meta {
    display: block;
    color: var(--text-muted);
    font-size: 0.8rem;
  }
}
.empty {
  padding: 1.5rem 0.75rem;
  color: var(--text-muted);
  text-align: center;
}
</style>
