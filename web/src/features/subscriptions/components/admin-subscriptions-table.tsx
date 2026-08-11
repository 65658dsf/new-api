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
import { useQuery } from '@tanstack/react-query'
import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
} from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'

import { getAdminSubscriptions } from '../api'
import { useAdminSubscriptionsColumns } from './admin-subscriptions-columns'

export function AdminSubscriptionsTable() {
  const { t } = useTranslation()
  const columns = useAdminSubscriptionsColumns()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: isMobile ? 10 : 20,
  })
  const [globalFilter, setGlobalFilter] = useState('')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
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
  )
}
