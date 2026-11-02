import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './api'
import type { Card, Limits, Notice, Project, RunEvent, ServerStatus } from './types'

type Live = Record<number, string> // cardId -> last activity line

function upsert<T extends { id: number }>(list: T[], item: T): T[] {
  return list.some(x => x.id === item.id) ? list.map(x => (x.id === item.id ? item : x)) : [...list, item]
}

export type BoardListener = (type: string, data: any) => void

// One SSE connection feeds projects, the open project's cards, live lines, runs and notices.
export function useBoard() {
  const [projects, setProjects] = useState<Project[]>([])
  const [projectId, setProjectId] = useState<number | null>(null)
  const [cards, setCards] = useState<Card[]>([])
  const [live, setLive] = useState<Live>({})
  const [notices, setNotices] = useState<Notice[]>([])
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [online, setOnline] = useState(true)
  const [status, setStatus] = useState<ServerStatus>({ max: 2, running: 0, queued: 0 })
  const [limits, setLimits] = useState<Limits | null>(null)
  const [update, setUpdate] = useState<{ cardId: number; title: string } | null>(null)

  const projectIdRef = useRef<number | null>(null)
  projectIdRef.current = projectId
  const runToCard = useRef(new Map<number, number>())
  const listeners = useRef(new Set<BoardListener>())
  const noticeSeq = useRef(0)

  const loadProjects = useCallback(async () => {
    const ps = await api.projects()
    setProjects(ps)
    // `cardume` opens the board with ?project=<id>; honor it once, then clean the URL
    const url = new URL(location.href)
    const wanted = Number(url.searchParams.get('project'))
    if (url.searchParams.has('project')) {
      url.searchParams.delete('project')
      history.replaceState(null, '', url)
    }
    setProjectId(cur => {
      if (wanted && ps.some(p => p.id === wanted)) return wanted
      return cur && ps.some(p => p.id === cur) ? cur : (ps[0]?.id ?? null)
    })
    setLoaded(true)
  }, [])

  const loadCards = useCallback(async (pid: number) => {
    setCards(await api.cards(pid))
  }, [])

  useEffect(() => { loadProjects().catch(e => { setError(e.message); setLoaded(true) }) }, [loadProjects])
  useEffect(() => {
    if (projectId == null) { setCards([]); return }
    loadCards(projectId).catch(e => setError(e.message))
  }, [projectId, loadCards])

  useEffect(() => {
    const es = new EventSource('/api/events')
    es.onopen = () => {
      setOnline(true)
      // events missed while offline: resync
      api.status().then(setStatus).catch(() => {})
      api.limits().then(setLimits).catch(() => {})
      api.pendingUpdate().then(r => setUpdate(r.pending)).catch(() => {})
      if (projectIdRef.current != null) loadCards(projectIdRef.current).catch(() => {})
    }
    es.onerror = () => setOnline(false)
    es.onmessage = m => {
      const { type, data } = JSON.parse(m.data)
      switch (type) {
        case 'project': setProjects(l => upsert(l, data).sort((a, b) => a.name.localeCompare(b.name))); break
        case 'project_deleted': loadProjects().catch(() => {}); break
        case 'card':
          if (data.projectId === projectIdRef.current) setCards(l => upsert(l, data))
          break
        case 'card_deleted': setCards(l => l.filter(c => c.id !== data.id)); break
        case 'run':
          runToCard.current.set(data.id, data.cardId)
          if (data.status !== 'running') setLive(l => { const n = { ...l }; delete n[data.cardId]; return n })
          break
        case 'run_event': {
          const ev = data as RunEvent
          const cid = runToCard.current.get(ev.runId)
          if (cid != null && (ev.kind === 'text' || ev.kind === 'tool')) {
            setLive(l => ({ ...l, [cid]: ev.data.split('\n')[0].slice(0, 120) }))
          }
          break
        }
        case 'status': setStatus(data); break
        case 'limits': setLimits(data); break
        case 'update': setUpdate(data.pending); break
        case 'notify': {
          const n = { ...data, id: ++noticeSeq.current } as Notice
          setNotices(l => [...l, n])
          setTimeout(() => setNotices(l => l.filter(x => x.id !== n.id)), 8000)
          break
        }
      }
      listeners.current.forEach(fn => fn(type, data))
    }
    return () => es.close()
  }, [loadCards, loadProjects])

  // keep the sidebar usage meters current without the user opening the usage panel
  useEffect(() => {
    const t = setInterval(() => { api.refreshLimits().then(setLimits).catch(() => {}) }, 60_000)
    return () => clearInterval(t)
  }, [])

  const subscribe = useCallback((fn: BoardListener) => {
    listeners.current.add(fn)
    return () => { listeners.current.delete(fn) }
  }, [])

  const dismissNotice = (id: number) => setNotices(l => l.filter(n => n.id !== id))

  return {
    update,
    projects, projectId, setProjectId, cards, setCards, live, notices, dismissNotice,
    error, setError, loaded, online, status, setStatus, limits, setLimits, subscribe, reloadProjects: loadProjects,
  }
}
