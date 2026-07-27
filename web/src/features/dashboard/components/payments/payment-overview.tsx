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
import { useMemo, useState, type ElementType, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import {
  Banknote,
  CreditCard,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useChartTheme } from '@/lib/use-chart-theme'
import { cn } from '@/lib/utils'
import { VCHART_OPTION } from '@/lib/vchart'
import { getTopUpOverview } from '../../api'
import type { TopUpOverviewDays } from '../../types'

const DAYS_OPTIONS: TopUpOverviewDays[] = [7, 30, 90]
const DAYS_LABEL_KEYS: Record<TopUpOverviewDays, string> = {
  7: '7 Days',
  30: '30 Days',
  90: '90 Days',
}

function StatBlock(props: {
  title: string
  value: string
  note: string
  icon: ElementType
}) {
  const Icon = props.icon
  return (
    <div className='bg-card rounded-lg border p-4'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs font-medium'>
        <Icon className='size-4 shrink-0' />
        <span className='truncate'>{props.title}</span>
      </div>
      <div className='mt-2 text-2xl font-semibold tabular-nums'>
        {props.value}
      </div>
      <div className='text-muted-foreground mt-1 text-xs'>{props.note}</div>
    </div>
  )
}

function SectionPanel(props: {
  title: string
  description?: string
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('bg-card overflow-hidden rounded-lg border', props.className)}>
      <div className='border-b px-4 py-3'>
        <div className='text-sm font-semibold'>{props.title}</div>
        {props.description ? (
          <div className='text-muted-foreground mt-0.5 text-xs'>
            {props.description}
          </div>
        ) : null}
      </div>
      <div className='p-4'>{props.children}</div>
    </div>
  )
}

function formatDateLabel(date: string): string {
  const [year, month, day] = date.split('-').map(Number)
  if (!year || !month || !day) return date
  return `${month.toString().padStart(2, '0')}-${day.toString().padStart(2, '0')}`
}

