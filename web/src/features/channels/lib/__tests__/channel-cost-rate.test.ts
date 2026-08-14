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

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form.ts'

function validForm(costRate: number) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'upstream',
    key: 'test-key',
    models: 'gpt-test',
    cost_rate: costRate,
  }
}

describe('channel cost rate', () => {
  test('defaults to one and accepts zero or values above one', () => {
    assert.equal(CHANNEL_FORM_DEFAULT_VALUES.cost_rate, 1)
    assert.equal(channelFormSchema.safeParse(validForm(0)).success, true)
    assert.equal(channelFormSchema.safeParse(validForm(1.25)).success, true)
  })

  test('rejects negative and non-finite values', () => {
    for (const value of [-0.1, Number.NaN, Number.POSITIVE_INFINITY]) {
      const result = channelFormSchema.safeParse(validForm(value))
      assert.equal(result.success, false)
      if (!result.success) {
        assert.equal(
          result.error.issues.some((issue) => issue.path[0] === 'cost_rate'),
          true
        )
      }
    }
  })

  test('rejects an empty number input instead of treating it as zero', () => {
    const result = channelFormSchema.safeParse(validForm(Number.NaN))

    assert.equal(result.success, false)
    if (!result.success) {
      assert.equal(result.error.issues[0]?.path[0], 'cost_rate')
    }
  })

  test('preserves explicit zero through edit defaults and payloads', () => {
    const channel = {
      id: 7,
      type: 1,
      name: 'upstream',
      key: '',
      models: 'gpt-test',
      group: 'default',
      cost_rate: 0,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
    } as Parameters<typeof transformChannelToFormDefaults>[0]
    const defaults = transformChannelToFormDefaults(channel)
    assert.equal(defaults.cost_rate, 0)

    const form = validForm(0)
    assert.equal(transformFormDataToCreatePayload(form).channel.cost_rate, 0)
    assert.equal(transformFormDataToUpdatePayload(form, 7).cost_rate, 0)
  })
})
