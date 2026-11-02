import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import type { BoardListener } from '../useBoard'
import { isImage } from '../types'
import type { Attachment, Card, Comment, Mode, Project, Run, RunEvent } from '../types'
import { STATUS_LABEL } from './CardView'
import { Branch, Clip, Close } from './Icons'
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
  const [tagsText, setTagsText] = useState(card.tags.join(', '))
  const [files, setFiles] = useState<Attachment[]>([])
  const fileInput = useRef<HTMLInputElement>(null)
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
  // Fields the user has not touched follow the server, so an AI title arriving while the drawer is open
  // shows up instead of being kept stale (and later overwritten by "Salvar").
  const synced = useRef({ title: card.title, description: card.description, mode: card.mode, tags: card.tags.join(', ') })
  useEffect(() => {
    const prev = synced.current
    const next = { title: card.title, description: card.description, mode: card.mode, tags: card.tags.join(', ') }
    setTitle(t => (t === prev.title ? next.title : t))
    setDescription(d => (d === prev.description ? next.description : d))
    setMode(m => (m === prev.mode ? next.mode : m))
    setTagsText(t => (t === prev.tags ? next.tags : t))
    synced.current = next
  }, [card.title, card.description, card.mode, card.tags.join(', ')]) // eslint-disable-line react-hooks/exhaustive-deps

  const tagList = tagsText.split(',').map(t => t.trim()).filter(Boolean)
  const dirty = title !== card.title || description !== card.description || mode !== card.mode || tagList.join(', ') !== card.tags.join(', ')

  const save = () => api.updateCard(card.id, { title, description, mode, tags: tagList })
  const revert = () => { setTitle(card.title); setDescription(card.description); setMode(card.mode); setTagsText(card.tags.join(', ')) }

  // Ctrl+S saves; the ref keeps the listener pointing at the latest field values
  const saveShortcut = useRef<() => void>(() => {})
  saveShortcut.current = () => { if (dirty && title.trim() && !busy) void guard(save) }
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); saveShortcut.current() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  async function upload(list: File[]) {
    for (const f of list) {
      if (f.size > 15 * 1024 * 1024) { onError(`"${f.name}" passa de 15 MB.`); continue }
      try { await api.uploadAttachment(card.id, f) } catch (e) { onError((e as Error).message) }
    }
  }

  const guard = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true)
    try { await fn() } catch (e) { onError((e as Error).message) } finally { setBusy(false) }
  }, [onError])

  useEffect(() => { api.runs(card.id).then(setRuns).catch(e => onError(e.message)) }, [card.id, onError])
  useEffect(() => { api.comments(card.id).then(setComments).catch(() => {}) }, [card.id])
  useEffect(() => { api.attachments(card.id).then(setFiles).catch(() => {}) }, [card.id])
  useEffect(() => {
    setEvents([])
    if (runId != null) api.events(runId).then(setEvents).catch(() => {})
  }, [runId])

  useEffect(() => subscribe((type, data) => {
    if (type === 'run' && data.cardId === card.id) {
      setRuns(l => (l.some(r => r.id === data.id) ? l.map(r => (r.id === data.id ? data : r)) : [data, ...l]))
    }
    if (type === 'attachment' && data.cardId === card.id) setFiles(l => (l.some(a => a.id === data.id) ? l : [...l, data]))
    if (type === 'attachment_deleted' && data.cardId === card.id) setFiles(l => l.filter(a => a.id !== data.id))
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
      <aside
        className="drawer" aria-label={`Card: ${card.title}`}
        onPaste={e => { const pasted = [...e.clipboardData.files]; if (pasted.length) { e.preventDefault(); void upload(pasted) } }}
      >
        <header className="drawer-head">
          <span className="dot" style={{ background: project.color }} />
          <span className="drawer-project">{project.name}</span>
          <button className="icon-btn" onClick={onClose} aria-label="Fechar"><Close /></button>
        </header>

        <div className="drawer-scroll">
          {card.titling && <div className="naming" role="status"><i aria-hidden />A IA está nomeando e marcando este card…</div>}
          <textarea
            className="title-input" rows={2} value={title} onChange={e => setTitle(e.target.value.replace(/\n/g, ' '))} aria-label="Título"
            onKeyDown={e => { if (e.key === 'Enter') e.preventDefault() }}
          />
          <textarea className="desc-input" rows={4} value={description} onChange={e => setDescription(e.target.value)} placeholder="Descreva o que a IA deve fazer." aria-label="Descrição" />
          <label className="field">Tags
            <input className="tags-input" value={tagsText} onChange={e => setTagsText(e.target.value)} placeholder="bug, frontend, relatórios" />
          </label>

          <section className="attachments" aria-label="Anexos">
            <div className="attachments-head">
              <strong>Anexos</strong>
              <button className="btn ghost" onClick={() => fileInput.current?.click()}><Clip />Anexar</button>
              <input ref={fileInput} type="file" multiple hidden onChange={e => { void upload([...(e.target.files ?? [])]); e.target.value = '' }} />
            </div>
            {files.length === 0
              ? <p className="muted attachments-empty">Cole um print aqui (Ctrl+V) ou use Anexar. A IA consegue ver imagens e PDFs.</p>
              : (
                <ul className="attachment-grid">
                  {files.map(a => (
                    <li key={a.id}>
                      <a href={`/api/attachments/${a.id}`} target="_blank" rel="noreferrer" title={a.name}>
                        {isImage(a.mime) ? <img src={`/api/attachments/${a.id}`} alt={a.name} loading="lazy" /> : <span className="file-ext">{(a.name.split('.').pop() || 'arq').slice(0, 4)}</span>}
                        <span className="attachment-name">{a.name}</span>
                      </a>
                      <button onClick={() => { if (confirm(`Remover "${a.name}"?`)) guard(() => api.deleteAttachment(a.id)) }} aria-label={`Remover ${a.name}`}><Close /></button>
                    </li>
                  ))}
                </ul>
              )}
          </section>

          <div className="field">Execução<ModeToggle value={mode} onChange={setMode} /></div>

          <section className="run-panel" data-status={status}>
            <div className="run-head">
              <strong>{status ? STATUS_LABEL[status] : 'Ainda não executou'}</strong>
              {direct && run && <span className="branch"><Branch />direto na pasta</span>}
              {!direct && run?.branch && <span className="branch"><Branch />{run.branch}</span>}
            </div>
            {run?.error && <p className="run-error">{run.error}</p>}

            <div className="actions wrap">
              {status === 'awaiting_approval' && <>
                <button className="btn primary" disabled={busy || !title.trim()} onClick={() => guard(async () => { if (dirty) await save(); await api.approve(run!.id) })}>
                  {dirty ? 'Salvar e aprovar' : 'Aprovar e executar'}
                </button>
                <button className="btn ghost" disabled={busy} onClick={() => guard(() => api.cancel(run!.id))}>Recusar</button>
              </>}
              {status === 'running' && <button className="btn danger" disabled={busy} onClick={() => guard(() => api.cancel(run!.id))}>Parar execução</button>}
              {status === 'queued' && <>
                <span className="muted queue-note">Todas as vagas de execução estão ocupadas. Começa assim que uma liberar.</span>
                <button className="btn ghost" disabled={busy} onClick={() => guard(() => api.cancel(run!.id))}>Sair da fila</button>
              </>}
              {finished && <>
                <button className="btn primary" disabled={busy} onClick={() => guard(() => api.merge(run!.id))}>{direct ? 'Aceitar mudanças' : 'Fazer merge na branch atual'}</button>
                <button className="btn ghost" disabled={busy} onClick={() => {
                  const msg = direct
                    ? 'Restaurar a pasta ao estado de antes desta execução? Qualquer alteração feita na pasta depois disso, inclusive por você, também será desfeita.'
                    : 'Descartar a branch e o worktree desta execução?'
                  if (confirm(msg)) guard(() => api.discard(run!.id))
                }}>{direct ? 'Desfazer na pasta' : 'Descartar'}</button>
              </>}
              {status === 'accepted' && <span className="muted queue-note">As mudanças já estão no projeto. Para continuar, peça um ajuste abaixo.</span>}
              {(!status || status === 'failed' || status === 'cancelled' || status === 'discarded') && (
                <button className="btn primary" disabled={busy || !title.trim()} onClick={() => guard(async () => { if (dirty) await save(); await api.runCard(card.id) })}>
                  {dirty ? 'Salvar e executar' : status ? 'Executar de novo' : 'Executar com IA'}
                </button>
              )}
            </div>

            {(finished || status === 'failed' || status === 'accepted') && (
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

        <footer className="drawer-foot" data-dirty={dirty || undefined}>
          {dirty ? (
            <>
              <span className="unsaved"><i aria-hidden />Alterações não salvas</span>
              <div className="actions">
                <button className="btn ghost" disabled={busy} onClick={revert}>Desfazer</button>
                <button className="btn primary" disabled={busy || !title.trim()} onClick={() => guard(save)} title="Salvar (Ctrl+S)">Salvar</button>
              </div>
            </>
          ) : (
            <button className="btn ghost danger-text" onClick={() => { if (confirm('Excluir este card?')) guard(async () => { await api.deleteCard(card.id); onClose() }) }}>Excluir card</button>
          )}
        </footer>
      </aside>
    </>
  )
}
