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
import {
  Delete02Icon,
  PencilEdit01Icon,
  Refresh01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { GroupBadge } from '@/components/group-badge'
import { TableId } from '@/components/table-id'
import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { formatQuota } from '@/lib/format'

import { formatTimestamp } from '../lib'
import { getSubscriptionRemainingQuota } from '../lib/subscription-display'
import type { AdminSubscriptionRecord } from '../types'
import { SubscriptionStatusBadge } from './subscription-status-badge'

interface AdminSubscriptionColumnActions {
  onEdit: (record: AdminSubscriptionRecord) => void
  onReset: (record: AdminSubscriptionRecord) => void
  onDelete: (record: AdminSubscriptionRecord) => void
}

export function useAdminSubscriptionsColumns(
  actions: AdminSubscriptionColumnActions
): ColumnDef<AdminSubscriptionRecord>[] {
  const { t } = useTranslation()
  const { onDelete, onEdit, onReset } = actions

  return useMemo(
    (): ColumnDef<AdminSubscriptionRecord>[] => [
      {
        accessorFn: (row) => row.subscription.id,
        id: 'id',
        header: t('ID'),
        meta: { mobileHidden: true },
        cell: ({ row }) => <TableId value={row.original.subscription.id} />,
        size: 64,
      },
      {
        accessorFn: (row) => row.user?.username || row.subscription.user_id,
        id: 'user',
        header: t('Subscriber'),
        meta: { mobileTitle: true },
        cell: ({ row }) => {
          const record = row.original
          const user = record.user
          const primaryName =
            user?.display_name ||
            user?.username ||
            `#${record.subscription.user_id}`
          const showUsername =
            user?.display_name && user.username !== user.display_name
          const userDetails = [
            showUsername ? `@${user.username}` : null,
            user?.email || null,
            `${t('User ID')}: ${record.subscription.user_id}`,
          ]
            .filter(Boolean)
            .join(' / ')

          return (
            <div className='max-w-full min-w-0'>
              <div className='truncate font-medium'>{primaryName}</div>
              <div className='text-muted-foreground truncate text-xs'>
                {userDetails}
              </div>
            </div>
          )
        },
        size: 190,
      },
      {
        accessorFn: (row) => row.plan?.title || row.subscription.plan_id,
        id: 'plan',
        header: t('Plan'),
        cell: ({ row }) => (
          <div className='max-w-full min-w-0'>
            <div className='truncate font-medium'>
              {row.original.plan?.title ||
                `#${row.original.subscription.plan_id}`}
            </div>
            <div className='text-muted-foreground text-xs'>
              ID: {row.original.subscription.plan_id}
            </div>
          </div>
        ),
        size: 180,
      },
      {
        accessorFn: (row) => row.subscription.status,
        id: 'status',
        header: t('Status'),
        meta: { mobileBadge: true },
        cell: ({ row }) => (
          <SubscriptionStatusBadge subscription={row.original.subscription} />
        ),
        size: 100,
      },
      {
        id: 'remaining_quota',
        header: t('Remaining quota'),
        cell: ({ row }) => {
          const remaining = getSubscriptionRemainingQuota(
            row.original.subscription
          )
          return (
            <span className='font-medium tabular-nums'>
              {remaining === null ? t('Unlimited') : formatQuota(remaining)}
            </span>
          )
        },
        size: 132,
      },
      {
        accessorFn: (row) => row.subscription.amount_total,
        id: 'total_quota',
        header: t('Total Quota'),
        cell: ({ row }) => {
          const total = row.original.subscription.amount_total
          return (
            <span className='tabular-nums'>
              {total <= 0 ? t('Unlimited') : formatQuota(total)}
            </span>
          )
        },
        size: 120,
      },
      {
        id: 'validity',
        header: t('Validity'),
        cell: ({ row }) => (
          <div className='text-sm tabular-nums'>
            <div>{formatTimestamp(row.original.subscription.start_time)}</div>
            <div className='text-muted-foreground'>
              {formatTimestamp(row.original.subscription.end_time)}
            </div>
          </div>
        ),
        size: 170,
      },
      {
        accessorFn: (row) => row.subscription.source || '-',
        id: 'source',
        header: t('Source'),
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {row.original.subscription.source || '-'}
          </span>
        ),
        size: 96,
      },
      {
        accessorFn: (row) => row.subscription.billing_group || '',
        id: 'billing_group',
        header: t('Subscription Quota Group'),
        cell: ({ row }) => {
          const group = row.original.subscription.billing_group
          return group ? (
            <GroupBadge group={group} />
          ) : (
            <span className='text-muted-foreground'>{t('All Groups')}</span>
          )
        },
        size: 150,
      },
      {
        accessorFn: (row) => row.subscription.created_at || 0,
        id: 'created_at',
        header: t('Created At'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <span className='text-muted-foreground tabular-nums'>
            {formatTimestamp(row.original.subscription.created_at || 0)}
          </span>
        ),
        size: 170,
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => {
          const record = row.original
          const subscription = record.subscription
          const isActive =
            subscription.status === 'active' &&
            subscription.start_time > 0 &&
            subscription.start_time <= Date.now() / 1000 &&
            subscription.end_time > Date.now() / 1000
          return (
            <DataTableRowActionMenu
              ariaLabel={t('Actions')}
              triggerLabel={t('Actions')}
            >
              <DropdownMenuGroup>
                <DropdownMenuItem
                  disabled={!isActive}
                  onClick={() => onReset(record)}
                >
                  <HugeiconsIcon icon={Refresh01Icon} strokeWidth={2} />
                  {t('Reset quota')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onEdit(record)}>
                  <HugeiconsIcon icon={PencilEdit01Icon} strokeWidth={2} />
                  {t('Modify subscription')}
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem
                  variant='destructive'
                  onClick={() => onDelete(record)}
                >
                  <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
                  {t('Delete subscription')}
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DataTableRowActionMenu>
          )
        },
        size: 112,
        meta: { pinned: 'right' as const },
      },
    ],
    [onDelete, onEdit, onReset, t]
  )
}
