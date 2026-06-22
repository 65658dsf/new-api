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
import {
  CircleCheckBig,
  CircleEllipsis,
  CircleX,
  Eye,
  Search,
  RefreshCcw,
  UserRound,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { StatusBadge } from '@/components/status-badge'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { completeTopUpOrder, getTopUpOrders } from '../../api'
import type {
  TopUpOrdersQuery,
  TopUpRecord,
  TopUpStatus,
} from '../../types'

const STATUS_OPTIONS: Array<{ value: 'all' | TopUpStatus; labelKey: string }> = [
  { value: 'all', labelKey: 'All Status' },
  { value: 'success', labelKey: 'Success' },
  { value: 'pending', labelKey: 'Pending' },
  { value: 'failed', labelKey: 'Failed' },
  { value: 'expired', labelKey: 'Expired' },
]

const PAYMENT_METHOD_OPTIONS = [
  { value: 'all', labelKey: 'All Payment Methods' },
  { value: 'stripe', labelKey: 'Stripe' },
  { value: 'creem', labelKey: 'Creem' },
  { value: 'waffo', labelKey: 'Waffo' },
  { value: 'waffo_pancake', labelKey: 'Waffo Pancake' },
  { value: 'balance', labelKey: 'Balance' },
] as const

function statusVariant(status: TopUpStatus) {
  if (status === 'success') return 'success' as const
  if (status === 'pending') return 'warning' as const
  if (status === 'failed') return 'danger' as const
  return 'neutral' as const
}

function statusIcon(status: TopUpStatus) {
  if (status === 'success') return CircleCheckBig
  if (status === 'pending') return CircleEllipsis
  if (status === 'failed') return CircleX
  return CircleEllipsis
}

function statusTimeline(status: TopUpStatus) {
  if (status === 'success') {
    return [
      {
        key: 'ORDER_CREATED',
        label: 'Order created',
        variant: 'neutral' as const,
      },
      {
        key: 'ORDER_PAID',
        label: 'Payment confirmed',
        variant: 'success' as const,
      },
    ]
  }

  if (status === 'pending') {
    return [
      {
        key: 'ORDER_CREATED',
        label: 'Order created',
        variant: 'neutral' as const,
      },
      {
        key: 'ORDER_PENDING',
        label: 'Waiting for payment',
        variant: 'warning' as const,
      },
    ]
  }

  if (status === 'failed') {
    return [
      {
        key: 'ORDER_CREATED',
        label: 'Order created',
        variant: 'neutral' as const,
      },
      {
        key: 'ORDER_FAILED',
        label: 'Payment failed',
        variant: 'danger' as const,
      },
    ]
  }

  return [
    {
      key: 'ORDER_CREATED',
      label: 'Order created',
      variant: 'neutral' as const,
    },
    {
      key: 'ORDER_EXPIRED',
      label: 'Order expired',
      variant: 'danger' as const,
    },
  ]
}

function OrderDetailDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: TopUpRecord | null
  onComplete: (order: TopUpRecord) => void
  completing: boolean
}) {
  const { t } = useTranslation()
  const order = props.order

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Order Details')}
      description={t('View payment order details and status timeline')}
      contentClassName='sm:max-w-3xl'
      bodyClassName='space-y-4'
    >
      {!order ? null : (
        <div className='space-y-4'>
          <div className='grid gap-3 sm:grid-cols-2'>
            <InfoItem label={t('Order Number')} value={order.trade_no} />
            <InfoItem
              label={t('Status')}
              value={
                <StatusBadge
                  label={t(order.status)}
                  variant={statusVariant(order.status)}
                  copyable={false}
                />
              }
            />
            <InfoItem
              label={t('User')}
              value={
                <div className='flex min-w-0 items-center gap-2'>
                  <UserRound className='text-muted-foreground size-4 shrink-0' />
                  <span className='truncate'>
                    {order.user?.display_name ||
                      order.user?.username ||
                      order.user?.email ||
                      String(order.user_id)}
                  </span>
                </div>
              }
            />
            <InfoItem
              label={t('Payment Method')}
              value={order.payment_method}
            />
            <InfoItem
              label={t('Amount')}
              value={formatBillingCurrencyFromUSD(order.amount)}
            />
            <InfoItem
              label={t('Payment')}
              value={formatBillingCurrencyFromUSD(order.money)}
            />
            <InfoItem
              label={t('Created At')}
              value={formatTimestampToDate(order.create_time)}
            />
            <InfoItem
              label={t('Completed At')}
              value={formatTimestampToDate(order.complete_time ?? 0)}
            />
          </div>

          <div className='space-y-2'>
            <div className='text-sm font-semibold'>{t('Operation Log')}</div>
            <div className='space-y-2'>
              {statusTimeline(order.status).map((item, index) => (
                <div
                  key={item.key}
                  className='bg-muted/30 flex items-center justify-between rounded-lg px-3 py-2'
                >
                  <div className='flex items-center gap-2'>
                    <StatusBadge
                      label={item.key}
                      variant={item.variant}
                      copyable={false}
                    />
                    <span className='text-sm'>{t(item.label)}</span>
                  </div>
                  <span className='text-muted-foreground text-xs'>
                    {index === 0
                      ? formatTimestampToDate(order.create_time)
                      : formatTimestampToDate(
                          order.complete_time || order.create_time
                        )}
                  </span>
                </div>
              ))}
            </div>
          </div>

          {order.status === 'pending' ? (
            <div className='flex justify-end gap-2'>
              <Button
                onClick={() => props.onComplete(order)}
                disabled={props.completing}
              >
                {props.completing ? t('Processing...') : t('Complete Order')}
              </Button>
            </div>
          ) : null}
        </div>
      )}
    </Dialog>
  )
}

