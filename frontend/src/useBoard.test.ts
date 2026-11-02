import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { installMockEventSource, MockEventSource } from './test/mockEventSource'
import type { Card, Project } from './types'

vi.mock('./api', () => ({
  api: {
    projects: vi.fn(),
    cards: vi.fn(),
    status: vi.fn(),
    limits: vi.fn(),
    pendingUpdate: vi.fn(),
    refreshLimits: vi.fn(),
  },
}))

import { api } from './api'
import { useBoard } from './useBoard'

const project = (id: number, name: string): Project => ({ id, name, path: `C:\\p${id}`, color: '#000', isolation: 'worktree' })
const card = (id: number, projectId: number, overrides: Partial<Card> = {}): Card => ({
  id, projectId, title: `Card ${id}`, description: '', column: 'backlog', position: id * 1024,
  mode: 'approval', runStatus: '', commentCount: 0, tags: [], attachmentCount: 0, dependsOn: 0, ...overrides,
})

beforeEach(() => {
  installMockEventSource()
  vi.mocked(api.projects).mockResolvedValue([project(1, 'Alpha'), project(2, 'Beta')])
  vi.mocked(api.cards).mockResolvedValue([])
  vi.mocked(api.status).mockResolvedValue({ max: 2, running: 0, queued: 0 })
  vi.mocked(api.limits).mockResolvedValue({ fiveHour: null, sevenDay: null, status: 'ok', updatedAt: 0 })
  vi.mocked(api.pendingUpdate).mockResolvedValue({ pending: null })
  vi.mocked(api.refreshLimits).mockResolvedValue({ fiveHour: null, sevenDay: null, status: 'ok', updatedAt: 0 })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('useBoard', () => {
  it('loads projects, picks the first one, and loads its cards', async () => {
    vi.mocked(api.cards).mockResolvedValue([card(1, 1)])
    const { result } = renderHook(() => useBoard())

    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(result.current.projectId).toBe(1)
    await waitFor(() => expect(result.current.cards).toHaveLength(1))
    expect(api.cards).toHaveBeenCalledWith(1)
  })

  it('reloads cards when the selected project changes', async () => {
    vi.mocked(api.cards).mockImplementation(async (pid: number) => (pid === 2 ? [card(2, 2)] : []))
    const { result } = renderHook(() => useBoard())
    await waitFor(() => expect(result.current.loaded).toBe(true))

    act(() => result.current.setProjectId(2))

    await waitFor(() => expect(result.current.cards).toHaveLength(1))
    expect(result.current.cards[0].projectId).toBe(2)
  })

  it('applies a "card" SSE event only when it belongs to the open project', async () => {
    const { result } = renderHook(() => useBoard())
    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(result.current.projectId).toBe(1)

    const es = MockEventSource.instances[0]
    act(() => es.emit('card', card(5, 1, { title: 'Moved card' })))
    await waitFor(() => expect(result.current.cards.some(c => c.id === 5)).toBe(true))

    act(() => es.emit('card', card(9, 2)))
    expect(result.current.cards.some(c => c.id === 9)).toBe(false)
  })

  it('updates an existing card in place on a "card" event with the same id', async () => {
    vi.mocked(api.cards).mockResolvedValue([card(1, 1, { column: 'todo' })])
    const { result } = renderHook(() => useBoard())
    await waitFor(() => expect(result.current.cards).toHaveLength(1))

    const es = MockEventSource.instances[0]
    act(() => es.emit('card', card(1, 1, { column: 'doing' })))

    await waitFor(() => expect(result.current.cards[0].column).toBe('doing'))
    expect(result.current.cards).toHaveLength(1)
  })

  it('removes a card on "card_deleted"', async () => {
    vi.mocked(api.cards).mockResolvedValue([card(1, 1), card(2, 1)])
    const { result } = renderHook(() => useBoard())
    await waitFor(() => expect(result.current.cards).toHaveLength(2))

    const es = MockEventSource.instances[0]
    act(() => es.emit('card_deleted', { id: 1 }))

    await waitFor(() => expect(result.current.cards).toHaveLength(1))
    expect(result.current.cards[0].id).toBe(2)
  })

  it('turns a "notify" SSE event into a notice', async () => {
    const { result } = renderHook(() => useBoard())
    await waitFor(() => expect(result.current.loaded).toBe(true))

    const es = MockEventSource.instances[0]
    act(() => es.emit('notify', { kind: 'succeeded', title: 'Pronto', body: 'Card concluído', cardId: 1 }))

    await waitFor(() => expect(result.current.notices).toHaveLength(1))
    expect(result.current.notices[0].title).toBe('Pronto')
  })

  it('marks the board offline on SSE error and back online on open', async () => {
    const { result } = renderHook(() => useBoard())
    await waitFor(() => expect(result.current.loaded).toBe(true))
    const es = MockEventSource.instances[0]

    act(() => es.onerror?.())
    expect(result.current.online).toBe(false)

    act(() => es.onopen?.())
    await waitFor(() => expect(result.current.online).toBe(true))
  })
})
