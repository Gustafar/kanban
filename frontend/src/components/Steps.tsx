import type { RunEvent } from '../types'
import { Markdown } from './Markdown'

// The run's step-by-step: what the AI said, and which tools it used, as compact rows instead of a raw log.

function unescape(s: string) {
  try { return JSON.parse(`"${s}"`) as string } catch { return s }
}

// Tool events are stored as `Name {json…}` (the JSON may be cut off), so pull the interesting argument by regex.
function describeTool(data: string): { name: string; arg: string } {
  const sp = data.indexOf(' ')
  const name = sp < 0 ? data : data.slice(0, sp)
  const rest = sp < 0 ? '' : data.slice(sp + 1)
  for (const key of ['file_path', 'command', 'pattern', 'path', 'query', 'description']) {
    const m = new RegExp(`"${key}":"((?:[^"\\\\]|\\\\.)*)`).exec(rest)
    if (m) return { name, arg: unescape(m[1]) }
  }
  return { name, arg: rest.length > 2 ? rest : '' }
}

const shorten = (s: string, n = 90) => (s.length > n ? '…' + s.slice(-n) : s)

export function Steps({ events, awaiting }: { events: RunEvent[]; awaiting: boolean }) {
  if (events.length === 0) {
    return <p className="empty-note">{awaiting ? 'Aprove a execução para a IA começar. Os passos aparecem aqui.' : 'Nenhum passo registrado nesta execução.'}</p>
  }
  return (
    <ol className="steps">
      {events.map(e => {
        if (e.kind === 'tool') {
          const t = describeTool(e.data)
          return (
            <li key={e.id} className="step step-tool" title={e.data}>
              <b>{t.name}</b>{t.arg && <code>{shorten(t.arg.replace(/\s+/g, ' '))}</code>}
            </li>
          )
        }
        if (e.kind === 'system') return <li key={e.id} className="step step-system">{e.data}</li>
        if (e.kind === 'error') return <li key={e.id} className="step step-error"><b>Erro</b><Markdown text={e.data} /></li>
        return (
          <li key={e.id} className={`step step-${e.kind}`}>
            {e.kind === 'result' && <b className="step-label">Resumo final</b>}
            <Markdown text={e.data} />
          </li>
        )
      })}
    </ol>
  )
}
