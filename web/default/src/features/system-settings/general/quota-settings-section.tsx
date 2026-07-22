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
import type { ChangeEvent } from 'react'
import { useWatch, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Alert, AlertDescription } from '@/components/ui/alert'
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
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { formatQuota } from '@/lib/format'

import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
  SettingsFormGrid,
  SettingsFormGridItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const createQuotaSchema = (t: (key: string) => string) =>
  z.object({
    QuotaForNewUser: z.coerce.number().min(0),
    PreConsumedQuota: z.coerce.number().min(0),
    QuotaForInviter: z.coerce.number().min(0, t('Value must be 0 or greater')),
    QuotaForInvitee: z.coerce.number().min(0),
    TopUpLink: z.string(),
    general_setting: z.object({
      docs_link: z.string(),
    }),
    quota_setting: z.object({
      enable_free_model_pre_consume: z.boolean(),
      inviter_reward_mode: z.enum(['fixed', 'percentage']),
      inviter_reward_percentage: z.coerce
        .number()
        .min(0, t('Percentage must be between 0 and 100'))
        .max(100, t('Percentage must be between 0 and 100')),
    }),
  })

type QuotaFormValues = z.infer<ReturnType<typeof createQuotaSchema>>
type QuotaInputValue = number | ''

function formatQuotaInputValue(value: QuotaInputValue): string {
  return formatQuota(value === '' ? 0 : value)
}

type QuotaSettingsSectionProps = {
  defaultValues: QuotaFormValues
  complianceConfirmed?: boolean
}

