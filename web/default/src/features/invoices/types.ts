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

export type InvoiceStatus = 'pending' | 'approved' | 'rejected' | 'completed'

export type InvoiceBuyerType = 'individual' | 'company'

export type InvoiceTopUpStatus = 'success' | 'pending' | 'failed' | 'expired'

export interface InvoiceUserInfo {
  id: number
  username: string
  display_name: string
  email: string
}

export interface InvoiceTopUpRecord {
  id: number
  user_id: number
  amount: number
  money: number
  trade_no: string
  payment_method: string
  payment_provider?: string
  create_time: number
  complete_time?: number
  status: InvoiceTopUpStatus
  invoice_application_id?: number
  external_invoice_id?: number
  invoice_status?: InvoiceStatus
  invoice_applied: boolean
}

export interface InvoiceApplicationOrder {
  id?: number
  invoice_application_id?: number
  user_id?: number
  topup_id?: number
  trade_no: string
  amount: number
  money: number
  created_at?: number
}

export interface InvoiceApplication {
  id: number
  user_id: number
  topup_id: number
  trade_no: string
  amount: number
  money: number
  buyer_type: InvoiceBuyerType
  title: string
  tax_id: string
  buyer_address: string
  buyer_phone: string
  bank_name: string
  bank_account: string
  recipient_email: string
  external_invoice_id?: number
  review_note?: string
  status: InvoiceStatus
  reject_reason?: string
  pdf_file_name?: string
  created_at: number
  updated_at: number
  handled_at?: number
  handler_id?: number
  user?: InvoiceUserInfo
  orders?: InvoiceApplicationOrder[]
  has_pdf?: boolean
}

export interface InvoiceApplicationPayload {
  trade_no: string
  trade_nos?: string[]
  buyer_type: InvoiceBuyerType
  title: string
  tax_id: string
  buyer_address: string
  buyer_phone: string
  bank_name: string
  bank_account: string
  recipient_email: string
}

export interface InvoiceListQuery {
  p: number
  page_size: number
  keyword?: string
  status?: InvoiceStatus
}

export interface InvoicePendingCount {
  pending_count: number
}

export interface InvoicePageResponse<T> {
  items: T[]
  total: number
}

export interface InvoiceApiResponse<T = unknown> {
  success?: boolean
  message?: string
  data?: T
}
