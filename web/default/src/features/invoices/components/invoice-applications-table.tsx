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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type ColumnDef, type PaginationState } from '@tanstack/react-table'
import { Download, FilePenLine, RefreshCcw, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import {
  cancelInvoiceApplication,
  downloadInvoicePDF,
  getUserInvoiceApplications,
  submitInvoiceApplication,
} from '../api'
import type {
  InvoiceApplication,
  InvoiceApplicationOrder,
  InvoiceApplicationPayload,
} from '../types'
import { InvoiceApplicationDialog } from './invoice-application-dialog'
import { InvoiceStatusBadge } from './invoice-status-badge'

function invoiceOrderLines(
  application: InvoiceApplication
): InvoiceApplicationOrder[] {
  if (application.orders?.length) {
    return application.orders
  }
  return [
    {
      topup_id: application.topup_id,
      trade_no: application.trade_no,
      amount: application.amount,
      money: application.money,
    },
  ]
}

export function InvoiceApplicationsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [downloadingId, setDownloadingId] = useState<number | null>(null)
  const [editingApplication, setEditingApplication] =
    useState<InvoiceApplication | null>(null)
  const [cancelingApplication, setCancelingApplication] =
    useState<InvoiceApplication | null>(null)
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

  const submitMutation = useMutation({
    mutationFn: (payload: InvoiceApplicationPayload) =>
      submitInvoiceApplication(payload),
    onSuccess: async (result) => {
      if (!result.success) {
        toast.error(result.message || t('Submit failed'))
        return
      }
      toast.success(t('Invoice application resubmitted successfully'))
      setEditingApplication(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['invoices', 'orders'] }),
        queryClient.invalidateQueries({
          queryKey: ['invoices', 'applications'],
        }),
      ])
    },
    onError: () => {
      toast.error(t('Submit failed'))
    },
  })

  const cancelMutation = useMutation({
    mutationFn: (id: number) => cancelInvoiceApplication(id),
    onSuccess: async (result) => {
      if (!result.success) {
        toast.error(result.message || t('Cancel failed'))
        return
      }
      toast.success(t('Invoice application canceled successfully'))
      setCancelingApplication(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['invoices', 'orders'] }),
        queryClient.invalidateQueries({
          queryKey: ['invoices', 'applications'],
        }),
      ])
    },
    onError: () => {
      toast.error(t('Cancel failed'))
    },
  })

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
        cell: ({ row }) => {
          const orders = invoiceOrderLines(row.original)
          return (
            <div className='min-w-0 space-y-1'>
              {orders.slice(0, 3).map((order) => (
                <div
                  key={order.trade_no}
                  className='flex min-w-0 items-center justify-between gap-3 text-sm'
                >
                  <span className='truncate font-mono'>{order.trade_no}</span>
                  <span className='text-muted-foreground shrink-0 tabular-nums'>
                    {formatBillingCurrencyFromUSD(order.amount)}
                  </span>
                </div>
              ))}
              {orders.length > 3 ? (
                <div className='text-muted-foreground text-xs'>
                  {t('{{count}} more orders', { count: orders.length - 3 })}
                </div>
              ) : null}
              <div className='text-muted-foreground truncate text-xs'>
                {row.original.title}
              </div>
            </div>
          )
        },
        size: 320,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'amount',
        header: t('Total Amount'),
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
          const canCancel = row.original.status !== 'approved'
          if (row.original.status === 'rejected') {
            return (
              <div className='flex items-center gap-1.5'>
                <Button
                  variant='default'
                  size='sm'
                  onClick={() => setEditingApplication(row.original)}
                >
                  <FilePenLine className='size-4' />
                  {t('Edit and Resubmit')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setCancelingApplication(row.original)}
                >
                  <RotateCcw className='size-4' />
                  {t('Cancel Application')}
                </Button>
              </div>
            )
          }
          if (canDownload) {
            return (
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
            )
          }
          if (canCancel) {
            return (
              <Button
                variant='outline'
                size='sm'
                onClick={() => setCancelingApplication(row.original)}
              >
                <RotateCcw className='size-4' />
                {t('Cancel Application')}
              </Button>
            )
          }
          return (
            <span className='text-muted-foreground text-sm'>
              {t('Not available')}
            </span>
          )
        },
        size: 260,
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

  const handleResubmit = async (values: InvoiceApplicationPayload) => {
    try {
      await submitMutation.mutateAsync(values)
    } catch {
      /* handled by mutation */
    }
  }

  const handleCancel = async () => {
    if (!cancelingApplication) return
    try {
      await cancelMutation.mutateAsync(cancelingApplication.id)
    } catch {
      /* handled by mutation */
    }
  }

  return (
    <>
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
          emptyDescription={t(
            'Submitted invoice applications will appear here.'
          )}
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
      <InvoiceApplicationDialog
        open={Boolean(editingApplication)}
        application={editingApplication}
        isSubmitting={submitMutation.isPending}
        onOpenChange={(open) => !open && setEditingApplication(null)}
        onSubmit={handleResubmit}
      />
      <Dialog
        open={Boolean(cancelingApplication)}
        onOpenChange={(open) => !open && setCancelingApplication(null)}
        title={t('Cancel Invoice Application')}
        description={t(
          'After cancellation, the related orders can be selected for a new invoice application.'
        )}
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setCancelingApplication(null)}
              disabled={cancelMutation.isPending}
            >
              {t('Keep Application')}
            </Button>
            <Button
              type='button'
              variant='destructive'
              onClick={() => void handleCancel()}
              disabled={cancelMutation.isPending}
            >
              {cancelMutation.isPending
                ? t('Processing...')
                : t('Confirm Cancel')}
            </Button>
          </>
        }
      >
        <div className='space-y-2 text-sm'>
          <div className='font-medium'>{cancelingApplication?.title}</div>
          <div className='text-muted-foreground'>
            {t('This action cannot be undone.')}
          </div>
        </div>
      </Dialog>
    </>
  )
}
