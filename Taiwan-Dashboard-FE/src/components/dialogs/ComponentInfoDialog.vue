<script setup lang="ts">
import { useDialogs } from '../../stores/dialog'
import ComponentInfoBody from './ComponentInfoBody.vue'
import DialogContainer from './DialogContainer.vue'

const dialogs = useDialogs()
</script>

<template>
  <DialogContainer
    :open="!!dialogs.info"
    :label="dialogs.info ? `${dialogs.info.component.name}組件資訊` : '組件資訊'"
    @close="dialogs.closeAll"
  >
    <ComponentInfoBody
      v-if="dialogs.info"
      class="in-dialog"
      :component="dialogs.info.component"
      :county="dialogs.info.county"
    />
  </DialogContainer>
</template>
