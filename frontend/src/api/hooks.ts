import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { isApiError, request, withQuery } from './client'
import type {
  AuditEventType,
  AuditPage,
  ChangeStatusRequest,
  CreateDisputeRequest,
  Dispute,
  DisputeDetail,
  DisputePage,
  DisputeStatus,
  LockedAccount,
  NotificationPage,
  Overview,
  TransactionPage,
} from './types'

// Query keys are centralised so invalidation after a mutation can't miss one.
export const queryKeys = {
  transactions: ['transactions'] as const,
  disputes: (status?: DisputeStatus) => ['disputes', status ?? 'ALL'] as const,
  allDisputes: ['disputes'] as const,
  dispute: (id: string) => ['dispute', id] as const,
  lockedAccounts: ['locked-accounts'] as const,
  overview: ['admin', 'overview'] as const,
  audit: (event?: AuditEventType) => ['admin', 'audit', event ?? 'ALL'] as const,
  notifications: (status?: string) => ['admin', 'notifications', status ?? 'ALL'] as const,
}

const PAGE_SIZE = 20

export function useTransactions() {
  return useInfiniteQuery({
    queryKey: queryKeys.transactions,
    queryFn: ({ pageParam, signal }) =>
      request<TransactionPage>(withQuery('/api/transactions', { cursor: pageParam, limit: PAGE_SIZE }), { signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor,
  })
}

export function useDisputes(status?: DisputeStatus) {
  return useInfiniteQuery({
    queryKey: queryKeys.disputes(status),
    queryFn: ({ pageParam, signal }) =>
      request<DisputePage>(withQuery('/api/disputes', { status, cursor: pageParam, limit: PAGE_SIZE }), { signal }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor,
  })
}

export function useDispute(id: string) {
  return useQuery({
    queryKey: queryKeys.dispute(id),
    queryFn: ({ signal }) => request<DisputeDetail>(`/api/disputes/${encodeURIComponent(id)}`, { signal }),
  })
}

export interface CreateDisputeInput extends CreateDisputeRequest {
  idempotencyKey: string
}

export function useCreateDispute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ idempotencyKey, ...body }: CreateDisputeInput) =>
      request<Dispute>('/api/disputes', { method: 'POST', body, headers: { 'Idempotency-Key': idempotencyKey } }),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.transactions }),
        queryClient.invalidateQueries({ queryKey: queryKeys.allDisputes }),
      ]),
  })
}

export function useChangeStatus(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: ChangeStatusRequest) =>
      request<DisputeDetail>(`/api/disputes/${encodeURIComponent(id)}/status`, { method: 'PATCH', body }),
    onSuccess: async (updated) => {
      queryClient.setQueryData(queryKeys.dispute(id), updated)
      await queryClient.invalidateQueries({ queryKey: queryKeys.allDisputes })
    },
    // Someone else changed it first: load their version so the agent sees what changed.
    onError: (error) =>
      isApiError(error, 'STALE_VERSION') ? queryClient.invalidateQueries({ queryKey: queryKeys.dispute(id) }) : undefined,
  })
}

export function useLockedAccounts() {
  return useQuery({
    queryKey: queryKeys.lockedAccounts,
    queryFn: ({ signal }) => request<{ items: LockedAccount[] }>('/api/support/locked-accounts', { signal }),
    select: (data) => data.items,
  })
}

export function useUnlockAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userId: string) =>
      request<void>(`/api/support/accounts/${encodeURIComponent(userId)}/unlock`, { method: 'POST' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.lockedAccounts }),
  })
}

export function useOverview() {
  return useQuery({
    queryKey: queryKeys.overview,
    queryFn: ({ signal }) => request<Overview>('/api/admin/overview', { signal }),
    refetchInterval: 30_000,
  })
}

export function useAuditEvents(event?: AuditEventType) {
  return useInfiniteQuery({
    queryKey: queryKeys.audit(event),
    queryFn: ({ pageParam, signal }) =>
      request<AuditPage>(withQuery('/api/admin/audit-events', { event, before: pageParam, limit: 50 }), { signal }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.nextBefore,
  })
}

export function useNotifications(status?: string) {
  return useInfiniteQuery({
    queryKey: queryKeys.notifications(status),
    queryFn: ({ pageParam, signal }) =>
      request<NotificationPage>(withQuery('/api/admin/notifications', { status, before: pageParam, limit: 50 }), {
        signal,
      }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.nextBefore,
  })
}