export function QuotaSettingsSection({
  defaultValues,
  complianceConfirmed = true,
}: QuotaSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const quotaSchema = createQuotaSchema(t)
  const handleNumberChange =
    (onChange: (value: QuotaInputValue) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      const value = event.currentTarget.valueAsNumber
      onChange(Number.isNaN(value) ? '' : value)
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<QuotaFormValues>({
      resolver: zodResolver(quotaSchema) as Resolver<
        QuotaFormValues,
        unknown,
        QuotaFormValues
      >,
      defaultValues,
      onSubmit: async (data, changedFields) => {
        const rewardModeKey = 'quota_setting.inviter_reward_mode'
        const rewardPercentageKey = 'quota_setting.inviter_reward_percentage'
        const rewardChanged =
          rewardModeKey in changedFields || rewardPercentageKey in changedFields
        const changedEntries = Object.entries(changedFields).filter(
          ([key]) => key !== rewardModeKey && key !== rewardPercentageKey
        )

        const fixedRewardIndex = changedEntries.findIndex(
          ([key]) => key === 'QuotaForInviter'
        )
        if (
          rewardChanged &&
          data.quota_setting.inviter_reward_mode === 'fixed' &&
          fixedRewardIndex >= 0
        ) {
          const [fixedReward] = changedEntries.splice(fixedRewardIndex, 1)
          await updateOption.mutateAsync({
            key: fixedReward[0],
            value: fixedReward[1] as number,
          })
        }

        if (rewardChanged) {
          await updateOption.mutateAsync({
            key: 'quota_setting.inviter_reward',
            value: JSON.stringify({
              mode: data.quota_setting.inviter_reward_mode,
              percentage: data.quota_setting.inviter_reward_percentage,
            }),
          })
        }

        for (const [key, value] of changedEntries) {
          await updateOption.mutateAsync({
            key,
            value: value as string | number | boolean,
          })
        }
      },
    })
  const inviterRewardMode = useWatch({
    control: form.control,
    name: 'quota_setting.inviter_reward_mode',
  })

  return (
    <SettingsSection title={t('Quota Settings')}>
      <FormNavigationGuard when={isDirty} />

      {!complianceConfirmed ? (
        <Alert variant='destructive'>
          <AlertDescription>
            {t(
              'Non-zero invitation rewards require compliance confirmation in Payment Gateway settings.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
          />
          <FormDirtyIndicator isDirty={isDirty} />
          <SettingsFormGrid>
            <FormField
              control={form.control}
              name='QuotaForNewUser'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('New User Quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Initial quota given to new users ({{formattedQuota}})',
                      {
                        formattedQuota: formatQuotaInputValue(field.value),
                      }
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='PreConsumedQuota'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Pre-Consumed Quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Quota consumed before charging users')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <SettingsFormGridItem span='full'>
              <FormField
                control={form.control}
                name='quota_setting.inviter_reward_mode'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel id='inviter-reward-mode-label'>
                      {t('Inviter Reward Mode')}
                    </FormLabel>
                    <FormControl>
                      <ToggleGroup
                        value={[field.value]}
                        onValueChange={(value) => {
                          const nextMode = value.find(
                            (item) => item !== field.value
                          )
                          if (
                            nextMode === 'fixed' ||
                            nextMode === 'percentage'
                          ) {
                            if (nextMode === 'fixed') {
                              const percentage = Number(
                                form.getValues(
                                  'quota_setting.inviter_reward_percentage'
                                )
                              )
                              if (
                                !Number.isFinite(percentage) ||
                                percentage < 0 ||
                                percentage > 100
                              ) {
                                form.setValue(
                                  'quota_setting.inviter_reward_percentage',
                                  Number.isFinite(percentage)
                                    ? Math.min(100, Math.max(0, percentage))
                                    : 0,
                                  { shouldDirty: true, shouldValidate: true }
                                )
                              }
                            } else {
                              const fixedReward = Number(
                                form.getValues('QuotaForInviter')
                              )
                              if (
                                !Number.isFinite(fixedReward) ||
                                fixedReward < 0
                              ) {
                                form.setValue('QuotaForInviter', 0, {
                                  shouldDirty: true,
                                  shouldValidate: true,
                                })
                              }
                            }
                            field.onChange(nextMode)
                          }
                        }}
                        aria-labelledby='inviter-reward-mode-label'
                        variant='outline'
                        size='sm'
                        spacing={2}
                        className='grid w-full grid-cols-1 gap-2 sm:grid-cols-2'
                      >
                        <ToggleGroupItem
                          value='fixed'
                          className='h-auto min-h-12 w-full px-3 py-2'
                        >
                          {t('Fixed Reward')}
                        </ToggleGroupItem>
                        <ToggleGroupItem
                          value='percentage'
                          className='h-auto min-h-12 w-full px-3 py-2'
                        >
                          {t('Top-up Percentage')}
                        </ToggleGroupItem>
                      </ToggleGroup>
                    </FormControl>
                    <FormDescription>
                      {inviterRewardMode === 'percentage'
                        ? t(
                            'Reward inviters after invited users complete top-ups.'
                          )
                        : t(
                            'Grant a fixed quota reward when an invited user registers.'
                          )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SettingsFormGridItem>

            {inviterRewardMode === 'percentage' ? (
              <FormField
                control={form.control}
                name='quota_setting.inviter_reward_percentage'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Inviter Reward Percentage')}</FormLabel>
                    <InputGroup>
                      <FormControl>
                        <InputGroupInput
                          type='number'
                          min={0}
                          max={100}
                          step='0.01'
                          inputMode='decimal'
                          value={field.value ?? ''}
                          onChange={handleNumberChange(field.onChange)}
                          name={field.name}
                          onBlur={field.onBlur}
                          ref={field.ref}
                        />
                      </FormControl>
                      <InputGroupAddon align='inline-end'>%</InputGroupAddon>
                    </InputGroup>
                    <FormDescription>
                      {t(
                        "The inviter receives an extra {{percentage}}% of each invited user's credited top-up quota; the invited user's credited quota is unchanged.",
                        { percentage: field.value ?? 0 }
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : (
              <FormField
                control={form.control}
                name='QuotaForInviter'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Fixed Inviter Reward')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        value={field.value ?? ''}
                        onChange={handleNumberChange(field.onChange)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Fixed quota awarded when an invited user registers ({{formattedQuota}})',
                        {
                          formattedQuota: formatQuotaInputValue(field.value),
                        }
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name='QuotaForInvitee'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Invitee Reward')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      value={field.value ?? ''}
                      onChange={handleNumberChange(field.onChange)}
                      name={field.name}
                      onBlur={field.onBlur}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Quota given to invited users ({{formattedQuota}})', {
                      formattedQuota: formatQuotaInputValue(field.value),
                    })}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <SettingsFormGridItem span='full'>
              <FormField
                control={form.control}
                name='quota_setting.enable_free_model_pre_consume'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Pre-Consume for Free Models')}</FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, zero-cost models also pre-consume quota before final settlement.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={updateOption.isPending}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </SettingsFormGridItem>

            <FormField
              control={form.control}
              name='TopUpLink'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Top-Up Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://example.com/topup')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('External link for users to purchase quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='general_setting.docs_link'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Documentation Link')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('https://docs.example.com')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Link to your documentation site')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
