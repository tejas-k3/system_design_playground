import { fetchScenarios } from '../api/client'
import { dailyChallenge, loadStreak, localDate, nextStreak, saveStreak } from '../model/daily'
import { useStore } from './store'

/** When a challenge is solved and it is today's daily one, extend the streak. Nothing here touches the main save. */
export function startDailyWatch(): void {
  let seen: unknown
  useStore.subscribe((s) => {
    const result = s.scenario?.result
    if (result === seen) return
    seen = result
    if (!result?.pass || !s.scenario) return
    const id = s.scenario.def.id
    const today = localDate()
    fetchScenarios()
      .then((list) => {
        if (dailyChallenge(today, list.map((x) => x.id)) === id) saveStreak(nextStreak(loadStreak(), today))
      })
      .catch(() => undefined)
  })
}
