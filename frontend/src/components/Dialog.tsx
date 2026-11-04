import { useEffect, useRef, type ReactNode } from 'react'

interface DialogProps {
  title: string
  open: boolean
  onClose: () => void
  children: ReactNode
}

// Native <dialog> with showModal() gives focus trapping, Escape to close and an
// inert background for free, with correct semantics for screen readers.
export function Dialog({ title, open, onClose, children }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dialog = ref.current
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal?.()
    if (!open && dialog.open) dialog.close?.()
  }, [open])

  return (
    <dialog ref={ref} className="dialog" aria-labelledby="dialog-title" onClose={onClose}>
      <div className="dialog__header">
        <h2 id="dialog-title">{title}</h2>
        <button type="button" className="dialog__close" onClick={onClose} aria-label="Close">
          ×
        </button>
      </div>
      {open && children}
    </dialog>
  )
}
