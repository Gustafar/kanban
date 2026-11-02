import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'

interface Props {
  trigger: (s: { open: boolean; toggle: () => void }) => ReactNode
  children: (close: () => void) => ReactNode
  up?: boolean // prefer opening above the trigger (for controls at the bottom of the screen)
  align?: 'left' | 'right'
  width?: number
}

const GAP = 8
const MARGIN = 10

// A small menu anchored to its trigger. It is positioned against the viewport, so scrolling or clipping
// containers (like the card's scrolling column) never cut it off; it flips above/below to fit.
// Closes on outside click, on scroll/resize, and on Esc (without closing whatever is behind it).
export function Popover({ trigger, children, up = false, align = 'left', width = 280 }: Props) {
  const [open, setOpen] = useState(false)
  const [style, setStyle] = useState<CSSProperties>({ visibility: 'hidden' })
  const ref = useRef<HTMLDivElement>(null)
  const panel = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (!open || !ref.current || !panel.current) return
    const r = ref.current.getBoundingClientRect()
    const wanted = panel.current.scrollHeight
    const below = window.innerHeight - r.bottom - GAP - MARGIN
    const above = r.top - GAP - MARGIN
    const goUp = up ? above >= Math.min(wanted, 220) || above > below : wanted > below && above > below
    const room = Math.max(160, goUp ? above : below)
    const height = Math.min(wanted, room)
    const w = Math.min(width, window.innerWidth - 2 * MARGIN)
    const left = Math.min(Math.max(align === 'right' ? r.right - w : r.left, MARGIN), window.innerWidth - w - MARGIN)
    setStyle({ position: 'fixed', left, width: w, maxHeight: room, top: goUp ? r.top - GAP - height : r.bottom + GAP, visibility: 'visible' })
  }, [open, up, align, width])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
    const onMove = (e: Event) => { if (!panel.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', onDown)
    window.addEventListener('scroll', onMove, true)
    window.addEventListener('resize', onMove)
    return () => {
      document.removeEventListener('mousedown', onDown)
      window.removeEventListener('scroll', onMove, true)
      window.removeEventListener('resize', onMove)
    }
  }, [open])

  const toggle = () => { setStyle({ visibility: 'hidden' }); setOpen(o => !o) }

  return (
    <div className="pop" ref={ref} onKeyDown={e => { if (e.key === 'Escape' && open) { e.stopPropagation(); setOpen(false) } }}>
      {trigger({ open, toggle })}
      {open && <div className="pop-panel" ref={panel} style={style} role="menu">{children(() => setOpen(false))}</div>}
    </div>
  )
}
