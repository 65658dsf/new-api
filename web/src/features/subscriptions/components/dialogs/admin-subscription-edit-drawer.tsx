/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import {
  CalendarClockIcon,
  Refresh01Icon,
  Settings02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { TFunction } from 'i18next'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { getCurrencyLabel } from '@/lib/currency'
import {
  getEditableQuotaStep,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'

import { getGroups, updateUserSubscription } from '../../api'
import { getResetPeriodOptions } from '../../constants'
import type {
  AdminSubscriptionRecord,
  AdminUpdateUserSubscriptionRequest,
} from '../../types'

const SUBSCRIPTION_STATUSES = ['active', 'expired', 'cancelled'] as const
const RESET_PERIODS = ['never', 'daily', 'weekly', 'monthly', 'custom'] as const

function getSubscriptionFormSchema(t: TFunction) {
  return z
    .object({
      amount_total: z.coerce
        .number()
        .min(0, t('Quota must be zero or greater')),
      start_time: z.date().nullable(),
      end_time: z.date().nullable(),
      status: z.enum(SUBSCRIPTION_STATUSES),
      allow_wallet_overflow: z.boolean(),
      upgrade_group: z.string(),
      downgrade_group: z.string(),
      billing_group: z.string(),
      quota_reset_period: z.enum(RESET_PERIODS),
      quota_reset_custom_seconds: z.coerce.number(),
    })
    .superRefine((values, context) => {
      const now = Date.now()

      if (!values.start_time) {
        context.addIssue({
          code: 'custom',
          path: ['start_time'],
          message: t('Start time is required'),
        })
      }
      if (!values.end_time) {
        context.addIssue({
          code: 'custom',
          path: ['end_time'],
          message: t('End time is required'),
        })
      }
      if (
        values.start_time &&
        values.end_time &&
        values.end_time.getTime() <= values.start_time.getTime()
      ) {
        context.addIssue({
          code: 'custom',
          path: ['end_time'],
          message: t('End time must be later than start time'),
        })
      }
      if (
        values.status === 'active' &&
        values.start_time &&
        values.start_time.getTime() > now
      ) {
        context.addIssue({
          code: 'custom',
          path: ['start_time'],
          message: t(
            'Start time cannot be in the future for an active subscription'
          ),
        })
      }
      if (
        values.status === 'active' &&
        values.end_time &&
        values.end_time.getTime() <= now
      ) {
        context.addIssue({
          code: 'custom',
          path: ['end_time'],
          message: t(
            'End time must be in the future for an active subscription'
          ),
        })
      }
      if (
        values.quota_reset_period === 'custom' &&
        (!Number.isInteger(values.quota_reset_custom_seconds) ||
          values.quota_reset_custom_seconds <= 0)
      ) {
        context.addIssue({
          code: 'custom',
          path: ['quota_reset_custom_seconds'],
          message: t('Custom reset seconds must be a positive integer'),
        })
      }
    })
}

type SubscriptionFormValues = z.infer<
  ReturnType<typeof getSubscriptionFormSchema>
>

const FORM_DEFAULTS: SubscriptionFormValues = {
  amount_total: 0,
  start_time: null,
  end_time: null,
  status: 'active',
  allow_wallet_overflow: true,
  upgrade_group: '',
  downgrade_group: '',
  billing_group: '',
  quota_reset_period: 'never',
  quota_reset_custom_seconds: 0,
}

function subscriptionToFormValues(
  record: AdminSubscriptionRecord
): SubscriptionFormValues {
  const subscription = record.subscription
  const storedStatus = subscription.status.trim().toLowerCase()
  const status = SUBSCRIPTION_STATUSES.includes(
    storedStatus as (typeof SUBSCRIPTION_STATUSES)[number]
  )
    ? (storedStatus as (typeof SUBSCRIPTION_STATUSES)[number])
    : 'expired'
  const storedQuotaResetPeriod =
    subscription.quota_reset_period || record.plan?.quota_reset_period
  const quotaResetPeriod = RESET_PERIODS.includes(
    storedQuotaResetPeriod as (typeof RESET_PERIODS)[number]
  )
    ? (storedQuotaResetPeriod as (typeof RESET_PERIODS)[number])
    : 'never'
  const isLegacyResetSnapshot = !subscription.quota_reset_period

  return {
    amount_total: quotaUnitsToDollars(Number(subscription.amount_total || 0)),
    start_time:
      subscription.start_time > 0
        ? new Date(subscription.start_time * 1000)
        : null,
    end_time:
      subscription.end_time > 0 ? new Date(subscription.end_time * 1000) : null,
    status,
    allow_wallet_overflow: subscription.allow_wallet_overflow !== false,
    upgrade_group: subscription.upgrade_group || '',
    downgrade_group: subscription.downgrade_group || '',
    billing_group: subscription.billing_group || '',
    quota_reset_period: quotaResetPeriod,
    quota_reset_custom_seconds: isLegacyResetSnapshot
      ? Number(
          record.plan?.quota_reset_custom_seconds ||
            subscription.quota_reset_custom_seconds ||
            0
        )
      : Number(subscription.quota_reset_custom_seconds || 0),
  }
}

function formDateToTimestamp(value: Date | null): number {
  return value ? Math.floor(value.getTime() / 1000) : 0
}

interface Props {
  open: boolean
  record: AdminSubscriptionRecord | null
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function AdminSubscriptionEditDrawer(props: Props) {
  const { t } = useTranslation()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [groups, setGroups] = useState<string[]>([])
  const schema = getSubscriptionFormSchema(t)
  const form = useForm<SubscriptionFormValues>({
    resolver: zodResolver(
      schema
    ) as unknown as Resolver<SubscriptionFormValues>,
    defaultValues: FORM_DEFAULTS,
  })

  useEffect(() => {
    if (!props.open) return
    if (props.record) {
      form.reset(subscriptionToFormValues(props.record))
    } else {
      form.reset(FORM_DEFAULTS)
    }
    getGroups()
      .then((response) => {
        setGroups(response.success ? response.data || [] : [])
      })
      .catch(() => setGroups([]))
  }, [form, props.open, props.record])

  const currentSubscription = props.record?.subscription
  const groupOptions = useMemo(() => {
    const currentGroups = currentSubscription
      ? [
          currentSubscription.upgrade_group,
          currentSubscription.downgrade_group,
          currentSubscription.billing_group,
        ]
      : []
    return [
      ...new Set(
        [...groups, ...currentGroups].filter(
          (group): group is string => !!group
        )
      ),
    ]
  }, [currentSubscription, groups])

  const quotaResetPeriod = form.watch('quota_reset_period')

  const onSubmit = async (values: SubscriptionFormValues) => {
    if (!props.record) return
    const baseline = subscriptionToFormValues(props.record)
    const payload: AdminUpdateUserSubscriptionRequest = {}

    const amountTotal = parseQuotaFromDollars(Number(values.amount_total || 0))
    if (
      amountTotal !== parseQuotaFromDollars(Number(baseline.amount_total || 0))
    ) {
      payload.amount_total = amountTotal
    }

    const startTime = formDateToTimestamp(values.start_time)
    if (startTime !== formDateToTimestamp(baseline.start_time)) {
      payload.start_time = startTime
    }

    const endTime = formDateToTimestamp(values.end_time)
    if (endTime !== formDateToTimestamp(baseline.end_time)) {
      payload.end_time = endTime
    }

    if (values.status !== baseline.status) {
      payload.status = values.status
    }
    if (values.allow_wallet_overflow !== baseline.allow_wallet_overflow) {
      payload.allow_wallet_overflow = values.allow_wallet_overflow
    }

    const upgradeGroup = values.upgrade_group.trim()
    if (upgradeGroup !== baseline.upgrade_group.trim()) {
      payload.upgrade_group = upgradeGroup
    }

    const downgradeGroup = values.downgrade_group.trim()
    if (downgradeGroup !== baseline.downgrade_group.trim()) {
      payload.downgrade_group = downgradeGroup
    }

    const billingGroup = values.billing_group.trim()
    if (billingGroup !== baseline.billing_group.trim()) {
      payload.billing_group = billingGroup
    }

    const resetPeriodChanged =
      values.quota_reset_period !== baseline.quota_reset_period
    const resetCustomSeconds = Number(values.quota_reset_custom_seconds || 0)
    const resetCustomSecondsChanged =
      resetCustomSeconds !== Number(baseline.quota_reset_custom_seconds || 0)
    if (resetPeriodChanged) {
      payload.quota_reset_period = values.quota_reset_period
    }
    if (
      values.quota_reset_period === 'custom' &&
      (resetPeriodChanged || resetCustomSecondsChanged)
    ) {
      payload.quota_reset_custom_seconds = resetCustomSeconds
      if (!props.record.subscription.quota_reset_period) {
        payload.quota_reset_period = values.quota_reset_period
      }
    }

    if (Object.keys(payload).length === 0) {
      toast.success(t('Updated successfully'))
      props.onOpenChange(false)
      return
    }

    setIsSubmitting(true)
    try {
      const response = await updateUserSubscription(
        props.record.subscription.id,
        payload
      )
      if (!response.success) {
        toast.error(response.message || t('Update failed'))
        return
      }
      toast.success(t('Updated successfully'))
      props.onOpenChange(false)
      props.onSuccess?.()
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const resetPeriodOptions = getResetPeriodOptions(t)

  return (
    <Sheet
      open={props.open}
      onOpenChange={(open) => {
        props.onOpenChange(open)
        if (!open) form.reset(FORM_DEFAULTS)
      }}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[600px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Modify subscription')}</SheetTitle>
          <SheetDescription>
            {t(
              'Changes apply only to this subscription instance; the original plan remains unchanged.'
            )}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='admin-subscription-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            <SideDrawerSection>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <IconBadge tone='chart-4' size='xs'>
                  <HugeiconsIcon icon={Settings02Icon} strokeWidth={2} />
                </IconBadge>
                {t('Subscription')}
              </h3>

              <FormField
                control={form.control}
                name='amount_total'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Quota ({{currency}})', {
                        currency: getCurrencyLabel(),
                      })}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={0}
                        step={getEditableQuotaStep()}
                        onChange={(event) =>
                          field.onChange(
                            Number.parseFloat(event.target.value) || 0
                          )
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Total quota included in the plan, usable per billing period. 0 means unlimited.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Status')}</FormLabel>
                    <Select
                      items={[
                        { value: 'active', label: t('Active') },
                        { value: 'expired', label: t('Expired') },
                        { value: 'cancelled', label: t('Invalidated') },
                      ]}
                      value={field.value}
                      onValueChange={(value) => value && field.onChange(value)}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='active'>{t('Active')}</SelectItem>
                          <SelectItem value='expired'>
                            {t('Expired')}
                          </SelectItem>
                          <SelectItem value='cancelled'>
                            {t('Invalidated')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='allow_wallet_overflow'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between gap-3 rounded-md border px-3 py-2'>
                    <FormLabel className='!mt-0'>
                      {t('Allow wallet balance after quota used up')}
                    </FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <SideDrawerSection>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <IconBadge tone='chart-4' size='xs'>
                  <HugeiconsIcon icon={CalendarClockIcon} strokeWidth={2} />
                </IconBadge>
                {t('Validity')}
              </h3>

              <FormField
                control={form.control}
                name='start_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Start Time')}</FormLabel>
                    <FormControl>
                      <DateTimePicker
                        value={field.value || undefined}
                        onChange={(date) => field.onChange(date ?? null)}
                        placeholder={t('Select start time')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='end_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('End Time')}</FormLabel>
                    <FormControl>
                      <DateTimePicker
                        value={field.value || undefined}
                        onChange={(date) => field.onChange(date ?? null)}
                        placeholder={t('Select end time')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <SideDrawerSection>
              <h3 className='flex items-center gap-2 text-sm font-medium'>
                <IconBadge tone='success' size='xs'>
                  <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
                </IconBadge>
                {t('Quota Reset')}
              </h3>

              <FormField
                control={form.control}
                name='quota_reset_period'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Reset Cycle')}</FormLabel>
                    <Select
                      items={resetPeriodOptions}
                      value={field.value}
                      onValueChange={(value) => value && field.onChange(value)}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {resetPeriodOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='quota_reset_custom_seconds'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Custom Seconds')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={1}
                        step={1}
                        disabled={quotaResetPeriod !== 'custom'}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? 0
                              : Number(event.target.value)
                          )
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <SideDrawerSection>
              <h3 className='text-sm font-medium'>
                {t('Subscription Quota Group')}
              </h3>

              <FormField
                control={form.control}
                name='upgrade_group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Upgrade Group')}</FormLabel>
                    <Select
                      items={[
                        { value: '__none__', label: t('No Upgrade') },
                        ...groupOptions.map((group) => ({
                          value: group,
                          label: group,
                        })),
                      ]}
                      value={field.value || '__none__'}
                      onValueChange={(value) =>
                        field.onChange(value === '__none__' ? '' : value || '')
                      }
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('No Upgrade')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='__none__'>
                            {t('No Upgrade')}
                          </SelectItem>
                          {groupOptions.map((group) => (
                            <SelectItem key={group} value={group}>
                              {group}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='downgrade_group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Downgrade Group')}</FormLabel>
                    <Select
                      items={[
                        {
                          value: '__none__',
                          label: t('Downgrade to pre-purchase group'),
                        },
                        ...groupOptions.map((group) => ({
                          value: group,
                          label: group,
                        })),
                      ]}
                      value={field.value || '__none__'}
                      onValueChange={(value) =>
                        field.onChange(value === '__none__' ? '' : value || '')
                      }
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue
                            placeholder={t('Downgrade to pre-purchase group')}
                          />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='__none__'>
                            {t('Downgrade to pre-purchase group')}
                          </SelectItem>
                          {groupOptions.map((group) => (
                            <SelectItem key={group} value={group}>
                              {group}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='billing_group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Billing group')}</FormLabel>
                    <Select
                      items={[
                        { value: '__all__', label: t('All Groups') },
                        ...groupOptions.map((group) => ({
                          value: group,
                          label: group,
                        })),
                      ]}
                      value={field.value || '__all__'}
                      onValueChange={(value) =>
                        field.onChange(value === '__all__' ? '' : value || '')
                      }
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('All Groups')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='__all__'>
                            {t('All Groups')}
                          </SelectItem>
                          {groupOptions.map((group) => (
                            <SelectItem key={group} value={group}>
                              {group}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      {t(
                        'When set, this subscription quota is used only for requests in the selected group. Other groups use wallet balance.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>
          </form>
        </Form>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button
            form='admin-subscription-form'
            type='submit'
            disabled={isSubmitting || !props.record}
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
