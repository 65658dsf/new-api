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

import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Dialog } from '@/components/dialog'
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
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import type {
  InvoiceApplication,
  InvoiceApplicationOrder,
  InvoiceApplicationPayload,
  InvoiceTopUpRecord,
} from '../types'
import {
  getInvoiceApplicationSchema,
  INVOICE_APPLICATION_DEFAULT_VALUES,
  type InvoiceApplicationFormValues,
} from '../lib/validation'

type InvoiceApplicationDialogProps = {
  open: boolean
  orders?: InvoiceTopUpRecord[]
  application?: InvoiceApplication | null
  isSubmitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: InvoiceApplicationPayload) => Promise<void>
}

function invoiceOrderLines(
  application?: InvoiceApplication | null,
  orders?: InvoiceTopUpRecord[]
): InvoiceApplicationOrder[] {
  if (application?.orders?.length) {
    return application.orders
  }
  if (orders?.length) {
    return orders.map((order) => ({
      topup_id: order.id,
      trade_no: order.trade_no,
      amount: order.amount,
      money: order.money,
    }))
  }
  if (application?.trade_no) {
    return [
      {
        topup_id: application.topup_id,
        trade_no: application.trade_no,
        amount: application.amount,
        money: application.money,
      },
    ]
  }
  return []
}

