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

import { api } from '@/lib/api'
import type {
  InvoiceApiResponse,
  InvoiceApplication,
  InvoiceApplicationPayload,
  InvoiceListQuery,
  InvoicePageResponse,
  InvoiceTopUpRecord,
} from './types'

function buildInvoiceQueryParams(params: InvoiceListQuery) {
  const queryParams = new URLSearchParams({
    p: params.p.toString(),
    page_size: params.page_size.toString(),
  })
  if (params.keyword) {
    queryParams.set('keyword', params.keyword)
  }
  if (params.status) {
    queryParams.set('status', params.status)
  }
  return queryParams
}

export async function getInvoiceTopUpRecords(
  params: InvoiceListQuery
): Promise<InvoiceApiResponse<InvoicePageResponse<InvoiceTopUpRecord>>> {
  const queryParams = buildInvoiceQueryParams(params)
  const res = await api.get(`/api/user/invoices/orders?${queryParams}`, {
    skipBusinessError: true,
    disableDuplicate: true,
  })
  return res.data
}

export async function submitInvoiceApplication(
  payload: InvoiceApplicationPayload
): Promise<InvoiceApiResponse<InvoiceApplication>> {
  const res = await api.post('/api/user/invoices', payload, {
    skipBusinessError: true,
  })
  return res.data
}

export async function getUserInvoiceApplications(
  params: InvoiceListQuery
): Promise<InvoiceApiResponse<InvoicePageResponse<InvoiceApplication>>> {
  const queryParams = buildInvoiceQueryParams(params)
  const res = await api.get(`/api/user/invoices?${queryParams}`, {
    skipBusinessError: true,
    disableDuplicate: true,
  })
  return res.data
}

export async function downloadInvoicePDF(id: number): Promise<Blob> {
  const res = await api.get(`/api/user/invoices/${id}/download`, {
    responseType: 'blob',
    skipBusinessError: true,
    disableDuplicate: true,
  })
  return res.data as Blob
}

export async function getAdminInvoiceApplications(
  params: InvoiceListQuery
): Promise<InvoiceApiResponse<InvoicePageResponse<InvoiceApplication>>> {
  const queryParams = buildInvoiceQueryParams(params)
  const res = await api.get(`/api/user/invoices/admin?${queryParams}`, {
    skipBusinessError: true,
    disableDuplicate: true,
  })
  return res.data
}

export async function approveInvoiceApplication(
  id: number,
  pdf: File
): Promise<InvoiceApiResponse<InvoiceApplication>> {
  const formData = new FormData()
  formData.append('pdf', pdf)
  const res = await api.post(`/api/user/invoices/${id}/approve`, formData, {
    skipBusinessError: true,
  })
  return res.data
}

export async function rejectInvoiceApplication(
  id: number,
  reason: string
): Promise<InvoiceApiResponse<InvoiceApplication>> {
  const res = await api.post(
    `/api/user/invoices/${id}/reject`,
    { reason },
    { skipBusinessError: true }
  )
  return res.data
}
