<script setup lang="ts">
defineProps<{
  label: string
}>()

const emit = defineEmits<{
  'resize-start': [event: MouseEvent]
  'auto-fit': []
}>()

function onResizerMouseDown(event: MouseEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  event.stopPropagation()
  emit('resize-start', event)
}

function onResizerDoubleClick(event: MouseEvent) {
  event.preventDefault()
  event.stopPropagation()
  emit('auto-fit')
}
</script>

<template>
  <div class="task-table-header">
    <span class="task-table-header__label">{{ label }}</span>
    <span
      class="task-table-header__resizer"
      title="拖动调整列宽，双击自动适应"
      @mousedown="onResizerMouseDown"
      @dblclick="onResizerDoubleClick"
    />
  </div>
</template>

<style scoped lang="scss">
.task-table-header {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 23px;
  padding-right: 10px;
  user-select: none;
}

.task-table-header__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-table-header__resizer {
  position: absolute;
  top: 0;
  right: -6px;
  z-index: 2;
  width: 10px;
  height: 100%;
  cursor: col-resize;
  touch-action: none;

  &::after {
    content: '';
    position: absolute;
    top: 18%;
    bottom: 18%;
    right: 4px;
    width: 1px;
    background: color-mix(in srgb, var(--el-border-color) 70%, transparent);
    transition: background-color var(--dd-motion-fast) var(--dd-ease-standard);
  }

  &:hover::after {
    background: var(--el-color-primary);
  }
}
</style>