export function InvoiceApplicationDialog(
  props: InvoiceApplicationDialogProps
) {
  const { t } = useTranslation()
  const [pendingPayload, setPendingPayload] =
    useState<InvoiceApplicationPayload | null>(null)
  const form = useForm<InvoiceApplicationFormValues>({
    resolver: zodResolver(getInvoiceApplicationSchema(t)),
    defaultValues: INVOICE_APPLICATION_DEFAULT_VALUES,
  })
  const orderLines = invoiceOrderLines(props.application, props.orders)
  const firstTradeNo = orderLines[0]?.trade_no ?? ''
  const totalAmount = orderLines.reduce((sum, order) => sum + order.amount, 0)

  useEffect(() => {
    if (!props.open) {
      form.reset(INVOICE_APPLICATION_DEFAULT_VALUES)
      setPendingPayload(null)
      return
    }
    form.reset({
      ...INVOICE_APPLICATION_DEFAULT_VALUES,
      trade_no: firstTradeNo,
      title: props.application?.title ?? '',
      tax_id: props.application?.tax_id ?? '',
      buyer_address: props.application?.buyer_address ?? '',
      buyer_phone: props.application?.buyer_phone ?? '',
      bank_name: props.application?.bank_name ?? '',
      bank_account: props.application?.bank_account ?? '',
    })
  }, [firstTradeNo, form, props.application, props.open])

  const handleInvalidSubmit = () => {
    toast.error(t('Please check the invoice form'))
  }

  const handleValidSubmit = (values: InvoiceApplicationFormValues) => {
    const tradeNos = orderLines.map((order) => order.trade_no)
    setPendingPayload({
      ...values,
      trade_no: tradeNos[0] ?? values.trade_no,
      trade_nos: tradeNos,
    })
  }

  const handleConfirmedSubmit = async () => {
    if (!pendingPayload) return
    await props.onSubmit(pendingPayload)
    setPendingPayload(null)
  }

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={
          props.application
            ? t('Resubmit Invoice Application')
            : t('Apply for Invoice')
        }
        description={
          props.application
            ? t(
                'Update invoice title information and submit it for review again.'
              )
            : t('Fill in the buyer invoice title information.')
        }
        contentClassName='sm:max-w-3xl'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => props.onOpenChange(false)}
              disabled={props.isSubmitting}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='submit'
              form='invoice-application-form'
              disabled={props.isSubmitting || Boolean(pendingPayload)}
            >
              {props.isSubmitting
                ? t('Submitting...')
                : props.application
                  ? t('Resubmit Application')
                  : t('Submit Application')}
            </Button>
          </>
        }
      >
        <Form {...form}>
          <form
            id='invoice-application-form'
            className='space-y-4'
            onSubmit={form.handleSubmit(handleValidSubmit, handleInvalidSubmit)}
          >
            <FormField
              control={form.control}
              name='trade_no'
              render={({ field }) => <input type='hidden' {...field} />}
            />

            <div className='space-y-2'>
              <div className='text-sm font-medium'>{t('Selected Orders')}</div>
              <div className='border-border divide-border overflow-hidden rounded-md border'>
                {orderLines.map((order) => (
                  <div
                    key={order.trade_no}
                    className='flex flex-col gap-1 px-3 py-2 sm:flex-row sm:items-center sm:justify-between'
                  >
                    <span className='truncate font-mono text-sm'>
                      {order.trade_no}
                    </span>
                    <span className='text-sm font-medium tabular-nums'>
                      {formatBillingCurrencyFromUSD(order.amount)}
                    </span>
                  </div>
                ))}
                <div className='bg-muted/40 flex items-center justify-between px-3 py-2 text-sm font-semibold'>
                  <span>{t('Total Amount')}</span>
                  <span className='tabular-nums'>
                    {formatBillingCurrencyFromUSD(totalAmount)}
                  </span>
                </div>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('The selected order numbers are submitted automatically.')}
              </p>
            </div>

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='title'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Invoice Title')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Enter the full invoice title')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Please enter the full legal name of the buyer.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='tax_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('Unified Social Credit Code / Taxpayer ID')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Enter tax identification number')}
                        onChange={(event) =>
                          field.onChange(event.target.value.toUpperCase())
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Supports 18-digit unified social credit code or 15/17/20-digit taxpayer ID.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='buyer_phone'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Phone')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t('Mobile or landline number')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Optional. Mobile or landline formats are supported.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='bank_name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Buyer Bank')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Bank name')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='bank_account'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Bank Account')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        inputMode='numeric'
                        placeholder={t('Bank account number')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='buyer_address'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Buyer Address')}</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder={t('Buyer address')}
                      className='min-h-20 resize-none'
                    />
                  </FormControl>
                  <FormDescription>{t('Optional')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
      </Dialog>

      <Dialog
        open={Boolean(pendingPayload)}
        onOpenChange={(open) => !open && setPendingPayload(null)}
        title={t('Confirm Invoice Application')}
        description={t(
          'Please confirm the invoice application information before submitting.'
        )}
        contentClassName='sm:max-w-xl'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setPendingPayload(null)}
              disabled={props.isSubmitting}
            >
              {t('Back to Edit')}
            </Button>
            <Button
              type='button'
              onClick={() => void handleConfirmedSubmit()}
              disabled={props.isSubmitting}
            >
              {props.isSubmitting ? t('Submitting...') : t('Confirm and Submit')}
            </Button>
          </>
        }
      >
        <div className='space-y-4'>
          <div className='border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100 rounded-md border p-3 text-sm'>
            {t(
              'Invoice results are usually issued within 24 hours after approval.'
            )}
          </div>
          <div className='space-y-2 text-sm'>
            <div className='grid gap-1 sm:grid-cols-[120px_1fr]'>
              <span className='text-muted-foreground'>{t('Invoice Title')}</span>
              <span className='min-w-0 break-words'>
                {pendingPayload?.title || '-'}
              </span>
            </div>
            <div className='grid gap-1 sm:grid-cols-[120px_1fr]'>
              <span className='text-muted-foreground'>
                {t('Unified Social Credit Code / Taxpayer ID')}
              </span>
              <span className='min-w-0 break-words font-mono'>
                {pendingPayload?.tax_id || '-'}
              </span>
            </div>
          </div>
          <div className='border-border divide-border overflow-hidden rounded-md border'>
            {orderLines.map((order) => (
              <div
                key={order.trade_no}
                className='flex flex-col gap-1 px-3 py-2 sm:flex-row sm:items-center sm:justify-between'
              >
                <span className='truncate font-mono text-sm'>
                  {order.trade_no}
                </span>
                <span className='text-sm font-medium tabular-nums'>
                  {formatBillingCurrencyFromUSD(order.amount)}
                </span>
              </div>
            ))}
            <div className='bg-muted/40 flex items-center justify-between px-3 py-2 text-sm font-semibold'>
              <span>{t('Total Amount')}</span>
              <span className='tabular-nums'>
                {formatBillingCurrencyFromUSD(totalAmount)}
              </span>
            </div>
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Please ensure the invoice title and tax ID are accurate before submitting.'
            )}
          </p>
        </div>
      </Dialog>
    </>
  )
}
