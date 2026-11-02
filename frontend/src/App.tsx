import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from './api'
import { useBoard } from './useBoard'
import type { ColumnId } from './types'
import { Board } from './components/Board'
import { CardDrawer } from './components/CardDrawer'
import { Modal, NewCardDialog, ProjectDialog } from './components/Dialogs'
import { Bell, Gear, Plus, Search } from './components/Icons'
import { QuickBar } from './components/QuickBar'
import type { Card } from './types'

type Dialog = { kind: 'card'; column: ColumnId } | { kind: 'project'; edit: boolean } | { kind: 'help' } | null
type Filter = 'all' | 'attention' | 'running'

const FILTERS: { id: Filter; label: string; hint: string }[] = [
  { id: 'all', label: 'Todos', hint: 'Todos os cards' },
  { id: 'attention', label: 'Precisa de mim', hint: 'Aguardando aprovação, com falha ou em revisão' },
  { id: 'running', label: 'IA trabalhando', hint: 'Execuções em andamento' },
]

const matchesFilter = (c: Card, f: Filter) =>
  f === 'all' ||
  (f === 'running' ? c.runStatus === 'running' : c.runStatus === 'awaiting_approval' || c.runStatus === 'failed' || c.column === 'review')

const SHORTCUTS: [string, string][] = [
  ['n', 'Escrever uma tarefa (barra de baixo)'], ['Enter', 'Criar o card na barra (Shift+Enter quebra a linha)'], ['/', 'Buscar'], ['1 – 9', 'Trocar de projeto'], ['Esc', 'Fechar card ou limpar busca'], ['?', 'Mostrar atalhos'],
]

const canNotify = typeof Notification !== 'undefined' && window.isSecureContext

