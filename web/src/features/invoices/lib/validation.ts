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

import type { TFunction } from 'i18next'
import { z } from 'zod'

const taxIdPattern = /^(?:[0-9A-Z]{18}|\d{15,20})$/
const personalIdPattern = /^\d{17}[\dX]$/
const mobilePhonePattern = /^1[3-9]\d{9}$/
const landlinePattern = /^0\d{2,3}-?\d{7,8}(?:-\d{1,6})?$/
const bankAccountPattern = /^\d{8,32}$/

export const INVOICE_APPLICATION_DEFAULT_VALUES = {
  trade_no: '',
  buyer_type: 'company' as const,
  title: '',
  tax_id: '',
  buyer_address: '',
  buyer_phone: '',
  bank_name: '',
  bank_account: '',
  recipient_email: '',
}

export function getInvoiceApplicationSchema(t: TFunction) {
  return z
    .object({
      trade_no: z.string().trim().min(1, t('Order number is required')),
      buyer_type: z.enum(['individual', 'company']),
      title: z
        .string()
        .trim()
        .min(1, t('Please enter the full invoice title'))
        .max(50, t('Invoice title cannot exceed 50 characters')),
      tax_id: z
        .string()
        .trim()
        .transform((value) => value.toUpperCase())
        .refine((value) => value.length > 0, {
          message: t('Please enter the tax identification number'),
        }),
      buyer_address: z
        .string()
        .trim()
        .max(255, t('Buyer address cannot exceed 255 characters')),
      buyer_phone: z
        .string()
        .trim()
        .refine(
          (value) => {
            if (!value) return true
            const normalized = value.replaceAll(' ', '')
            return (
              mobilePhonePattern.test(normalized) ||
              landlinePattern.test(normalized)
            )
          },
          { message: t('Please enter a valid phone number') }
        ),
      bank_name: z
        .string()
        .trim()
        .max(255, t('Bank name cannot exceed 255 characters')),
      bank_account: z
        .string()
        .trim()
        .transform((value) => value.replaceAll(/[\s-]/g, ''))
        .refine((value) => !value || bankAccountPattern.test(value), {
          message: t('Bank account must contain 8 to 32 digits'),
        }),
      recipient_email: z
        .string()
        .trim()
        .min(1, t('Please enter a valid email address'))
        .max(254, t('Please enter a valid email address'))
        .email(t('Please enter a valid email address')),
    })
    .superRefine((value, ctx) => {
      const pattern =
        value.buyer_type === 'individual' ? personalIdPattern : taxIdPattern
      if (!pattern.test(value.tax_id)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['tax_id'],
          message:
            value.buyer_type === 'individual'
              ? t('Please enter a valid resident identity card number')
              : t('Please enter a valid tax identification number'),
        })
      }
    })
}

export type InvoiceApplicationFormValues = z.infer<
  ReturnType<typeof getInvoiceApplicationSchema>
>
