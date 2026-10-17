import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SimSocket } from './socket'

class FakeWebSocket {
  static OPEN = 1
  static instances: FakeWebSocket[] = []
  readyState = 0
  sent: string[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: ((e: { code: number }) => void) | null = null
  onerror: (() => void) | null = null
  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }
  send(d: string) {
    this.sent.push(d)
  }
  close() {
    this.finish(1000)
  }
  open() {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.()
  }
  finish(code: number) {
    this.readyState = 3
    this.onclose?.({ code })
  }
}

let states: string[]
const make = () => new SimSocket(() => undefined, (s) => states.push(s), 'ws://test/api/sim')
const last = () => FakeWebSocket.instances[FakeWebSocket.instances.length - 1]

beforeEach(() => {
  vi.useFakeTimers()
  FakeWebSocket.instances = []
  states = []
  vi.stubGlobal('WebSocket', FakeWebSocket)
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('SimSocket', () => {
  it('refuses to send while closed and sends once open', () => {
    const s = make()
    s.connect()
    expect(s.send({ type: 'play' })).toBe(false)
    last().open()
    expect(s.send({ type: 'play' })).toBe(true)
    expect(last().sent).toEqual(['{"type":"play"}'])
  })

  it('reconnects with growing delays after an abnormal close', () => {
    const s = make()
    s.connect()
    last().finish(1006)
    expect(FakeWebSocket.instances).toHaveLength(1)
    vi.advanceTimersByTime(500)
    expect(FakeWebSocket.instances).toHaveLength(2)
    last().finish(1006)
    vi.advanceTimersByTime(999)
    expect(FakeWebSocket.instances).toHaveLength(2)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(3)
  })

  it('goes dormant when the server hangs up on an idle session and wakes on demand', () => {
    const s = make()
    s.connect()
    last().open()
    last().finish(1001)
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1) // no reconnect storm
    expect(s.asleep).toBe(true)

    const ran = vi.fn()
    s.whenOpen(ran)
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(ran).not.toHaveBeenCalled()
    last().open()
    expect(ran).toHaveBeenCalledTimes(1)
    expect(s.asleep).toBe(false)
  })

  it('runs whenOpen immediately when already connected', () => {
    const s = make()
    s.connect()
    last().open()
    const ran = vi.fn()
    s.whenOpen(ran)
    expect(ran).toHaveBeenCalledTimes(1)
  })

  it('stops reconnecting after close()', () => {
    const s = make()
    s.connect()
    s.close()
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1)
  })
})
