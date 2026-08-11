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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'

import { getSubscriptionDisplayStatus } from '../lib/subscription-display'
import type { UserSubscription } from '../types'

interface SubscriptionStatusBadgeProps {
  subscription: UserSubscription
}

export function SubscriptionStatusBadge(props: SubscriptionStatusBadgeProps) {
  const { t } = useTranslation()
  // eslint-disable-next-line react-hooks/purity
  const status = getSubscriptionDisplayStatus(
    props.subscription,
    Date.now() / 1000
  )

  if (status === 'active') {
    return (
      <StatusBadge label={t('Active')} variant='success' copyable={false} />
    )
  }
  if (status === 'cancelled') {
    return (
      <StatusBadge
        label={t('Invalidated')}
        variant='neutral'
        copyable={false}
      />
    )
  }
  return <StatusBadge label={t('Expired')} variant='neutral' copyable={false} />
}
