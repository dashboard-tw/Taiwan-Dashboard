<script setup lang="ts">
import { useCatalog } from '../stores/catalog'

defineProps<{ modelValue: string | null; nationalLabel?: string }>()
const emit = defineEmits<{ 'update:modelValue': [code: string | null] }>()
const catalog = useCatalog()
</script>

<template>
  <label>
    <span class="visually-hidden">選擇縣市</span>
    <select
      class="select"
      :value="modelValue ?? ''"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value || null)"
    >
      <option value="">{{ nationalLabel ?? '全國' }}</option>
      <option v-for="c in catalog.counties" :key="c.code" :value="c.code">{{ c.name }}</option>
    </select>
  </label>
</template>
