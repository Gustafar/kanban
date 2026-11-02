import { useEffect } from 'react'
import type { ReactNode } from 'react'
import type { Notice } from '../types'
import { Bell, Close } from './Icons'
import { Popover } from './Popover'

interface Props {
  notices: Notice[]
  online: boolean
  update: { cardId: number; title: string; error?: string } | null
  updating: boolean
  onApplyUpdate: () => void
  onOpenCard: (id: number) => void
  onDismiss: (id: number) => void
  onClear: () => void
  onRead: () => void
}

function ago(at: number) {
  const s = Math.max(0, Math.round((Date.now() - at) / 1000))
  if (s < 60) return 'agora'
  if (s < 3600) return `${Math.floor(s / 60)} min`
  if (s < 86400) return `${Math.floor(s / 3600)} h`
  return new Date(at).toLocaleDateString('pt-BR')
}

// Marks everything as read when the panel closes, so the "new" markers survive while it is open.
function Panel({ onRead, children }: { onRead: () => void; children: ReactNode }) {
  useEffect(() => onRead, [onRead])
  return <>{children}</>
}

export function NotificationCenter({ notices, online, update, updating, onApplyUpdate, onOpenCard, onDismiss, onClear, onRead }: Props) {
  const unread = notices.filter(n => !n.read).length + (update ? 1 : 0) + (online ? 0 : 1)
  const empty = notices.length === 0 && !update && online

  return (
    <Popover
      align="right" width={380}
      trigger={({ open, toggle }) => (
        <button className="icon-btn bell-btn" onClick={toggle} aria-expanded={open} aria-label={unread ? `Notificações (${unread} novas)` : 'Notificações'} title="Notificações">
          <Bell />
          {unread > 0 && <span className="bell-count">{unread > 9 ? '9+' : unread}</span>}
        </button>
      )}
    >
      {close => (
        <Panel onRead={onRead}>
          <div className="inbox-head">
            <strong>Notificações</strong>
            {notices.length > 0 && <button className="inbox-clear" onClick={onClear}>Limpar tudo</button>}
          </div>
          {empty && <p className="inbox-empty">Nada por aqui. Avisos de execuções e erros aparecem nesta lista.</p>}
          {!online && (
            <div className="inbox-item" data-kind="warn" data-pinned>
              <div className="inbox-text"><b>Sem conexão com o servidor</b><span>Tentando reconectar…</span></div>
            </div>
          )}
          {update && (
            <div className="inbox-item" data-kind={updating || !update.error ? 'update' : 'warn'} data-pinned>
              <div className="inbox-text">
                <b>{update.error && !updating ? 'Falha ao atualizar' : 'Novas atualizações'}</b>
                <span>
                  {updating
                    ? 'Atualizando… o app reinicia em instantes.'
                    : update.error || `Do card “${update.title || 'sem título'}”.`}
                </span>
                <button className="inbox-action" onClick={onApplyUpdate} disabled={updating}>
                  {updating ? 'Atualizando…' : update.error ? 'Tentar novamente' : 'Atualizar'}
                </button>
              </div>
            </div>
          )}
          {notices.map(n => (
            <div key={n.id} className="inbox-item" data-kind={n.kind} data-unread={!n.read || undefined}>
              <button
                className="inbox-text" disabled={n.cardId == null}
                onClick={() => { if (n.cardId != null) { onOpenCard(n.cardId); close() } }}
              >
                <b>{n.title}</b>
                <span>{n.body}</span>
                <small>{ago(n.at)}</small>
              </button>
              <button className="icon-btn inbox-x" onClick={() => onDismiss(n.id)} aria-label="Dispensar"><Close /></button>
            </div>
          ))}
        </Panel>
      )}
    </Popover>
  )
}
