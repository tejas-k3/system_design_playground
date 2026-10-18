// Interview timer accuracy: start a session, then over ~5 minutes of wall time compare the on-screen countdown with the
// real clock, including a period where the page's JavaScript is PAUSED (a debugger pause stands in for a frozen background tab) and then resumed.
// usage: URL=http://localhost:8091 node timer-accuracy.mjs [seconds=300]   exit 0 = every reading within +/- 1 s
import { spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { chromeArgs, chromePath, waitForPage } from './chrome.mjs'

const url = process.env.URL ?? 'http://localhost:8091'
const total = Number(process.argv[2] ?? 300)
const port = 9500 + Math.floor(Math.random() * 300)
const chrome = spawn(chromePath(), chromeArgs(port, mkdtempSync(join(tmpdir(), 'tm-'))), { stdio: 'ignore' })
const page = await waitForPage(port)
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise((r) => (ws.onopen = r))
let id = 0
const pending = new Map()
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id) } }
const send = (method, params = {}) => new Promise((res) => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params })) })
const ev = async (expression) => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.result?.value
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

await send('Page.enable')
await send('Page.navigate', { url })
await sleep(3000)
await ev(`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='Practise an interview').click()`)
await sleep(1000)
await sleep(600) // let the session reach localStorage
const t0 = Date.now()
// One in-page read: the countdown on screen and the true time since the session started (from its saved start
// timestamp), taken in the same call so no network or scripting latency enters the comparison.
const reading = () => ev(`(() => {
  const t = document.querySelector('[role=timer]').innerText.split(':').map(Number)
  const left = t.length === 3 ? t[0] * 3600 + t[1] * 60 + t[2] : t[0] * 60 + t[1]
  const s = JSON.parse(localStorage.getItem('sdp:v1')).interview.session
  return { shown: s.totalMs / 1000 - left, truth: (Date.now() - s.startedAt) / 1000 }
})()`)
const rows = []
const check = async (label) => {
  const { shown, truth } = await reading()
  rows.push({ label, wall: ((Date.now() - t0) / 1000).toFixed(1), shown: shown.toFixed(0), diff: (shown - truth).toFixed(2) })
}
const freezeFrom = total * 0.3
const freezeFor = total * 0.3
let frozen = false
while ((Date.now() - t0) / 1000 < total) {
  const el = (Date.now() - t0) / 1000
  if (!frozen && el >= freezeFrom) {
    await check('before freeze')
    await send('Debugger.enable')
    await send('Debugger.pause') // no script runs: no ticks, no renders, as in a frozen tab; Date.now keeps going
    frozen = true
    await sleep(freezeFor * 1000)
    await send('Debugger.resume')
    await sleep(700)
    await check('just after thaw')
  }
  await sleep(10_000)
  await check(frozen ? 'after thaw' : 'running')
}
await check('end')
ws.close()
chrome.kill()
let bad = 0
for (const r of rows) {
  const ok = Math.abs(Number(r.diff)) <= 1.0
  if (!ok) bad++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${r.label.padEnd(16)} wall ${r.wall.padStart(6)} s   clock ${String(r.shown).padStart(4)} s   diff ${r.diff} s`)
}
console.log(bad === 0 ? `PASS: ${rows.length} readings over ${total} s, including a ${freezeFor} s period with the page's script stopped, all within 1 s` : `FAIL: ${bad} readings off by more than 1 s`)
process.exit(bad === 0 ? 0 : 1)
