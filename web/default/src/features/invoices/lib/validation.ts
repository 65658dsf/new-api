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

const taxIdPattern = /^(?:[0-9A-Z]{18}|[0-9A-Z]{15}|[0-9A-Z]{17}|[0-9A-Z]{20})$/
const mobilePhonePattern = /^1[3-9]\d{9}$/
const landlinePattern = /^0\d{2,3}-?\d{7,8}(?:-\d{1,6})?$/

export const INVOICE_APPLICATION_DEFAULT_VALUES = {
  trade_no: '',
  title: '',
  tax_id: '',
  buyer_address: '',
  buyer_phone: '',
  bank_name: '',
  bank_account: '',
}

export function getInvoiceApplicationSchema(t: TFunction) {
  return z.object({
    trade_no: z.string().trim().min(1, t('Order number is required')),
    title: z.string().trim().min(1, t('Please enter the full invoice title')),
    tax_id: z
      .string()
      .trim()
      .transform((value) => value.toUpperCase())
      .refine((value) => value.length > 0, {
        message: t('Please enter the tax identification number'),
      })
      .refine((value) => taxIdPattern.test(value), {
        message: t('Please enter a valid tax identification number'),
      }),
    buyer_address: z.string().trim(),
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
    bank_name: z.string().trim(),
    bank_account: z.string().trim(),
  })
}

export type InvoiceApplicationFormValues = z.infer<
  ReturnType<typeof getInvoiceApplicationSchema>
>
