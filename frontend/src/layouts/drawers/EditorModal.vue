<template>
  <Modal :open="open" :title="title" :width="760" @close="$emit('close')">
    <div style="display: flex; flex-direction: column; flex: 1;">
      <slot name="before" />
      <textarea
        class="input mono editor-area"
        spellcheck="false"
        :value="controlled ? content : draft"
        @input="onInput(($event.target as HTMLTextAreaElement).value)"
      />
      <slot name="after" />
    </div>
    <template #footer>
      <Btn sm @click="$emit('close')">{{ $t('ui.cancel') }}</Btn>
      <Btn variant="primary" sm @click="save">{{ $t('actions.save') }}</Btn>
    </template>
  </Modal>
</template>

<script lang="ts" setup>
import { ref, watch } from 'vue'
import Modal from '@/components/ui/Modal.vue'
import Btn from '@/components/ui/Btn.vue'

const props = defineProps<{
  open: boolean
  title: string
  content: string
  controlled?: boolean
  keepOpenOnSave?: boolean
}>()
const emit = defineEmits<{ close: []; save: [value: string]; 'update:content': [value: string] }>()

const draft = ref('')
watch(() => props.open, (v) => { if (v && !props.controlled) draft.value = props.content }, { immediate: true })
const onInput = (value: string) => {
  if (props.controlled) emit('update:content', value)
  else draft.value = value
}
const save = () => {
  emit('save', props.controlled ? props.content : draft.value)
  if (!props.keepOpenOnSave) emit('close')
}
</script>

<style scoped>
.editor-area {
  border: none;
  border-radius: 0;
  min-height: 380px;
  resize: vertical;
  line-height: 1.6;
  padding: 16px;
  font-size: 12.5px;
  background: var(--surface-3);
}
.editor-area:focus { box-shadow: none; border: none; background: var(--surface-3); }
</style>
