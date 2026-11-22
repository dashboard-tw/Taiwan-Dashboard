<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <footer class="site-footer">
    <p>{{ t('site.unofficialNotice') }}</p>
    <nav aria-label="頁尾">
      <RouterLink to="/sitemap">網站導覽</RouterLink>
      <RouterLink to="/about">{{ t('nav.about') }}</RouterLink>
      <RouterLink to="/about#sources">{{ t('footer.sources') }}</RouterLink>
      <RouterLink to="/developers">{{ t('footer.apiDocs') }}</RouterLink>
      <RouterLink to="/showcase/app">{{ t('footer.apps') }}</RouterLink>
      <RouterLink to="/privacy">{{ t('footer.privacy') }}</RouterLink>
      <RouterLink to="/terms">{{ t('footer.terms') }}</RouterLink>
      <a href="https://github.com/dashboard-tw" rel="noopener">GitHub</a>
      <a href="https://discord.gg/gU5gndueZH" rel="noopener">Discord</a>
    </nav>
  </footer>
</template>

<style scoped>
.site-footer {
  margin-top: 2rem;
  padding: 1.5rem 1rem 2rem;
  border-top: 1px solid var(--border);
  font-size: 0.85rem;
  color: var(--text-muted);
  text-align: center;
}
p {
  margin: 0 0 0.6rem;
}
nav {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.4rem 1.2rem;
}
a {
  color: var(--text-secondary);
}
</style>
