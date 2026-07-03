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
import { CheckCircle2, RefreshCcw, Search, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import { Textarea } from '@/components/ui/textarea'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import {
  approveInvoiceApplication,
  getAdminInvoiceApplications,
  rejectInvoiceApplication,
} from '@/features/invoices/api'
import type {
  InvoiceApplication,
  InvoiceStatus,
} from '@/features/invoices/types'
import { InvoiceStatusBadge } from '@/features/invoices/components/invoice-status-badge'

const MAX_INVOICE_PDF_BYTES = 10 * 1024 * 1024

const STATUS_OPTIONS: Array<{ value: 'all' | InvoiceStatus; labelKey: string }> =
  [
    { value: 'all', labelKey: 'All Status' },
    { value: 'pending', labelKey: 'Pending Review' },
    { value: 'approved', labelKey: 'Approved' },
    { value: 'rejected', labelKey: 'Rejected' },
  ]

function applicantName(application: InvoiceApplication) {
  return (
    application.user?.display_name ||
    application.user?.username ||
    application.user?.email ||
    String(application.user_id)
  )
}

function isValidPdfFile(file: File | null) {
  if (!file) return false
  return (
    file.size <= MAX_INVOICE_PDF_BYTES &&
    (file.type === 'application/pdf' ||
      file.name.toLowerCase().endsWith('.pdf'))
  )
}

function InfoBlock(props: { primary?: string; secondary?: string }) {
  return (
    <div className='min-w-0'>
      <div className='truncate text-sm'>{props.primary || '-'}</div>
      {props.secondary ? (
        <div className='text-muted-foreground truncate text-xs'>
          {props.secondary}
        </div>
      ) : null}
    </div>
  )
}

export function InvoiceApplications() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState<'all' | InvoiceStatus>('all')
  const [rejectingApplication, setRejectingApplication] =
    useState<InvoiceApplication | null>(null)
  const [approvingApplication, setApprovingApplication] =
    useState<InvoiceApplication | null>(null)
  const [rejectReason, setRejectReason] = useState('')
  const [selectedPdf, setSelectedPdf] = useState<File | null>(null)
  const [fileError, setFileError] = useState('')
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })

  const queryParams = useMemo(
    () => ({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
      keyword: keyword.trim() || undefined,
      status: status === 'all' ? undefined : status,
    }),
    [keyword, pagination.pageIndex, pagination.pageSize, status]
  )

  const applicationsQuery = useQuery({
    queryKey: ['dashboard', 'invoice-applications', queryParams],
    queryFn: async () => {
      const result = await getAdminInvoiceApplications(queryParams)
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return result.data ?? { items: [], total: 0 }
    },
    placeholderData: (previous) => previous,
  })

  const approveMutation = useMutation({
    mutationFn: (variables: { id: number; pdf: File }) =>
      approveInvoiceApplication(variables.id, variables.pdf),
    onSuccess: async (result) => {
      if (!result.success) {
        toast.error(result.message || t('Approval failed'))
        return
      }
      toast.success(t('Invoice application approved successfully'))
      setApprovingApplication(null)
      setSelectedPdf(null)
      setFileError('')
      await queryClient.invalidateQueries({
        queryKey: ['dashboard', 'invoice-applications'],
      })
    },
    onError: () => {
      toast.error(t('Approval failed'))
    },
  })

  const rejectMutation = useMutation({
    mutationFn: (variables: { id: number; reason: string }) =>
      rejectInvoiceApplication(variables.id, variables.reason),
    onSuccess: async (result) => {
      if (!result.success) {
        toast.error(result.message || t('Rejection failed'))
        return
      }
      toast.success(t('Invoice application rejected successfully'))
      setRejectingApplication(null)
      setRejectReason('')
      await queryClient.invalidateQueries({
        queryKey: ['dashboard', 'invoice-applications'],
      })
    },
    onError: () => {
      toast.error(t('Rejection failed'))
    },
  })

  const rows = applicationsQuery.data?.items ?? []
  const total = applicationsQuery.data?.total ?? 0

  const resetToFirstPage = useCallback(() => {
    setPagination((prev) =>
      prev.pageIndex === 0 ? prev : { ...prev, pageIndex: 0 }
    )
  }, [])

  const openApproveDialog = useCallback((application: InvoiceApplication) => {
    setApprovingApplication(application)
    setSelectedPdf(null)
    setFileError('')
  }, [])

  const openRejectDialog = useCallback((application: InvoiceApplication) => {
    setRejectingApplication(application)
    setRejectReason('')
  }, [])

  const handlePdfChange = (file: File | null) => {
    setSelectedPdf(file)
    if (!file) {
      setFileError(t('Please upload an invoice PDF file'))
      return
    }
    if (!file.name.toLowerCase().endsWith('.pdf')) {
      setFileError(t('Only PDF files are allowed'))
      return
    }
    if (file.size > MAX_INVOICE_PDF_BYTES) {
      setFileError(t('PDF file size cannot exceed 10MB'))
      return
    }
    if (!isValidPdfFile(file)) {
      setFileError(t('Only PDF files are allowed'))
      return
    }
    setFileError('')
  }

  const handleApprove = async () => {
    if (!approvingApplication) return
    if (!selectedPdf || !isValidPdfFile(selectedPdf)) {
      setFileError(t('Please upload an invoice PDF file'))
      return
    }
    try {
      await approveMutation.mutateAsync({
        id: approvingApplication.id,
        pdf: selectedPdf,
      })
    } catch {
      /* handled by mutation */
    }
  }

  const handleReject = async () => {
    if (!rejectingApplication) return
    const reason = rejectReason.trim()
    if (!reason) {
      toast.error(t('Please enter the rejection reason'))
      return
    }
    try {
      await rejectMutation.mutateAsync({
        id: rejectingApplication.id,
        reason,
      })
    } catch {
      /* handled by mutation */
    }
  }

  const columns = useMemo<ColumnDef<InvoiceApplication>[]>(
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
        accessorKey: 'user_id',
        header: t('Applicant'),
        cell: ({ row }) => (
          <InfoBlock
            primary={applicantName(row.original)}
            secondary={row.original.user?.email || String(row.original.user_id)}
          />
        ),
        size: 190,
        meta: { mobileTitle: true },
      },
      {
        accessorKey: 'trade_no',
        header: t('Order Number'),
        cell: ({ row }) => (
          <InfoBlock
            primary={row.original.trade_no}
            secondary={formatBillingCurrencyFromUSD(row.original.amount)}
          />
        ),
        size: 240,
      },
      {
        accessorKey: 'title',
        header: t('Invoice Header'),
        cell: ({ row }) => (
          <InfoBlock
            primary={row.original.title}
            secondary={row.original.tax_id}
          />
        ),
        size: 260,
      },
      {
        accessorKey: 'buyer_address',
        header: t('Buyer Contact'),
        cell: ({ row }) => (
          <InfoBlock
            primary={row.original.buyer_address}
            secondary={row.original.buyer_phone}
          />
        ),
        size: 220,
      },
      {
        accessorKey: 'bank_name',
        header: t('Bank Info'),
        cell: ({ row }) => (
          <InfoBlock
            primary={row.original.bank_name}
            secondary={row.original.bank_account}
          />
        ),
        size: 220,
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
              <div className='text-muted-foreground max-w-52 truncate text-xs'>
                {row.original.reject_reason}
              </div>
            ) : null}
          </div>
        ),
        size: 150,
        meta: { mobileBadge: true },
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) =>
          row.original.status === 'pending' ? (
            <div className='flex items-center gap-1.5'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => openApproveDialog(row.original)}
              >
                <CheckCircle2 className='size-4' />
                {t('Approve and Upload')}
              </Button>
              <Button
                variant='destructive'
                size='sm'
                onClick={() => openRejectDialog(row.original)}
              >
                <XCircle className='size-4' />
                {t('Reject')}
              </Button>
            </div>
          ) : (
            <span className='text-muted-foreground text-sm'>
              {t('Processed')}
            </span>
          ),
        size: 260,
        meta: { pinned: 'right' as const },
      },
    ],
    [openApproveDialog, openRejectDialog, t]
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
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={applicationsQuery.isLoading}
        isFetching={applicationsQuery.isFetching}
        emptyTitle={t('No invoice applications found')}
        emptyDescription={t('User invoice applications will appear here.')}
        skeletonKeyPrefix='dashboard-invoice-applications-skeleton'
        applyHeaderSize
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
                placeholder={t(
                  'Search by order, title, tax ID, user or email...'
                )}
                className='h-8 pl-9'
              />
            </div>
            <Select
              value={status}
              onValueChange={(value) => {
                setStatus(value as 'all' | InvoiceStatus)
                resetToFirstPage()
              }}
            >
              <SelectTrigger className='h-8 w-[150px]'>
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

      <Dialog
        open={Boolean(approvingApplication)}
        onOpenChange={(open) => !open && setApprovingApplication(null)}
        title={t('Approve and Issue Invoice')}
        description={t('Upload the issued invoice PDF to approve this request.')}
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setApprovingApplication(null)}
              disabled={approveMutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              onClick={() => void handleApprove()}
              disabled={approveMutation.isPending}
            >
              {approveMutation.isPending ? t('Uploading...') : t('Submit')}
            </Button>
          </>
        }
      >
        <div className='space-y-3'>
          <InfoBlock
            primary={approvingApplication?.trade_no}
            secondary={approvingApplication?.title}
          />
          <Input
            type='file'
            accept='application/pdf,.pdf'
            onChange={(event) =>
              handlePdfChange(event.target.files?.item(0) ?? null)
            }
          />
          <p className='text-muted-foreground text-sm'>
            {t('Only PDF files up to 10MB are allowed.')}
          </p>
          {fileError ? (
            <p className='text-destructive text-sm'>{fileError}</p>
          ) : null}
        </div>
      </Dialog>

      <Dialog
        open={Boolean(rejectingApplication)}
        onOpenChange={(open) => !open && setRejectingApplication(null)}
        title={t('Reject Invoice Application')}
        description={t('Enter a clear reason so the user can correct it.')}
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setRejectingApplication(null)}
              disabled={rejectMutation.isPending}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              variant='destructive'
              onClick={() => void handleReject()}
              disabled={rejectMutation.isPending}
            >
              {rejectMutation.isPending ? t('Processing...') : t('Reject')}
            </Button>
          </>
        }
      >
        <div className='space-y-3'>
          <InfoBlock
            primary={rejectingApplication?.trade_no}
            secondary={rejectingApplication?.title}
          />
          <Textarea
            value={rejectReason}
            onChange={(event) => setRejectReason(event.target.value)}
            placeholder={t('Enter rejection reason')}
            className='min-h-28 resize-none'
          />
        </div>
      </Dialog>
    </>
  )
}
