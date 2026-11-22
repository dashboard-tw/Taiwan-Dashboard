import type { ComponentInfo, Dashboard } from '../api/types'
import { chartKind } from './chartKind'

export type Kind = 'bar' | 'donut' | 'line' | 'map'

export interface SlotHint {
  kind: Kind
  wide: boolean
  layers: boolean
  pills: boolean
}

export function slotOf(c: ComponentInfo): SlotHint {
  return {
    kind: chartKind(c.chart_types[0]),
    wide: c.size === 'wide',
    layers: (c.map_layers?.length ?? 0) > 0,
    pills: c.chart_types.length > 1,
  }
}

export function slotsOf(d: Dashboard | null): SlotHint[] {
  return d ? d.components.map(slotOf) : UNKNOWN_SLOTS
}

export const UNKNOWN_SLOTS: SlotHint[] = Array.from({ length: 3 }, () => ({
  kind: 'bar',
  wide: false,
  layers: false,
  pills: false,
}))
