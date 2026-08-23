<template>
  <div class="terminal-page dd-fixed-page dd-page-hide-heading">
    <div class="page-header">
      <div>
        <h2 class="page-title-with-icon"><el-icon><Monitor /></el-icon><span>终端管理</span></h2>
        <p class="page-subtitle">在面板运行环境里执行命令，例如安装 Playwright 浏览器</p>
      </div>
    </div>

    <div class="terminal-toolbar">
      <div class="terminal-toolbar__meta">
        <el-tag :type="statusTagType" size="small">{{ statusLabel }}</el-tag>
        <span v-if="info.work_dir" class="terminal-toolbar__path" :title="info.work_dir">
          {{ info.work_dir }}
        </span>
        <span v-if="info.shell" class="terminal-toolbar__shell">{{ info.shell }}</span>
      </div>
      <div class="terminal-toolbar__actions">
        <el-button size="small" :loading="connecting" @click="reconnect">重新连接</el-button>
        <el-button size="small" @click="clearScreen">清屏</el-button>
      </div>
    </div>

    <div class="terminal-shortcuts">
      <button
        v-for="item in shortcuts"
        :key="item.command"
        type="button"
        class="terminal-chip"
        :disabled="status !== 'connected'"
        @click="runShortcut(item.command)"
      >
        {{ item.label }}
      </button>
    </div>

    <div class="terminal-card" @click="focusTerminal">
      <div ref="hostRef" class="terminal-host"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onActivated, onBeforeUnmount, onMounted, ref } from 'vue'
import { Monitor } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { buildTerminalWsUrl, terminalApi, type TerminalInfo } from '@/api/terminal'
import { extractError } from '@/utils/error'
import { useThemeStore } from '@/stores/theme'

type TerminalStatus = 'idle' | 'connecting' | 'connected' | 'disconnected'

const shortcuts = [
  { label: 'playwright install chromium', command: 'playwright install chromium' },
  { label: 'npx playwright install chromium', command: 'npx playwright install chromium' },
  { label: 'python -m playwright install chromium', command: 'python -m playwright install chromium' },
  { label: 'playwright install-deps', command: 'playwright install-deps' },
]

const themeStore = useThemeStore()
const hostRef = ref<HTMLElement>()
const connecting = ref(false)
const status = ref<TerminalStatus>('idle')
const info = ref<TerminalInfo>({
  available: true,
  work_dir: '',
  shell: '',
  python: '',
  message: '',
})

const statusLabel = computed(() => {
  switch (status.value) {
    case 'connecting':
      return '连接中'
    case 'connected':
      return '已连接'
    case 'disconnected':
      return '已断开'
    default:
      return '未连接'
  }
})

const statusTagType = computed(() => {
  switch (status.value) {
    case 'connected':
      return 'success'
    case 'connecting':
      return 'warning'
    case 'disconnected':
      return 'danger'
    default:
      return 'info'
  }
})

let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let socket: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null
let connectGeneration = 0
let demoLine = ''

function isDemoMode() {
  return import.meta.env.VITE_DEMO === '1'
}

function writeBanner(payload?: { work_dir?: string; shell?: string; python?: string }) {
  if (!term) return
  const workDir = payload?.work_dir || info.value.work_dir
  const shell = payload?.shell || info.value.shell
  const python = payload?.python || info.value.python
  term.writeln('\x1b[32m面板终端\x1b[0m  ·  与任务执行同一套运行环境')
  if (workDir) term.writeln(`工作目录：${workDir}`)
  if (shell) term.writeln(`Shell：${shell}`)
  if (python) term.writeln(`Python：${python}`)
  term.writeln('常用：playwright install chromium   （需先在依赖管理里装好 playwright）')
  term.writeln('已内置 pip / npm / Playwright / Go 国内镜像，装依赖和浏览器不用再自己 export')
  if (info.value.message) term.writeln(`\x1b[33m${info.value.message}\x1b[0m`)
  term.writeln('')
}

