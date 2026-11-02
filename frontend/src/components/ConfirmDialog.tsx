import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'

export interface ConfirmOptions {
  title: string
  message?: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
}

type Ask = (o: ConfirmOptions) => Promise<boolean>

const Ctx = createContext<Ask>(() => Promise.resolve(false))

// Replaces window.confirm: `if (await confirm({ ... })) { ... }`
export const useConfirm = () => useContext(Ctx)

function ConfirmModal({ opts, onDone }: { opts: ConfirmOptions; onDone: (ok: boolean) => void }) {
  const ref = useRef<HTMLDialogElement>(null)
  const result = useRef(false)
  useEffect(() => { ref.current?.showModal() }, [])
  const finish = (ok: boolean) => { result.current = ok; ref.current?.close() }
  return (
    <dialog
      ref={ref} className="modal confirm" role="alertdialog" aria-labelledby="confirm-title"
      onClose={() => onDone(result.current)}
      onClick={e => { if (e.target === ref.current) finish(false) }}
    >
      <div className="modal-inner">
        <h2 className="modal-title" id="confirm-title">{opts.title}</h2>
        {opts.message && <div className="confirm-msg">{opts.message}</div>}
        <div className="actions">
          <button type="button" className="btn ghost" autoFocus={opts.danger} onClick={() => finish(false)}>{opts.cancelLabel ?? 'Cancelar'}</button>
          <button type="button" className={`btn ${opts.danger ? 'danger-solid' : 'primary'}`} autoFocus={!opts.danger} onClick={() => finish(true)}>
            {opts.confirmLabel ?? 'Confirmar'}
          </button>
        </div>
      </div>
    </dialog>
  )
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<{ opts: ConfirmOptions; resolve: (ok: boolean) => void } | null>(null)
  const ask = useCallback<Ask>(opts => new Promise(resolve => setPending({ opts, resolve })), [])
  return (
    <Ctx.Provider value={ask}>
      {children}
      {pending && <ConfirmModal opts={pending.opts} onDone={ok => { pending.resolve(ok); setPending(null) }} />}
    </Ctx.Provider>
  )
}
