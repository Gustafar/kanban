import { forwardRef, useEffect, useRef, useState } from 'react'
import type { ClipboardEvent, DragEvent, KeyboardEvent } from 'react'
import { api } from '../api'
import type { ColumnId, Project } from '../types'
import { Close, Clip, Send } from './Icons'

const MAX_FILE = 15 * 1024 * 1024
const TARGETS: { id: ColumnId; label: string }[] = [
  { id: 'backlog', label: 'Backlog' },
  { id: 'todo', label: 'A fazer' },
  { id: 'doing', label: 'Executar agora' },
]

interface Pending { key: number; file: File; url: string | null }

// Bottom-centre capture bar: write what you want, the card is created at once and the AI names and tags it.
export const QuickBar = forwardRef<HTMLTextAreaElement, { project: Project; onError: (m: string) => void }>(function QuickBar({ project, onError }, ref) {
  const [text, setText] = useState('')
  const [target, setTarget] = useState<ColumnId>('backlog')
  const [files, setFiles] = useState<Pending[]>([])
  const [busy, setBusy] = useState(false)
  const [over, setOver] = useState(false)
  const seq = useRef(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const areaRef = useRef<HTMLTextAreaElement | null>(null)

  useEffect(() => () => files.forEach(f => f.url && URL.revokeObjectURL(f.url)), []) // eslint-disable-line react-hooks/exhaustive-deps

  function grow() {
    const el = areaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 168) + 'px'
  }
  useEffect(grow, [text])

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
      l.find(f => f.key === key)?.url && URL.revokeObjectURL(l.find(f => f.key === key)!.url!)
      return l.filter(f => f.key !== key)
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

  async function submit() {
    const value = text.trim()
    if (!value || busy) return
    setBusy(true)
    try {
      const card = await api.quick(project.id, { text: value, column: target, mode: 'approval' })
      const failed: string[] = []
      for (const f of files) {
        try { await api.uploadAttachment(card.id, f.file) } catch { failed.push(f.file.name || 'arquivo') }
      }
      if (failed.length) onError(`Card criado, mas não consegui anexar: ${failed.join(', ')}.`)
      files.forEach(f => f.url && URL.revokeObjectURL(f.url))
      setFiles([])
      setText('')
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void submit() }
    if (e.key === 'Escape') (e.currentTarget as HTMLElement).blur()
  }

  return (
    <div className="quick" data-over={over || undefined} onDragOver={e => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)} onDrop={onDrop}>
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
      <div className="quick-row">
        <button type="button" className="icon-btn" onClick={() => inputRef.current?.click()} aria-label="Anexar arquivo" title="Anexar (ou cole um print)"><Clip /></button>
        <input ref={inputRef} type="file" multiple hidden onChange={e => { addFiles([...(e.target.files ?? [])]); e.target.value = '' }} />
        <textarea
          ref={el => { areaRef.current = el; if (typeof ref === 'function') ref(el); else if (ref) ref.current = el }}
          rows={1} value={text} onChange={e => setText(e.target.value)} onKeyDown={onKeyDown} onPaste={onPaste}
          placeholder="O que você quer fazer? A IA dá o título e as tags."
          aria-label="Descrever uma nova tarefa" disabled={busy}
        />
        <select value={target} onChange={e => setTarget(e.target.value as ColumnId)} aria-label="Destino do card" title="Onde o card entra">
          {TARGETS.map(t => <option key={t.id} value={t.id}>{t.label}</option>)}
        </select>
        <button type="button" className="send" onClick={() => void submit()} disabled={busy || !text.trim()} aria-label="Criar card" title="Criar card (Enter)">
          {busy ? <span className="spinner" /> : <Send />}
        </button>
      </div>
    </div>
  )
})
