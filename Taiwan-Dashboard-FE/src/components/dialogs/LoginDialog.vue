<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useDialogs } from '../../stores/dialog'
import BrandMark from '../layout/BrandMark.vue'
import LoginPanel from '../LoginPanel.vue'
import DialogContainer from './DialogContainer.vue'

const dialogs = useDialogs()
</script>

<template>
  <DialogContainer :open="!!dialogs.login" label="登入臺灣儀表板" @close="dialogs.closeLogin()">
    <div v-if="dialogs.login" class="login-dialog">
      <button type="button" class="close" aria-label="關閉" @click="dialogs.closeLogin()">
        <span class="mi" aria-hidden="true">close</span>
      </button>
      <header class="brand">
        <BrandMark class="logo" />
        <div>
          <p class="name">臺灣儀表板</p>
          <p class="name-en">Taiwan Dashboard</p>
        </div>
      </header>
      <p class="lead">登入後即可免費申請 API 憑證</p>
      <LoginPanel
        :key="dialogs.login.redirect + (dialogs.login.error ?? '')"
        :redirect="dialogs.login.redirect"
        :error="dialogs.login.error"
        @done="dialogs.closeLogin()"
      />
      <p class="terms">
        登入即代表同意
        <RouterLink to="/terms" @click="dialogs.closeLogin()">使用條款</RouterLink>
        與
        <RouterLink to="/privacy" @click="dialogs.closeLogin()">隱私權政策</RouterLink>
      </p>
    </div>
  </DialogContainer>
</template>

<style scoped>
.login-dialog {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: min(340px, calc(100vw - 4rem));
  padding: 18px 8px 4px;
}
.close {
  position: absolute;
  top: -6px;
  right: -6px;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
.close:hover {
  background: var(--surface-muted);
  color: var(--text-primary);
}
.brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.logo {
  width: 2.25rem;
  height: 2.25rem;
  color: var(--text-primary);
}
.name {
  font-size: 1.35rem;
  font-weight: 700;
  line-height: 1.2;
}
.name-en {
  color: var(--text-secondary);
  font-size: 0.8rem;
  letter-spacing: 0.02em;
}
.lead {
  color: var(--text-muted);
  font-size: 0.88rem;
  text-align: center;
}
.terms {
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.75;
  text-align: center;
}
.terms a {
  color: var(--accent);
}
</style>
