import type { LimitWindow, Limits } from '../types'

// Plan usage windows as reported by the Claude CLI: "Sessão (5 h)" and "Semanal (7 dias)".

const WINDOWS = {
  session: { label: 'Sessão (5 h)', short: 'Sessão' },
  week: { label: 'Semanal (7 dias)', short: 'Semana' },
} as const

export type WindowKind = keyof typeof WINDOWS

const expired = (w: LimitWindow) => w.resetsAt * 1000 <= Date.now()

// A window that already reset has nothing consumed yet; the stored value is from before the reset.
export const percent = (w: LimitWindow | null) => (!w || expired(w) ? 0 : Math.min(100, Math.round(w.utilization * 100)))

const level = (p: number) => (p >= 90 ? 'high' : p >= 70 ? 'warn' : 'ok')

export function resetText(w: LimitWindow): string {
  if (expired(w)) return 'Já reiniciou. Atualize para ver o uso atual.'
  const d = new Date(w.resetsAt * 1000)
  const time = d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
  if (d.toDateString() === new Date().toDateString()) return `Reinicia hoje às ${time}`
  const date = d.toLocaleDateString('pt-BR', { weekday: 'short', day: 'numeric', month: 'short' }).replace('.,', ',')
  return `Reinicia ${date} às ${time}`
}

export function ageText(updatedAt: number): string {
  if (!updatedAt) return 'sem dados ainda'
  const s = Math.max(0, Math.round(Date.now() / 1000 - updatedAt))
  if (s < 90) return 'agora há pouco'
  if (s < 3600) return `há ${Math.round(s / 60)} min`
  if (s < 86400) return `há ${Math.round(s / 3600)} h`
  return `há ${Math.round(s / 86400)} d`
}

export function LimitMeter({ kind, window }: { kind: WindowKind; window: LimitWindow | null }) {
  const p = percent(window)
  return (
    <div className="meter" data-level={level(p)}>
      <div className="meter-top"><span>{WINDOWS[kind].label}</span><strong>{window ? `${p}%` : '—'}</strong></div>
      <div className="meter-bar" role="progressbar" aria-label={WINDOWS[kind].label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={p}><i style={{ width: `${p}%` }} /></div>
      <div className="meter-sub">{window ? resetText(window) : 'Sem dados ainda'}</div>
    </div>
  )
}

// Compact version for the sidebar; the whole block opens the usage panel.
export function MiniLimits({ limits, onOpen }: { limits: Limits | null; onOpen: () => void }) {
  const rows: [WindowKind, LimitWindow | null][] = [['session', limits?.fiveHour ?? null], ['week', limits?.sevenDay ?? null]]
  const hasData = !!(limits?.fiveHour || limits?.sevenDay)
  const detail = rows.map(([k, w]) => (w ? `${WINDOWS[k].label}: ${percent(w)}%, ${resetText(w).toLowerCase()}` : '')).filter(Boolean).join(' • ')
  return (
    <button className="mini-limits" onClick={onOpen} title={hasData ? `${detail}. Atualizado ${ageText(limits!.updatedAt)}.` : 'Ver o uso do plano'}>
      {hasData ? rows.map(([k, w]) => {
        const p = percent(w)
        return (
          <span key={k} className="mini-row" data-level={level(p)}>
            <span>{WINDOWS[k].short}</span>
            <span className="meter-bar"><i style={{ width: `${p}%` }} /></span>
            <b>{w ? `${p}%` : '—'}</b>
          </span>
        )
      }) : <span className="mini-empty">Uso do plano</span>}
    </button>
  )
}
