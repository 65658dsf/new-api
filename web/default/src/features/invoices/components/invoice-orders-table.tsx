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
import { CircleCheckBig, FilePlus2, RefreshCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import {
  getInvoiceTopUpRecords,
  submitInvoiceApplication,
} from '../api'
import type {
  InvoiceApplicationPayload,
  InvoiceTopUpRecord,
} from '../types'
import { InvoiceApplicationDialog } from './invoice-application-dialog'

export function InvoiceOrdersTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [selectedOrder, setSelectedOrder] = useState<InvoiceTopUpRecord | null>(
    null
  )
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  })

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      keyword: keyword.trim() || undefined,
    }),
    [keyword, pagination.pageIndex, pagination.pageSize]
  )

  const ordersQuery = useQuery({
    queryKey: ['invoices', 'orders', queryParams],
    queryFn: async () => {
      const result = await getInvoiceTopUpRecords(queryParams)
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return result.data ?? { items: [], total: 0 }
    },
    placeholderData: (previous) => previous,
  })

  const submitMutation = useMutation({
    mutationFn: (payload: InvoiceApplicationPayload) =>
      submitInvoiceApplication(payload),
    onSuccess: async (result) => {
      if (!result.success) {
        toast.error(result.message || t('Submit failed'))
        return
      }
      toast.success(t('Invoice application submitted successfully'))
      setSelectedOrder(null)
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

  const rows = ordersQuery.data?.items ?? []
  const total = ordersQuery.data?.total ?? 0

  const columns = useMemo<ColumnDef<InvoiceTopUpRecord>[]>(
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
              {formatTimestampToDate(row.original.create_time)}
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
        size: 140,
      },
      {
        accessorKey: 'money',
        header: t('Payment Amount'),
        cell: ({ row }) => formatBillingCurrencyFromUSD(row.original.money),
        size: 140,
      },
      {
        accessorKey: 'status',
        header: t('Order Status'),
        cell: ({ row }) => (
          <StatusBadge
            label={t(row.original.status)}
            icon={CircleCheckBig}
            variant='success'
            copyable={false}
          />
        ),
        size: 140,
        meta: { mobileBadge: true },
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => {
          const disabled = row.original.invoice_applied
          return (
            <Button
              variant={disabled ? 'outline' : 'default'}
              size='sm'
              onClick={() => setSelectedOrder(row.original)}
              disabled={disabled}
            >
              <FilePlus2 className='size-4' />
              {disabled ? t('Applied') : t('Apply for Invoice')}
            </Button>
          )
        },
        size: 180,
        meta: { pinned: 'right' as const },
      },
    ],
    [t]
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

  const resetToFirstPage = useCallback(() => {
    setPagination((prev) =>
      prev.pageIndex === 0 ? prev : { ...prev, pageIndex: 0 }
    )
  }, [])

  const handleSubmit = async (values: InvoiceApplicationPayload) => {
    try {
      await submitMutation.mutateAsync(values)
    } catch {
      /* handled by mutation */
    }
  }

  return (
    <div className='space-y-3'>
      <div>
        <h2 className='text-base font-semibold'>{t('Recharge Orders')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t('Paid recharge orders that can be used for invoice applications.')}
        </p>
      </div>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={ordersQuery.isLoading}
        isFetching={ordersQuery.isFetching}
        emptyTitle={t('No invoiceable orders found')}
        emptyDescription={t('Paid recharge orders will appear here.')}
        skeletonKeyPrefix='invoice-orders-skeleton'
        applyHeaderSize
        fixedHeight={false}
        paginationInFooter={false}
        toolbar={
          <div className='flex flex-wrap items-center gap-2'>
            <div className='relative min-w-0 flex-1'>
              <Search className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
              <Input
                value={keyword}
                onChange={(event) => {
                  setKeyword(event.target.value)
                  resetToFirstPage()
                }}
                placeholder={t('Search by order number...')}
                className='h-8 pl-9'
              />
            </div>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void ordersQuery.refetch()}
            >
              <RefreshCcw className='size-4' />
              {t('Refresh')}
            </Button>
          </div>
        }
      />
      <InvoiceApplicationDialog
        open={Boolean(selectedOrder)}
        order={selectedOrder}
        isSubmitting={submitMutation.isPending}
        onOpenChange={(open) => !open && setSelectedOrder(null)}
        onSubmit={handleSubmit}
      />
    </div>
  )
}