function applyTheme() {
  if (!term) return
  term.options.theme = themeStore.isDark
    ? {
        background: '#0f1115',
        foreground: '#e6edf3',
        cursor: '#e6edf3',
        selectionBackground: '#3d4f6f',
      }
    : {
        background: '#111318',
        foreground: '#e6edf3',
        cursor: '#e6edf3',
        selectionBackground: '#3d4f6f',
      }
}

function focusTerminal() {
  term?.focus()
}

function clearScreen() {
  term?.clear()
  term?.scrollToBottom()
}

function fitTerminal() {
  if (!term || !fitAddon || !hostRef.value) return
  try {
    fitAddon.fit()
  } catch {
    // 容器尚未量到尺寸时 fit 会抛，下一帧再试即可
  }
}

function sendResize() {
  if (!term || !socket || socket.readyState !== WebSocket.OPEN) return
  socket.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
}

function runShortcut(command: string) {
  if (!term || status.value !== 'connected') return
  term.focus()
  if (isDemoMode()) {
    term.write(`${command}\r\n`)
    handleDemoCommand(command)
    demoLine = ''
    term.write('\x1b[32mpanel\x1b[0m:/opt/panel/scripts$ ')
    return
  }
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(`${command}\r`)
  }
}

function closeSocket() {
  if (!socket) return
  const current = socket
  socket = null
  current.onopen = null
  current.onmessage = null
  current.onerror = null
  current.onclose = null
  if (current.readyState === WebSocket.OPEN || current.readyState === WebSocket.CONNECTING) {
    current.close()
  }
}

function startDemoTerminal() {
  if (!term) return
  status.value = 'connected'
  info.value = {
    available: true,
    work_dir: '/opt/panel/scripts',
    shell: '/bin/bash',
    python: '/opt/panel/data/deps/python/3.12/bin/python',
    message: '演示环境不会在真实机器上执行命令',
  }
  writeBanner()
  demoLine = ''
  term.write('\x1b[32mpanel\x1b[0m:/opt/panel/scripts$ ')
}

function handleDemoInput(data: string) {
  if (!term) return
  if (data === '\r') {
    term.write('\r\n')
    handleDemoCommand(demoLine.trim())
    demoLine = ''
    term.write('\x1b[32mpanel\x1b[0m:/opt/panel/scripts$ ')
    return
  }
  if (data === '\u007f') {
    if (!demoLine) return
    demoLine = demoLine.slice(0, -1)
    term.write('\b \b')
    return
  }
  if (data >= ' ') {
    demoLine += data
    term.write(data)
  }
}

function handleDemoCommand(command: string) {
  if (!term) return
  if (!command) return
  if (command.includes('playwright')) {
    term.writeln('Downloading Chromium 141.0.x (playwright build vxxxx)')
    term.writeln('Chromium downloaded to /opt/panel/data/.home/.cache/ms-playwright/chromium-xxxx')
    term.writeln('\x1b[33m演示环境未真正下载浏览器。\x1b[0m')
    return
  }
  term.writeln(`演示环境不会执行：${command}`)
}

async function connect() {
  if (isDemoMode() || !term) return

  const generation = ++connectGeneration
  connecting.value = true
  status.value = 'connecting'
  closeSocket()

  try {
    const summary = await terminalApi.info()
    if (generation !== connectGeneration) return
    info.value = summary.data
  } catch (error) {
    if (generation !== connectGeneration) return
    connecting.value = false
    status.value = 'disconnected'
    ElMessage.error(extractError(error, '获取终端信息失败'))
    return
  }

  try {
    const ticket = await terminalApi.ticket()
    if (generation !== connectGeneration) return
    const ws = new WebSocket(buildTerminalWsUrl(ticket.ticket, term.cols, term.rows))
    ws.binaryType = 'arraybuffer'
    socket = ws

    ws.onopen = () => {
      if (generation !== connectGeneration) return
      connecting.value = false
      status.value = 'connected'
      sendResize()
      term?.focus()
    }

    ws.onmessage = (event) => {
      if (!term || generation !== connectGeneration) return
      if (typeof event.data === 'string') {
        handleControlMessage(event.data)
        return
      }
      term.write(new Uint8Array(event.data as ArrayBuffer))
    }

    ws.onerror = () => {
      if (generation !== connectGeneration) return
      connecting.value = false
      status.value = 'disconnected'
    }

    ws.onclose = () => {
      if (generation !== connectGeneration) return
      connecting.value = false
      if (status.value !== 'disconnected') {
        status.value = 'disconnected'
        term?.writeln('\r\n\x1b[33m终端已断开。\x1b[0m')
      }
    }
  } catch (error) {
    if (generation !== connectGeneration) return
    connecting.value = false
    status.value = 'disconnected'
    ElMessage.error(extractError(error, '无法建立终端连接。如果前面有反向代理，需要放行 WebSocket Upgrade。'))
  }
}

