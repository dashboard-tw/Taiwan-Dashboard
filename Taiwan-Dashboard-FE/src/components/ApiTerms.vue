<script setup lang="ts">
withDefaults(defineProps<{ perDay: number; perMinute: number; mbPerDay?: number }>(), {
  mbPerDay: 300,
})
</script>

<template>
  <div class="terms">
    <h3>一、資料來源與授權</h3>
    <ul>
      <li>
        資料整理自環境部、經濟部水利署、台灣電力公司等機關的政府開放資料，依
        <a href="https://data.gov.tw/license" target="_blank" rel="noopener"
          >政府資料開放授權條款－第 1 版</a
        >
        提供，可自由使用，包括商業用途。
      </li>
      <li>
        使用時請標示<strong>原始提供機關</strong>，見回應中的 <code>meta.source</code>。
        也歡迎註明「資料整理：dashboard.tw」。
      </li>
    </ul>

    <h3>二、使用額度</h3>
    <ul>
      <li>
        每個帳號最多 3 組憑證，額度以帳號計算，所有憑證共用：每天 {{ perDay }} 次、{{ mbPerDay }} MB
        流量，每分鐘 {{ perMinute }} 次，每天午夜歸零。超過上限會回應 <code>429</code>。
      </li>
      <li>流量以回應內容的大小計算。資料沒變的 <code>304</code> 回應也算一次。</li>
      <li>請自行快取結果。需要長期歷史資料，請改用每月的大量下載。</li>
      <li>不得以多個帳號或其他方式規避額度。</li>
      <li>
        如需獲取更多的使用次數與流量的話可以參考申請應用活化，經我方審核通過後除展示以外也可以獲取更多的使用額度與流量。請見<RouterLink
          to="/me/showcase"
          >申請應用活化</RouterLink
        >。
      </li>
    </ul>

    <h3>三、保管憑證</h3>
    <ul>
      <li>請勿將 Client Secret 或 Access Token 分享給他人，或放在公開的網頁、程式碼庫中。</li>
      <li>如有外洩疑慮，請立即重新產生，舊的 key 會馬上失效。</li>
      <li>使用你的 key 送出的請求，由你負責。</li>
    </ul>

    <h3>四、禁止行為</h3>
    <ul>
      <li>對服務造成過度負擔、干擾其他使用者，或存取不屬於你的資料。</li>
      <li>用於違法用途，或宣稱本站資料為政府公告、本站為政府網站。</li>
    </ul>

    <h3>五、資料正確性</h3>
    <ul>
      <li>
        dashboard.tw
        是雲際科技工作室的公益專案。資料來源可能延遲、更正或中斷，本站不保證資料完全正確。
        涉及安全、健康或財產的決定，請以各機關的官方公告為準。
      </li>
    </ul>

    <h3>六、服務變更與停用</h3>
    <ul>
      <li>額度、資料集與欄位可能調整，重大變更會先在網站與社群中公告。</li>
      <li>違反本規範者，本站得停用其憑證或帳號。</li>
    </ul>

    <h3>七、使用調查與個人資料</h3>
    <ul>
      <li>
        使用調查僅做內部數據統計用途，不會提供給第三方機構，刪除帳號時將一併刪除。請見<RouterLink
          to="/privacy"
          >隱私權政策</RouterLink
        >。
      </li>
    </ul>
  </div>
</template>

<style scoped>
.terms {
  font-size: 0.875rem;
  line-height: 1.7;
}
h3 {
  margin: 0.75rem 0 0.25rem;
  font-size: 0.9375rem;
}
h3:first-child {
  margin-top: 0;
}
ul {
  margin: 0;
  padding-left: 1.2rem;
}
li {
  margin-bottom: 0.25rem;
}
a {
  color: var(--accent);
}
code {
  padding: 0 3px;
  border-radius: 3px;
  background: var(--surface-muted);
  font-size: 0.8125rem;
}
</style>
