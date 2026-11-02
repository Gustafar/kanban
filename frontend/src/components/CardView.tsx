import { forwardRef } from 'react'
import type { CSSProperties, HTMLAttributes } from 'react'
import type { Card } from '../types'
import { Bolt, Hand } from './Icons'

export const STATUS_LABEL: Record<string, string> = {
  awaiting_approval: 'Aguardando sua aprovação',
  running: 'Trabalhando…',
  succeeded: 'Pronto para revisão',
  failed: 'Falhou',
  cancelled: 'Cancelado',
}

interface Props extends HTMLAttributes<HTMLDivElement> {
  card: Card
  liveLine?: string
  dragging?: boolean
}

// Presentational card; drag wiring is passed in through the HTML props.
export const CardView = forwardRef<HTMLDivElement, Props>(function CardView({ card, liveLine, dragging, style, ...rest }, ref) {
  const status = card.runStatus
  const showStatus = status && !(status === 'succeeded' && card.column === 'done')
  return (
    <div
      ref={ref}
      className="card"
      data-status={status || undefined}
      data-dragging={dragging || undefined}
      style={style as CSSProperties}
      role="button"
      tabIndex={0}
      {...rest}
    >
      <div className="card-title">{card.title}</div>
      {card.description && <p className="card-desc">{card.description}</p>}
      <div className="card-foot">
        <span className="card-mode" title={card.mode === 'auto' ? 'Roda sozinha ao entrar em execução' : 'Pede sua aprovação antes de rodar'}>
          {card.mode === 'auto' ? <Bolt /> : <Hand />}
          {card.mode === 'auto' ? 'Automático' : 'Com aprovação'}
        </span>
      </div>
      {showStatus && (
        <div className="card-status">
          <span className="status-label">{STATUS_LABEL[status]}</span>
          {status === 'running' && liveLine && <code className="live-line">{liveLine}</code>}
        </div>
      )}
    </div>
  )
})
