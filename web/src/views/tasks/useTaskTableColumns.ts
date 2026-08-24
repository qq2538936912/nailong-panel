import { computed, nextTick, onBeforeUnmount, ref, type ComputedRef, type Ref } from 'vue'
import type { TableInstance } from 'element-plus'

const STORAGE_KEY = 'dd:tasks:table_column_widths'

export const TASK_TABLE_COLUMN_KEYS = [
  'name',
  'command',
  'cron',
  'status',
  'lastRun',
  'nextRun',
  'lastResult',
  'duration',
] as const

export type TaskTableColumnKey = (typeof TASK_TABLE_COLUMN_KEYS)[number]

type ColumnMeta = {
  label: string
  min: number
  max: number
  defaultWidth: number
  narrowWidth: number
  className: string
}

const COLUMN_META: Record<TaskTableColumnKey, ColumnMeta> = {
  name: { label: '任务名称', min: 80, max: 520, defaultWidth: 220, narrowWidth: 160, className: 'task-col-name' },
  command: { label: '命令 / 脚本', min: 70, max: 560, defaultWidth: 240, narrowWidth: 150, className: 'task-col-command' },
  cron: { label: '定时规则', min: 70, max: 360, defaultWidth: 180, narrowWidth: 130, className: 'task-col-cron' },
  status: { label: '状态', min: 80, max: 140, defaultWidth: 110, narrowWidth: 90, className: 'task-col-status' },
  lastRun: { label: '最后运行', min: 120, max: 200, defaultWidth: 160, narrowWidth: 160, className: 'task-col-last-run' },
  nextRun: { label: '下次运行', min: 120, max: 200, defaultWidth: 160, narrowWidth: 160, className: 'task-col-next-run' },
  lastResult: { label: '上次结果', min: 80, max: 140, defaultWidth: 100, narrowWidth: 84, className: 'task-col-last-result' },
  duration: { label: '耗时', min: 70, max: 120, defaultWidth: 90, narrowWidth: 90, className: 'task-col-duration' },
}

type StoredColumnWidths = Partial<Record<TaskTableColumnKey, number>>

function clampWidth(key: TaskTableColumnKey, value: number) {
  const meta = COLUMN_META[key]
  return Math.max(meta.min, Math.min(meta.max, Math.round(value)))
}

function readStoredColumnWidths(): StoredColumnWidths {
  if (typeof window === 'undefined') {
    return {}
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as StoredColumnWidths
    if (!parsed || typeof parsed !== 'object') return {}

    const next: StoredColumnWidths = {}
    for (const key of TASK_TABLE_COLUMN_KEYS) {
      const value = Number(parsed[key])
      if (Number.isFinite(value) && value > 0) {
        next[key] = clampWidth(key, value)
      }
    }
    return next
  } catch {
    return {}
  }
}

function persistColumnWidths(value: StoredColumnWidths) {
  if (typeof window === 'undefined') {
    return
  }
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(value))
}

function measureCellContentWidth(cell: HTMLElement) {
  const clone = cell.cloneNode(true) as HTMLElement
  clone.style.position = 'absolute'
  clone.style.visibility = 'hidden'
  clone.style.pointerEvents = 'none'
  clone.style.left = '-9999px'
  clone.style.top = '0'
  clone.style.width = 'auto'
  clone.style.maxWidth = 'none'
  clone.style.overflow = 'visible'
  clone.style.whiteSpace = 'nowrap'
  document.body.appendChild(clone)
  const width = clone.getBoundingClientRect().width
  document.body.removeChild(clone)
  return width + 24
}

export function useTaskTableColumns(options: {
  tableRef: Ref<TableInstance | undefined>
  isNarrowDesktop: ComputedRef<boolean>
}) {
  const storedWidths = ref<StoredColumnWidths>(readStoredColumnWidths())

  const visibleColumnKeys = computed(() =>
    TASK_TABLE_COLUMN_KEYS.filter((key) => {
      if (options.isNarrowDesktop.value && (key === 'lastRun' || key === 'duration')) {
        return false
      }
      return true
    }),
  )

  function defaultWidth(key: TaskTableColumnKey) {
    const meta = COLUMN_META[key]
    return options.isNarrowDesktop.value ? meta.narrowWidth : meta.defaultWidth
  }

  function columnWidth(key: TaskTableColumnKey) {
    return storedWidths.value[key] ?? defaultWidth(key)
  }

  function columnClassName(key: TaskTableColumnKey) {
    return COLUMN_META[key].className
  }

  function columnLabel(key: TaskTableColumnKey) {
    return COLUMN_META[key].label
  }

  function updateColumnWidth(key: TaskTableColumnKey, width: number) {
    storedWidths.value = {
      ...storedWidths.value,
      [key]: clampWidth(key, width),
    }
    persistColumnWidths(storedWidths.value)
  }

  let resizing:
    | {
        key: TaskTableColumnKey
        startX: number
        startWidth: number
      }
    | null = null

  function stopResize() {
    if (!resizing) return
    resizing = null
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    document.removeEventListener('mousemove', handleResizeMove)
    document.removeEventListener('mouseup', stopResize)
  }

  function handleResizeMove(event: MouseEvent) {
    if (!resizing) return
    const delta = event.clientX - resizing.startX
    updateColumnWidth(resizing.key, resizing.startWidth + delta)
  }

  function startColumnResize(key: TaskTableColumnKey, event: MouseEvent) {
    resizing = {
      key,
      startX: event.clientX,
      startWidth: columnWidth(key),
    }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    document.addEventListener('mousemove', handleResizeMove)
    document.addEventListener('mouseup', stopResize)
  }

  async function autoFitColumn(key: TaskTableColumnKey) {
    await nextTick()
    const tableEl = options.tableRef.value?.$el as HTMLElement | undefined
    if (!tableEl) return

    const className = COLUMN_META[key].className
    const cells = tableEl.querySelectorAll(`th.${className} .cell, td.${className} .cell`)
    let maxWidth = defaultWidth(key)

    cells.forEach((node) => {
      maxWidth = Math.max(maxWidth, measureCellContentWidth(node as HTMLElement))
    })

    updateColumnWidth(key, maxWidth)
  }

  async function autoFitAllColumns() {
    for (const key of visibleColumnKeys.value) {
      await autoFitColumn(key)
    }
  }

  function resetColumnWidths() {
    storedWidths.value = {}
    persistColumnWidths({})
  }

  onBeforeUnmount(() => {
    stopResize()
  })

  return {
    columnWidth,
    columnClassName,
    columnLabel,
    startColumnResize,
    autoFitColumn,
    autoFitAllColumns,
    resetColumnWidths,
  }
}
