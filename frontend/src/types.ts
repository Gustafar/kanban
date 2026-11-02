export type ColumnId = 'backlog' | 'todo' | 'doing' | 'review' | 'done'
export type Mode = 'approval' | 'auto'
export type RunStatus = 'awaiting_approval' | 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'accepted' | 'discarded'
export interface ServerStatus { max: number; running: number; queued: number }

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
  tags: string[]; attachmentCount: number; titling?: boolean; polishing?: boolean; createdAt?: string; model?: string; effort?: string
  dependsOn: number
}
export interface Attachment { id: number; cardId: number; name: string; mime: string; size: number; createdAt: string }
export const isImage = (mime: string) => /^image\/(png|jpeg|gif|webp)/.test(mime)
export interface Comment { id: number; cardId: number; author: 'user' | 'ai'; body: string; runId: number; file: string; line: number; createdAt: string }
export interface Chat { id: number; projectId: number; title: string; model: string; effort: string; busy: boolean; createdAt: string; updatedAt: string }
export interface ChatAttachment { id: number; chatId: number; messageId: number; name: string; mime: string; size: number; createdAt: string }
export interface ChatMessage { id: number; chatId: number; role: 'user' | 'assistant' | 'tool' | 'error'; body: string; createdAt: string; attachments: ChatAttachment[] }
export interface Run {
  id: number; cardId: number; status: RunStatus; sessionId: string
  branch: string; worktreePath: string; prompt: string; error: string
  costUsd: number; inputTokens: number; outputTokens: number; cacheReadTokens: number; cacheCreationTokens: number
  startedAt: string | null; finishedAt: string | null; createdAt: string
}
export interface RunEvent { id: number; runId: number; kind: 'text' | 'tool' | 'result' | 'error' | 'system'; data: string; createdAt: string }
// kind: 'succeeded' | 'failed' | 'warn' from the server, 'error' for failed requests in the UI
export interface Notice { id: number; kind: string; title: string; body: string; cardId?: number; at: number; read: boolean }

export interface LimitWindow { utilization: number; resetsAt: number } // utilization 0..1, resetsAt unix seconds
export interface Limits { fiveHour: LimitWindow | null; sevenDay: LimitWindow | null; status: string; updatedAt: number }

export interface Usage { costUsd: number; inputTokens: number; outputTokens: number; cacheReadTokens: number; cacheCreationTokens: number; runs: number }
export interface ProjectUsage extends Usage { projectId: number; name: string; color: string }
export interface DailyUsage extends Usage { date: string }
export interface UsageReport { total: Usage; byProject: ProjectUsage[]; byDay: DailyUsage[] }
