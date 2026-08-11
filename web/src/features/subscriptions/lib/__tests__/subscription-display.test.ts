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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { UserSubscription } from '../../types'
import {
  getSubscriptionDisplayStatus,
  getSubscriptionRemainingQuota,
} from '../subscription-display.ts'

function createSubscription(
  overrides: Partial<UserSubscription> = {}
): UserSubscription {
  return {
    id: 1,
    user_id: 10,
    plan_id: 20,
    status: 'active',
    start_time: 1_000,
    end_time: 3_000,
    amount_total: 1_000,
    amount_used: 250,
    ...overrides,
  }
}

describe('subscription display values', () => {
  test('treats an elapsed active database record as expired', () => {
    const subscription = createSubscription({ end_time: 1_500 })

    assert.equal(getSubscriptionDisplayStatus(subscription, 2_000), 'expired')
  })

  test('treats a zero end time as inactive like subscription billing does', () => {
    const subscription = createSubscription({ end_time: 0 })

    assert.equal(getSubscriptionDisplayStatus(subscription, 2_000), 'expired')
  })

  test('keeps cancelled records invalidated regardless of their end time', () => {
    const subscription = createSubscription({
      status: 'cancelled',
      end_time: 4_000,
    })

    assert.equal(getSubscriptionDisplayStatus(subscription, 2_000), 'cancelled')
  })

  test('returns null for unlimited quota and clamps overuse to zero', () => {
    assert.equal(
      getSubscriptionRemainingQuota(
        createSubscription({ amount_total: 0, amount_used: 50 })
      ),
      null
    )
    assert.equal(
      getSubscriptionRemainingQuota(
        createSubscription({ amount_total: 100, amount_used: 120 })
      ),
      0
    )
  })

  test('returns the exact remaining quota for a finite subscription', () => {
    assert.equal(getSubscriptionRemainingQuota(createSubscription()), 750)
  })
})
