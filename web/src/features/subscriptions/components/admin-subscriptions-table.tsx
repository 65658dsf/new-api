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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
} from '@tanstack/react-table'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'

import {
  deleteUserSubscription,
  getAdminSubscriptions,
  resetUserSubscription,
} from '../api'
import type { AdminSubscriptionRecord } from '../types'
import { useAdminSubscriptionsColumns } from './admin-subscriptions-columns'
import { AdminSubscriptionEditDrawer } from './dialogs/admin-subscription-edit-drawer'

export function AdminSubscriptionsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [editRecord, setEditRecord] = useState<AdminSubscriptionRecord | null>(
    null
  )
  const [resetRecord, setResetRecord] =
    useState<AdminSubscriptionRecord | null>(null)
  const [deleteRecord, setDeleteRecord] =
    useState<AdminSubscriptionRecord | null>(null)
  const [actionLoading, setActionLoading] = useState(false)
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: isMobile ? 10 : 20,
  })
  const [globalFilter, setGlobalFilter] = useState('')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])

  const refreshSubscriptions = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['admin-subscriptions'] })
  }, [queryClient])

  const handleReset = useCallback(async () => {
    if (!resetRecord) return
    setActionLoading(true)
    try {
      const result = await resetUserSubscription(resetRecord.subscription.id, {
        advance_reset_time: true,
      })
      if (!result.success) {
        toast.error(result.message || t('Operation failed'))
        return
      }
      toast.success(t('Reset completed'))
      setResetRecord(null)
      refreshSubscriptions()
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setActionLoading(false)
    }
  }, [refreshSubscriptions, resetRecord, t])

  const handleDelete = useCallback(async () => {
    if (!deleteRecord) return
    setActionLoading(true)
    try {
      const result = await deleteUserSubscription(deleteRecord.subscription.id)
      if (!result.success) {
        toast.error(result.message || t('Operation failed'))
        return
      }
      toast.success(t('Deleted successfully'))
      setDeleteRecord(null)
      refreshSubscriptions()
    } catch {
      toast.error(t('Request failed'))
    } finally {
      setActionLoading(false)
    }
  }, [deleteRecord, refreshSubscriptions, t])

  const handleEditAction = useCallback(
    (record: AdminSubscriptionRecord) => setEditRecord(record),
    []
  )
  const handleResetAction = useCallback(
    (record: AdminSubscriptionRecord) => setResetRecord(record),
    []
  )
  const handleDeleteAction = useCallback(
    (record: AdminSubscriptionRecord) => setDeleteRecord(record),
    []
  )
  const columns = useAdminSubscriptionsColumns({
    onEdit: handleEditAction,
    onReset: handleResetAction,
    onDelete: handleDeleteAction,
  })
  const statusFilter =
    (
      columnFilters.find((filter) => filter.id === 'status')?.value as
        | string[]
        | undefined
    )?.[0] ?? ''

  const handleGlobalFilterChange: OnChangeFn<string> = (updater) => {
    const next = typeof updater === 'function' ? updater(globalFilter) : updater
    setGlobalFilter(next)
    setPagination((previous) => ({ ...previous, pageIndex: 0 }))
  }

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (
    updater
  ) => {
    setColumnFilters((previous) =>
      typeof updater === 'function' ? updater(previous) : updater
    )
    setPagination((previous) => ({ ...previous, pageIndex: 0 }))
  }

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'admin-subscriptions',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter,
    ],
    queryFn: async () => {
      const result = await getAdminSubscriptions({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter.trim() || undefined,
        status: statusFilter || undefined,
      })
      if (!result.success) {
        toast.error(result.message || t('Loading failed'))
        return { items: [], total: 0 }
      }
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items || [],
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onColumnFiltersChange: handleColumnFiltersChange,
    onGlobalFilterChange: handleGlobalFilterChange,
    onPaginationChange: setPagination,
    manualFiltering: true,
    manualPagination: true,
    totalCount: data?.total || 0,
    initialColumnVisibility: {
      billing_group: false,
      created_at: false,
      source: false,
    },
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No subscription records')}
        emptyDescription={t('No subscriptions have been activated yet.')}
        skeletonKeyPrefix='admin-subscriptions-skeleton'
        applyHeaderSize
        toolbarProps={{
          searchPlaceholder: t('Filter by user, plan, or ID...'),
          searchDebounceMs: 500,
          filters: [
            {
              columnId: 'status',
              title: t('Status'),
              singleSelect: true,
              options: [
                { label: t('Active'), value: 'active' },
                { label: t('Expired'), value: 'expired' },
                { label: t('Invalidated'), value: 'cancelled' },
              ],
            },
          ],
        }}
      />

      <AdminSubscriptionEditDrawer
        open={editRecord !== null}
        record={editRecord}
        onOpenChange={(open) => !open && setEditRecord(null)}
        onSuccess={refreshSubscriptions}
      />

      <ConfirmDialog
        open={resetRecord !== null}
        onOpenChange={(open) => !open && !actionLoading && setResetRecord(null)}
        title={t('Reset subscription quota')}
        desc={t(
          "Reset this subscription's used quota to zero? Its next reset time will follow this subscription's settings."
        )}
        confirmText={t('Reset quota')}
        handleConfirm={handleReset}
        isLoading={actionLoading}
      />

      <ConfirmDialog
        open={deleteRecord !== null}
        onOpenChange={(open) =>
          !open && !actionLoading && setDeleteRecord(null)
        }
        title={t('Confirm delete')}
        desc={t(
          'This removes the subscription from normal views but keeps its billing record for pending settlement. Continue?'
        )}
        confirmText={t('Delete')}
        handleConfirm={handleDelete}
        destructive
        isLoading={actionLoading}
      />
    </>
  )
}
