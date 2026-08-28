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
import { after, afterEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'HTMLInputElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'KeyboardEvent',
  'PointerEvent',
  'MouseEvent',
  'FocusEvent',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

Object.defineProperty(domWindow, 'matchMedia', {
  configurable: true,
  value: () => ({
    matches: false,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }),
})

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { PageFooterProvider } =
  await import('@/components/layout/components/page-footer')
const { api } = await import('@/lib/api')
const { AdminSubscriptionsTable } = await import('../admin-subscriptions-table')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type ApiGet = (
  url: string,
  config?: { params?: Record<string, unknown> }
) => Promise<{ data: unknown }>
type ApiMutation = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiGet
  patch: ApiMutation
  post: ApiMutation
  delete: ApiMutation
}
type RenderedTable = {
  footer: HTMLDivElement
  host: HTMLDivElement
  root: ReturnType<typeof createRoot>
  queryClient: InstanceType<typeof QueryClient>
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPatch = apiClient.patch
const originalPost = apiClient.post
const originalDelete = apiClient.delete
let renderedTable: RenderedTable | null = null
let requests: Array<Record<string, unknown>> = []
let mutationRequests: Array<{
  method: 'patch' | 'post' | 'delete'
  url: string
  data?: unknown
}> = []

const aliceRecord = {
  subscription: {
    id: 1,
    user_id: 10,
    plan_id: 20,
    status: 'active',
    start_time: 1_000,
    end_time: Math.floor(Date.now() / 1000) + 3_600,
    amount_total: 1_000,
    amount_used: 250,
    source: 'order',
    billing_group: '',
    created_at: 1_000,
  },
  user: {
    id: 10,
    username: 'alice',
    display_name: 'Alice',
    email: 'alice@example.com',
  },
  plan: { id: 20, title: 'Starter Plan' },
}

const expiredRecord = {
  ...aliceRecord,
  subscription: {
    ...aliceRecord.subscription,
    id: 3,
    status: 'expired',
    end_time: Math.floor(Date.now() / 1000) - 3_600,
  },
  user: {
    ...aliceRecord.user,
    id: 12,
    username: 'expired',
    display_name: 'Expired User',
  },
}

const elapsedActiveRecord = {
  ...expiredRecord,
  subscription: {
    ...expiredRecord.subscription,
    status: 'active',
  },
}

const bobRecord = {
  ...aliceRecord,
  subscription: { ...aliceRecord.subscription, id: 2, user_id: 11 },
  user: {
    id: 11,
    username: 'bob',
    display_name: 'Bob',
    email: 'bob@example.com',
  },
}

function response(items: unknown[], page: number) {
  return {
    data: {
      success: true,
      data: { items, total: 21, page, page_size: 20 },
    },
  }
}

function installApiFixtures(items: unknown[] = [aliceRecord]) {
  requests = []
  mutationRequests = []
  apiClient.get = async (url, config) => {
    if (url === '/api/group') {
      return { data: { success: true, data: ['group-a', 'group-b'] } }
    }
    assert.equal(url, '/api/subscription/admin/subscriptions')
    const params = config?.params || {}
    requests.push(
      Object.fromEntries(
        Object.entries(params).filter(([, value]) => value !== undefined)
      )
    )
    if (params.keyword === 'Bob') return response([bobRecord], 1)
    return response(items, Number(params.p || 1))
  }
  apiClient.patch = async (url, data) => {
    mutationRequests.push({ method: 'patch', url, data })
    return { data: { success: true, data: {} } }
  }
  apiClient.post = async (url, data) => {
    mutationRequests.push({ method: 'post', url, data })
    return { data: { success: true, data: {} } }
  }
  apiClient.delete = async (url, data) => {
    mutationRequests.push({ method: 'delete', url, data })
    return { data: { success: true, data: {} } }
  }
}

async function waitForCondition(
  condition: () => boolean,
  failureMessage: string
): Promise<void> {
  if (condition()) return

  await new Promise<void>((resolve, reject) => {
    const observer = new MutationObserver(() => {
      if (!condition()) return
      clearTimeout(timeoutId)
      observer.disconnect()
      resolve()
    })
    const timeoutId = setTimeout(() => {
      observer.disconnect()
      reject(new Error(`${failureMessage}: ${document.body.textContent}`))
    }, 1500)

    observer.observe(document, {
      attributes: true,
      childList: true,
      characterData: true,
      subtree: true,
    })
  })
}

async function renderTable() {
  const host = document.createElement('div')
  const footer = document.createElement('div')
  document.body.append(host, footer)
  const root = createRoot(host)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  renderedTable = { footer, host, root, queryClient }

  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <PageFooterProvider container={footer}>
            <AdminSubscriptionsTable />
          </PageFooterProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )
  })
  await act(async () =>
    waitForCondition(
      () =>
        requests.length === 1 &&
        document.body.textContent?.includes('Alice') === true,
      'initial subscription page did not load'
    )
  )
}

