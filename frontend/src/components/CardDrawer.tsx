import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api'
import type { BoardListener } from '../useBoard'
import { COLUMNS, isImage } from '../types'
import type { Attachment, Card, Comment, Mode, Project, Run, RunEvent } from '../types'
import { STATUS_LABEL } from './CardView'
import { Branch, Clip, Close, Send } from './Icons'
import { ModeToggle } from './Dialogs'
import { DiffView } from './DiffView'
import { Markdown } from './Markdown'
import { ModelPicker } from './ModelPicker'
import { Steps } from './Steps'

interface Props {
  card: Card
  project: Project
  subscribe: (fn: BoardListener) => () => void
  onClose: () => void
  onError: (m: string) => void
}

type Tab = 'thread' | 'steps' | 'diff'
type Composer = 'comment' | 'adjust' | 'polish'

const money = (n: number) => n.toLocaleString('pt-BR', { style: 'currency', currency: 'USD' })

function stamp(iso: string) {
  const d = new Date(iso)
  const today = d.toDateString() === new Date().toDateString()
  return d.toLocaleString('pt-BR', today ? { hour: '2-digit', minute: '2-digit' } : { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function took(r: Run) {
  if (!r.startedAt || !r.finishedAt) return ''
  const s = Math.round((Date.parse(r.finishedAt) - Date.parse(r.startedAt)) / 1000)
  if (s <= 0) return ''
  return s < 60 ? `${s} s` : `${Math.floor(s / 60)} min ${s % 60} s`
}

function TagEditor({ tags, onChange }: { tags: string[]; onChange: (t: string[]) => void }) {
  const [draft, setDraft] = useState('')
  const add = () => {
    const parts = draft.split(',').map(t => t.trim().replace(/^#/, '')).filter(Boolean)
    if (parts.length) onChange([...tags, ...parts.filter(p => !tags.includes(p))])
    setDraft('')
  }
  return (
    <div className="tag-editor">
      {tags.map(t => (
        <span className="tag-chip" key={t}>#{t}<button type="button" onClick={() => onChange(tags.filter(x => x !== t))} aria-label={`Remover tag ${t}`}><Close /></button></span>
      ))}
      <input
        value={draft} onChange={e => setDraft(e.target.value)} onBlur={add} aria-label="Adicionar tag" placeholder={tags.length ? '' : 'Adicionar tag e apertar Enter'}
        onKeyDown={e => {
          if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); add() }
          else if (e.key === 'Backspace' && !draft && tags.length) onChange(tags.slice(0, -1))
        }}
      />
    </div>
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
  const [comments, setComments] = useState<Comment[]>([])
  const [diff, setDiff] = useState('')
  const [tab, setTab] = useState<Tab>('thread')
  const [pane, setPane] = useState<'task' | 'thread'>('task') // narrow screens show one column at a time
  const [draft, setDraft] = useState('')
  const [composer, setComposer] = useState<Composer>('comment')
  const [busy, setBusy] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

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

  const guard = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true)
    try { await fn() } catch (e) { onError((e as Error).message) } finally { setBusy(false) }
  }, [onError])

  // Ctrl+S saves; the ref keeps the listener pointing at the latest field values
  const saveShortcut = useRef<() => void>(() => {})
  saveShortcut.current = () => { if (dirty && title.trim() && !busy) void guard(save) }
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); saveShortcut.current() }
      else if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  async function upload(list: File[]) {
    for (const f of list) {
      if (f.size > 15 * 1024 * 1024) { onError(`"${f.name}" passa de 15 MB.`); continue }
      try { await api.uploadAttachment(card.id, f) } catch (e) { onError((e as Error).message) }
    }
  }

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

  // the right column behaves like a chat: new items scroll into view, the diff starts at the top
  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTo({ top: tab === 'diff' ? 0 : el.scrollHeight })
  }, [events.length, comments.length, runs.length, tab, card.polishing])

  const status = run?.status
  const finished = status === 'succeeded'
  const direct = project.isolation === 'direct'

  // comments and run markers share one chronological thread
  const timeline = useMemo(() => {
    const asc = [...runs].reverse()
    return [
      ...comments.map(c => ({ key: `c${c.id}`, at: Date.parse(c.createdAt), comment: c, run: null as Run | null, n: 0 })),
      ...asc.map((r, i) => ({ key: `r${r.id}`, at: Date.parse(r.createdAt), comment: null as Comment | null, run: r, n: i + 1 })),
    ].sort((a, b) => a.at - b.at)
  }, [comments, runs])

  const canAdjust = status === 'succeeded' || status === 'failed' || status === 'accepted'
  const canPolish = !run
  const composers: { id: Composer; label: string }[] = [
    { id: 'comment', label: 'Comentar' },
    ...(canAdjust ? [{ id: 'adjust' as const, label: 'Pedir ajuste à IA' }] : []),
    ...(canPolish ? [{ id: 'polish' as const, label: 'Conversar com a IA' }] : []),
  ]
  const active: Composer = composers.some(c => c.id === composer) ? composer : 'comment'
  const placeholder = { comment: 'Escrever um comentário', adjust: 'O que a IA deve ajustar? Ela continua a mesma conversa.', polish: 'Uma dúvida ou ajuste antes de começar (opcional)' }[active]
  const canSend = !busy && (active === 'polish' ? !card.polishing : !!draft.trim())

  function send() {
    const text = draft.trim()
    if (!canSend) return
    void guard(async () => {
      if (active === 'comment') await api.addComment(card.id, text)
      else if (active === 'adjust') await api.runCard(card.id, text)
      else await api.polishCard(card.id, text)
      setDraft('')
    })
  }

  const colTitle = COLUMNS.find(c => c.id === card.column)?.title ?? card.column

  return (
    <>
      <div className="scrim" onClick={onClose} />
      <aside
        className="drawer detail" data-pane={pane} aria-label={`Card: ${card.title}`}
        onPaste={e => { const pasted = [...e.clipboardData.files]; if (pasted.length) { e.preventDefault(); void upload(pasted) } }}
      >
        <header className="detail-head">
          <span className="dot" style={{ background: project.color }} />
          <span className="drawer-project">{project.name}</span>
          <span className="crumb">#{card.id}</span>
          <span className="pill" data-col={card.column}>{colTitle}</span>
          <span className="spacer" />
          <div className="pane-switch" role="tablist" aria-label="Seção">
            <button role="tab" aria-selected={pane === 'task'} onClick={() => setPane('task')}>Tarefa</button>
            <button role="tab" aria-selected={pane === 'thread'} onClick={() => setPane('thread')}>Conversa{comments.length > 0 && <span className="tab-count">{comments.length}</span>}</button>
          </div>
          <button className="icon-btn" onClick={onClose} aria-label="Fechar"><Close /></button>
        </header>

        <div className="detail-body">
          {/* ---------- the task: what the AI is asked to do ---------- */}
          <section className="pane pane-task" aria-label="Tarefa">
            {card.titling && <div className="naming" role="status"><i aria-hidden />A IA está nomeando e marcando este card…</div>}
            <textarea
              className="title-input" rows={1} value={title} onChange={e => setTitle(e.target.value.replace(/\n/g, ' '))} aria-label="Título"
              onKeyDown={e => { if (e.key === 'Enter') e.preventDefault() }}
            />

            <div className="field-block">
              <div className="field-title">Descrição <small>é o pedido que a IA recebe</small></div>
              <textarea className="desc-grow" value={description} onChange={e => setDescription(e.target.value)} placeholder="Descreva o que a IA deve fazer, onde mexer e o que não pode mudar." aria-label="Descrição" />
            </div>

            <div className="field-block">
              <div className="field-title">Tags</div>
              <TagEditor tags={tagList} onChange={list => setTagsText(list.join(', '))} />
            </div>

            <section className="field-block attachments" aria-label="Anexos">
              <div className="attachments-head">
                <div className="field-title">Anexos{files.length > 0 && <small>{files.length}</small>}</div>
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

            <div className="field-block">
              <div className="field-title">Execução</div>
              <ModeToggle value={mode} onChange={setMode} />
            </div>

            <div className="field-block">
              <div className="field-title">Modelo e esforço <small>valem para as execuções deste card</small></div>
              <ModelPicker
                model={card.model ?? ''} effort={card.effort ?? ''} disabled={busy}
                onChange={(m, e) => void guard(() => api.updateCard(card.id, { title: card.title, description: card.description, mode: card.mode, model: m, effort: e }))}
              />
            </div>

            {card.createdAt && <p className="muted created">Criado em {stamp(card.createdAt)}</p>}
          </section>

          {/* ---------- the conversation: status, actions, thread, composer ---------- */}
          <section className="pane pane-thread" aria-label="Conversa e execução">
            <div className="run-panel run-compact" data-status={status}>
              <div className="run-head">
                <strong>{status ? STATUS_LABEL[status] : 'Ainda não executou'}</strong>
                {direct && run && <span className="branch"><Branch />direto na pasta</span>}
                {!direct && run?.branch && <span className="branch"><Branch />{run.branch}</span>}
              </div>
              {run && (took(run) || run.costUsd > 0) && (
                <p className="run-meta">{[took(run) && `Durou ${took(run)}`, run.costUsd > 0 && money(run.costUsd), events.length > 0 && `${events.length} passos`].filter(Boolean).join(' · ')}</p>
              )}
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
                  <span className="muted queue-note">Na fila: as vagas estão ocupadas ou outra execução está editando a mesma pasta. Começa sozinha assim que possível.</span>
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
                  <button className="btn ghost" onClick={() => setTab('diff')}>Ver mudanças</button>
                </>}
                {status === 'accepted' && <span className="muted queue-note">As mudanças já estão no projeto. Para continuar, peça um ajuste abaixo.</span>}
                {(!status || status === 'failed' || status === 'cancelled' || status === 'discarded') && (
                  <button className="btn primary" disabled={busy || !title.trim()} onClick={() => guard(async () => { if (dirty) await save(); await api.runCard(card.id) })}>
                    {dirty ? 'Salvar e executar' : status ? 'Executar de novo' : 'Executar com IA'}
                  </button>
                )}
              </div>
            </div>

            <div className="tabs" role="tablist">
              <button role="tab" aria-selected={tab === 'thread'} onClick={() => setTab('thread')}>Conversa{comments.length > 0 && <span className="tab-count">{comments.length}</span>}</button>
              <button role="tab" aria-selected={tab === 'steps'} onClick={() => setTab('steps')} disabled={!run}>Passos{events.length > 0 && <span className="tab-count">{events.length}</span>}</button>
              <button role="tab" aria-selected={tab === 'diff'} onClick={() => setTab('diff')} disabled={!run}>Mudanças</button>
            </div>

            <div className="thread-scroll" ref={scrollRef}>
              {tab === 'thread' && (
                <ol className="thread">
                  {timeline.length === 0 && !card.polishing && (
                    <li className="empty-note">Nada por aqui ainda. Comentários, resumos da IA e execuções aparecem em ordem, como numa conversa.</li>
                  )}
                  {timeline.map(it => it.comment ? (
                    <li key={it.key} className="msg" data-author={it.comment.author}>
                      <span className="msg-avatar" aria-hidden>{it.comment.author === 'ai' ? 'IA' : 'V'}</span>
                      <div className="msg-body">
                        <header><strong>{it.comment.author === 'ai' ? 'IA' : 'Você'}</strong><time dateTime={it.comment.createdAt}>{stamp(it.comment.createdAt)}</time></header>
                        <Markdown text={it.comment.body} />
                      </div>
                    </li>
                  ) : (
                    <li key={it.key} className="run-mark" data-status={it.run!.status}>
                      <i aria-hidden />
                      <span>Execução {it.n}</span>
                      <b>{STATUS_LABEL[it.run!.status]}</b>
                      <time dateTime={it.run!.createdAt}>{stamp(it.run!.createdAt)}</time>
                      {it.run!.costUsd > 0 && <span>{money(it.run!.costUsd)}</span>}
                    </li>
                  ))}
                  {card.polishing && <li className="typing" role="status">A IA está pensando<span /><span /><span /></li>}
                </ol>
              )}
              {tab === 'steps' && run && <Steps events={events} awaiting={status === 'awaiting_approval'} />}
              {tab === 'diff' && run && <DiffView diff={diff} direct={direct} />}
            </div>

            <form className="composer" onSubmit={e => { e.preventDefault(); send() }}>
              {composers.length > 1 && (
                <div className="composer-modes" role="radiogroup" aria-label="O que fazer com a mensagem">
                  {composers.map(c => (
                    <button key={c.id} type="button" role="radio" aria-checked={active === c.id} data-on={active === c.id || undefined} onClick={() => setComposer(c.id)}>{c.label}</button>
                  ))}
                </div>
              )}
              <div className="composer-row">
                <textarea
                  value={draft} onChange={e => setDraft(e.target.value)} placeholder={placeholder} aria-label="Mensagem" rows={1}
                  onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); send() } }}
                />
                <button className="send" disabled={!canSend} aria-label={active === 'comment' ? 'Comentar' : active === 'adjust' ? 'Enviar à IA' : 'Conversar com a IA'}><Send /></button>
              </div>
            </form>
          </section>
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
