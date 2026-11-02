export type ColumnId = 'backlog' | 'todo' | 'doing' | 'review' | 'done'
export type Mode = 'approval' | 'auto'
export type RunStatus = 'awaiting_approval' | 'running' | 'succeeded' | 'failed' | 'cancelled'

export const COLUMNS: { id: ColumnId; title: string; hint: string }[] = [
  { id: 'backlog', title: 'Backlog', hint: 'Ideias e tarefas soltas' },
  { id: 'todo', title: 'A fazer', hint: 'Prontas para a IA pegar' },
  { id: 'doing', title: 'Em execução', hint: 'Mover para cá dispara a IA' },
  { id: 'review', title: 'Revisão', hint: 'Confira o resultado' },
  { id: 'done', title: 'Concluído', hint: 'Já está no código' },
]

export type Isolation = 'worktree' | 'direct'
export interface Project { id: number; name: string; path: string; color: string; isolation: Isolation }
export interface Inspection { exists: boolean; isGit: boolean; hasCommit: boolean; hasGitignore: boolean }
export interface Card {
  id: number; projectId: number; title: string; description: string
  column: ColumnId; position: number; mode: Mode; runStatus: RunStatus | ''; commentCount: number
  tags: string[]; attachmentCount: number
}
export interface Attachment { id: number; cardId: number; name: string; mime: string; size: number; createdAt: string }
export const isImage = (mime: string) => /^image\/(png|jpeg|gif|webp)/.test(mime)
export interface Comment { id: number; cardId: number; author: 'user' | 'ai'; body: string; createdAt: string }
export interface Run {
  id: number; cardId: number; status: RunStatus; sessionId: string
  branch: string; worktreePath: string; prompt: string; error: string
  startedAt: string | null; finishedAt: string | null; createdAt: string
}
export interface RunEvent { id: number; runId: number; kind: 'text' | 'tool' | 'result' | 'error' | 'system'; data: string; createdAt: string }
export interface Notice { id: number; kind: string; title: string; body: string; cardId: number }
