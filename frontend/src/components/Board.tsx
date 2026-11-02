import { useMemo, useState } from 'react'
import {
  DndContext, DragOverlay, MouseSensor, TouchSensor, closestCorners, useDroppable, useSensor, useSensors,
} from '@dnd-kit/core'
import type { DragEndEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { COLUMNS } from '../types'
import type { Card, ColumnId } from '../types'
import { CardView } from './CardView'

interface Props {
  cards: Card[]
  live: Record<number, string>
  onMove: (id: number, column: ColumnId, position: number) => void
  onOpen: (id: number) => void
  onAdd: (column: ColumnId) => void
}

function SortableCard({ card, live, onOpen }: { card: Card; live?: string; onOpen: (id: number) => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: card.id })
  return (
    <CardView
      ref={setNodeRef}
      card={card}
      liveLine={live}
      style={{ transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.35 : 1 }}
      onClick={() => onOpen(card.id)}
      onKeyDown={e => { if (e.key === 'Enter') onOpen(card.id) }}
      {...attributes}
      {...listeners}
    />
  )
}

function Column({ id, title, hint, cards, live, onOpen, onAdd }: {
  id: ColumnId; title: string; hint: string; cards: Card[]; live: Record<number, string>
  onOpen: (id: number) => void; onAdd: (column: ColumnId) => void
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${id}` })
  return (
    <section className="column" data-over={isOver || undefined} data-col={id} aria-label={title}>
      <header className="column-head">
        <i className="col-dot" aria-hidden />
        <h2>{title}</h2>
        <span className="count">{cards.length}</span>
        <button className="icon-btn" onClick={() => onAdd(id)} aria-label={`Novo card em ${title}`}>+</button>
      </header>
      <div ref={setNodeRef} className="column-body">
        <SortableContext items={cards.map(c => c.id)} strategy={verticalListSortingStrategy}>
          {cards.map(c => <SortableCard key={c.id} card={c} live={live[c.id]} onOpen={onOpen} />)}
        </SortableContext>
        {cards.length === 0 && <p className="column-empty">{hint}</p>}
      </div>
    </section>
  )
}

// Position between neighbours so a move touches a single row.
function positionBetween(prev?: Card, next?: Card): number {
  if (prev && next) return (prev.position + next.position) / 2
  if (prev) return prev.position + 1024
  if (next) return next.position / 2
  return 1024
}

export function Board({ cards, live, onMove, onOpen, onAdd }: Props) {
  const [activeId, setActiveId] = useState<number | null>(null)
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    // long-press on touch so horizontal/vertical scrolling still works
    useSensor(TouchSensor, { activationConstraint: { delay: 220, tolerance: 8 } }),
  )

  const byColumn = useMemo(() => {
    const map = Object.fromEntries(COLUMNS.map(c => [c.id, [] as Card[]])) as Record<ColumnId, Card[]>
    for (const c of [...cards].sort((a, b) => a.position - b.position)) map[c.column]?.push(c)
    return map
  }, [cards])

  const active = cards.find(c => c.id === activeId)

  function onDragStart(e: DragStartEvent) { setActiveId(Number(e.active.id)) }

  function onDragEnd(e: DragEndEvent) {
    setActiveId(null)
    const { active, over } = e
    if (!over) return
    const id = Number(active.id)
    const card = cards.find(c => c.id === id)
    if (!card) return

    let target: ColumnId
    let index: number // insertion index within the target column, without the dragged card
    const overId = String(over.id)
    if (overId.startsWith('col:')) {
      target = overId.slice(4) as ColumnId
      index = byColumn[target].filter(c => c.id !== id).length
    } else {
      const overCard = cards.find(c => c.id === Number(over.id))
      if (!overCard) return
      target = overCard.column
      const list = byColumn[target].filter(c => c.id !== id)
      const overIdx = list.findIndex(c => c.id === overCard.id)
      const from = byColumn[target].findIndex(c => c.id === id)
      const to = byColumn[target].findIndex(c => c.id === overCard.id)
      // dragging downwards inside a column lands after the card we're over
      index = from !== -1 && from < to ? overIdx + 1 : overIdx
    }
    const list = byColumn[target].filter(c => c.id !== id)
    const position = positionBetween(list[index - 1], list[index])
    if (target === card.column && position === card.position) return
    onMove(id, target, position)
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={onDragStart} onDragEnd={onDragEnd} onDragCancel={() => setActiveId(null)}>
      <div className="board">
        {COLUMNS.map(col => (
          <Column key={col.id} {...col} cards={byColumn[col.id]} live={live} onOpen={onOpen} onAdd={onAdd} />
        ))}
      </div>
      <DragOverlay dropAnimation={{ duration: 160 }}>
        {active ? <CardView card={active} liveLine={live[active.id]} dragging /> : null}
      </DragOverlay>
    </DndContext>
  )
}