function InfoItem(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='space-y-1'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='text-sm font-medium'>{props.value}</div>
    </div>
  )
}

export function PaymentOrders() {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState<'all' | TopUpStatus>('all')
  const [paymentMethod, setPaymentMethod] = useState<'all' | string>('all')
  const [selectedOrder, setSelectedOrder] = useState<TopUpRecord | null>(null)
  const [confirmOrder, setConfirmOrder] = useState<TopUpRecord | null>(null)
  const [completingTradeNo, setCompletingTradeNo] = useState<string | null>(null)
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })

  const queryParams = useMemo<TopUpOrdersQuery>(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      keyword: keyword.trim() || undefined,
      status: status === 'all' ? undefined : status,
      payment_method: paymentMethod === 'all' ? undefined : paymentMethod,
    }),
    [keyword, pagination.pageIndex, pagination.pageSize, paymentMethod, status]
  )

  const ordersQuery = useQuery({
    queryKey: ['dashboard', 'payment-orders', queryParams],
    queryFn: async () => {
      const result = await getTopUpOrders(queryParams)
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return result.data ?? { items: [], total: 0 }
    },
    placeholderData: (previous) => previous,
  })

  const rows = ordersQuery.data?.items ?? []
  const total = ordersQuery.data?.total ?? 0

  const columns = useMemo<ColumnDef<TopUpRecord>[]>(
    () => [
      {
        accessorKey: 'id',
        header: t('ID'),
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>{row.original.id}</span>
        ),
        size: 80,
        meta: { mobileHidden: true },
      },
      {
        accessorKey: 'trade_no',
        header: t('Order Number'),
        cell: ({ row }) => (
          <div className='min-w-0'>
            <div className='truncate font-mono text-sm'>
              {row.original.trade_no}
            </div>
            <div className='text-muted-foreground truncate text-xs'>
              {row.original.user?.display_name ||
                row.original.user?.username ||
                row.original.user?.email ||
                ''}
            </div>
          </div>
        ),
        size: 240,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'amount',
        header: t('Amount'),
        cell: ({ row }) => formatBillingCurrencyFromUSD(row.original.amount),
        size: 120,
      },
      {
        accessorKey: 'money',
        header: t('Payment'),
        cell: ({ row }) => formatBillingCurrencyFromUSD(row.original.money),
        size: 120,
      },
      {
        accessorKey: 'payment_method',
        header: t('Payment Method'),
        cell: ({ row }) => row.original.payment_method,
        size: 140,
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => (
          <StatusBadge
            label={t(row.original.status)}
            icon={statusIcon(row.original.status)}
            variant={statusVariant(row.original.status)}
            copyable={false}
          />
        ),
        size: 120,
        meta: { mobileBadge: true },
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => (
          <div className='flex items-center gap-1.5'>
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => setSelectedOrder(row.original)}
            >
              <Eye className='size-4' />
            </Button>
            {row.original.status === 'pending' ? (
              <Button
                variant='outline'
                size='sm'
                onClick={() => setConfirmOrder(row.original)}
              >
                {t('Complete Order')}
              </Button>
            ) : null}
          </div>
        ),
        size: 180,
        meta: { pinned: 'right' as const },
      },
    ],
    [t]
  )

  const resetToFirstPage = useCallback(() => {
    setPagination((prev) =>
      prev.pageIndex === 0 ? prev : { ...prev, pageIndex: 0 }
    )
  }, [])

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

  const handleRefresh = () => {
    void ordersQuery.refetch()
  }

  const handleConfirmComplete = async () => {
    if (!confirmOrder || completingTradeNo) return
    const tradeNo = confirmOrder.trade_no
    setCompletingTradeNo(tradeNo)
    try {
      const result = await completeTopUpOrder(tradeNo)
      if (!result.success) {
        toast.error(result.message || t('Failed to complete order'))
        return
      }
      toast.success(t('Order completed successfully'))
      setConfirmOrder(null)
      setSelectedOrder(null)
      await ordersQuery.refetch()
    } finally {
      setCompletingTradeNo(null)
    }
  }

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={ordersQuery.isLoading}
        isFetching={ordersQuery.isFetching}
        emptyTitle={t('No billing records found')}
        emptyDescription={t('Your transaction history will appear here')}
        skeletonKeyPrefix='payment-orders-skeleton'
        applyHeaderSize
        toolbar={
          <div className='flex flex-wrap items-center gap-2'>
            <div className='relative min-w-0 flex-1'>
              <Search className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
              <Input
                value={keyword}
                onChange={(e) => {
                  setKeyword(e.target.value)
                  resetToFirstPage()
                }}
                placeholder={t('Search by order number, user or email...')}
                className='h-8 pl-9'
              />
            </div>
            <Select
              value={status}
              onValueChange={(value) => {
                setStatus(value as 'all' | TopUpStatus)
                resetToFirstPage()
              }}
            >
              <SelectTrigger className='h-8 w-[140px]'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {STATUS_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Select
              value={paymentMethod}
              onValueChange={(value) => {
                setPaymentMethod(value)
                resetToFirstPage()
              }}
            >
              <SelectTrigger className='h-8 w-[160px]'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {PAYMENT_METHOD_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Button variant='outline' size='sm' onClick={handleRefresh}>
              <RefreshCcw className='size-4' />
              {t('Refresh')}
            </Button>
          </div>
        }
      />

      <OrderDetailDialog
        open={Boolean(selectedOrder)}
        onOpenChange={(open) => !open && setSelectedOrder(null)}
        order={selectedOrder}
        onComplete={(order) => setConfirmOrder(order)}
        completing={Boolean(completingTradeNo)}
      />

      <ConfirmDialog
        open={Boolean(confirmOrder)}
        onOpenChange={(open) => !open && setConfirmOrder(null)}
        title={t('Complete the selected pending order?')}
        desc={t(
          'This will manually mark the order as paid and credit the user account. Continue only after verifying the payment externally.'
        )}
        confirmText={
          completingTradeNo ? t('Processing...') : t('Confirm Completion')
        }
        disabled={Boolean(completingTradeNo)}
        isLoading={Boolean(completingTradeNo)}
        handleConfirm={handleConfirmComplete}
      />
    </>
  )
}
