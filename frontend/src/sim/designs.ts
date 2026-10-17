import { fetchDesign, saveDesign } from '../api/client'
import { sanitizeDiagram } from '../model/diagram'
import { designUrl } from '../model/links'
import { useStore } from '../state/store'
import { reset } from './bridge'

const CLIPBOARD_TIMEOUT_MS = 1500

/** Copy text to the clipboard; false if that was refused, unavailable or did not answer in time. */
async function copyText(text: string): Promise<boolean> {
  try {
    const timeout = new Promise<never>((_, reject) => setTimeout(() => reject(new Error('clipboard timed out')), CLIPBOARD_TIMEOUT_MS))
    await Promise.race([navigator.clipboard.writeText(text), timeout]) // writeText can wait forever behind a permission prompt
    return true
  } catch {
    return false
  }
}

/** Save the current design on the server and put a link to it on the clipboard. */
export async function shareCurrentDesign(): Promise<void> {
  const st = useStore.getState()
  if (st.diagram.nodes.length === 0) {
    st.toast('Nothing to share yet: add a few components first', 'warn')
    return
  }
  try {
    const url = designUrl(location.origin, await saveDesign(st.diagram))
    if (await copyText(url)) st.toast(`Link copied: ${url}`, 'info', 6000)
    else st.toast(`Share this link: ${url}`, 'info', 15000) // clipboard blocked or stuck: show it instead
  } catch (e) {
    st.toast(`Could not share: ${(e as Error).message}`, 'error', 5000)
  }
}

/** Open a shared design in the sandbox. The previous design stays one undo away. */
export async function openDesign(id: string): Promise<void> {
  const st = useStore.getState()
  try {
    const raw = await fetchDesign(id)
    const clean = st.reg ? sanitizeDiagram(st.reg, raw) : null
    if (!clean || clean.diagram.nodes.length === 0) throw new Error('that design is empty or damaged')
    if (st.scenario) {
      reset()
      st.exitScenario()
    }
    useStore.getState().importDiagram(clean.diagram)
    useStore.getState().requestFit()
    st.toast('Opened a shared design. Undo (Ctrl/⌘ Z) brings your previous one back.', 'info', 5000)
  } catch (e) {
    st.toast(`Could not open that design: ${(e as Error).message}`, 'error', 5000)
  }
}
