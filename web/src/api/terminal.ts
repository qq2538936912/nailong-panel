import request from './request'

export interface TerminalInfo {
  available: boolean
  work_dir: string
  shell: string
  python: string
  message: string
}

export interface TerminalTicket {
  ticket: string
  expires_at: string
  ws_path: string
}

export const terminalApi = {
  info() {
    return request.get('/terminal/info') as Promise<{ data: TerminalInfo }>
  },

  ticket() {
    return request.post('/terminal/ticket') as Promise<TerminalTicket>
  },
}

export function buildTerminalWsUrl(ticket: string, cols: number, rows: number) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const params = new URLSearchParams({
    ticket,
    cols: String(cols || 80),
    rows: String(rows || 24),
  })
  return `${protocol}//${window.location.host}/api/v1/terminal/ws?${params}`
}
