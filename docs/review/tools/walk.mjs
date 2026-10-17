// usage: node walk.mjs <width> <height> <outDir> '<steps json>'
// steps: {type:"text"} types into the focused element, {key:"Enter"} presses a key, {goto:url} {click:"text or css"} {move:[x,y]} {drag:[x1,y1,x2,y2]} {eval:"js", log:"label"} {shot:"name"} {wait:ms}
// WITHIN one step the order is: click, move, drag, eval, shot, THEN wait. To read or capture AFTER a delay, put the
// eval/shot in the NEXT step. (Reading in the same step as the wait reads too early; this has caused false findings.)
import { spawn } from 'node:child_process'
import { writeFileSync, mkdtempSync, mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
const [w, h, outDir, stepsJson] = process.argv.slice(2)
mkdirSync(outDir, { recursive: true })
const port = 9400 + Math.floor(Math.random() * 400)
const chrome = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', ['--headless=new', '--disable-gpu', `--remote-debugging-port=${port}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), 'cdp-'))}`, '--no-first-run', 'about:blank'], { stdio: 'ignore' })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
let targets
for (let i = 0; i < 50; i++) { try { targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json(); if (targets.some((t) => t.type === 'page')) break } catch {} await sleep(150) }
const ws = new WebSocket(targets.find((t) => t.type === 'page').webSocketDebuggerUrl)
await new Promise((r) => (ws.onopen = r))
let id = 0; const pending = new Map()
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id) } }
const send = (method, params = {}) => new Promise((res) => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params })) })
const ev = async (expression) => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.result?.value
await send('Page.enable'); await send('Emulation.setDeviceMetricsOverride', { width: +w, height: +h, deviceScaleFactor: 1, mobile: +w < 700 })
await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }] })
const t0 = Date.now()
for (const st of JSON.parse(stepsJson)) {
  if (st.goto) { await send('Page.navigate', { url: st.goto }); await sleep(st.wait ?? 2000); continue }
  if (st.click) { const ok = await ev(`(()=>{const q=${JSON.stringify(st.click)};let el=document.querySelector(q.startsWith('css:')?q.slice(4):'__none__');if(!el)el=[...document.querySelectorAll('button,a,[role=tab]')].find(b=>b.textContent.trim().startsWith(q));if(!el)return false;el.click();return true})()`); console.log('click', st.click, ok) }
  if (st.type) await send('Input.insertText', { text: st.type })
  if (st.key) { const code = { Enter: 13, Tab: 9, Escape: 27, a: 65 }[st.key] ?? 0; for (const type of ['keyDown', 'keyUp']) await send('Input.dispatchKeyEvent', { type, key: st.key, code: st.key, windowsVirtualKeyCode: code, modifiers: st.mod ?? 0 }) }
  if (st.eval) { const v = await ev(st.eval); if (st.log) console.log(st.log, JSON.stringify(v)) }
  if (st.move) await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: st.move[0], y: st.move[1] })
  if (st.drag) { const [x1, y1, x2, y2] = st.drag; await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: x1, y: y1 }); await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: x1, y: y1, button: 'left', clickCount: 1 }); for (let k = 1; k <= 8; k++) { await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: x1 + ((x2 - x1) * k) / 8, y: y1 + ((y2 - y1) * k) / 8, buttons: 1 }); await sleep(20) } await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: x2, y: y2, button: 'left', clickCount: 1 }) }
  if (st.pre) await sleep(st.pre)
  if (st.shot) { const r = await send('Page.captureScreenshot', { format: 'png' }); writeFileSync(join(outDir, st.shot + '.png'), Buffer.from(r.result.data, 'base64')); console.log('shot', st.shot, `+${Date.now() - t0}ms`) }
  if (st.wait) await sleep(st.wait)
}
ws.close(); chrome.kill()
