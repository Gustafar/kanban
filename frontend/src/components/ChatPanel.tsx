import { useCallback, useEffect, useRef, useState } from 'react'
import type { ClipboardEvent, DragEvent, KeyboardEvent } from 'react'
import { api } from '../api'
import type { BoardListener } from '../useBoard'
import { isImage } from '../types'
import type { Chat, ChatMessage, Project } from '../types'
import { Check, Chevron, Chat as ChatIcon, Clip, Close, Plus, Send } from './Icons'
import { Markdown } from './Markdown'
import { ModelPicker } from './ModelPicker'
import { Popover } from './Popover'

interface Props {
  project: Project
  subscribe: (fn: BoardListener) => () => void
  onClose: () => void
  onError: (m: string) => void
}

const MAX_FILE = 15 * 1024 * 1024

interface Pending { key: number; file: File; url: string | null }

const byRecent = (a: Chat, b: Chat) => b.updatedAt.localeCompare(a.updatedAt)

function upsertChat(list: Chat[], c: Chat) {
  return (list.some(x => x.id === c.id) ? list.map(x => (x.id === c.id ? c : x)) : [c, ...list]).sort(byRecent)
}

const mergeMessages = (list: ChatMessage[], m: ChatMessage) => (list.some(x => x.id === m.id) ? list : [...list, m])

// Turns the CLI's "tool_name {json}" line into something readable.
function toolLabel(body: string): string {
  const i = body.indexOf(' ')
  const name = i < 0 ? body : body.slice(0, i)
  let a: Record<string, unknown> = {}
  try { a = JSON.parse(i < 0 ? '{}' : body.slice(i + 1)) } catch { /* truncated input: fall back to the name only */ }
  const card = a.card_id != null ? `#${String(a.card_id).replace('#', '')}` : 'do card'
  switch (name) {
    case 'list_projects': return 'Consultando o projeto'
    case 'list_cards': return 'Consultando os cards'
    case 'get_card': return `Abrindo o card ${card}`
    case 'create_card': return `Criando card${a.title ? ` “${a.title}”` : ''}${a.column ? ` em ${a.column}` : ''}`
    case 'update_card': return `Atualizando o card ${card}`
    case 'move_card': return `Movendo o card ${card}${a.column ? ` para ${a.column}` : ''}`
    case 'add_note': return `Anotando no card ${card}`
    case 'Read': return `Lendo ${a.file_path ?? 'um arquivo'}`
    case 'Glob': case 'Grep': return 'Procurando no código'
    default: return name
  }
}

const stamp = (iso: string) => new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })

function ChatPicker({ chats, activeId, onPick, onDelete }: { chats: Chat[]; activeId: number | null; onPick: (id: number) => void; onDelete: (c: Chat) => void }) {
  const active = chats.find(c => c.id === activeId)
  return (
    <Popover width={340} trigger={({ open, toggle }) => (
      <button type="button" className="chat-pick" onClick={toggle} aria-haspopup="menu" aria-expanded={open}>
        <ChatIcon />
        <span>{active ? active.title || 'Nova conversa' : 'Nova conversa'}</span>
        <Chevron />
      </button>
    )}>
      {close => (
        <>
          <p className="pop-title">Conversas deste projeto</p>
          {chats.length === 0 && <p className="chat-pick-empty">Nenhuma conversa ainda.</p>}
          {chats.map(c => (
            <div key={c.id} className="chat-pick-row">
              <button type="button" role="menuitemradio" aria-checked={c.id === activeId} className="pop-item" data-on={c.id === activeId || undefined} onClick={() => { onPick(c.id); close() }}>
                <span className="pop-item-text">
                  <b>{c.title || 'Nova conversa'}</b>
                  <small>{c.busy ? 'IA respondendo…' : new Date(c.updatedAt).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}</small>
                </span>
                {c.id === activeId && <Check />}
              </button>
              <button type="button" className="icon-btn chat-del" onClick={() => onDelete(c)} aria-label={`Excluir conversa ${c.title}`} title="Excluir conversa"><Close /></button>
            </div>
          ))}
        </>
      )}
    </Popover>
  )
}

