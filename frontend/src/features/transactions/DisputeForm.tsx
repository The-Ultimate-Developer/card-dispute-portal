import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useNavigate } from 'react-router'
import { z } from 'zod'

import { isApiError } from '../../api/client'
import { useCreateDispute } from '../../api/hooks'
import type { DisputeReason, Transaction } from '../../api/types'
import { ErrorNotice } from '../../components/Feedback'
import { reasonLabels } from '../../lib/disputes'
import { formatDate, formatMoney } from '../../lib/format'

const reasons = Object.keys(reasonLabels) as [DisputeReason, ...DisputeReason[]]

// The same limits the API enforces; the server stays the source of truth.
const schema = z.object({
  reason: z.enum(reasons, { error: 'Choose what went wrong' }),
  description: z
    .string()
    .trim()
    .min(10, 'Tell us a little more (at least 10 characters)')
    .max(1000, 'Keep it under 1000 characters'),
})

type FormValues = z.infer<typeof schema>

export function DisputeForm({ transaction, onCancel }: { transaction: Transaction; onCancel: () => void }) {
  const navigate = useNavigate()
  const createDispute = useCreateDispute()

  // One key per opening of the form: a double-click or a network retry reuses it,
  // so the server returns the same dispute instead of creating a second one.
  const [idempotencyKey] = useState(() => crypto.randomUUID())

  const {
    register,
    handleSubmit,
    control,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema), defaultValues: { description: '' } })

  const length = useWatch({ control, name: 'description' }).trim().length

  const onSubmit = async (values: FormValues) => {
    try {
      const dispute = await createDispute.mutateAsync({ ...values, transactionId: transaction.id, idempotencyKey })
      await navigate(`/disputes/${dispute.id}`, { state: { justSubmitted: true } })
    } catch (error) {
      if (isApiError(error)) {
        for (const [field, message] of Object.entries(error.fieldErrors)) {
          if (field === 'reason' || field === 'description') setError(field, { message })
        }
      }
    }
  }

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate className="dispute-form">
      <dl className="summary summary--compact">
        <div>
          <dt>Merchant</dt>
          <dd>{transaction.merchant}</dd>
        </div>
        <div>
          <dt>Amount</dt>
          <dd className="numeric">{formatMoney(transaction.amountCents, transaction.currency)}</dd>
        </div>
        <div>
          <dt>Date</dt>
          <dd>{formatDate(transaction.occurredAt)}</dd>
        </div>
        <div>
          <dt>Card</dt>
          <dd>
            {transaction.cardBrand} ending {transaction.cardLast4}
          </dd>
        </div>
      </dl>

      <fieldset className="field" aria-describedby={errors.reason ? 'reason-error' : undefined}>
        <legend>What went wrong?</legend>
        <div className="choices">
          {reasons.map((reason) => (
            <label key={reason} className="choice">
              <input type="radio" value={reason} {...register('reason')} />
              {reasonLabels[reason]}
            </label>
          ))}
        </div>
        {errors.reason && (
          <p id="reason-error" className="field__error">
            {errors.reason.message}
          </p>
        )}
      </fieldset>

      <div className="field">
        <label htmlFor="description">Tell us what happened</label>
        <p id="description-hint" className="field__hint">
          Include anything that helps, like an order number or when you cancelled. Never share your PIN or password.
        </p>
        <textarea
          id="description"
          rows={4}
          aria-invalid={errors.description ? true : undefined}
          aria-describedby={`description-hint description-count${errors.description ? ' description-error' : ''}`}
          {...register('description')}
        />
        <p id="description-count" className="field__count">
          {length} / 1000
        </p>
        {errors.description && (
          <p id="description-error" className="field__error">
            {errors.description.message}
          </p>
        )}
      </div>

      {createDispute.isError && Object.keys(isApiError(createDispute.error) ? createDispute.error.fieldErrors : {}).length === 0 && (
        <ErrorNotice error={createDispute.error} />
      )}

      <div className="form-actions">
        <button type="button" className="button button--quiet" onClick={onCancel}>
          Cancel
        </button>
        <button type="submit" className="button button--primary" disabled={isSubmitting}>
          {isSubmitting ? 'Submitting…' : 'Submit dispute'}
        </button>
      </div>
    </form>
  )
}
