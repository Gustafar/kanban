import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import type { BoardListener } from '../useBoard'
import type { Card, Comment, Mode, Project, Run, RunEvent } from '../types'
import { STATUS_LABEL } from './CardView'
import { Branch, Close } from './Icons'
import { ModeToggle } from './Dialogs'

interface Props {
  card: Card
  project: Project
  subscribe: (fn: BoardListener) => () => void
  onClose: () => void
  onError: (m: string) => void
}

function DiffView({ diff }: { diff: string }) {
  if (!diff.trim()) return <p className="muted pad">Nenhuma mudança na branch ainda.</p>
  return (
    <pre className="diff">
      {diff.split('\n').map((l, i) => {
        const kind = l.startsWith('+++') || l.startsWith('---') || l.startsWith('diff ') || l.startsWith('index ') ? 'meta'
          : l.startsWith('@@') ? 'hunk' : l.startsWith('+') ? 'add' : l.startsWith('-') ? 'del' : ''
        return <span key={i} className={kind}>{l + '\n'}</span>
      })}
    </pre>
  )
}

export function CardDrawer({ card, project, subscribe, onClose, onError }: Props) {
  const [title, setTitle] = useState(card.title)
  const [description, setDescription] = useState(card.description)
  const [mode, setMode] = useState<Mode>(card.mode)
  const [runs, setRuns] = useState<Run[]>([])
  const [events, setEvents] = useState<RunEvent[]>([])
  const [tab, setTab] = useState<'log' | 'diff' | 'comments'>('log')
  const [comments, setComments] = useState<Comment[]>([])
  const [commentText, setCommentText] = useState('')
  const [diff, setDiff] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const logRef = useRef<HTMLDivElement>(null)

  const run = runs[0] // newest first
  const runId = run?.id
  const dirty = title !== card.title || description !== card.description || mode !== card.mode

  const guard = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true)
    try { await fn() } catch (e) { onError((e as Error).message) } finally { setBusy(false) }
  }, [onError])

  useEffect(() => { api.runs(card.id).then(setRuns).catch(e => onError(e.message)) }, [card.id, onError])
  useEffect(() => { api.comments(card.id).then(setComments).catch(() => {}) }, [card.id])
  useEffect(() => {
    setEvents([])
    if (runId != null) api.events(runId).then(setEvents).catch(() => {})
  }, [runId])

  useEffect(() => subscribe((type, data) => {
    if (type === 'run' && data.cardId === card.id) {
      setRuns(l => (l.some(r => r.id === data.id) ? l.map(r => (r.id === data.id ? data : r)) : [data, ...l]))
    }
    if (type === 'comment' && data.cardId === card.id) setComments(l => (l.some(c => c.id === data.id) ? l : [...l, data]))
    if (type === 'run_event' && data.runId === runId) setEvents(l => (l.some(e => e.id === data.id) ? l : [...l, data]))
  }), [subscribe, card.id, runId])

  useEffect(() => {
    if (tab === 'diff' && runId != null) api.diff(runId).then(d => setDiff(d.diff)).catch(e => onError(e.message))
  }, [tab, runId, run?.status, onError])

  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }) }, [events.length, comments.length, tab])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const status = run?.status
  const finished = status === 'succeeded'
  const direct = project.isolation === 'direct'

  return (
    <>
      <div className="scrim" onClick={onClose} />
      <aside className="drawer" aria-label={`Card: ${card.title}`}>
        <header className="drawer-head">
          <span className="dot" style={{ background: project.color }} />
          <span className="drawer-project">{project.name}</span>
          <button className="icon-btn" onClick={onClose} aria-label="Fechar"><Close /></button>
        </header>

        <div className="drawer-scroll">
          <input className="title-input" value={title} onChange={e => setTitle(e.target.value)} aria-label="Título" />
          <textarea className="desc-input" rows={4} value={description} onChange={e => setDescription(e.target.value)} placeholder="Descreva o que a IA deve fazer." aria-label="Descrição" />
          <div className="field">Execução<ModeToggle value={mode} onChange={setMode} /></div>
          {dirty && (
            <div className="actions">
              <button className="btn ghost" onClick={() => { setTitle(card.title); setDescription(card.description); setMode(card.mode) }}>Desfazer</button>
              <button className="btn primary" disabled={busy || !title.trim()} onClick={() => guard(() => api.updateCard(card.id, { title, description, mode }))}>Salvar</button>
            </div>
          )}

          <section className="run-panel" data-status={status}>
            <div className="run-head">
              <strong>{status ? STATUS_LABEL[status] : 'Ainda não executou'}</strong>
              {direct && run && <span className="branch"><Branch />direto na pasta</span>}
              {!direct && run?.branch && <span className="branch"><Branch />{run.branch}</span>}
            </div>
            {run?.error && <p className="run-error">{run.error}</p>}

            <div className="actions wrap">
              {status === 'awaiting_approval' && <>
                <button className="btn primary" disabled={busy} onClick={() => guard(() => api.approve(run!.id))}>Aprovar e executar</button>
                <button className="btn ghost" disabled={busy} onClick={() => guard(() => api.cancel(run!.id))}>Recusar</button>
              </>}
              {status === 'running' && <button className="btn danger" disabled={busy} onClick={() => guard(() => api.cancel(run!.id))}>Parar execução</button>}
              {finished && <>
                <button className="btn primary" disabled={busy} onClick={() => guard(() => api.merge(run!.id))}>{direct ? 'Aceitar mudanças' : 'Fazer merge na branch atual'}</button>
                <button className="btn ghost" disabled={busy} onClick={() => {
                  const msg = direct
                    ? 'Restaurar a pasta ao estado de antes desta execução? Qualquer alteração feita na pasta depois disso, inclusive por você, também será desfeita.'
                    : 'Descartar a branch e o worktree desta execução?'
                  if (confirm(msg)) guard(() => api.discard(run!.id))
                }}>{direct ? 'Desfazer na pasta' : 'Descartar'}</button>
              </>}
              {(!status || status === 'failed' || status === 'cancelled') && (
                <button className="btn primary" disabled={busy || dirty} onClick={() => guard(() => api.runCard(card.id))}>
                  {status ? 'Executar de novo' : 'Executar com IA'}
                </button>
              )}
            </div>

            {(finished || status === 'failed') && (
              <form className="followup" onSubmit={e => { e.preventDefault(); if (message.trim()) guard(async () => { await api.runCard(card.id, message.trim()); setMessage('') }) }}>
                <input value={message} onChange={e => setMessage(e.target.value)} placeholder="Pedir um ajuste (continua a mesma conversa)" aria-label="Pedir ajuste" />
                <button className="btn" disabled={busy || !message.trim()}>Enviar</button>
              </form>
            )}
          </section>

          <div className="tabs" role="tablist">
            <button role="tab" aria-selected={tab === 'log'} onClick={() => setTab('log')} disabled={!run}>Atividade</button>
            <button role="tab" aria-selected={tab === 'diff'} onClick={() => setTab('diff')} disabled={!run}>Mudanças</button>
            <button role="tab" aria-selected={tab === 'comments'} onClick={() => setTab('comments')}>
              Comentários{comments.length > 0 && <span className="tab-count">{comments.length}</span>}
            </button>
          </div>
          {tab === 'comments' ? (
            <>
              <div className="log comments" ref={logRef}>
                {comments.length === 0 && <p className="muted pad">Nenhum comentário ainda. Use para registrar decisões ou contexto; a IA também lê e escreve aqui.</p>}
                {comments.map(c => (
                  <article key={c.id} className="comment" data-author={c.author}>
                    <header><strong>{c.author === 'ai' ? 'IA' : 'Você'}</strong><time dateTime={c.createdAt}>{new Date(c.createdAt).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}</time></header>
                    <p>{c.body}</p>
                  </article>
                ))}
              </div>
              <form className="followup" onSubmit={e => { e.preventDefault(); if (commentText.trim()) guard(async () => { await api.addComment(card.id, commentText.trim()); setCommentText('') }) }}>
                <input value={commentText} onChange={e => setCommentText(e.target.value)} placeholder="Escrever um comentário" aria-label="Comentário" />
                <button className="btn" disabled={busy || !commentText.trim()}>Comentar</button>
              </form>
            </>
          ) : run && (
            <>
              {tab === 'log' ? (
                <div className="log" ref={logRef}>
                  {events.length === 0 && <p className="muted pad">{status === 'awaiting_approval' ? 'Aprove para a IA começar.' : 'Sem atividade registrada.'}</p>}
                  {events.map(e => <div key={e.id} className={`ev ev-${e.kind}`}><span className="ev-kind">{e.kind === 'tool' ? 'ferramenta' : e.kind === 'text' ? 'ia' : e.kind === 'result' ? 'resumo' : e.kind === 'error' ? 'erro' : 'sistema'}</span><span className="ev-data">{e.data}</span></div>)}
                </div>
              ) : <div className="log" ref={logRef}><DiffView diff={diff} /></div>}
            </>
          )}
        </div>

        <footer className="drawer-foot">
          <button className="btn ghost danger-text" onClick={() => { if (confirm('Excluir este card?')) guard(async () => { await api.deleteCard(card.id); onClose() }) }}>Excluir card</button>
        </footer>
      </aside>
    </>
  )
}
