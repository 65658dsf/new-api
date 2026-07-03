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

import { useCallback, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { type ColumnDef, type PaginationState } from '@tanstack/react-table'
import { Download, RefreshCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { downloadInvoicePDF, getUserInvoiceApplications } from '../api'
import type { InvoiceApplication } from '../types'
import { InvoiceStatusBadge } from './invoice-status-badge'

export function InvoiceApplicationsTable() {
  const { t } = useTranslation()
  const [downloadingId, setDownloadingId] = useState<number | null>(null)
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  })

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    [pagination.pageIndex, pagination.pageSize]
  )

  const applicationsQuery = useQuery({
    queryKey: ['invoices', 'applications', queryParams],
    queryFn: async () => {
      const result = await getUserInvoiceApplications(queryParams)
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return result.data ?? { items: [], total: 0 }
    },
    placeholderData: (previous) => previous,
  })

  const rows = applicationsQuery.data?.items ?? []
  const total = applicationsQuery.data?.total ?? 0

  const handleDownload = useCallback(
    async (application: InvoiceApplication) => {
      if (application.status !== 'approved' || !application.has_pdf) {
        toast.error(t('Invoice PDF is not available yet'))
        return
      }

      setDownloadingId(application.id)
      try {
        const blob = await downloadInvoicePDF(application.id)
        if (!blob || blob.size === 0) {
          toast.error(t('Download failed'))
          return
        }
        const url = URL.createObjectURL(blob)
        const link = document.createElement('a')
        link.href = url
        link.download =
          application.pdf_file_name || `invoice-${application.id}.pdf`
        document.body.append(link)
        link.click()
        link.remove()
        URL.revokeObjectURL(url)
      } catch {
        toast.error(t('Download failed'))
      } finally {
        setDownloadingId(null)
      }
    },
    [t]
  )

  const columns = useMemo<ColumnDef<InvoiceApplication>[]>(
    () => [
      {
        accessorKey: 'trade_no',
        header: t('Order Number'),
        cell: ({ row }) => (
          <div className='min-w-0'>
            <div className='truncate font-mono text-sm'>
              {row.original.trade_no}
            </div>
            <div className='text-muted-foreground truncate text-xs'>
              {row.original.title}
            </div>
          </div>
        ),
        size: 260,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'amount',
        header: t('Order Amount'),
        cell: ({ row }) => formatBillingCurrencyFromUSD(row.original.amount),
        size: 130,
      },
      {
        accessorKey: 'tax_id',
        header: t('Tax ID'),
        cell: ({ row }) => (
          <span className='font-mono text-sm'>{row.original.tax_id}</span>
        ),
        size: 190,
      },
      {
        accessorKey: 'created_at',
        header: t('Applied At'),
        cell: ({ row }) => formatTimestampToDate(row.original.created_at),
        size: 170,
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => (
          <div className='space-y-1'>
            <InvoiceStatusBadge status={row.original.status} />
            {row.original.status === 'rejected' &&
            row.original.reject_reason ? (
              <div className='text-muted-foreground max-w-64 truncate text-xs'>
                {row.original.reject_reason}
              </div>
            ) : null}
          </div>
        ),
        size: 160,
        meta: { mobileBadge: true },
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => {
          const canDownload =
            row.original.status === 'approved' && row.original.has_pdf
          return canDownload ? (
            <Button
              variant='outline'
              size='sm'
              onClick={() => void handleDownload(row.original)}
              disabled={downloadingId === row.original.id}
            >
              <Download className='size-4' />
              {downloadingId === row.original.id
                ? t('Downloading...')
                : t('Download PDF')}
            </Button>
          ) : (
            <span className='text-muted-foreground text-sm'>
              {t('Not available')}
            </span>
          )
        },
        size: 170,
        meta: { pinned: 'right' as const },
      },
    ],
    [downloadingId, handleDownload, t]
  )

  const { table } = useDataTable({
    data: rows,
    columns,
    pagination,
    totalCount: total,
    manualPagination: true,
    manualFiltering: true,
    enableRowSelection: false,
    onPaginationChange: setPagination,
    ensurePageInRange: useCallback(
      (pageCount: number) => {
        if (pageCount > 0 && pagination.pageIndex + 1 > pageCount) {
          setPagination((prev) => ({
            ...prev,
            pageIndex: Math.max(0, pageCount - 1),
          }))
        }
      },
      [pagination.pageIndex]
    ),
  })

  return (
    <div className='space-y-3'>
      <div>
        <h2 className='text-base font-semibold'>
          {t('Invoice Applications')}
        </h2>
        <p className='text-muted-foreground text-sm'>
          {t('Track invoice review status and download approved PDFs.')}
        </p>
      </div>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={applicationsQuery.isLoading}
        isFetching={applicationsQuery.isFetching}
        emptyTitle={t('No invoice applications found')}
        emptyDescription={t('Submitted invoice applications will appear here.')}
        skeletonKeyPrefix='invoice-applications-skeleton'
        applyHeaderSize
        fixedHeight={false}
        paginationInFooter={false}
        toolbar={
          <div className='flex justify-end'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void applicationsQuery.refetch()}
            >
              <RefreshCcw className='size-4' />
              {t('Refresh')}
            </Button>
          </div>
        }
      />
    </div>
  )
}
