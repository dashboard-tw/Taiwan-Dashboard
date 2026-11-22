import { onBeforeUnmount, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'

const queue: (() => void)[] = []
let running = false

const channel = typeof MessageChannel === 'undefined' ? null : new MessageChannel()
const waiting: (() => void)[] = []
if (channel) channel.port1.onmessage = () => waiting.shift()?.()

function yieldThen(fn: () => void) {
  if (!channel) return void setTimeout(fn, 0)
  waiting.push(fn)
  channel.port2.postMessage(null)
}

function next() {
  const job = queue.shift()
  if (!job) {
    running = false
    return
  }
  job()
  yieldThen(next)
}

export function useRenderTurn(when: MaybeRefOrGetter<boolean>): Ref<boolean> {
  const ready = ref(false)
  const job = () => {
    ready.value = true
  }
  watch(
    () => toValue(when),
    (ok) => {
      if (!ok || ready.value || queue.includes(job)) return
      queue.push(job)
      if (!running) {
        running = true
        next()
      }
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    const i = queue.indexOf(job)
    if (i >= 0) queue.splice(i, 1)
  })
  return ready
}
