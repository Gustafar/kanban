import type { Attachment, Card, ColumnId, Comment, Inspection, Mode, Project, Run, RunEvent, ServerStatus } from './types'

type ProjectInput = Pick<Project, 'name' | 'path' | 'color' | 'isolation'>

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch('/api' + path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `Erro ${res.status}`)
  return data as T
}

export const api = {
  projects: () => req<Project[]>('GET', '/projects'),
  inspect: (path: string) => req<Inspection>('GET', `/inspect?path=${encodeURIComponent(path)}`),
  gitInit: (path: string) => req<Inspection>('POST', '/git/init', { path }),
  createProject: (p: ProjectInput) => req<Project>('POST', '/projects', p),
  updateProject: (id: number, p: ProjectInput) => req<Project>('PUT', `/projects/${id}`, p),
  deleteProject: (id: number) => req<void>('DELETE', `/projects/${id}`),
  cards: (pid: number) => req<Card[]>('GET', `/projects/${pid}/cards`),
  createCard: (pid: number, c: { title: string; description: string; column: ColumnId; mode: Mode }) =>
    req<Card>('POST', `/projects/${pid}/cards`, c),
  quick: (pid: number, c: { text: string; column: ColumnId; mode: Mode }) =>
    req<Card>('POST', `/projects/${pid}/quick`, { description: c.text, column: c.column, mode: c.mode }),
  updateCard: (id: number, c: { title: string; description: string; mode: Mode; tags?: string[] }) => req<Card>('PUT', `/cards/${id}`, c),
  attachments: (cardId: number) => req<Attachment[]>('GET', `/cards/${cardId}/attachments`),
  uploadAttachment: async (cardId: number, file: File) => {
    const res = await fetch(`/api/cards/${cardId}/attachments?name=${encodeURIComponent(file.name || 'colado.png')}`, {
      method: 'POST', headers: { 'Content-Type': file.type || 'application/octet-stream' }, body: file,
    })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(data.error || `Erro ${res.status}`)
    return data as Attachment
  },
  deleteAttachment: (id: number) => req<void>('DELETE', `/attachments/${id}`),
  moveCard: (id: number, column: ColumnId, position: number) => req<Card>('POST', `/cards/${id}/move`, { column, position }),
  deleteCard: (id: number) => req<void>('DELETE', `/cards/${id}`),
  runCard: (id: number, message = '') => req<Run>('POST', `/cards/${id}/run`, { message }),
  runs: (cardId: number) => req<Run[]>('GET', `/cards/${cardId}/runs`),
  comments: (cardId: number) => req<Comment[]>('GET', `/cards/${cardId}/comments`),
  addComment: (cardId: number, body: string) => req<Comment>('POST', `/cards/${cardId}/comments`, { body }),
  events: (runId: number) => req<RunEvent[]>('GET', `/runs/${runId}/events`),
  status: () => req<ServerStatus>('GET', '/status'),
  setMaxParallel: (n: number) => req<ServerStatus>('PUT', '/settings', { maxParallel: n }),
  approve: (runId: number) => req<void>('POST', `/runs/${runId}/approve`),
  cancel: (runId: number) => req<void>('POST', `/runs/${runId}/cancel`),
  diff: (runId: number) => req<{ diff: string }>('GET', `/runs/${runId}/diff`),
  merge: (runId: number) => req<void>('POST', `/runs/${runId}/merge`),
  discard: (runId: number) => req<void>('POST', `/runs/${runId}/discard`),
}
