<script setup lang="ts">
import ChartSkeleton from './ChartSkeleton.vue'

withDefaults(defineProps<{ kind?: 'bar' | 'donut' | 'line' | 'map'; pills?: boolean }>(), {
  kind: 'bar',
  pills: false,
})
</script>

<template>
  <div class="card-skeleton" role="status" aria-label="載入中">
    <div class="head">
      <span class="skeleton title"></span>
      <span class="skeleton tag"></span>
    </div>
    <span class="skeleton sub"></span>
    <span class="skeleton city"></span>
    <div v-if="pills" class="pills">
      <span class="skeleton pill"></span>
      <span class="skeleton pill"></span>
    </div>
    <div class="chart"><ChartSkeleton :kind="kind" /></div>
    <div class="foot">
      <span class="skeleton btn"></span>
      <slot name="foot-right"><span class="skeleton info"></span></slot>
    </div>
  </div>
</template>

<style scoped>
.card-skeleton {
  display: flex;
  flex-direction: column;
  gap: 6px;
  height: 330px;
  padding: var(--font-m);
  border-radius: var(--radius);
  background: var(--surface);
}
@media (min-width: 1050px) {
  .card-skeleton {
    height: 370px;
  }
}
@media (min-width: 1650px) {
  .card-skeleton {
    height: 400px;
  }
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.title {
  width: 45%;
  height: 1.1rem;
}
.tag {
  width: 4.5rem;
  height: 0.9rem;
}
.sub {
  width: 32%;
  height: 0.65rem;
}
.city {
  width: 2.2rem;
  height: 0.9rem;
}
.pills {
  display: flex;
  justify-content: center;
  gap: 4px;
  padding-top: 4px;
}
.pill {
  width: 3.5rem;
  height: 1rem;
}
.chart {
  flex: 1;
  min-height: 0;
  padding-top: 8px;
}
.foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.btn {
  width: 5rem;
  height: 1.3rem;
}
.info {
  width: 5.5rem;
  height: 1rem;
}
</style>
