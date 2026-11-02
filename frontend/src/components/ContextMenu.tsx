import { useEffect, useLayoutEffect, useRef, useState } from 'react'

export interface MenuItem {
  label: string
  onSelect: () => void
  danger?: boolean
  disabled?: boolean
  separator?: boolean // draws a divider above the item
  group?: string // small heading above the item
}

export function ContextMenu({ x, y, items, onClose }: { x: number; y: number; items: MenuItem[]; onClose: () => void }) {
  const ref = useRef<HTMLUListElement>(null)
  const [pos, setPos] = useState({ x, y })

  // keep the menu inside the viewport
  useLayoutEffect(() => {
    const r = ref.current?.getBoundingClientRect()
    if (!r) return
    setPos({ x: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)), y: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)) })
  }, [x, y])

  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
    const down = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) onClose() }
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.preventDefault(); onClose(); return }
      if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
      e.preventDefault()
      const btns = [...ref.current!.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
      const i = btns.indexOf(document.activeElement as HTMLButtonElement)
      btns[(i + (e.key === 'ArrowDown' ? 1 : -1) + btns.length) % btns.length]?.focus()
    }
    window.addEventListener('mousedown', down)
    window.addEventListener('keydown', key)
    window.addEventListener('blur', onClose)
    window.addEventListener('resize', onClose)
    window.addEventListener('wheel', onClose, { passive: true })
    return () => {
      window.removeEventListener('mousedown', down)
      window.removeEventListener('keydown', key)
      window.removeEventListener('blur', onClose)
      window.removeEventListener('resize', onClose)
      window.removeEventListener('wheel', onClose)
    }
  }, [onClose])

  return (
    <ul ref={ref} className="ctx-menu" role="menu" style={{ left: pos.x, top: pos.y }} onContextMenu={e => e.preventDefault()}>
      {items.map(it => (
        <li key={it.label} role="none" data-sep={it.separator || undefined}>
          {it.group && <span className="ctx-group">{it.group}</span>}
          <button role="menuitem" data-danger={it.danger || undefined} disabled={it.disabled} onClick={() => { onClose(); it.onSelect() }}>{it.label}</button>
        </li>
      ))}
    </ul>
  )
}
