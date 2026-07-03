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

import { CircleCheckBig, CircleEllipsis, CircleX } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/status-badge'
import type { InvoiceStatus } from '../types'

export function getInvoiceStatusLabelKey(status: InvoiceStatus) {
  if (status === 'approved') return 'Approved'
  if (status === 'rejected') return 'Rejected'
  return 'Pending Review'
}

function invoiceStatusVariant(status: InvoiceStatus) {
  if (status === 'approved') return 'success' as const
  if (status === 'rejected') return 'danger' as const
  return 'warning' as const
}

function invoiceStatusIcon(status: InvoiceStatus) {
  if (status === 'approved') return CircleCheckBig
  if (status === 'rejected') return CircleX
  return CircleEllipsis
}

export function InvoiceStatusBadge(props: { status: InvoiceStatus }) {
  const { t } = useTranslation()

  return (
    <StatusBadge
      label={t(getInvoiceStatusLabelKey(props.status))}
      icon={invoiceStatusIcon(props.status)}
      variant={invoiceStatusVariant(props.status)}
      copyable={false}
    />
  )
}
