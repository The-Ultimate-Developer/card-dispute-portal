import { useState } from 'react'

import { useLockedAccounts, useUnlockAccount } from '../../api/hooks'
import type { LockedAccount } from '../../api/types'
import { Dialog } from '../../components/Dialog'
import { EmptyState, ErrorNotice, Loading, Notice } from '../../components/Feedback'
import { formatDateTime } from '../../lib/format'

export function LockedAccountsPage() {
  const query = useLockedAccounts()
  const unlock = useUnlockAccount()
  const [confirming, setConfirming] = useState<LockedAccount | null>(null)
  const [unlocked, setUnlocked] = useState<string | null>(null)

  const confirmUnlock = async () => {
    if (!confirming) return
    try {
      await unlock.mutateAsync(confirming.id)
      setUnlocked(confirming.fullName)
      setConfirming(null)
    } catch {
      // Shown in the dialog via unlock.error.
    }
  }

  return (
    <>
      <header className="page-header">
        <h1>Locked accounts</h1>
        <p className="lede">
          Accounts lock after three incorrect passwords. Confirm the customer's identity before unlocking.
        </p>
      </header>

      {unlocked && <Notice tone="success">{unlocked} can sign in again.</Notice>}
      {query.isPending && <Loading label="Loading locked accounts" />}
      {query.isError && <ErrorNotice error={query.error} onRetry={() => void query.refetch()} />}
      {query.isSuccess && query.data.length === 0 && (
        <EmptyState title="No locked accounts">
          <p>When a customer is locked out, they'll appear here.</p>
        </EmptyState>
      )}

      {query.isSuccess && query.data.length > 0 && (
        <ul className="list" aria-label="Locked accounts">
          {query.data.map((account) => (
            <li key={account.id} className="list__row list__row--static">
              <div className="list__main">
                <span className="list__title">{account.fullName}</span>
                <span className="list__meta">
                  <span>{account.email}</span>
                  <span>Locked {formatDateTime(account.lockedAt)}</span>
                  <span>{account.lockReason}</span>
                </span>
              </div>
              <button
                type="button"
                className="button button--small"
                onClick={() => {
                  unlock.reset()
                  setUnlocked(null)
                  setConfirming(account)
                }}
              >
                Unlock<span className="visually-hidden"> {account.fullName}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      <Dialog title="Unlock account" open={confirming !== null} onClose={() => setConfirming(null)}>
        {confirming && (
          <div>
            <p>
              {confirming.fullName} will be able to sign in straight away, and their failed-attempt count is reset. This is
              recorded in the audit log under your name.
            </p>
            {unlock.isError && <ErrorNotice error={unlock.error} />}
            <div className="form-actions">
              <button type="button" className="button button--quiet" onClick={() => setConfirming(null)}>
                Cancel
              </button>
              <button type="button" className="button button--primary" onClick={confirmUnlock} disabled={unlock.isPending}>
                {unlock.isPending ? 'Unlocking…' : 'Unlock account'}
              </button>
            </div>
          </div>
        )}
      </Dialog>
    </>
  )
}
