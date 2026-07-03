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

import { useEffect } from 'react'
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
import type { InvoiceTopUpRecord } from '../types'
import {
  getInvoiceApplicationSchema,
  INVOICE_APPLICATION_DEFAULT_VALUES,
  type InvoiceApplicationFormValues,
} from '../lib/validation'

type InvoiceApplicationDialogProps = {
  open: boolean
  order: InvoiceTopUpRecord | null
  isSubmitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: InvoiceApplicationFormValues) => Promise<void>
}

export function InvoiceApplicationDialog(
  props: InvoiceApplicationDialogProps
) {
  const { t } = useTranslation()
  const form = useForm<InvoiceApplicationFormValues>({
    resolver: zodResolver(getInvoiceApplicationSchema(t)),
    defaultValues: INVOICE_APPLICATION_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!props.open) {
      form.reset(INVOICE_APPLICATION_DEFAULT_VALUES)
      return
    }
    form.reset({
      ...INVOICE_APPLICATION_DEFAULT_VALUES,
      trade_no: props.order?.trade_no ?? '',
    })
  }, [form, props.open, props.order])

  const handleInvalidSubmit = () => {
    toast.error(t('Please check the invoice form'))
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Apply for Invoice')}
      description={t('Fill in the buyer invoice title information.')}
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
            disabled={props.isSubmitting}
          >
            {props.isSubmitting ? t('Submitting...') : t('Submit Application')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='invoice-application-form'
          className='space-y-4'
          onSubmit={form.handleSubmit(props.onSubmit, handleInvalidSubmit)}
        >
          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='trade_no'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Order Number')}</FormLabel>
                  <FormControl>
                    <Input {...field} readOnly className='font-mono' />
                  </FormControl>
                  <FormDescription>
                    {t('The order number is filled automatically.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

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
                    {t('Supports 18-digit unified social credit code or 15/17/20-digit taxpayer ID.')}
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
  )
}