export default function App() {
  const b = useBoard()
  const [openCard, setOpenCard] = useState<number | null>(null)
  const [dialog, setDialog] = useState<Dialog>(null)
  const [perm, setPerm] = useState(canNotify ? Notification.permission : 'denied')

  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const searchRef = useRef<HTMLInputElement>(null)
  const quickRef = useRef<HTMLTextAreaElement>(null)

  const project = b.projects.find(p => p.id === b.projectId)
  const card = b.cards.find(c => c.id === openCard)

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    const bare = q.replace(/^#/, '')
    return b.cards.filter(c => matchesFilter(c, filter) && (!q ||
      c.title.toLowerCase().includes(q) || c.description.toLowerCase().includes(q) || c.tags.some(t => t.includes(bare))))
  }, [b.cards, query, filter])
  const attention = useMemo(() => b.cards.filter(c => matchesFilter(c, 'attention')).length, [b.cards])
  const filtering = filter !== 'all' || query.trim() !== ''

  // keyboard shortcuts: ignored while typing or with a dialog open
  const { projects, setProjectId } = b
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return
      const t = e.target as HTMLElement | null
      if (t?.closest?.('input, textarea, select, [contenteditable]') || document.querySelector('dialog[open]')) return
      if (e.key === '/') { e.preventDefault(); searchRef.current?.focus() }
      else if (e.key === 'n' && project) { e.preventDefault(); quickRef.current?.focus() }
      else if (e.key === '?') setDialog({ kind: 'help' })
      else if (/^[1-9]$/.test(e.key) && projects[+e.key - 1]) { setProjectId(projects[+e.key - 1].id); setOpenCard(null) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [project, projects, setProjectId])

  useEffect(() => { document.title = project ? `${project.name} · Kanban` : 'Kanban' }, [project])

  // browser notification only when the tab is in the background (the server also fires a Windows toast)
  useEffect(() => b.subscribe((type, data) => {
    if (type === 'notify' && canNotify && Notification.permission === 'granted' && document.hidden) {
      new Notification(data.title, { body: data.body, tag: `card-${data.cardId}` })
    }
  }), [b.subscribe])

  const { setError } = b
  const onError = useCallback((m: string) => setError(m), [setError])

  function move(id: number, column: ColumnId, position: number) {
    b.setCards(l => l.map(c => (c.id === id ? { ...c, column, position } : c)))
    api.moveCard(id, column, position).catch(e => {
      onError(e.message)
      if (b.projectId != null) api.cards(b.projectId).then(b.setCards)
    })
  }

  if (!b.loaded) return <div className="splash">Carregando…</div>

  return (
    <div className="app">
      <nav className="rail" aria-label="Projetos">
        <div className="brand">Kanban</div>
        <ul>
          {b.projects.map(p => (
            <li key={p.id}>
              <button className="proj" data-on={p.id === b.projectId || undefined} onClick={() => { b.setProjectId(p.id); setOpenCard(null) }}>
                <span className="dot" style={{ background: p.color }} />
                <span className="proj-name">{p.name}</span>
              </button>
            </li>
          ))}
        </ul>
        <button className="proj add" onClick={() => setDialog({ kind: 'project', edit: false })}><Plus /><span className="proj-name">Novo projeto</span></button>
      </nav>

      <main className="main">
        {project ? (
          <>
            <header className="topbar">
              <div className="topbar-title">
                <h1>{project.name}</h1>
                <span className="path" title={project.path}>{project.path}</span>
              </div>
              <div className="topbar-actions">
                {canNotify && perm === 'default' && (
                  <button className="btn ghost" onClick={() => Notification.requestPermission().then(setPerm)}><Bell /><span className="btn-label">Ativar avisos</span></button>
                )}
                <button className="icon-btn" onClick={() => setDialog({ kind: 'project', edit: true })} aria-label="Configurar projeto"><Gear /></button>
                <button className="btn primary" onClick={() => setDialog({ kind: 'card', column: 'backlog' })}><Plus />Novo card</button>
              </div>
            </header>
            <div className="toolbar">
              <label className="search">
                <Search />
                <input
                  ref={searchRef} value={query} onChange={e => setQuery(e.target.value)} placeholder="Buscar cards  ( / )" aria-label="Buscar cards"
                  onKeyDown={e => { if (e.key === 'Escape') { setQuery(''); e.currentTarget.blur() } }}
                />
              </label>
              <div className="chips" role="group" aria-label="Filtrar cards">
                {FILTERS.map(f => (
                  <button key={f.id} className="chip" aria-pressed={filter === f.id} title={f.hint} onClick={() => setFilter(f.id)}>
                    {f.label}{f.id === 'attention' && attention > 0 && <span className="chip-count">{attention}</span>}
                  </button>
                ))}
              </div>
              <button className="icon-btn" onClick={() => setDialog({ kind: 'help' })} aria-label="Atalhos de teclado" title="Atalhos (?)">?</button>
            </div>
            {filtering && visible.length === 0 && <p className="no-results">Nenhum card combina com a busca ou o filtro.</p>}
            <Board cards={visible} live={b.live} onMove={move} onOpen={setOpenCard} onAdd={column => setDialog({ kind: 'card', column })} />
            <QuickBar key={project.id} ref={quickRef} project={project} onError={onError} />
          </>
        ) : (
          <div className="empty">
            <h1>Comece por um projeto</h1>
            <p>Cada projeto aponta para uma pasta com um repositório git. É lá que a IA vai trabalhar quando você mover um card para execução.</p>
            <button className="btn primary" onClick={() => setDialog({ kind: 'project', edit: false })}><Plus />Criar projeto</button>
          </div>
        )}
      </main>

      {card && project && <CardDrawer key={card.id} card={card} project={project} subscribe={b.subscribe} onClose={() => setOpenCard(null)} onError={onError} />}

      {dialog?.kind === 'card' && project && <NewCardDialog project={project} column={dialog.column} onClose={() => setDialog(null)} onError={onError} />}
      {dialog?.kind === 'help' && (
        <Modal title="Atalhos de teclado" onClose={() => setDialog(null)}>
          <dl className="shortcuts">
            {SHORTCUTS.map(([k, d]) => <div key={k}><dt><kbd>{k}</kbd></dt><dd>{d}</dd></div>)}
          </dl>
        </Modal>
      )}
      {dialog?.kind === 'project' && (
        <ProjectDialog
          project={dialog.edit ? project : undefined}
          onClose={() => setDialog(null)}
          onError={onError}
          onSaved={p => { if (p) b.setProjectId(p.id); b.reloadProjects().catch(() => {}) }}
        />
      )}

      <div className="toasts" aria-live="polite">
        {!b.online && <div className="toast warn">Sem conexão com o servidor. Tentando reconectar…</div>}
        {b.error && <div className="toast err" role="alert">{b.error}<button onClick={() => b.setError('')} aria-label="Dispensar">×</button></div>}
        {b.notices.map(n => (
          <button key={n.id} className="toast note" data-kind={n.kind} onClick={() => { setOpenCard(n.cardId); b.dismissNotice(n.id) }}>
            <strong>{n.title}</strong><span>{n.body}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
