import type { ClientMsg, ServerMsg } from '../api/types'

type Listener = (m: ServerMsg) => void

/** WebSocket to the simulation server with automatic reconnect (exponential backoff). */
export class SimSocket {
  private ws: WebSocket | null = null
  private retry = 0
  private timer: ReturnType<typeof setTimeout> | undefined
  private closed = false

  constructor(
    private onMessage: Listener,
    private onState: (s: 'connecting' | 'open' | 'closed') => void,
    private url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/sim`,
  ) {}

  connect(): void {
    this.closed = false
    this.onState('connecting')
    const ws = new WebSocket(this.url)
    this.ws = ws
    ws.onopen = () => {
      this.retry = 0
      this.onState('open')
    }
    ws.onmessage = (ev) => {
      try {
        this.onMessage(JSON.parse(String(ev.data)) as ServerMsg)
      } catch {
        /* ignore malformed frame */
      }
    }
    ws.onclose = () => {
      this.ws = null
      this.onState('closed')
      if (this.closed) return
      const delay = Math.min(8000, 500 * 2 ** this.retry++)
      this.timer = setTimeout(() => this.connect(), delay)
    }
    ws.onerror = () => ws.close()
  }

  get open(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  send(m: ClientMsg): boolean {
    if (!this.open) return false
    this.ws!.send(JSON.stringify(m))
    return true
  }

  close(): void {
    this.closed = true
    if (this.timer) clearTimeout(this.timer)
    this.ws?.close()
  }
}