async function changeInput(input: HTMLInputElement, value: string) {
  await act(async () => {
    const valueSetter = Object.getOwnPropertyDescriptor(
      domWindow.HTMLInputElement.prototype,
      'value'
    )?.set
    assert.ok(valueSetter)
    valueSetter.call(input, value)
    input.dispatchEvent(
      new domWindow.Event('input', { bubbles: true }) as unknown as Event
    )
  })
}

afterEach(async () => {
  apiClient.get = originalGet
  apiClient.patch = originalPatch
  apiClient.post = originalPost
  apiClient.delete = originalDelete
  if (renderedTable) {
    await act(async () => renderedTable?.root.unmount())
    renderedTable.queryClient.clear()
    renderedTable.footer.remove()
    renderedTable.host.remove()
    renderedTable = null
  }
  document.body.replaceChildren()
})

after(() => {
  domWindow.close()
})

describe('admin subscriptions table', () => {
  test('requests server pagination, search, status, and next page state', async () => {
    installApiFixtures()
    await renderTable()

    assert.deepEqual(requests[0], { p: 1, page_size: 20 })
    assert.equal(document.body.textContent?.includes('Starter Plan'), true)
    assert.equal(document.body.textContent?.includes('$0.0015'), true)

    const search = document.querySelector<HTMLInputElement>(
      'input[placeholder="Filter by user, plan, or ID..."]'
    )
    assert.ok(search)
    await changeInput(search, 'Bob')
    await act(async () =>
      waitForCondition(
        () => requests.some((params) => params.keyword === 'Bob'),
        'search request was not sent'
      )
    )
    assert.deepEqual(requests.at(-1), {
      p: 1,
      page_size: 20,
      keyword: 'Bob',
    })
    assert.equal(document.body.textContent?.includes('Bob'), true)

    const statusButton = [...document.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Status'
    )
    assert.ok(statusButton)
    await act(async () => statusButton.click())
    const activeOption = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="command-item"]'),
    ].find((item) => item.textContent?.trim() === 'Active')
    assert.ok(activeOption)
    await act(async () => activeOption.click())
    await act(async () =>
      waitForCondition(
        () => requests.some((params) => params.status === 'active'),
        'status request was not sent'
      )
    )
    assert.equal(requests.at(-1)?.status, 'active')

    const nextButton = [...document.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Go to next page')
    )
    assert.ok(nextButton)
    await act(async () => nextButton.click())
    await act(async () =>
      waitForCondition(
        () => requests.some((params) => params.p === 2),
        'next page request was not sent'
      )
    )
    assert.equal(requests.at(-1)?.p, 2)
  })

  test('shows reset, modify, and delete actions in each subscription row menu', async () => {
    installApiFixtures()
    await renderTable()

    const actionsTrigger = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Actions"]'
    )
    assert.ok(actionsTrigger)
    assert.equal(actionsTrigger.textContent?.trim(), 'Actions')

    await act(async () => actionsTrigger.click())
    await act(async () =>
      waitForCondition(
        () =>
          document.body.textContent?.includes('Reset quota') === true &&
          document.body.textContent?.includes('Modify subscription') === true &&
          document.body.textContent?.includes('Delete subscription') === true,
        'subscription action menu did not open'
      )
    )

    assert.equal(document.body.textContent?.includes('Reset quota'), true)
    assert.equal(
      document.body.textContent?.includes('Modify subscription'),
      true
    )
    assert.equal(
      document.body.textContent?.includes('Delete subscription'),
      true
    )
  })

  test('keeps actions as the last table column and exposes an action label', async () => {
    installApiFixtures()
    await renderTable()

    const headers = [
      ...document.querySelectorAll<HTMLElement>('[data-column-id]'),
    ]
    assert.equal(headers.at(-1)?.dataset.columnId, 'actions')
    assert.equal(
      document
        .querySelector<HTMLButtonElement>('button[aria-label="Actions"]')
        ?.textContent?.trim(),
      'Actions'
    )
  })

  test('opens the edit drawer from the modify action', async () => {
    installApiFixtures()
    await renderTable()

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Actions"]'
    )
    assert.ok(trigger)
    await act(async () => trigger.click())
    const item = [
      ...document.querySelectorAll<HTMLElement>(
        '[data-slot="dropdown-menu-item"]'
      ),
    ].find((element) => element.textContent?.includes('Modify subscription'))
    assert.ok(item)
    await act(async () => item.click())
    await act(async () =>
      waitForCondition(
        () =>
          document.body.textContent?.includes(
            'Changes apply only to this subscription instance'
          ) === true,
        'edit drawer did not open'
      )
    )
    assert.equal(
      document.body.textContent?.includes('Modify subscription'),
      true
    )
  })

  test('shows the persisted status when an active subscription window has elapsed', async () => {
    installApiFixtures([elapsedActiveRecord])
    await renderTable()

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Actions"]'
    )
    assert.ok(trigger)
    await act(async () => trigger.click())
    const item = [
      ...document.querySelectorAll<HTMLElement>(
        '[data-slot="dropdown-menu-item"]'
      ),
    ].find((element) => element.textContent?.includes('Modify subscription'))
    assert.ok(item)
    await act(async () => item.click())
    await act(async () =>
      waitForCondition(
        () =>
          document
            .querySelector<HTMLButtonElement>(
              '#admin-subscription-form [data-slot="select-trigger"]'
            )
            ?.textContent?.trim() === 'Active',
        'edit form did not preserve the stored subscription status'
      )
    )
  })

  test('opens reset and delete confirmations from the action menu', async () => {
    installApiFixtures()
    await renderTable()

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Actions"]'
    )
    assert.ok(trigger)
    await act(async () => trigger.click())
    const resetItem = [
      ...document.querySelectorAll<HTMLElement>(
        '[data-slot="dropdown-menu-item"]'
      ),
    ].find((element) => element.textContent?.includes('Reset quota'))
    assert.ok(resetItem)
    await act(async () => resetItem.click())
    await act(async () =>
      waitForCondition(
        () =>
          document.body.textContent?.includes('Reset subscription quota') ===
          true,
        'reset confirmation did not open'
      )
    )
    assert.equal(
      document.body.textContent?.includes(
        "Reset this subscription's used quota"
      ),
      true
    )

    const cancel = [
      ...document.querySelectorAll<HTMLButtonElement>('button'),
    ].find((button) => button.textContent?.trim() === 'Cancel')
    assert.ok(cancel)
    await act(async () => cancel.click())

    await act(async () => trigger.click())
    const deleteItem = [
      ...document.querySelectorAll<HTMLElement>(
        '[data-slot="dropdown-menu-item"]'
      ),
    ].find((element) => element.textContent?.includes('Delete subscription'))
    assert.ok(deleteItem)
    await act(async () => deleteItem.click())
    await act(async () =>
      waitForCondition(
        () => document.body.textContent?.includes('Confirm delete') === true,
        'delete confirmation did not open'
      )
    )
    assert.equal(
      document.body.textContent?.includes(
        'This removes the subscription from normal views but keeps its billing record'
      ),
      true
    )
  })

  test('disables reset for an expired subscription', async () => {
    installApiFixtures([expiredRecord])
    await renderTable()

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Actions"]'
    )
    assert.ok(trigger)
    await act(async () => trigger.click())
    const resetItem = [
      ...document.querySelectorAll<HTMLElement>(
        '[data-slot="dropdown-menu-item"]'
      ),
    ].find((element) => element.textContent?.includes('Reset quota'))
    assert.ok(resetItem)
    assert.equal(
      resetItem.getAttribute('aria-disabled') === 'true' ||
        resetItem.hasAttribute('data-disabled'),
      true
    )
  })

  test('sends only changed fields when saving an edited quota', async () => {
    installApiFixtures()
    await renderTable()

    const trigger = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Actions"]'
    )
    assert.ok(trigger)
    await act(async () => trigger.click())
    const item = [
      ...document.querySelectorAll<HTMLElement>(
        '[data-slot="dropdown-menu-item"]'
      ),
    ].find((element) => element.textContent?.includes('Modify subscription'))
    assert.ok(item)
    await act(async () => item.click())
    await act(async () =>
      waitForCondition(
        () =>
          document.querySelector<HTMLInputElement>(
            '#admin-subscription-form input[type="number"]'
          )?.value === '0.001',
        'edit form did not open'
      )
    )

    const quotaInput = document.querySelector<HTMLInputElement>(
      '#admin-subscription-form input[type="number"]'
    )
    assert.ok(quotaInput)
    await changeInput(quotaInput, '2')
    const saveButton = document.querySelector<HTMLButtonElement>(
      'button[form="admin-subscription-form"]'
    )
    assert.ok(saveButton)
    await act(async () => saveButton.click())
    await act(async () =>
      waitForCondition(
        () => mutationRequests.some((request) => request.method === 'patch'),
        'subscription update request was not sent'
      )
    )
    assert.deepEqual(mutationRequests.at(-1), {
      method: 'patch',
      url: '/api/subscription/admin/user_subscriptions/1',
      data: { amount_total: 2_000_000 },
    })
  })
})
