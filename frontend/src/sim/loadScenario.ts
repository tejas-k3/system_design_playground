import { fetchScenario } from '../api/client'
import { useStore } from '../state/store'
import { reset } from './bridge'

export async function loadScenario(id: string, opts?: { dailySeed?: number }) {
  const st = useStore.getState()
  try {
    reset()
    st.openScenario(await fetchScenario(id), opts)
    st.requestFit()
  } catch (e) {
    st.toast(`Could not load challenge: ${(e as Error).message}`, 'error')
  }
}
