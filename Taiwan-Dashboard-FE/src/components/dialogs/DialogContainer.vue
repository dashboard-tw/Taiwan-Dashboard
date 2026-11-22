<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{ open: boolean; label: string; instant?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const panel = ref<HTMLElement | null>(null)
let previous: Element | null = null

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    emit('close')
    return
  }
  if (e.key !== 'Tab' || !panel.value) return
  const items = [...panel.value.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
    (el) => el.getClientRects().length > 0,
  )
  const first = items[0]
  const last = items[items.length - 1]
  if (!first || !last) {
    e.preventDefault()
    return
  }
  const active = document.activeElement
  if (e.shiftKey && (active === first || active === panel.value)) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && active === last) {
    e.preventDefault()
    first.focus()
  }
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      previous = document.activeElement
      await nextTick()
      panel.value?.focus()
    } else if (previous instanceof HTMLElement) {
      previous.focus()
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (previous instanceof HTMLElement) previous.focus()
})
</script>

<template>
  <Teleport to="body">
    <Transition name="dialog">
      <div v-if="open" :class="['dialog-container', { instant }]" @keydown="onKey">
        <div class="dialog-background" @click="emit('close')"></div>
        <div
          ref="panel"
          class="dialog-panel"
          role="dialog"
          aria-modal="true"
          :aria-label="label"
          tabindex="-1"
        >
          <slot />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dialog-container {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
}
.dialog-background {
  position: absolute;
  inset: 0;
  background: var(--overlay);
}
.dialog-panel {
  position: relative;
  max-width: calc(100vw - 2rem);
  max-height: calc(100vh - 2rem);
  overflow: auto;
  padding: var(--font-m);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text-primary);
  outline: none;
}
.dialog-enter-from,
.dialog-leave-to {
  opacity: 0;
}
.dialog-enter-from .dialog-panel,
.dialog-leave-to .dialog-panel {
  transform: translateY(-2.25rem);
}
.dialog-enter-active,
.dialog-leave-active {
  transition: opacity 0.3s ease;
}
.dialog-enter-active .dialog-panel,
.dialog-leave-active .dialog-panel {
  transition: transform 0.3s ease;
}
.instant.dialog-enter-active,
.instant.dialog-enter-active .dialog-panel {
  transition: none;
}
</style>
