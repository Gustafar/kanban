import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { Limits, UsageReport } from '../types'
import { Modal } from './Dialogs'
import { ageText, LimitMeter } from './LimitMeters'

const money = (n: number) => n.toLocaleString('pt-BR', { style: 'currency', currency: 'USD' })
const tokens = (n: number) => (n >= 1000 ? `${(n / 1000).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} mil` : String(n))

const STALE_AFTER = 5 * 60 // seconds: older than this and opening the panel refreshes the limits

interface Props {
  limits: Limits | null
  onLimits: (l: Limits) => void
  onClose: () => void
  onError: (m: string) => void
}

export function UsageDialog({ limits, onLimits, onClose, onError }: Props) {
  const [report, setReport] = useState<UsageReport | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [problem, setProblem] = useState('')
  const [, tick] = useState(0)

  useEffect(() => { api.usage().then(setReport).catch(e => onError(e.message)) }, [onError])

  const refresh = useCallback(async () => {
    setRefreshing(true)
    setProblem('')
    try { onLimits(await api.refreshLimits()) } catch (e) { setProblem((e as Error).message) } finally { setRefreshing(false) }
  }, [onLimits])

  // opening the panel brings stale numbers up to date (one tiny call), fresh ones are left alone
  useEffect(() => {
    if (!limits || Date.now() / 1000 - limits.updatedAt > STALE_AFTER) void refresh()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // keep "atualizado há X" honest while the panel stays open
  useEffect(() => {
    const t = setInterval(() => tick(n => n + 1), 30_000)
    return () => clearInterval(t)
  }, [])

  const total = report?.total
  const maxDay = Math.max(1, ...(report?.byDay.map(d => d.costUsd) ?? [0]))
  const warn = limits?.status === 'allowed_warning' ? 'Você está perto do limite.' : limits?.status === 'rejected' ? 'Limite atingido: novas execuções vão falhar até reiniciar.' : ''

  return (
    <Modal title="Uso do Claude" onClose={onClose}>
      <div className="usage">
        <section aria-label="Limites do plano">
          <div className="usage-head">
            <h3 className="usage-h">Limites do plano</h3>
            <span className="usage-age">{refreshing ? 'Atualizando…' : `Atualizado ${ageText(limits?.updatedAt ?? 0)}`}</span>
            <button className="btn ghost usage-refresh" onClick={() => void refresh()} disabled={refreshing}>Atualizar</button>
          </div>
          <div className="meters">
            <LimitMeter kind="session" window={limits?.fiveHour ?? null} />
            <LimitMeter kind="week" window={limits?.sevenDay ?? null} />
          </div>
          {warn && <p className="usage-warn" role="status">{warn}</p>}
          {problem && <p className="usage-warn" role="alert">{problem}</p>}
          <p className="usage-note">Conta todo o uso da sua conta Claude, não só o deste app. Os números vêm do próprio Claude Code a cada execução; "Atualizar" faz uma chamada mínima só para ler.</p>
        </section>

        <section aria-label="Consumo do Kanban">
          <h3 className="usage-h">Consumo registrado pelo Kanban</h3>
          {!report ? <p className="muted">Carregando…</p> : (
            <>
              <div className="usage-stats">
                <div className="usage-stat"><strong>{money(total!.costUsd)}</strong><span>Custo equivalente em API</span></div>
                <div className="usage-stat"><strong>{total!.runs}</strong><span>Execuções com dados</span></div>
                <div className="usage-stat"><strong>{tokens(total!.inputTokens + total!.outputTokens)}</strong><span>Tokens (entrada + saída)</span></div>
                <div className="usage-stat"><strong>{tokens(total!.cacheReadTokens)}</strong><span>Tokens lidos do cache</span></div>
              </div>

              {report.byProject.length === 0 ? (
                <p className="muted pad">Nenhuma execução reportou custo ainda. Rode um card para começar a ver dados aqui.</p>
              ) : (
                <>
                  <h3 className="usage-h usage-h2">Por projeto</h3>
                  <ul className="usage-list">
                    {report.byProject.map(p => (
                      <li key={p.projectId}>
                        <span className="dot" style={{ background: p.color }} />
                        <span className="usage-name">{p.name}</span>
                        <span className="usage-bar"><i style={{ width: `${(p.costUsd / total!.costUsd) * 100 || 0}%` }} /></span>
                        <span className="usage-cost">{money(p.costUsd)}</span>
                      </li>
                    ))}
                  </ul>

                  {report.byDay.length > 0 && (
                    <>
                      <h3 className="usage-h usage-h2">Últimos dias</h3>
                      <ul className="usage-days" aria-label="Custo por dia">
                        {[...report.byDay].reverse().map(d => (
                          <li key={d.date} title={`${d.date}: ${money(d.costUsd)}`}>
                            <i style={{ height: `${Math.max(4, (d.costUsd / maxDay) * 100)}%` }} />
                          </li>
                        ))}
                      </ul>
                    </>
                  )}
                </>
              )}
            </>
          )}
        </section>
      </div>
    </Modal>
  )
}
