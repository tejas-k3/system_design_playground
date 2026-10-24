import { fetchAnswerHint, fetchDebrief } from '../api/client'
import { useStore } from './store'

/** Fetch the debrief (and the optional deeper paragraph) for a challenge into the store. A failure leaves it unset. */
export async function loadDebrief(id: string, fetcher: typeof fetchDebrief = fetchDebrief): Promise<void> {
  try {
    const d = await fetcher(id)
    useStore.getState().setDebrief(d.debrief, d.deeper)
  } catch {
    /* no debrief is shown */
  }
}

/** Fetch the full answer into the store and record that it was used. When the server has none (404 or an empty reply) say so and record nothing. */
export async function loadAnswer(id: string, fetcher: typeof fetchAnswerHint = fetchAnswerHint): Promise<boolean> {
  const st = useStore.getState()
  try {
    const d = await fetcher(id)
    if (typeof d.hint !== 'string' || !d.hint.trim()) throw new Error('empty')
    st.showAnswer(d.hint)
    return true
  } catch {
    st.toast('No separate answer is available for this challenge.', 'info')
    return false
  }
}