export function PaymentOverview() {
  const { t } = useTranslation()
  const { customization } = useThemeCustomization()
  const { resolvedTheme, themeReady } = useChartTheme()
  const [days, setDays] = useState<TopUpOverviewDays>(30)

  const overviewQuery = useQuery({
    queryKey: ['dashboard', 'payment-overview', days],
    queryFn: async () => {
      const result = await getTopUpOverview(days)
      return result.success ? (result.data ?? null) : null
    },
    staleTime: 60_000,
  })

  const overview = overviewQuery.data
  const loading = overviewQuery.isLoading

  const chartData = useMemo(() => overview?.daily ?? [], [overview?.daily])
  const chartSpec = useMemo(() => {
    const data = chartData.map((item) => ({
      date: formatDateLabel(item.date),
      income: item.income,
      orders: item.orders,
    }))

    return {
      type: 'line' as const,
      data: [{ id: 'income', values: data }],
      xField: 'date',
      yField: 'income',
      color: [resolvedTheme === 'dark' ? '#60a5fa' : '#2563eb'],
      smooth: true,
      point: {
        visible: false,
      },
      line: {
        style: {
          strokeWidth: 2,
        },
      },
      area: {
        visible: true,
        style: {
          fillOpacity: 0.14,
        },
      },
      axes: [
        {
          orient: 'left',
          label: {
            style: {
              fill:
                resolvedTheme === 'dark'
                  ? 'rgba(255, 255, 255, 0.68)'
                  : 'rgba(15, 23, 42, 0.58)',
            },
          },
          tick: {
            visible: false,
          },
          grid: {
            visible: true,
            style: {
              lineDash: [3, 3],
              stroke:
                resolvedTheme === 'dark'
                  ? 'rgba(255, 255, 255, 0.12)'
                  : 'rgba(15, 23, 42, 0.12)',
            },
          },
        },
        {
          orient: 'bottom',
          label: {
            style: {
              fill:
                resolvedTheme === 'dark'
                  ? 'rgba(255, 255, 255, 0.68)'
                  : 'rgba(15, 23, 42, 0.58)',
            },
          },
        },
      ],
    }
  }, [chartData, resolvedTheme])

  const topUsers = overview?.top_users ?? []
  const paymentMethods = overview?.payment_methods ?? []
  const daily = overview?.daily ?? []

  return (
    <div className='flex min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-end gap-2'>
        <div className='bg-muted/60 inline-flex rounded-lg border p-0.5'>
          {DAYS_OPTIONS.map((item) => (
            <Button
              key={item}
              variant={days === item ? 'default' : 'ghost'}
              size='sm'
              className='h-7 px-3 text-xs'
              onClick={() => setDays(item)}
            >
              {t(DAYS_LABEL_KEYS[item])}
            </Button>
          ))}
        </div>
      </div>

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <StatBlock
          title={t('Today Income')}
          value={
            loading
              ? '...'
              : formatBillingCurrencyFromUSD(overview?.today_income ?? 0)
          }
          note={t('Successful orders completed today')}
          icon={Banknote}
        />
        <StatBlock
          title={t('Range Income')}
          value={
            loading
              ? '...'
              : formatBillingCurrencyFromUSD(overview?.range_income ?? 0)
          }
          note={t('Successful orders in the selected range')}
          icon={Wallet}
        />
        <StatBlock
          title={t('Today Orders')}
          value={loading ? '...' : formatNumber(overview?.today_orders ?? 0)}
          note={t('Orders completed today')}
          icon={CreditCard}
        />
        <StatBlock
          title={t('Average Amount')}
          value={
            loading
              ? '...'
              : formatBillingCurrencyFromUSD(overview?.average_amount ?? 0)
          }
          note={t('Average successful order amount')}
          icon={Users}
        />
      </div>

      <div className='grid min-h-0 gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]'>
        <SectionPanel
          title={t('Income Trend')}
          description={t('Daily successful order income and order count')}
        >
          <div className='h-[320px]'>
            {loading || !themeReady ? (
              <Skeleton className='h-full w-full' />
            ) : (
              <VChart
                spec={{
                  ...chartSpec,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
                key={`${days}-${resolvedTheme}-${customization.preset}`}
              />
            )}
          </div>
        </SectionPanel>

        <div className='grid gap-4'>
          <SectionPanel
            title={t('Payment Methods')}
            description={t('Successful orders grouped by method')}
          >
            <div className='space-y-2'>
              {paymentMethods.length === 0 && !loading ? (
                <div className='text-muted-foreground py-6 text-center text-sm'>
                  {t('No data available')}
                </div>
              ) : (
                paymentMethods.map((item) => (
                  <div
                    key={`${item.payment_provider || 'none'}-${item.payment_method}`}
                    className='bg-muted/30 flex items-center justify-between rounded-lg px-3 py-2'
                  >
                    <div className='min-w-0'>
                      <div className='truncate text-sm font-medium'>
                        {item.payment_method}
                      </div>
                      <div className='text-muted-foreground truncate text-xs'>
                        {item.payment_provider || '-'}
                      </div>
                    </div>
                    <div className='text-right'>
                      <div className='text-sm font-semibold tabular-nums'>
                        {formatBillingCurrencyFromUSD(item.income)}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {formatNumber(item.orders)}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </SectionPanel>

          <SectionPanel
            title={t('Top Users')}
            description={t('Users ranked by successful payment volume')}
          >
            <div className='space-y-2'>
              {topUsers.length === 0 && !loading ? (
                <div className='text-muted-foreground py-6 text-center text-sm'>
                  {t('No data available')}
                </div>
              ) : (
                topUsers.map((item, index) => (
                  <div
                    key={item.user.id || index}
                    className='bg-muted/30 flex items-center justify-between rounded-lg px-3 py-2'
                  >
                    <div className='min-w-0'>
                      <div className='truncate text-sm font-medium'>
                        {item.user.display_name ||
                          item.user.username ||
                          item.user.email ||
                          t('User')}
                      </div>
                      <div className='text-muted-foreground truncate text-xs'>
                        {item.user.email || item.user.username}
                      </div>
                    </div>
                    <div className='text-right'>
                      <div className='text-sm font-semibold tabular-nums'>
                        {formatBillingCurrencyFromUSD(item.income)}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {formatNumber(item.orders)}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </SectionPanel>

          <SectionPanel
            title={t('Daily Orders')}
            description={t('Orders completed on each day in the selected range')}
          >
            <div className='space-y-2'>
              {daily.length === 0 && !loading ? (
                <div className='text-muted-foreground py-6 text-center text-sm'>
                  {t('No data available')}
                </div>
              ) : (
                daily.map((item) => (
                  <div
                    key={item.date}
                    className='bg-muted/30 flex items-center justify-between rounded-lg px-3 py-2'
                  >
                    <div className='text-sm font-medium'>{item.date}</div>
                    <div className='text-right'>
                      <div className='text-sm font-semibold tabular-nums'>
                        {formatBillingCurrencyFromUSD(item.income)}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {formatNumber(item.orders)}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </SectionPanel>
        </div>
      </div>
    </div>
  )
}
