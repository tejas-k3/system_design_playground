import { useEffect, useState } from 'react'
import { fetchScenariosOrCached } from '../api/client'
import type { ScenarioSummary } from '../api/types'
import { loadSummaries } from '../model/summaries'

export interface ScenarioList {
  list: ScenarioSummary[] | null
  /** True once the server has failed or been too slow and the list on show is the copy kept in this browser. */
  fromCache: boolean
  /** The fetch has not settled yet (the cached copy, if any, is on show for the first paint). */
  pending: boolean
  /** Set only when there is neither an answer nor a kept copy. */
  error: string
}

/** The challenge list for display: the last kept copy at first paint, replaced by the server's answer when it arrives. */
export function useScenarioList(active: boolean): ScenarioList {
  const [state, setState] = useState<ScenarioList>({ list: null, fromCache: false, pending: false, error: '' })
  useEffect(() => {
    if (!active) return
    let live = true
    setState((s) => ({ list: s.list ?? loadSummaries(), fromCache: false, pending: true, error: '' }))
    fetchScenariosOrCached()
      .then((r) => live && setState({ list: r.list, fromCache: r.cached, pending: false, error: '' }))
      .catch((e: Error) => live && setState((s) => ({ list: s.list, fromCache: false, pending: false, error: e.message })))
    return () => { live = false }
  }, [active])
  return state
}
