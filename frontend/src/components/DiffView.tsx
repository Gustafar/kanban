// A run's diff, grouped by file and collapsible, with +/- counts per file and per-line comments.
import { useState } from 'react'
import type { Comment } from '../types'

interface DiffLine { raw: string; kind: '' | 'hunk' | 'add' | 'del' | 'meta'; line: number | null }
interface FileDiff { name: string; adds: number; dels: number; lines: DiffLine[] }

const hunkRe = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/

function split(diff: string): FileDiff[] {
  const files: FileDiff[] = []
  let cur: FileDiff | null = null
  let oldLine = 0
  let newLine = 0
  for (const l of diff.split('\n')) {
    if (l.startsWith('diff --git ')) {
      const m = / b\/(.+)$/.exec(l)
      cur = { name: m ? m[1] : l.slice(11), adds: 0, dels: 0, lines: [] }
      files.push(cur)
      oldLine = newLine = 0
      continue
    }
    if (!cur) continue
    if (l.startsWith('index ') || l.startsWith('--- ') || l.startsWith('+++ ')) continue // noise the file header already conveys
    const hunk = hunkRe.exec(l)
    if (hunk) {
      oldLine = Number(hunk[1])
      newLine = Number(hunk[2])
      cur.lines.push({ raw: l, kind: 'hunk', line: null })
    } else if (l.startsWith('+')) {
      cur.adds++
      cur.lines.push({ raw: l, kind: 'add', line: newLine++ })
    } else if (l.startsWith('-')) {
      cur.dels++
      cur.lines.push({ raw: l, kind: 'del', line: -(oldLine++) }) // negative: anchors the old side, distinct from new-side numbers
    } else if (l.startsWith('new file') || l.startsWith('deleted file') || l.startsWith('similarity') || l.startsWith('rename')) {
      cur.lines.push({ raw: l, kind: 'meta', line: null })
    } else {
      cur.lines.push({ raw: l, kind: '', line: newLine })
      oldLine++
      newLine++
    }
  }
  return files
}

function LineComments({ comments, line, busy, onAdd }: { comments: Comment[]; line: number; busy: boolean; onAdd: (body: string) => void }) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState('')
  const here = comments.filter(c => c.line === line)
  const submit = () => {
    const body = draft.trim()
    if (!body) return
    onAdd(body)
    setDraft('')
    setOpen(false)
  }
  return (
    <>
      {here.length > 0 && (
        <div className="diff-comments">
          {here.map(c => (
            <div key={c.id} className="diff-comment" data-author={c.author}>
              <strong>{c.author === 'ai' ? 'IA' : 'Você'}</strong>
              <p>{c.body}</p>
            </div>
          ))}
        </div>
      )}
      {open ? (
        <form className="diff-comment-form" onSubmit={e => { e.preventDefault(); submit() }}>
          <textarea
            autoFocus value={draft} onChange={e => setDraft(e.target.value)} placeholder="Comentar esta linha" rows={2}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit() } else if (e.key === 'Escape') setOpen(false) }}
          />
          <div className="actions">
            <button type="button" className="btn ghost" onClick={() => setOpen(false)}>Cancelar</button>
            <button className="btn primary" disabled={busy || !draft.trim()}>Comentar</button>
          </div>
        </form>
      ) : (
        <button type="button" className="diff-comment-add" onClick={() => setOpen(true)} aria-label="Comentar esta linha">+ comentar</button>
      )}
    </>
  )
}

interface Props {
  diff: string
  direct: boolean
  comments?: Comment[]
  busy?: boolean
  onComment?: (file: string, line: number, body: string) => void
}

export function DiffView({ diff, direct, comments = [], busy = false, onComment }: Props) {
  if (!diff.trim()) {
    return <p className="empty-note">{direct ? 'Nenhuma mudança nos arquivos desta execução.' : 'Nenhuma mudança na branch desta execução.'}</p>
  }
  const files = split(diff)
  const adds = files.reduce((n, f) => n + f.adds, 0)
  const dels = files.reduce((n, f) => n + f.dels, 0)
  return (
    <div className="diff-view">
      <p className="diff-total">{files.length} {files.length === 1 ? 'arquivo' : 'arquivos'} <span className="plus">+{adds}</span> <span className="minus">−{dels}</span></p>
      {files.map(f => {
        const fileComments = comments.filter(c => c.file === f.name)
        return (
          <details key={f.name} className="diff-file" open={files.length <= 3}>
            <summary>
              <span className="diff-name" title={f.name}>{f.name}</span>
              <span className="diff-counts"><span className="plus">+{f.adds}</span> <span className="minus">−{f.dels}</span></span>
            </summary>
            <div className="diff">
              {f.lines.map((l, i) => (
                <div key={i} className={`diff-row diff-row-${l.kind || 'ctx'}`}>
                  <span className="diff-line-text">{l.raw}</span>
                  {onComment && l.line !== null && (
                    <LineComments comments={fileComments} line={l.line} busy={busy} onAdd={body => onComment(f.name, l.line as number, body)} />
                  )}
                </div>
              ))}
            </div>
          </details>
        )
      })}
    </div>
  )
}
