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
import type { Table } from '@tanstack/react-table'
import { ArrowUp, ChevronDown, Loader2, PowerOff, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthStore } from '@/stores/auth-store'

import { deleteUser, manageUser } from '../api'
import { USER_ROLE, isUserDeleted } from '../constants'
import type { ManageUserAction, User } from '../types'
import { useUsers } from './users-provider'

type BulkUserAction = Extract<
  ManageUserAction,
  'disable' | 'delete' | 'promote'
>

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const currentUserRole = useAuthStore((state) => state.auth.user?.role ?? 0)
  const [pendingUsers, setPendingUsers] = useState<User[]>([])
  const [pendingAction, setPendingAction] = useState<BulkUserAction | null>(
    null
  )
  const [pendingTotal, setPendingTotal] = useState(0)
  const [processingAction, setProcessingAction] =
    useState<BulkUserAction | null>(null)

  const selectedUsers = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original)

  const clearPendingAction = () => {
    setPendingAction(null)
    setPendingUsers([])
    setPendingTotal(0)
  }

  const handleBulkAction = async (
    action: BulkUserAction,
    users: User[],
    totalCount = users.length
  ) => {
    if (users.length === 0) return

    setProcessingAction(action)
    const results = await Promise.allSettled(
      users.map((user) =>
        action === 'delete' ? deleteUser(user.id) : manageUser(user.id, action)
      )
    )
    const successful = results.filter(
      (result) => result.status === 'fulfilled' && result.value.success
    ).length
    const failed = totalCount - successful

    if (successful > 0) {
      toast.success(
        t('Processed {{success}} of {{total}} selected users.', {
          success: successful,
          total: totalCount,
        })
      )
      triggerRefresh()
    }
    if (failed > 0) {
      toast.error(
        t('Failed to process {{count}} selected user(s).', { count: failed })
      )
    }

    table.resetRowSelection()
    setProcessingAction(null)
    clearPendingAction()
  }

  const requestBulkAction = (action: BulkUserAction) => {
    if (selectedUsers.length === 0 || processingAction) return

    const actionUsers = selectedUsers.filter((user) => {
      if (action === 'promote') {
        return (
          !isUserDeleted(user) &&
          currentUserRole === USER_ROLE.ROOT &&
          user.role < USER_ROLE.ADMIN
        )
      }

      return (
        !isUserDeleted(user) &&
        user.role !== USER_ROLE.ROOT &&
        (currentUserRole === USER_ROLE.ROOT || currentUserRole > user.role)
      )
    })

    if (actionUsers.length === 0) return

    if (action === 'delete') {
      setPendingUsers(actionUsers)
      setPendingTotal(selectedUsers.length)
      setPendingAction(action)
      return
    }

    void handleBulkAction(action, actionUsers, selectedUsers.length)
  }

  const canDisable = selectedUsers.some(
    (user) =>
      !isUserDeleted(user) &&
      user.role !== USER_ROLE.ROOT &&
      (currentUserRole === USER_ROLE.ROOT || currentUserRole > user.role)
  )
  const canPromote = selectedUsers.some(
    (user) =>
      !isUserDeleted(user) &&
      currentUserRole === USER_ROLE.ROOT &&
      user.role < USER_ROLE.ADMIN
  )
  const canDelete = canDisable

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant='outline'
                size='sm'
                disabled={processingAction !== null}
                aria-label={t('Operation')}
              />
            }
          >
            {processingAction ? (
              <Loader2 className='animate-spin' />
            ) : (
              <span>{t('Operation')}</span>
            )}
            <ChevronDown />
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-40'>
            <DropdownMenuItem
              onClick={() => requestBulkAction('disable')}
              disabled={processingAction !== null || !canDisable}
            >
              {t('Disable')}
              <DropdownMenuShortcut>
                <PowerOff />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => requestBulkAction('promote')}
              disabled={processingAction !== null || !canPromote}
            >
              {t('Promote')}
              <DropdownMenuShortcut>
                <ArrowUp />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => requestBulkAction('delete')}
              disabled={processingAction !== null || !canDelete}
              className='text-destructive focus:text-destructive'
            >
              {t('Delete')}
              <DropdownMenuShortcut>
                <Trash2 />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </BulkActionsToolbar>

      <ConfirmDialog
        open={pendingAction === 'delete'}
        onOpenChange={(open) => {
          if (!open && !processingAction) clearPendingAction()
        }}
        title={t('Confirm Action')}
        desc={t(
          'Delete {{count}} selected user(s)? This action cannot be undone.',
          { count: pendingTotal }
        )}
        confirmText={processingAction ? t('Processing...') : t('Delete')}
        destructive
        isLoading={processingAction === 'delete'}
        handleConfirm={() => {
          if (pendingAction) {
            void handleBulkAction(pendingAction, pendingUsers, pendingTotal)
          }
        }}
      />
    </>
  )
}
