import { useCallback, useEffect, useState } from 'react'
import { api } from './api'
import { useBoard } from './useBoard'
import type { ColumnId } from './types'
import { Board } from './components/Board'
import { CardDrawer } from './components/CardDrawer'
import { NewCardDialog, ProjectDialog } from './components/Dialogs'
import { Bell, Gear, Plus } from './components/Icons'

type Dialog = { kind: 'card'; column: ColumnId } | { kind: 'project'; edit: boolean } | null

const canNotify = typeof Notification !== 'undefined' && window.isSecureContext

export default function App() {
  const b = useBoard()
  const [openCard, setOpenCard] = useState<number | null>(null)
  const [dialog, setDialog] = useState<Dialog>(null)
  const [perm, setPerm] = useState(canNotify ? Notification.permission : 'denied')

  const project = b.projects.find(p => p.id === b.projectId)
  const card = b.cards.find(c => c.id === openCard)

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
            <Board cards={b.cards} live={b.live} onMove={move} onOpen={setOpenCard} onAdd={column => setDialog({ kind: 'card', column })} />
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
