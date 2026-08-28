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
import type { UserSubscription } from '../types'

export type SubscriptionDisplayStatus = 'active' | 'expired' | 'cancelled'

export function getSubscriptionDisplayStatus(
  subscription: UserSubscription,
  now: number
): SubscriptionDisplayStatus {
  if (subscription.status === 'cancelled') return 'cancelled'
  if (
    subscription.status === 'active' &&
    subscription.start_time > 0 &&
    subscription.start_time <= now &&
    subscription.end_time > now
  ) {
    return 'active'
  }
  return 'expired'
}

export function getSubscriptionRemainingQuota(
  subscription: UserSubscription
): number | null {
  if (subscription.amount_total <= 0) return null
  return Math.max(subscription.amount_total - subscription.amount_used, 0)
}
