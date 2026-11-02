import { Check, Chevron, Sparkle } from './Icons'
import { Popover } from './Popover'

// Which model a card's runs use, and how hard it thinks. Empty means "the Claude CLI's default".

export const MODELS = [
  { id: '', name: 'Padrão', desc: 'O modelo padrão da sua conta · recomendado' },
  { id: 'opus', name: 'Opus', desc: 'Para tarefas complexas do dia a dia' },
  { id: 'fable', name: 'Fable', desc: 'O mais capaz, para as tarefas mais difíceis e longas · exige créditos de uso' },
  { id: 'sonnet', name: 'Sonnet', desc: 'Eficiente para tarefas de rotina' },
  { id: 'haiku', name: 'Haiku', desc: 'O mais rápido, para respostas curtas' },
] as const

export const EFFORTS = [
  { id: 'low', name: 'Baixo' },
  { id: 'medium', name: 'Médio' },
  { id: 'high', name: 'Alto' },
  { id: 'xhigh', name: 'Muito alto' },
  { id: 'max', name: 'Máximo' },
] as const

export const modelName = (id: string) => MODELS.find(m => m.id === id)?.name ?? id
export const effortName = (id: string) => EFFORTS.find(e => e.id === id)?.name ?? id

// "Opus · Alto", "Sonnet", "Esforço alto" or "" when everything is default (used by the card badge too)
export function modelSummary(model: string, effort: string): string {
  if (model && effort) return `${modelName(model)} · ${effortName(effort)}`
  if (model) return modelName(model)
  if (effort) return `Esforço ${effortName(effort).toLowerCase()}`
  return ''
}

interface Props {
  model: string
  effort: string
  onChange: (model: string, effort: string) => void
  disabled?: boolean
}

export function ModelPicker({ model, effort, onChange, disabled }: Props) {
  const idx = EFFORTS.findIndex(e => e.id === effort)
  return (
    <Popover width={360} trigger={({ open, toggle }) => (
      <button type="button" className="model-btn" onClick={toggle} disabled={disabled} aria-haspopup="menu" aria-expanded={open}>
        <Sparkle />
        <span>{modelSummary(model, effort) || 'Modelo padrão'}</span>
        <Chevron />
      </button>
    )}>
      {() => (
        <>
          <p className="pop-title">Escolher modelo</p>
          {MODELS.map(m => (
            <button key={m.id || 'default'} type="button" role="menuitemradio" aria-checked={m.id === model} className="pop-item" data-on={m.id === model || undefined} onClick={() => onChange(m.id, effort)}>
              <span className="pop-item-text"><b>{m.name}</b><small>{m.desc}</small></span>
              {m.id === model && <Check />}
            </button>
          ))}
          <div className="effort">
            <div className="effort-head">
              <span>Esforço</span>
              <b>{effort ? effortName(effort) : 'Padrão'}</b>
              {effort && <button type="button" className="effort-reset" onClick={() => onChange(model, '')}>Voltar ao padrão</button>}
            </div>
            <div className="effort-track" role="radiogroup" aria-label="Esforço de raciocínio">
              {EFFORTS.map((e, i) => (
                <button key={e.id} type="button" role="radio" aria-checked={e.id === effort} aria-label={e.name} title={e.name}
                  className="effort-dot" data-filled={idx >= 0 && i <= idx || undefined} data-on={e.id === effort || undefined} onClick={() => onChange(model, e.id)} />
              ))}
            </div>
            <p className="effort-hint">Mais esforço pensa mais antes de agir: respostas melhores em tarefas difíceis, porém mais lentas e com mais consumo do plano.</p>
          </div>
        </>
      )}
    </Popover>
  )
}
