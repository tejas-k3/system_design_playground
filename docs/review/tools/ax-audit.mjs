// usage: node ax-audit.mjs [url]   Reads the browser's accessibility tree (what a screen reader is given) and walks the page with Tab.
// Not a substitute for VoiceOver/NVDA: it checks roles, names, landmarks, live regions, dialogs and the keyboard order.
import { spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { chromeArgs, chromePath, waitForPage } from './chrome.mjs'
const url = process.argv[2] ?? 'http://localhost:8091/'
const port = 9800 + Math.floor(Math.random() * 100)
const chrome = spawn(chromePath(), chromeArgs(port, mkdtempSync(join(tmpdir(), 'ax-'))), { stdio: 'ignore' })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const page = await waitForPage(port)
const ws = new WebSocket(page.webSocketDebuggerUrl)
await new Promise((r) => (ws.onopen = r))
let id = 0; const pending = new Map()
ws.onmessage = (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id) } }
const send = (method, params = {}) => new Promise((res) => { const i = ++id; pending.set(i, res); ws.send(JSON.stringify({ id: i, method, params })) })
const ev = async (expression) => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.result?.value
const key = (k, code, vk) => send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, windowsVirtualKeyCode: vk }).then(() => send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code, windowsVirtualKeyCode: vk }))
await send('Page.enable'); await send('Accessibility.enable'); await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false })

const failures = []
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
  for (const x of interactive.filter((y) => !y.name)) failures.push(`${label}: an interactive ${x.role} has no accessible name`)
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
const live = run.filter((x) => ['status', 'alert', 'log'].includes(x.role))
console.log('announced while running? status/alert/log roles present:', live.map((x) => `${x.role}:"${x.name}"`).join(' | ') || 'none')
if (live.length === 0) failures.push('while running: no live region (status/alert/log) is exposed')
await ev(`[...document.querySelectorAll('button')].find(b=>/Help|How it works/.test(b.getAttribute('aria-label')||b.title||''))?.click()`); await sleep(500)
await tree('help dialog open')
const focusIn = await ev(`!!document.activeElement.closest('[role=dialog]')`)
console.log('focus is inside the dialog after opening:', focusIn)
if (!focusIn) failures.push('help dialog: focus did not move inside the dialog')
// Interview mode: every stage, reached with the keyboard (arrow keys on the stage tabs), then present mode
await send('Page.navigate', { url }); await sleep(2500)
await ev(`document.querySelector('button[aria-label="Interview mode"]').click()`); await sleep(800)
await ev(`document.querySelector('.stages [role=tab][aria-selected=true]').focus()`)
for (let i = 0; i < 7; i++) {
  const label = await ev(`document.querySelector('.stages [role=tab][aria-selected=true]').innerText`)
  const focused = await ev(`document.activeElement===document.querySelector('.stages [role=tab][aria-selected=true]')`)
  if (!focused) failures.push(`interview stage "${label}": focus is not on the selected stage tab`)
  await ev(`[...document.querySelectorAll('.iv-tool button')].filter(b=>/^Add an? /.test(b.textContent.trim())).forEach(b=>b.click())`); await sleep(200) // give every pad a row, so its inputs are audited too
  await tree(`interview stage ${i + 1}: ${label}`)
  if (i < 6) { await key('ArrowRight', 'ArrowRight', 39); await sleep(350) }
}
await ev(`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='Present').click()`); await sleep(500)
await tree('present mode')
const presentLabel = await ev(`document.querySelector('[aria-label=Presentation]')?.getAttribute('aria-label')||''`)
if (!presentLabel) failures.push('present mode: no labelled Presentation region')
ws.close(); chrome.kill()
if (failures.length) {
  console.log('\nFAIL:\n  ' + failures.join('\n  '))
  process.exit(1)
}
console.log('\nPASS: no unnamed controls (also on all 7 interview stages and in present mode), a live region while running, focus moves into dialogs')