function handleControlMessage(raw: string) {
  if (!term) return
  try {
    const payload = JSON.parse(raw) as {
      type?: string
      message?: string
      work_dir?: string
      shell?: string
      python?: string
      code?: number
    }
    if (payload.type === 'ready') {
      if (payload.work_dir) info.value.work_dir = payload.work_dir
      if (payload.shell) info.value.shell = payload.shell
      if (payload.python) info.value.python = payload.python
      writeBanner(payload)
      return
    }
    if (payload.type === 'error') {
      status.value = 'disconnected'
      term.writeln(`\x1b[31m${payload.message || '终端启动失败'}\x1b[0m`)
      ElMessage.error(payload.message || '终端启动失败')
      return
    }
    if (payload.type === 'exit') {
      status.value = 'disconnected'
      term.writeln(`\r\n\x1b[33m进程已退出（${payload.code ?? '?'}）。\x1b[0m`)
      return
    }
  } catch {
    term.write(raw)
  }
}

async function reconnect() {
  if (isDemoMode()) {
    term?.clear()
    startDemoTerminal()
    return
  }
  term?.clear()
  await connect()
}

async function setupTerminal() {
  if (term || !hostRef.value) return

  term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace',
    scrollback: 5000,
    convertEol: false,
  })
  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.open(hostRef.value)
  applyTheme()
  term.onData((data) => {
    if (isDemoMode()) {
      handleDemoInput(data)
      return
    }
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(data)
    }
  })
  await nextTick()
  fitTerminal()

  resizeObserver = new ResizeObserver(() => {
    fitTerminal()
    sendResize()
  })
  resizeObserver.observe(hostRef.value)

  if (isDemoMode()) {
    startDemoTerminal()
    return
  }
  await connect()
}

onMounted(async () => {
  await setupTerminal()
})

onActivated(() => {
  fitTerminal()
  sendResize()
  term?.focus()
})

onBeforeUnmount(() => {
  connectGeneration += 1
  resizeObserver?.disconnect()
  resizeObserver = null
  closeSocket()
  term?.dispose()
  term = null
  fitAddon = null
})
</script>

<style scoped lang="scss">
.terminal-page {
  min-width: 0;
}

.terminal-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
  min-width: 0;
}

.terminal-toolbar__meta {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.terminal-toolbar__path,
.terminal-toolbar__shell {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terminal-toolbar__actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.terminal-shortcuts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}

.terminal-chip {
  border: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-light);
  color: var(--el-text-color-regular);
  font-size: 12px;
  line-height: 1;
  padding: 7px 10px;
  cursor: pointer;

  &:hover:not(:disabled) {
    color: var(--el-color-primary);
    border-color: var(--el-color-primary-light-5);
  }

  &:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }
}

.terminal-card {
  flex: 1 1 0;
  height: 0;
  min-height: 280px;
  min-width: 0;
  background: #111318;
  border: 1px solid var(--el-border-color-lighter);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.terminal-host {
  flex: 1 1 0;
  min-height: 0;
  padding: 10px;
}

.terminal-host :deep(.xterm),
.terminal-host :deep(.xterm-viewport) {
  height: 100%;
}

@media screen and (max-width: 768px) {
  .terminal-toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .terminal-toolbar__actions {
    justify-content: flex-end;
  }

  .terminal-card {
    min-height: 360px;
  }
}
</style>
