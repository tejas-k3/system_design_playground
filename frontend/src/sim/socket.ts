import type { ClientMsg, ServerMsg } from '../api/types'

type Listener = (m: ServerMsg) => void

/** Close code the server uses when it hangs up on an idle session. */
const GOING_AWAY = 1001

/**
 * WebSocket to the simulation server with automatic reconnect (exponential backoff).
 * When the server closes an idle session the socket goes dormant instead of reconnecting, so
 * an abandoned tab does not keep taking a server slot; it reconnects when the user acts.
 */
export class SimSocket {
  private ws: WebSocket | null = null
  private retry = 0
  private timer: ReturnType<typeof setTimeout> | undefined
  private closed = false
  private dormant = false
  private waiting: (() => void)[] = []

  constructor(
    private onMessage: Listener,
    private onState: (s: 'connecting' | 'open' | 'closed') => void,
    private url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/sim`,
  ) {}

  connect(): void {
    this.closed = false
    this.dormant = false
    this.onState('connecting')
    const ws = new WebSocket(this.url)
    this.ws = ws
    ws.onopen = () => {
      this.retry = 0
      this.onState('open')
      this.waiting.splice(0).forEach((fn) => fn())
    }
    ws.onmessage = (ev) => {
      try {
        this.onMessage(JSON.parse(String(ev.data)) as ServerMsg)
      } catch {
        /* ignore malformed frame */
      }
    }
    ws.onclose = (ev) => {
      this.ws = null
      if (ev.code === GOING_AWAY) this.dormant = true
      this.onState('closed')
      if (this.closed || this.dormant) return
      const delay = Math.min(8000, 500 * 2 ** this.retry++)
      this.timer = setTimeout(() => this.connect(), delay)
    }
    ws.onerror = () => ws.close()
  }

  get open(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  /** True after the server hung up on an idle session; call whenOpen to reconnect. */
  get asleep(): boolean {
    return this.dormant
  }

  /** Run fn once the socket is open, reconnecting first if it went dormant. */
  whenOpen(fn: () => void): void {
    if (this.open) return fn()
    this.waiting.push(fn)
    if (this.dormant) this.connect()
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
