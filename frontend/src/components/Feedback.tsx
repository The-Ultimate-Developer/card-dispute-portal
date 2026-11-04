import type { ReactNode } from 'react'

import { isApiError } from '../api/client'

export function Loading({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="loading" role="status">
      <span className="loading__dot" aria-hidden="true" />
      {label}
    </div>
  )
}

export function errorMessage(error: unknown): string {
  if (isApiError(error)) return error.message
  return 'Something went wrong. Please try again.'
}

export function ErrorNotice({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const reference = isApiError(error) ? error.requestId : undefined
  return (
    <div className="notice notice--error" role="alert">
      <p>{errorMessage(error)}</p>
      {reference && <p className="notice__meta">Reference {reference.slice(0, 12)}</p>}
      {onRetry && (
        <button type="button" className="button button--quiet" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  )
}

export function Notice({ tone = 'info', children }: { tone?: 'info' | 'success'; children: ReactNode }) {
  return (
    <div className={`notice notice--${tone}`} role="status">
      {children}
    </div>
  )
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <p className="empty__title">{title}</p>
      {children}
    </div>
  )
}

interface LoadMoreProps {
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onLoadMore: () => void
}

export function LoadMore({ hasNextPage, isFetchingNextPage, onLoadMore }: LoadMoreProps) {
  if (!hasNextPage) return null
  return (
    <div className="load-more">
      <button type="button" className="button button--quiet" onClick={onLoadMore} disabled={isFetchingNextPage}>
        {isFetchingNextPage ? 'Loading…' : 'Show more'}
      </button>
    </div>
  )
}
