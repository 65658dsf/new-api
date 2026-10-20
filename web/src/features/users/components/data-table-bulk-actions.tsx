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
import type { RequestVerificationOptions } from '@/features/auth/secure-verification'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
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
  const { triggerRefresh, requestVerification, verificationActive } = useUsers()
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
    let successful = 0
    let attempted = 0
    let canceled = false
    try {
      for (const user of users) {
        const request: RequestVerificationOptions =
          action === 'delete'
            ? { scope: 'admin.user.delete', context: { user_id: user.id } }
            : {
                scope: 'admin.user.manage',
                context: { user_id: user.id, action },
              }
        const proof = await requestVerification({
          ...request,
          title: t('Confirm Action'),
          description: t(
            'Confirm your identity before changing the account {{username}}.',
            { username: user.username }
          ),
        })
        if (!proof) {
          canceled = true
          break
        }
        attempted++
        const result =
          action === 'delete'
            ? await deleteUser(user.id, proof.proof_token)
            : await manageUser(user.id, action, proof.proof_token)
        if (result.success) successful++
      }
    } catch (error) {
      canceled = true
      handleServerError(AuthOperationError.from(error), t('Request failed'))
    }
    const failed = canceled ? attempted - successful : totalCount - successful

    if (successful > 0) {
      toast.success(
        t('Processed {{success}} of {{total}} selected users.', {
          success: successful,
          total: canceled ? attempted : totalCount,
        })
      )
      triggerRefresh()
    }
    if (failed > 0) {
      toast.error(
        t('Failed to process {{count}} selected user(s).', { count: failed })
      )
    }

    if (!canceled || successful > 0) table.resetRowSelection()
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
                disabled={processingAction !== null || verificationActive}
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
        open={pendingAction === 'delete' && !verificationActive}
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
