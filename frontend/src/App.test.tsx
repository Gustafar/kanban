import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { installMockEventSource } from './test/mockEventSource'
import type { Card, Project } from './types'

vi.mock('./api', () => ({
  api: {
    projects: vi.fn(),
    cards: vi.fn(),
    status: vi.fn(),
    limits: vi.fn(),
    pendingUpdate: vi.fn(),
    refreshLimits: vi.fn(),
    moveCard: vi.fn(),
    runCard: vi.fn(),
    deleteCard: vi.fn(),
    setMaxParallel: vi.fn(),
  },
}))

import { api } from './api'
import App from './App'

const project = (id: number, name: string): Project => ({ id, name, path: `C:\\p${id}`, color: '#000', isolation: 'worktree' })
const makeCard = (id: number, overrides: Partial<Card> = {}): Card => ({
  id, projectId: 1, title: `Card ${id}`, description: '', column: 'backlog', position: id * 1024,
  mode: 'approval', runStatus: '', commentCount: 0, tags: [], attachmentCount: 0, dependsOn: 0, ...overrides,
})

beforeEach(() => {
  installMockEventSource()
  vi.mocked(api.projects).mockResolvedValue([project(1, 'Alpha')])
  vi.mocked(api.cards).mockResolvedValue([makeCard(1, { title: 'Minha tarefa' })])
  vi.mocked(api.status).mockResolvedValue({ max: 2, running: 0, queued: 0 })
  vi.mocked(api.limits).mockResolvedValue({ fiveHour: null, sevenDay: null, status: 'ok', updatedAt: 0 })
  vi.mocked(api.pendingUpdate).mockResolvedValue({ pending: null })
  vi.mocked(api.refreshLimits).mockResolvedValue({ fiveHour: null, sevenDay: null, status: 'ok', updatedAt: 0 })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('moving a card between columns', () => {
  it('moves a card to another column via the context menu and calls the API optimistically', async () => {
    vi.mocked(api.moveCard).mockResolvedValue(makeCard(1, { column: 'todo' }))
    render(<App />)

    const cardEl = await screen.findByText('Minha tarefa')
    fireEvent.contextMenu(cardEl)

    const menu = await screen.findByRole('menu')
    const target = within(menu).getByRole('menuitem', { name: 'A fazer' })
    fireEvent.click(target)

    await waitFor(() => expect(api.moveCard).toHaveBeenCalledTimes(1))
    const [id, column] = vi.mocked(api.moveCard).mock.calls[0]
    expect(id).toBe(1)
    expect(column).toBe('todo')

    // optimistic update: the card now renders under "A fazer" without waiting for the API response
    await waitFor(() => {
      const columnEl = document.querySelector('[data-col="todo"]')
      expect(columnEl).toContainElement(screen.getByText('Minha tarefa'))
    })
  })

  it('reverts the optimistic move and surfaces an error when the API call fails', async () => {
    vi.mocked(api.moveCard).mockRejectedValue(new Error('Falha ao mover'))
    render(<App />)

    const cardEl = await screen.findByText('Minha tarefa')
    fireEvent.contextMenu(cardEl)
    const menu = await screen.findByRole('menu')
    fireEvent.click(within(menu).getByRole('menuitem', { name: 'A fazer' }))

    await waitFor(() => expect(api.moveCard).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(api.cards).toHaveBeenCalledTimes(2)) // initial load + resync after failure

    fireEvent.click(screen.getByRole('button', { name: /Notificações/ }))
    await screen.findByText('Falha ao mover')
  })
})
