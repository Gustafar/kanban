import { useEffect, useRef, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { api } from '../api'
import { COLUMNS } from '../types'
import type { ColumnId, Inspection, Isolation, Mode, Project } from '../types'

export const SWATCHES = ['#3355FF', '#23915F', '#D9930D', '#D2425A', '#8A4FFF', '#0F9CB8', '#E2662B', '#5A6678']

export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => { ref.current?.showModal() }, [])
  return (
    <dialog ref={ref} className="modal" onClose={onClose} onClick={e => { if (e.target === ref.current) onClose() }}>
      <div className="modal-inner">
        <h2 className="modal-title">{title}</h2>
        {children}
      </div>
    </dialog>
  )
}

export function ModeToggle({ value, onChange }: { value: Mode; onChange: (m: Mode) => void }) {
  return (
    <div className="segmented" role="radiogroup" aria-label="Modo de execução">
      {([['approval', 'Pedir aprovação', 'A IA espera você aprovar antes de rodar'], ['auto', 'Rodar sozinha', 'A IA começa assim que o card entra em execução']] as const).map(([m, label, hint]) => (
        <button key={m} type="button" role="radio" aria-checked={value === m} data-on={value === m || undefined} onClick={() => onChange(m)} title={hint}>
          {label}
        </button>
      ))}
    </div>
  )
}

export function NewCardDialog({ project, column, onClose, onError }: {
  project: Project; column: ColumnId; onClose: () => void; onError: (m: string) => void
}) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [mode, setMode] = useState<Mode>('approval')
  const [col, setCol] = useState<ColumnId>(column)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!title.trim()) return
    setBusy(true)
    try {
      await api.createCard(project.id, { title, description, column: col, mode })
      onClose()
    } catch (err) {
      onError((err as Error).message)
      setBusy(false)
    }
  }

  return (
    <Modal title="Novo card" onClose={onClose}>
      <form onSubmit={submit} className="form">
        <label>Título
          <input autoFocus value={title} onChange={e => setTitle(e.target.value)} placeholder="Ex.: Adicionar filtro por data na listagem" />
        </label>
        <label>O que a IA deve fazer
          <textarea rows={5} value={description} onChange={e => setDescription(e.target.value)} placeholder="Descreva o resultado esperado, arquivos envolvidos e o que não pode mudar." />
        </label>
        <label>Coluna
          <select value={col} onChange={e => setCol(e.target.value as ColumnId)}>
            {COLUMNS.map(c => <option key={c.id} value={c.id}>{c.title}</option>)}
          </select>
        </label>
        <div className="field">Execução<ModeToggle value={mode} onChange={setMode} /></div>
        <div className="actions">
          <button type="button" className="btn ghost" onClick={onClose}>Cancelar</button>
          <button className="btn primary" disabled={busy || !title.trim()}>Criar card</button>
        </div>
      </form>
    </Modal>
  )
}

export function ProjectDialog({ project, onClose, onSaved, onError }: {
  project?: Project; onClose: () => void; onSaved: (p: Project | null) => void; onError: (m: string) => void
}) {
  const [name, setName] = useState(project?.name ?? '')
  const [path, setPath] = useState(project?.path ?? '')
  const [color, setColor] = useState(project?.color ?? SWATCHES[0])
  const [isolation, setIsolation] = useState<Isolation>(project?.isolation ?? 'direct')
  const [touched, setTouched] = useState(!!project) // once the user picks a mode, stop auto-detecting
  const [info, setInfo] = useState<Inspection | null>(null)
  const [busy, setBusy] = useState(false)

  // look at the folder as the user types: git repo -> isolated copy, plain folder -> edit directly
  useEffect(() => {
    const p = path.trim()
    if (!p) { setInfo(null); return }
    const t = setTimeout(() => {
      api.inspect(p).then(i => {
        setInfo(i)
        if (!touched && i.exists) setIsolation(i.isGit && i.hasCommit ? 'worktree' : 'direct')
      }).catch(() => setInfo(null))
    }, 300)
    return () => clearTimeout(t)
  }, [path, touched])

  const gitReady = !!info?.isGit && !!info.hasCommit
  const needsGit = isolation === 'worktree' && info?.exists && !gitReady

  async function startGit() {
    setBusy(true)
    try { setInfo(await api.gitInit(path.trim())) } catch (err) { onError((err as Error).message) } finally { setBusy(false) }
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const body = { name, path: path.trim(), color, isolation }
      onSaved(project ? await api.updateProject(project.id, body) : await api.createProject(body))
      onClose()
    } catch (err) {
      onError((err as Error).message)
      setBusy(false)
    }
  }

  async function remove() {
    if (!project || !confirm(`Excluir "${project.name}" e todos os cards? Os arquivos da pasta não são apagados.`)) return
    try { await api.deleteProject(project.id); onSaved(null); onClose() } catch (err) { onError((err as Error).message) }
  }

  return (
    <Modal title={project ? 'Editar projeto' : 'Novo projeto'} onClose={onClose}>
      <form onSubmit={submit} className="form">
        <label>Nome
          <input autoFocus value={name} onChange={e => setName(e.target.value)} placeholder="Ex.: finance_app" />
        </label>
        <label>Pasta do projeto
          <input value={path} onChange={e => setPath(e.target.value)} placeholder="C:\Projetos\finance_app" spellCheck={false} />
          <small>
            {!path.trim() ? 'Caminho da pasta no seu computador. Nada é enviado ao GitHub.'
              : info && !info.exists ? 'Pasta não encontrada.'
              : info ? (gitReady ? 'Pasta com git detectada.' : 'Esta pasta não usa git.') : ' '}
          </small>
        </label>
        <div className="field">Como a IA trabalha
          <div className="segmented" role="radiogroup" aria-label="Isolamento">
            {([['worktree', 'Cópia isolada (git)'], ['direct', 'Direto na pasta']] as const).map(([m, label]) => (
              <button key={m} type="button" role="radio" aria-checked={isolation === m} data-on={isolation === m || undefined}
                onClick={() => { setIsolation(m); setTouched(true) }}>{label}</button>
            ))}
          </div>
          <small className="hint">
            {isolation === 'worktree'
              ? 'A IA trabalha numa cópia em outra branch. Você revisa o diff e faz merge. Precisa de git local, sem GitHub.'
              : 'A IA edita os arquivos da pasta. O app guarda um snapshot antes de cada execução para você ver o diff e desfazer, sem criar .git no projeto.'}
          </small>
          {needsGit && (
            <div className="notice">
              <span>Para isolar, a pasta precisa ser um repositório git com um commit.</span>
              <button type="button" className="btn" disabled={busy} onClick={startGit}>Iniciar git nesta pasta</button>
            </div>
          )}
        </div>
        <div className="field">Cor
          <div className="swatches">
            {SWATCHES.map(c => (
              <button key={c} type="button" className="swatch" style={{ background: c }} data-on={c === color || undefined} aria-label={`Cor ${c}`} onClick={() => setColor(c)} />
            ))}
          </div>
        </div>
        <div className="actions">
          {project && <button type="button" className="btn danger" onClick={remove}>Excluir</button>}
          <span className="spacer" />
          <button type="button" className="btn ghost" onClick={onClose}>Cancelar</button>
          <button className="btn primary" disabled={busy || !name.trim() || !path.trim() || !info?.exists || !!needsGit}>{project ? 'Salvar' : 'Criar projeto'}</button>
        </div>
      </form>
    </Modal>
  )
}
