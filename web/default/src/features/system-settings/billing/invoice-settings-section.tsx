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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

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

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const invoiceSettingsSchema = z.object({
  clientId: z.string().max(255),
  clientSecret: z.string().max(1024),
})

type InvoiceSettingsValues = z.infer<typeof invoiceSettingsSchema>

export function InvoiceSettingsSection({
  defaultValues,
}: {
  defaultValues: InvoiceSettingsValues
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<InvoiceSettingsValues>({
    resolver: zodResolver(invoiceSettingsSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: InvoiceSettingsValues) => {
    const clientId = values.clientId.trim()
    const clientSecret = values.clientSecret.trim()
    const updates: Array<{ key: string; value: string }> = []

    if (clientId !== defaultValues.clientId.trim()) {
      updates.push({
        key: 'payment_setting.invoice_client_id',
        value: clientId,
      })
    }
    if (clientSecret) {
      updates.push({
        key: 'payment_setting.invoice_client_secret',
        value: clientSecret,
      })
    }
    if (updates.length === 0) {
      form.reset({ clientId, clientSecret: '' })
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      const result = await updateOption.mutateAsync(update)
      if (!result.success) return
    }
    form.reset({ clientId, clientSecret: '' })
  }

  const { isDirty, isSubmitting } = form.formState

  return (
    <SettingsSection title={t('Invoice Management')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save Changes'
          />
          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='clientId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Client ID')}</FormLabel>
                  <FormControl>
                    <Input {...field} autoComplete='off' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='clientSecret'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Client Secret')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='password'
                      autoComplete='new-password'
                      placeholder={t('Enter new key to update')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Leave blank to keep the existing key')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
