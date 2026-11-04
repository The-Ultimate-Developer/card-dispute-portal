import { useState } from 'react'

import { isApiError } from '../../api/client'
import { useChangeStatus } from '../../api/hooks'
import type { DisputeDetail, DisputeStatus } from '../../api/types'
import { ErrorNotice, Notice } from '../../components/Feedback'
import { actionLabel, isFinal, nextStatuses, statusLabel } from '../../lib/disputes'

export function StatusActions({ dispute }: { dispute: DisputeDetail }) {
  const changeStatus = useChangeStatus(dispute.id)
  const [target, setTarget] = useState<DisputeStatus | null>(null)
  const [note, setNote] = useState('')
  const [noteError, setNoteError] = useState<string | null>(null)
  const [done, setDone] = useState<DisputeStatus | null>(null)

  const options = nextStatuses(dispute.status)
  if (options.length === 0) {
    return (
      <div className="actions">
        <h2>Actions</h2>
        <p className="muted">This dispute is closed. No further actions are available.</p>
      </div>
    )
  }

  const noteRequired = target !== null && isFinal(target)

  const submit = async () => {
    if (!target) return
    if (noteRequired && note.trim().length < 3) {
      setNoteError('Add a note for the customer explaining the outcome')
      return
    }
    setNoteError(null)
    try {
      // The version we last read is sent back. If anyone changed the dispute in
      // between, the API answers 409 and nothing is overwritten.
      await changeStatus.mutateAsync({ status: target, version: dispute.version, note: note.trim() })
      setDone(target)
      setTarget(null)
      setNote('')
    } catch {
      // Shown below via changeStatus.error.
    }
  }

  const stale = isApiError(changeStatus.error, 'STALE_VERSION')

  return (
    <div className="actions">
      <h2>Actions</h2>

      {done && <Notice tone="success">Dispute moved to {statusLabel(done).toLowerCase()}. The customer has been emailed.</Notice>}
      {stale && (
        <Notice>Someone else updated this dispute while you were working on it. The latest details are shown; review them and try again.</Notice>
      )}
      {changeStatus.isError && !stale && <ErrorNotice error={changeStatus.error} />}

      <div className="actions__buttons" role="group" aria-label="Change status">
        {options.map((option) => (
          <button
            key={option}
            type="button"
            className={`button ${option === 'REJECTED' ? 'button--danger-quiet' : 'button--primary'}`}
            aria-pressed={target === option}
            onClick={() => {
              setTarget(option)
              setDone(null)
              changeStatus.reset()
            }}
          >
            {actionLabel(option)}
          </button>
        ))}
      </div>

      {target && (
        <div className="actions__confirm">
          <div className="field">
            <label htmlFor="status-note">
              Note for the customer{noteRequired ? '' : ' (optional)'}
            </label>
            <textarea
              id="status-note"
              rows={3}
              value={note}
              maxLength={500}
              aria-invalid={noteError ? true : undefined}
              aria-describedby={noteError ? 'status-note-error' : undefined}
              onChange={(event) => setNote(event.target.value)}
            />
            {noteError && (
              <p id="status-note-error" className="field__error">
                {noteError}
              </p>
            )}
          </div>
          <div className="form-actions">
            <button type="button" className="button button--quiet" onClick={() => setTarget(null)}>
              Cancel
            </button>
            <button type="button" className="button button--primary" onClick={submit} disabled={changeStatus.isPending}>
              {changeStatus.isPending ? 'Saving…' : `Confirm: ${actionLabel(target).toLowerCase()}`}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
