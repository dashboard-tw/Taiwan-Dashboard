<script setup lang="ts">
import { ref } from 'vue'
import { useDialogs } from '../../stores/dialog'
import DialogContainer from './DialogContainer.vue'

const dialogs = useDialogs()
const hide = ref(false)

function close() {
  if (hide.value) dialogs.dismissIntro()
  dialogs.intro = false
}
</script>

<template>
  <DialogContainer :open="dialogs.intro" label="臺灣儀表板使用說明" @close="close">
    <div class="intro">
      <h2>臺灣儀表板使用說明</h2>
      <p>
        歡迎使用臺灣儀表板。本站的目的是 1. 讓大家一眼看懂全臺的天氣、空品、水情與電力等現況 2.
        保留歷史紀錄，方便比較與研究 3. 推廣政府開放資料的應用。
      </p>
      <p>
        網站上的資料都來自各機關公開的政府開放資料，經過整理後在這裡展示，也提供開放資料 API
        給開發者免費使用。本站是雲際科技工作室的公益專案，不是政府網站。
      </p>
      <p>想申請開放資料 API，請點右上方的「登入」，使用 GitHub 或 Google 帳號登入。</p>
      <p class="tip">如果要在地圖上查看，請點正上方的「地圖模式」，再勾選想看的項目。</p>
      <footer>
        <label class="check">
          <input v-model="hide" type="checkbox" />
          下次不再顯示此視窗
        </label>
        <button type="button" class="btn-highlight" @click="close">確定了解</button>
      </footer>
    </div>
  </DialogContainer>
</template>

<style scoped>
.intro {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  width: min(340px, calc(100vw - 4rem));
  padding: 4px 2px 0;
  font-size: 0.875rem;
  line-height: 1.6;
}
h2 {
  margin-bottom: 0.25rem;
  font-size: 1.15rem;
  font-weight: 700;
}
p {
  margin: 0;
}
.tip {
  color: var(--danger-text);
}
footer {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 0.5rem;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 1rem;
  cursor: pointer;
}
.check input {
  width: 16px;
  height: 16px;
}
.btn-highlight {
  align-self: flex-end;
}
</style>
