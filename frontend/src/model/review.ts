import type { Diagram, Node } from '../api/types'
import { paramVisible, type Reg } from './diagram'

export interface SettingRow {
  label: string
  value: string
  changed: boolean // differs from the component's default
}

/** Every setting the inspector would show for a component, as readable text, so a reviewer can see what is (and is not) configured. */
export function settingRows(d: Diagram, n: Node, reg: Reg): SettingRow[] {
  const resolved = reg.resolve(n)
  const defaults = reg.defaults(n.kind)
  const out: SettingRow[] = []
  for (const spec of reg.params(n.kind)) {
    if (!paramVisible(d, n, spec, resolved)) continue
    const v = resolved[spec.key]
    const value = spec.type === 'boolean' ? (v ? 'on' : 'off') : spec.type === 'select' ? (reg.optionLabel(n.kind, spec.key, v) ?? String(v)) : `${v}${spec.unit ? ' ' + spec.unit : ''}`
    out.push({ label: spec.label, value, changed: v !== defaults[spec.key] })
  }
  return out
}

export const percent = (ratio: number): string => `${Math.round(ratio * 100)}%`
