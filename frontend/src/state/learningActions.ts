import { emptyInterview } from '../model/interview'
import { newSession, pause, resume, switchStage } from '../model/timer'
import { writeLevel } from '../model/level'
import { noteAnswer, noteHint, noteRun, noteWrong } from '../model/record'
import type { State } from './store'

type Set = (fn: Partial<State> | ((s: State) => Partial<State>)) => void

/** The state a learner builds up: the interview session, stars, per-challenge records, hints, answers, predictions,
 *  the debrief and the level. Spread into the store; entering and leaving a scenario stays in store.ts because it
 *  also resets the diagram, the run and the view. */
export const learningActions = (set: Set) => ({
  recordStars: (id: string, stars: number) => set((s) => ((s.progress[id] ?? 0) >= stars ? {} : { progress: { ...s.progress, [id]: stars } })),
  setInterview: (interview: State['interview']) => set({ interview }),
  startInterview: (minutes: number) => set((s) => ({ interview: { ...(s.interview ?? emptyInterview()), session: newSession(Math.min(360, Math.max(1, minutes)) * 60_000, Date.now()) }, ui: { ...s.ui, welcome: false } })),
  interviewStage: (stage: Parameters<State['interviewStage']>[0]) => set((s) => (s.interview?.session ? { interview: { ...s.interview, session: switchStage(s.interview.session, stage, Date.now()) } } : {})),
  interviewPause: (paused: boolean) => set((s) => (s.interview?.session ? { interview: { ...s.interview, session: (paused ? pause : resume)(s.interview.session, Date.now()) } } : {})),
  endInterview: () => set((s) => (s.interview ? { interview: { ...s.interview, session: null } } : {})),

  recordPrediction: (p: Parameters<State['recordPrediction']>[0], answer: Parameters<State['recordPrediction']>[1]) =>
    set((s) => {
      if (!s.scenario || p.id in s.scenario.predictions) return {}
      const id = s.scenario.def.id
      return {
        scenario: { ...s.scenario, predictions: { ...s.scenario.predictions, [p.id]: answer } },
        records: answer.correct ? s.records : { ...s.records, [id]: noteWrong(s.records[id]) },
      }
    }),
  addHint: (text: string) =>
    set((s) => {
      if (!s.scenario || s.scenario.hints.length >= s.scenario.def.hintCount) return {}
      const id = s.scenario.def.id
      const level = s.scenario.hints.length + 1
      return { scenario: { ...s.scenario, hints: [...s.scenario.hints, text], hintLevel: level }, records: { ...s.records, [id]: noteHint(s.records[id], level) } }
    }),
  showAnswer: (text: string) =>
    set((s) => (s.scenario ? { scenario: { ...s.scenario, answer: text }, records: { ...s.records, [s.scenario.def.id]: noteAnswer(s.records[s.scenario.def.id]) } } : {})),
  setLevel: (level: State['level']) => {
    writeLevel(level)
    set({ level })
  },
  importProgress: (progress: State['progress'], records: State['records']) => set({ progress, records }),
  setDebrief: (text: string, deeper?: string, weakPoints?: string[]) => set((s) => (s.scenario ? { scenario: { ...s.scenario, debrief: text, deeper: typeof deeper === 'string' && deeper.trim() ? deeper : null, weakPoints: weakPoints ?? [] } } : {})),
  setResult: (score: Parameters<State['setResult']>[0]) =>
    set((s) => {
      if (!s.scenario) return {}
      const id = s.scenario.def.id
      const better = score.stars > (s.progress[id] ?? 0)
      return { scenario: { ...s.scenario, result: score, resultFindings: s.findings }, ui: { ...s.ui, result: true }, progress: better ? { ...s.progress, [id]: score.stars } : s.progress, records: { ...s.records, [id]: noteRun(s.records[id], score.pass) } }
    }),
})