// Per-project chat with the AI: it reads and edits this project's board through the MCP tools.
export function ChatPanel({ project, subscribe, onClose, onError }: Props) {
  const [chats, setChats] = useState<Chat[]>([])
  const [activeId, setActiveId] = useState<number | null>(null) // null = a new conversation not created yet
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draftModel, setDraftModel] = useState({ model: '', effort: '' })
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [files, setFiles] = useState<Pending[]>([])
  const [over, setOver] = useState(false)
  const seq = useRef(0)
  const fileInput = useRef<HTMLInputElement>(null)
  const filesRef = useRef(files)
  filesRef.current = files
  const scrollRef = useRef<HTMLDivElement>(null)
  const areaRef = useRef<HTMLTextAreaElement>(null)
  const activeRef = useRef<number | null>(null)
  activeRef.current = activeId

  const active = chats.find(c => c.id === activeId)
  const busy = sending || !!active?.busy

  // open on the most recent conversation
  useEffect(() => {
    let live = true
    api.chats(project.id).then(cs => { if (live) { setChats(cs); setActiveId(cs[0]?.id ?? null) } }).catch(e => onError(e.message))
    return () => { live = false }
  }, [project.id, onError])

  useEffect(() => {
    if (activeId == null) { setMessages([]); return }
    let live = true
    api.chatMessages(activeId).then(ms => { if (live) setMessages(cur => ms.reduce(mergeMessages, cur.filter(m => m.chatId === activeId))) }).catch(e => onError(e.message))
    return () => { live = false }
  }, [activeId, onError])

  useEffect(() => subscribe((type, data) => {
    if (type === 'chat' && data.projectId === project.id) setChats(l => upsertChat(l, data))
    else if (type === 'chat_deleted' && data.projectId === project.id) {
      setChats(l => l.filter(c => c.id !== data.id))
      setActiveId(cur => (cur === data.id ? null : cur))
    } else if (type === 'chat_message' && data.chatId === activeRef.current) setMessages(l => mergeMessages(l, data))
  }), [subscribe, project.id])

  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messages, busy])

  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('dialog[open]')) onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  useEffect(() => { areaRef.current?.focus() }, [activeId])
  useEffect(() => () => filesRef.current.forEach(f => f.url && URL.revokeObjectURL(f.url)), [])

  function addFiles(list: File[]) {
    const ok: Pending[] = []
    for (const file of list) {
      if (file.size > MAX_FILE) { onError(`"${file.name || 'arquivo'}" passa de 15 MB e não foi anexado.`); continue }
      ok.push({ key: ++seq.current, file, url: file.type.startsWith('image/') ? URL.createObjectURL(file) : null })
    }
    if (ok.length) setFiles(l => [...l, ...ok])
  }

  function removeFile(key: number) {
    setFiles(l => {
      const f = l.find(x => x.key === key)
      if (f?.url) URL.revokeObjectURL(f.url)
      return l.filter(x => x.key !== key)
    })
  }

  function onPaste(e: ClipboardEvent) {
    const pasted = [...e.clipboardData.files]
    if (pasted.length) { e.preventDefault(); addFiles(pasted) }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    setOver(false)
    if (e.dataTransfer.files.length) addFiles([...e.dataTransfer.files])
  }

  const send = useCallback(async () => {
    const value = text.trim()
    if ((!value && files.length === 0) || busy) return
    setSending(true)
    try {
      let id = activeId
      if (id == null) {
        const c = await api.createChat(project.id, draftModel)
        setChats(l => upsertChat(l, c))
        activeRef.current = id = c.id
        setActiveId(c.id)
      }
      const ids: number[] = []
      for (const f of files) ids.push((await api.uploadChatAttachment(id, f.file)).id)
      files.forEach(f => f.url && URL.revokeObjectURL(f.url))
      setFiles([])
      setText('')
      await api.sendChat(id, value, ids)
    } catch (e) { onError((e as Error).message) } finally { setSending(false) }
  }, [text, files, busy, activeId, project.id, draftModel, onError])

  function onKeyDown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void send() }
  }

  async function remove(c: Chat) {
    try {
      await api.deleteChat(c.id)
      setChats(l => l.filter(x => x.id !== c.id))
      if (c.id === activeId) setActiveId(null)
    } catch (e) { onError((e as Error).message) }
  }

  function changeModel(model: string, effort: string) {
    if (active) {
      setChats(l => l.map(c => (c.id === active.id ? { ...c, model, effort } : c)))
      api.updateChat(active.id, { model, effort }).catch(e => onError(e.message))
    } else setDraftModel({ model, effort })
  }

  const model = active ? active.model : draftModel.model
  const effort = active ? active.effort : draftModel.effort

  return (
    <>
      <div className="scrim" onClick={onClose} />
      <aside className="drawer chat" data-over={over || undefined} aria-label={`Conversa com a IA — ${project.name}`}
        onDragOver={e => { e.preventDefault(); setOver(true) }} onDragLeave={e => { if (e.currentTarget === e.target) setOver(false) }} onDrop={onDrop}>
        <header className="chat-head">
          <span className="dot" style={{ background: project.color }} />
          <ChatPicker chats={chats} activeId={activeId} onPick={setActiveId} onDelete={c => void remove(c)} />
          <button type="button" className="icon-btn" onClick={() => setActiveId(null)} aria-label="Nova conversa" title="Nova conversa" disabled={activeId == null}><Plus /></button>
          <span className="chat-spacer" />
          <ModelPicker model={model} effort={effort} onChange={changeModel} />
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Fechar conversa"><Close /></button>
        </header>

        <div className="chat-scroll" ref={scrollRef}>
          {messages.length === 0 && !busy ? (
            <div className="chat-empty">
              <h2>Converse sobre {project.name}</h2>
              <p>Peça para a IA criar cards a partir de uma ideia, reorganizar o quadro ou resumir o que está em andamento. Ela só mexe neste projeto e nunca inicia execuções por conta própria.</p>
              <ul>
                {['Crie cards para: login com Google, recuperação de senha e testes do fluxo de cadastro', 'Mova para Concluído os cards que já estão em revisão', 'Resuma o que falta fazer e sugira o que atacar primeiro'].map(s => (
                  <li key={s}><button type="button" onClick={() => { setText(s); areaRef.current?.focus() }}>{s}</button></li>
                ))}
              </ul>
            </div>
          ) : (
            <ol className="chat-log">
              {messages.map(m => m.role === 'tool' ? (
                <li key={m.id} className="chat-tool">{toolLabel(m.body)}</li>
              ) : m.role === 'error' ? (
                <li key={m.id} className="chat-error">{m.body}</li>
              ) : (
                <li key={m.id} className="chat-msg" data-role={m.role}>
                  <div className="chat-bubble">
                    {m.attachments?.length > 0 && (
                      <ul className="chat-files">
                        {m.attachments.map(a => (
                          <li key={a.id}>
                            <a href={`/api/chat-attachments/${a.id}`} target="_blank" rel="noreferrer" title={a.name}>
                              {isImage(a.mime) ? <img src={`/api/chat-attachments/${a.id}`} alt={a.name} loading="lazy" /> : <span className="chat-file-doc"><Clip />{a.name}</span>}
                            </a>
                          </li>
                        ))}
                      </ul>
                    )}
                    {m.role === 'assistant' ? <Markdown text={m.body} /> : m.body && <p>{m.body}</p>}
                  </div>
                  <time dateTime={m.createdAt}>{stamp(m.createdAt)}</time>
                </li>
              ))}
              {busy && <li className="chat-typing" aria-live="polite"><span className="spinner" />A IA está trabalhando…</li>}
            </ol>
          )}
        </div>

        <div className="chat-composer">
          {files.length > 0 && (
            <ul className="quick-files" aria-label="Anexos pendentes">
              {files.map(f => (
                <li key={f.key} title={f.file.name}>
                  {f.url ? <img src={f.url} alt="" /> : <span className="file-ext">{(f.file.name.split('.').pop() || 'arq').slice(0, 4)}</span>}
                  <button type="button" onClick={() => removeFile(f.key)} aria-label={`Remover ${f.file.name || 'anexo'}`}><Close /></button>
                </li>
              ))}
            </ul>
          )}
          <div className="composer-row">
            <button type="button" className="icon-btn" onClick={() => fileInput.current?.click()} aria-label="Anexar arquivo" title="Anexar (ou cole um print)" disabled={busy}><Clip /></button>
            <input ref={fileInput} type="file" multiple hidden onChange={e => { addFiles([...(e.target.files ?? [])]); e.target.value = '' }} />
            <textarea
              ref={areaRef} rows={1} value={text} onChange={e => setText(e.target.value)} onKeyDown={onKeyDown} onPaste={onPaste}
              placeholder="Peça algo sobre o quadro… (Enter envia, cole prints com Ctrl+V)" aria-label="Mensagem para a IA"
            />
            {busy && active?.busy ? (
              <button type="button" className="btn" onClick={() => api.cancelChat(active.id).catch(e => onError(e.message))}>Parar</button>
            ) : (
              <button type="button" className="send" onClick={() => void send()} disabled={busy || (!text.trim() && files.length === 0)} aria-label="Enviar" title="Enviar (Enter)">
                {sending ? <span className="spinner" /> : <Send />}
              </button>
            )}
          </div>
        </div>
      </aside>
    </>
  )
}
