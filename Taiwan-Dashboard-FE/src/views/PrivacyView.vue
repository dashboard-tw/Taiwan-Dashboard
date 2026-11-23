<script setup lang="ts">
import { usePageTitle } from '../composables/usePageTitle'

usePageTitle('隱私權政策')

const CONTACT = 'support@dashboard.tw'
const TOC = [
  '適用範圍',
  '蒐集的個人資料',
  '蒐集目的與利用方式',
  '利用期間、地區、對象及方式',
  '第三方服務與跨境傳輸',
  'Cookie 與瀏覽器儲存',
  '資料安全',
  '保存期限與刪除',
  '您的權利',
  '未成年人',
  '外部連結',
  '政策修訂',
  '聯絡方式',
]
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">privacy_tip</span>隱私權政策</h1>

    <div class="doc">
      <p class="updated">制定日期：2026 年 11 月 21 日，修訂日期：2026 年 11 月 23 日</p>
      <p>
        dashboard.tw（以下稱「本站」）由雲際科技工作室經營，重視您的隱私。本政策依《個人資料保護法》（以下稱「個資法」）說明本站如何蒐集、處理、利用及保護您的個人資料。使用本站即表示您已閱讀並同意本政策。
      </p>

      <nav class="toc" aria-label="目錄">
        <ol>
          <li v-for="(t, i) in TOC" :key="t">
            <a :href="`#p${i + 1}`">{{ t }}</a>
          </li>
        </ol>
      </nav>

      <h2 id="p1">第一條　適用範圍</h2>
      <p>
        本政策適用於本站網站、開放資料 API、嵌入圖表與相關服務。本站連結的其他網站，不適用本政策。
      </p>

      <h2 id="p2">第二條　蒐集的個人資料</h2>
      <p>一、瀏覽網站：不需要登入，本站不要求您提供任何個人資料。</p>
      <p>二、使用帳號功能時，本站會蒐集以下資料：</p>
      <table class="doc-table">
        <thead>
          <tr>
            <th>項目</th>
            <th>內容</th>
            <th>個資類別</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>帳號資料</td>
            <td>顯示名稱、GitHub 或 Google 的使用者代碼、建立與最後登入時間</td>
            <td>C001 辨識個人者</td>
          </tr>
          <tr>
            <td>Email</td>
            <td>GitHub 或 Google 標示為已驗證的 Email</td>
            <td>C001 辨識個人者</td>
          </tr>
          <tr>
            <td>登入紀錄</td>
            <td>登入時的 IP 位址、瀏覽器資訊、最後使用時間</td>
            <td>C001 辨識個人者</td>
          </tr>
          <tr>
            <td>API 使用資料</td>
            <td>
              API 憑證的 Client Id、Client Secret 與 Access Token 的雜湊值，每日呼叫與被拒絕次數
            </td>
            <td>其他：服務使用紀錄</td>
          </tr>
          <tr>
            <td>使用調查</td>
            <td>身分類別、資料用途、關注的資料、應用名稱與連結、建議</td>
            <td>其他：問卷回覆</td>
          </tr>
          <tr>
            <td>應用活化投稿</td>
            <td>應用名稱、說明、連結、Logo、網站截圖、是否公開名稱</td>
            <td>其他：使用者上傳內容</td>
          </tr>
          <tr>
            <td>操作紀錄</td>
            <td>登入、登出、API 憑證異動、刪除帳號等事件與當時的 IP 位址</td>
            <td>其他：系統稽核紀錄</td>
          </tr>
        </tbody>
      </table>
      <p>
        三、本站不會向 GitHub 或 Google
        索取上述以外的資料，也不保存這兩個平台發給本站的登入憑證。本站不設密碼。
      </p>
      <p>四、「用我的位置找縣市」只在您的瀏覽器內計算，位置不會傳到本站伺服器。</p>

      <h2 id="p3">第三條　蒐集目的與利用方式</h2>
      <p>本站蒐集個人資料的特定目的如下：</p>
      <ul>
        <li>〇六九 契約、類似契約或其他法律關係事務：提供帳號、登入與 API 憑證服務。</li>
        <li>一三五 資（通）訊服務：提供網站、API 與嵌入圖表。</li>
        <li>一三六 資（通）訊與資料庫管理：維護系統安全，防止濫用與異常登入。</li>
        <li>一五七 調查、統計與研究分析：以統計方式了解資料用途，決定優先維護的資料。</li>
      </ul>
      <p>
        Email 用來寄送帳號與服務的通知，例如註冊成功、新裝置登入、登入方式或 API 憑證異動、API
        用量快到上限、應用審查結果。也用於帳號異常、服務重大變更時與您聯絡，以及確認來信者是否為帳號本人。本站不寄送廣告，也不出售、出租或交換您的個人資料。
      </p>

      <h2 id="p4">第四條　利用期間、地區、對象及方式</h2>
      <ul>
        <li>期間：自您建立帳號起，至您刪除帳號或本站停止服務為止，另依第八條的保存期限處理。</li>
        <li>地區：中華民國境內，以及第五條所列第三方服務的所在地。</li>
        <li>
          對象：雲際科技工作室。除第五條的服務提供者、依法令要求或司法機關依法調取外，不提供給第三方。
        </li>
        <li>方式：以電子方式蒐集、處理與利用，並依第七條的方式保護。</li>
      </ul>

      <h2 id="p5">第五條　第三方服務與跨境傳輸</h2>
      <p>本站使用以下服務，這些服務可能在中華民國境外處理資料：</p>
      <ul>
        <li>Cloudflare：網站連線與安全防護，會依其隱私權政策處理連線資料。</li>
        <li>GitHub、Google：提供登入。您登入時，由這兩個平台依其隱私權政策處理您的資料。</li>
        <li>Brevo：寄送通知信，會處理您的 Email 與信件內容。</li>
        <li>
          Discord：本站的社群。應用上架時，會把應用名稱、說明、Logo、截圖、連結，以及您選擇公開的名稱發到社群頻道。
        </li>
        <li>
          Google Analytics：統計瀏覽人數、看了哪些頁面與從哪裡來，用來改善網站。不會傳送您的名稱或
          Email。
        </li>
      </ul>
      <p>本站伺服器與資料庫位於中華民國境內。本站沒有廣告。</p>
      <p>
        不想被統計，可以安裝 Google 的<a
          href="https://tools.google.com/dlpage/gaoptout"
          rel="noopener"
          >停用 Google Analytics 外掛程式</a
        >，或在瀏覽器封鎖第三方 Cookie。
      </p>

      <h2 id="p6">第六條　Cookie 與瀏覽器儲存</h2>
      <p>
        本站使用維持服務運作所必要的 Cookie，以及 Google Analytics 的統計 Cookie，不使用廣告
        Cookie：
      </p>
      <table class="doc-table">
        <thead>
          <tr>
            <th>名稱</th>
            <th>用途</th>
            <th>效期</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>dtw_session</td>
            <td>維持登入狀態</td>
            <td>7 天，登出即失效</td>
          </tr>
          <tr>
            <td>dtw_oauth</td>
            <td>確認登入流程由同一個瀏覽器發起</td>
            <td>10 分鐘</td>
          </tr>
          <tr>
            <td>dtw_link</td>
            <td>綁定另一個登入方式時，暫存待確認的綁定</td>
            <td>10 分鐘</td>
          </tr>
          <tr>
            <td>_ga、_ga_*</td>
            <td>Google Analytics 區分不同的訪客與瀏覽階段</td>
            <td>2 年</td>
          </tr>
        </tbody>
      </table>
      <p>
        深淺色、側邊欄收合、說明視窗的顯示設定，以及加快載入的畫面資料暫存，皆保存在瀏覽器的
        localStorage 本地中。正式環境的 Cookie 名稱會加上 __Host- 前綴，只限本站使用。
      </p>

      <h2 id="p7">第七條　資料安全</h2>
      <ul>
        <li>全站使用 HTTPS 加密連線。</li>
        <li>
          登入憑證、Client Secret 與 Access Token 只保存雜湊值，資料庫外洩也無法用來登入或呼叫 API。
        </li>
        <li>Email 以 AES-256-GCM 加密保存，金鑰與資料庫分開存放。</li>
        <li>資料庫依帳號隔離存取權限，管理功能需另外授權。</li>
        <li>
          發生個人資料被竊取、洩漏或竄改時，本站會查明後以 Email 或網站公告通知受影響的使用者。
        </li>
      </ul>

      <h2 id="p8">第八條　保存期限與刪除</h2>
      <table class="doc-table">
        <thead>
          <tr>
            <th>資料</th>
            <th>保存期限</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>帳號資料、Email、API 使用資料、使用調查</td>
            <td>至您刪除帳號為止</td>
          </tr>
          <tr>
            <td>已刪除的 API 憑證與其用量紀錄</td>
            <td>刪除後 90 天</td>
          </tr>
          <tr>
            <td>應用活化投稿</td>
            <td>至您刪除應用或帳號為止</td>
          </tr>
          <tr>
            <td>登入紀錄</td>
            <td>登出或過期後 30 天</td>
          </tr>
          <tr>
            <td>通知信的寄送紀錄</td>
            <td>30 天，寄送失敗的保留 90 天</td>
          </tr>
          <tr>
            <td>退信或被檢舉為垃圾信的信箱</td>
            <td>只保存雜湊值，至恢復收信為止</td>
          </tr>
          <tr>
            <td>操作紀錄中的 IP 位址</td>
            <td>90 天</td>
          </tr>
          <tr>
            <td>操作紀錄</td>
            <td>1 年</td>
          </tr>
          <tr>
            <td>未完成的登入或綁定流程</td>
            <td>10 分鐘</td>
          </tr>
          <tr>
            <td>伺服器記憶體中的連線 IP</td>
            <td>僅在記憶體中短暫保存，不寫入資料庫</td>
          </tr>
          <tr>
            <td>Google Analytics 統計資料</td>
            <td>14 個月</td>
          </tr>
        </tbody>
      </table>
      <p>
        您刪除帳號後，上述資料會立即刪除。操作紀錄只保留不含帳號的事件，用於安全稽核。資料庫備份會依備份週期自動輪替刪除。
      </p>

      <h2 id="p9">第九條　您的權利</h2>
      <p>依個資法第三條，您可以就您的個人資料行使以下權利：</p>
      <ul>
        <li>查詢或請求閱覽。</li>
        <li>請求製給複製本。</li>
        <li>請求補充或更正。</li>
        <li>請求停止蒐集、處理或利用。</li>
        <li>請求刪除。</li>
      </ul>
      <p>
        您可以在<RouterLink to="/me">我的帳號</RouterLink
        >修改名稱、管理登入方式或刪除帳號，在<RouterLink to="/me/devices">登入裝置</RouterLink
        >登出任何裝置。其他請求請依第十三條與本站聯絡，本站會先確認您是帳號本人，並依個資法規定的期限回覆。
      </p>
      <p>您可以不提供個人資料，但將無法使用需要登入的功能，瀏覽網站不受影響。</p>

      <h2 id="p10">第十條　未成年人</h2>
      <p>
        未成年人應在法定代理人同意下使用需要登入的功能。法定代理人發現未成年人未經同意提供個人資料時，可依第十三條請求刪除。
      </p>

      <h2 id="p11">第十一條　外部連結</h2>
      <p>
        本站包含政府機關、GitHub 等外部網站的連結。這些網站的隱私權保護由其自行負責，不適用本政策。
      </p>

      <h2 id="p12">第十二條　政策修訂</h2>
      <p>
        本站可能因法令或服務調整修訂本政策，修訂後會公布於本頁並更新日期。重大變更會在網站公告，必要時以
        Email 通知。修訂後您繼續使用本站，視為同意修訂內容。
      </p>

      <h2 id="p13">第十三條　聯絡方式</h2>
      <p>對本政策或您的個人資料有任何問題，請透過以下方式聯絡：</p>
      <ul>
        <li>
          Email：<a :href="`mailto:${CONTACT}`">{{ CONTACT }}</a>
        </li>
        <li>
          <a href="https://github.com/dashboard-tw/Taiwan-Dashboard/issues" rel="noopener"
            >GitHub Issues</a
          >（公開，請勿在此留下個人資料）
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.doc-table {
  margin-bottom: 0.75rem;
}
.doc-table td {
  color: var(--text-secondary);
}
.doc h2 {
  scroll-margin-top: calc(var(--header-h) + 1rem);
}
.toc {
  margin: 1rem 0 0.5rem;
  padding: 0.75rem 1rem;
  border-radius: var(--radius);
  background: var(--surface);
}
.toc ol {
  columns: 2;
  margin: 0;
  padding-left: 1.5rem;
  font-size: 0.925rem;
}
.toc li + li {
  margin-top: 0.2rem;
}
.toc a {
  color: var(--text-secondary);
  text-decoration: none;
}
.toc a:hover {
  color: var(--accent);
}
@media (max-width: 37.5em) {
  .toc ol {
    columns: 1;
  }
}
</style>
