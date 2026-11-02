import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from './api'
import { useBoard } from './useBoard'
import type { ColumnId } from './types'
import { Board } from './components/Board'
import { CardDrawer } from './components/CardDrawer'
import { ContextMenu } from './components/ContextMenu'
import type { MenuItem } from './components/ContextMenu'
import { useConfirm } from './components/ConfirmDialog'
import { Modal, NewCardDialog, ProjectDialog } from './components/Dialogs'
import { Bell, Gear, Logo, Plus, Search, SidebarToggle } from './components/Icons'
import { MiniLimits } from './components/LimitMeters'
import { QuickBar } from './components/QuickBar'
import { UsageDialog } from './components/UsageDialog'
import { COLUMNS } from './types'
import type { Card } from './types'

type Dialog = { kind: 'card'; column: ColumnId } | { kind: 'project'; edit: boolean } | { kind: 'help' } | { kind: 'usage' } | null
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
  const [menu, setMenu] = useState<{ card: Card; x: number; y: number } | null>(null)
  const confirm = useConfirm()
  const [collapsed, setCollapsed] = useState(() => {
    try { return localStorage.getItem('rail-collapsed') === '1' } catch { return false }
  })
  const toggleRail = () => setCollapsed(c => {
    try { localStorage.setItem('rail-collapsed', c ? '0' : '1') } catch { /* storage unavailable */ }
    return !c
  })

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
  const [updating, setUpdating] = useState(false)
  async function applyUpdate() {
    setUpdating(true)
    try { await api.applyUpdate() } catch (e) { setUpdating(false); setError((e as Error).message); return }
    // the server stays up while building and goes down only for the swap: reload once it is back
    const started = Date.now()
    let down = false
    const poll = setInterval(async () => {
      const ok = await fetch('/api/status').then(r => r.ok).catch(() => false)
      if (!ok) down = true
      if ((ok && down) || Date.now() - started > 5 * 60_000) { clearInterval(poll); location.reload() }
    }, 2000)
  }
  const onError = useCallback((m: string) => setError(m), [setError])

  function move(id: number, column: ColumnId, position: number) {
    b.setCards(l => l.map(c => (c.id === id ? { ...c, column, position } : c)))
    api.moveCard(id, column, position).catch(e => {
      onError(e.message)
      if (b.projectId != null) api.cards(b.projectId).then(b.setCards)
    })
  }

  async function removeCard(c: Card) {
    const running = c.runStatus === 'running' || c.runStatus === 'queued'
    const ok = await confirm({
      title: 'Excluir este card?',
      message: <>
        <strong>{c.title || 'Sem título'}</strong> será removido com comentários, anexos e histórico de execuções.
        {running && ' A execução em andamento será interrompida.'} Isso não pode ser desfeito.
      </>,
      confirmLabel: 'Excluir card',
      danger: true,
    })
    if (!ok) return
    try {
      await api.deleteCard(c.id)
      b.setCards(l => l.filter(x => x.id !== c.id))
      setOpenCard(o => (o === c.id ? null : o))
    } catch (e) { onError((e as Error).message) }
  }

  function menuItems(c: Card): MenuItem[] {
    const idle = !c.runStatus || ['failed', 'cancelled', 'discarded'].includes(c.runStatus)
    const items: MenuItem[] = [{ label: 'Abrir', onSelect: () => setOpenCard(c.id) }]
    if (idle) items.push({ label: c.runStatus ? 'Executar de novo' : 'Executar com IA', onSelect: () => api.runCard(c.id).catch(e => onError(e.message)) })
    COLUMNS.filter(col => col.id !== c.column).forEach((col, i) => {
      const last = Math.max(0, ...b.cards.filter(x => x.column === col.id).map(x => x.position))
      items.push({ label: col.title, group: i === 0 ? 'Mover para' : undefined, separator: i === 0, onSelect: () => move(c.id, col.id, last + 1024) })
    })
    items.push({ label: 'Excluir card', danger: true, separator: true, onSelect: () => removeCard(c) })
    return items
  }

  if (!b.loaded) return <div className="splash">Carregando…</div>

  return (
    <div className="app" data-collapsed={collapsed || undefined}>
      <nav className="rail" aria-label="Projetos">
        <div className="brand">
          <Logo /><span>Kanban</span>
          <button className="icon-btn rail-toggle" onClick={toggleRail} aria-label="Minimizar barra lateral" title="Minimizar barra lateral"><SidebarToggle /></button>
        </div>
        <ul>
          {b.projects.map((p, i) => (
            <li key={p.id}>
              <button className="proj" data-on={p.id === b.projectId || undefined} onClick={() => { b.setProjectId(p.id); setOpenCard(null) }}>
                <span className="avatar" style={{ background: p.color }} aria-hidden>{p.name.trim().charAt(0).toUpperCase()}</span>
                <span className="proj-name">{p.name}</span>
                {i < 9 && <kbd className="proj-key">{i + 1}</kbd>}
              </button>
            </li>
          ))}
        </ul>
        <button className="proj add" onClick={() => setDialog({ kind: 'project', edit: false })}><span className="avatar ghost" aria-hidden><Plus /></span><span className="proj-name">Novo projeto</span></button>

        <div className="rail-foot">
          <div className="ai-meter" data-busy={b.status.running > 0 || undefined}>
            <span className="pulse" aria-hidden />
            <div className="ai-meter-text">
              <strong>{b.status.running === 0 ? 'IA livre' : `IA em ${b.status.running} ${b.status.running === 1 ? 'tarefa' : 'tarefas'}`}</strong>
              <span>{b.status.queued > 0 ? `${b.status.queued} na fila` : `até ${b.status.max} ao mesmo tempo`}</span>
            </div>
            <div className="stepper" role="group" aria-label="Execuções simultâneas">
              <button onClick={() => api.setMaxParallel(b.status.max - 1).then(b.setStatus).catch(e => onError(e.message))} disabled={b.status.max <= 1} aria-label="Diminuir limite">−</button>
              <output aria-label="Limite de execuções simultâneas">{b.status.max}</output>
              <button onClick={() => api.setMaxParallel(b.status.max + 1).then(b.setStatus).catch(e => onError(e.message))} disabled={b.status.max >= 8} aria-label="Aumentar limite">+</button>
            </div>
          </div>
          <MiniLimits limits={b.limits} onOpen={() => setDialog({ kind: 'usage' })} />
          <div className="conn" data-off={!b.online || undefined}><i />{b.online ? 'Conectado' : 'Reconectando…'}</div>
        </div>
      </nav>

      <main className="main">
        {project ? (
          <>
            <header className="topbar">
              {collapsed && <button className="icon-btn rail-expand" onClick={toggleRail} aria-label="Mostrar barra lateral" title="Mostrar barra lateral"><SidebarToggle /></button>}
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
            {b.cards.length === 0 ? (
              <div className="welcome">
                <Logo />
                <h2>Nenhuma tarefa em {project.name}</h2>
                <p>Escreva o que você quer fazer na barra abaixo. A IA dá o título e as tags, e você decide quando ela executa.</p>
                <span className="welcome-hint"><kbd>n</kbd> para começar a escrever</span>
              </div>
            ) : (
              <Board cards={visible} live={b.live} onMove={move} onOpen={setOpenCard} onAdd={column => setDialog({ kind: 'card', column })} onMenu={(card, x, y) => setMenu({ card, x, y })} />
            )}
            <QuickBar key={project.id} ref={quickRef} project={project} onError={onError} />
          </>
        ) : (
          <div className="empty">
            <h1>Comece por um projeto</h1>
            <p>Cada projeto aponta para uma pasta do seu computador. É lá que a IA vai trabalhar quando você mandar um card para execução.</p>
            <button className="btn primary" onClick={() => setDialog({ kind: 'project', edit: false })}><Plus />Criar projeto</button>
          </div>
        )}
      </main>

      {card && project && <CardDrawer key={card.id} card={card} project={project} subscribe={b.subscribe} onClose={() => setOpenCard(null)} onError={onError} />}

      {dialog?.kind === 'card' && project && <NewCardDialog project={project} column={dialog.column} onClose={() => setDialog(null)} onError={onError} />}
      {dialog?.kind === 'usage' && <UsageDialog limits={b.limits} onLimits={b.setLimits} onClose={() => setDialog(null)} onError={onError} />}
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

      {menu && <ContextMenu x={menu.x} y={menu.y} items={menuItems(menu.card)} onClose={() => setMenu(null)} />}

      <div className="toasts" aria-live="polite">
        {!b.online && <div className="toast warn">Sem conexão com o servidor. Tentando reconectar…</div>}
        {b.error && <div className="toast err" role="alert">{b.error}<button onClick={() => b.setError('')} aria-label="Dispensar">×</button></div>}
        {b.update && (
          <div className="toast update" role="status">
            <strong>Novas atualizações</strong>
            <span>{updating ? 'Atualizando… o app reinicia em instantes.' : `Do card “${b.update.title || 'sem título'}”.`}</span>
            <button onClick={applyUpdate} disabled={updating}>{updating ? 'Atualizando…' : 'Atualizar'}</button>
          </div>
        )}
        {b.notices.map(n => (
          <button key={n.id} className="toast note" data-kind={n.kind} onClick={() => { setOpenCard(n.cardId); b.dismissNotice(n.id) }}>
            <strong>{n.title}</strong><span>{n.body}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
