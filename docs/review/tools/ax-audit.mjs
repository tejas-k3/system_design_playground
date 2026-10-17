// usage: node ax-audit.mjs [url]   Reads the browser's accessibility tree (what a screen reader is given) and walks the page with Tab.
// Not a substitute for VoiceOver/NVDA: it checks roles, names, landmarks, live regions, dialogs and the keyboard order.
import { spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
const url = process.argv[2] ?? 'http://localhost:8091/'
const port = 9800 + Math.floor(Math.random() * 100)
const chrome = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', ['--headless=new', '--disable-gpu', `--remote-debugging-port=${port}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), 'ax-'))}`, '--no-first-run', 'about:blank'], { stdio: 'ignore' })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
let targets
for (let i = 0; i < 50; i++) { try { targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json(); if (targets.some((t) => t.type === 'page')) break } catch {} await sleep(150) }
const ws = new WebSocket(targets.find((t) => t.type === 'page').webSocketDebuggerUrl)
await new Promise((r) => (ws.onopen = r))
let id = 0; const pending = new Map()
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id) } }
const send = (method, params = {}) => new Promise((res) => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params })) })
const ev = async (expression) => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.result?.value
const key = (k, code, vk) => send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, windowsVirtualKeyCode: vk }).then(() => send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code, windowsVirtualKeyCode: vk }))
await send('Page.enable'); await send('Accessibility.enable'); await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false })

async function tree(label) {
  const { result } = await send('Accessibility.getFullAXTree')
  const nodes = result.nodes.filter((n) => !n.ignored)
  const val = (x) => x?.value ?? ''
  const rows = nodes.map((n) => ({ role: val(n.role), name: String(val(n.name)).trim(), props: Object.fromEntries((n.properties ?? []).map((p) => [p.name, p.value?.value])) }))
  const count = (r) => rows.filter((x) => x.role === r).length
  const interactive = rows.filter((x) => ['button', 'link', 'combobox', 'textbox', 'checkbox', 'slider', 'spinbutton', 'tab', 'menuitem', 'listbox', 'option', 'switch'].includes(x.role))
  console.log(`\n== ${label}: ${rows.length} exposed nodes`)
  console.log('landmarks/headings:', rows.filter((x) => ['banner', 'main', 'navigation', 'complementary', 'region', 'heading', 'dialog'].includes(x.role)).map((x) => `${x.role}${x.props.level ? x.props.level : ''}:"${x.name}"`).join('  '))
  console.log('unnamed interactive controls:', interactive.filter((x) => !x.name).map((x) => x.role).join(', ') || 'none')
  console.log('live regions (status/alert/log):', count('status') + count('alert') + count('log'), ' canvas/img nodes:', rows.filter((x) => ['image', 'img', 'Canvas', 'canvas'].includes(x.role)).map((x) => `${x.role}:"${x.name}"`).join(', ') || 'none')
  return rows
}
async function tabWalk(n) {
  const seen = []
  for (let i = 0; i < n; i++) {
    await key('Tab', 'Tab', 9)
    await sleep(40)
    const d = await ev(`(()=>{const e=document.activeElement;if(!e||e===document.body)return null;const cs=getComputedStyle(e);const r=e.getBoundingClientRect();return {tag:e.tagName.toLowerCase(),name:(e.getAttribute('aria-label')||e.innerText||e.title||e.value||'').trim().replace(/\\s+/g,' ').slice(0,40),role:e.getAttribute('role')||'',outline:cs.outlineStyle!=='none'&&parseFloat(cs.outlineWidth)>0,visible:r.width>0&&r.height>0}})()`)
    seen.push(d)
  }
  return seen
}
await send('Page.navigate', { url }); await sleep(2500)
await tree('welcome screen')
console.log('Tab order (first 14 stops):'); for (const d of await tabWalk(14)) console.log('  ', d ? `${d.tag}${d.role ? '[' + d.role + ']' : ''} "${d.name}" focusRing=${d.outline}` : '(body)')
await ev(`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='Load an example').click()`); await sleep(1500)
await tree('example loaded')
await ev(`document.querySelector('button.play').click()`); await sleep(11500)
console.log('live region text after 11 s of running:', JSON.stringify(await ev(`document.querySelector('[role=status][aria-live=polite].sr-only')?.textContent`)))
const run = await tree('example running')
console.log('announced while running? status/alert/log roles present:', run.filter((x) => ['status', 'alert', 'log'].includes(x.role)).map((x) => `${x.role}:"${x.name}"`).join(' | ') || 'none')
await ev(`[...document.querySelectorAll('button')].find(b=>/Help|How it works/.test(b.getAttribute('aria-label')||b.title||''))?.click()`); await sleep(500)
await tree('help dialog open')
const focusIn = await ev(`!!document.activeElement.closest('[role=dialog]')`)
console.log('focus is inside the dialog after opening:', focusIn)
ws.close(); chrome.kill()
