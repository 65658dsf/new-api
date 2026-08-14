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
import { VChart } from '@visactor/react-vchart'
import {
  BadgeDollarSign,
  CalendarDays,
  CircleAlert,
  Landmark,
  Percent,
  ReceiptText,
  RotateCw,
  TrendingUp,
} from 'lucide-react'
import { useMemo, useState, type ElementType } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { cn } from '@/lib/utils'
import { VCHART_OPTION } from '@/lib/vchart'

import { getChannelFinancialReport } from '../../api'
import type { ChannelFinancialReport, FinancialDimensionRow } from '../../types'

type Preset = 'today' | '7d' | '30d' | 'custom'
type Dimension = 'channel' | 'model'

const PRESET_LABELS: Record<Preset, string> = {
  today: 'Today',
  '7d': '7 Days',
  '30d': '30 Days',
  custom: 'Custom',
}

function rangeForPreset(preset: Exclude<Preset, 'custom'>) {
  const end = dayjs().endOf('day')
  const start =
    preset === 'today'
      ? dayjs().startOf('day')
      : end.subtract(preset === '7d' ? 6 : 29, 'day').startOf('day')
  return { start: start.toDate(), end: end.toDate() }
}

function revenueTone(value: number): 'positive' | 'negative' | undefined {
  if (value < 0) return 'negative'
  if (value > 0) return 'positive'
  return undefined
}

function StatBlock(props: {
  title: string
  value: string
  note: string
  icon: ElementType
  tone?: 'default' | 'positive' | 'negative'
}) {
  const Icon = props.icon
  return (
    <div className='border-border/70 min-w-0 border-b p-4 last:border-b-0 sm:border-r sm:border-b-0 sm:last:border-r-0'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs font-medium'>
        <Icon className='size-4 shrink-0' />
        <span className='truncate'>{props.title}</span>
      </div>
      <div
        className={cn(
          'mt-2 truncate text-xl font-semibold tabular-nums',
          props.tone === 'positive' && 'text-primary',
          props.tone === 'negative' && 'text-destructive'
        )}
      >
        {props.value}
      </div>
      <div className='text-muted-foreground mt-1 truncate text-xs'>
        {props.note}
      </div>
    </div>
  )
}

function DimensionTable(props: {
  rows: FinancialDimensionRow[]
  dimension: Dimension
  loading: boolean
}) {
  const { t } = useTranslation()
  if (props.loading) {
    return <Skeleton className='h-72 w-full' />
  }
  if (props.rows.length === 0) {
    return (
      <div className='text-muted-foreground flex h-72 items-center justify-center text-sm'>
        {t('No data available')}
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>
            {props.dimension === 'channel' ? t('Channel') : t('Model')}
          </TableHead>
          <TableHead className='text-right'>{t('Sales')}</TableHead>
          <TableHead className='text-right'>{t('Cost')}</TableHead>
          <TableHead className='text-right'>{t('Profit')}</TableHead>
          <TableHead className='text-right'>{t('Requests')}</TableHead>
          <TableHead className='text-right'>{t('Coverage')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.rows.map((row) => {
          const total = row.covered_count + row.uncovered_count
          const coverage = total > 0 ? row.covered_count / total : 0
          const name =
            props.dimension === 'channel'
              ? row.name || t('Deleted Channel')
              : row.model_name || row.name || '-'
          return (
            <TableRow key={`${props.dimension}-${row.id ?? name}-${name}`}>
              <TableCell className='max-w-72 truncate font-medium'>
                {name}
              </TableCell>
              <TableCell className='text-right'>
                {formatBillingCurrencyFromUSD(row.revenue_usd)}
              </TableCell>
              <TableCell className='text-right'>
                {formatBillingCurrencyFromUSD(row.cost_usd)}
              </TableCell>
              <TableCell
                className={cn(
                  'text-right font-medium',
                  row.profit_usd < 0 && 'text-destructive'
                )}
              >
                {formatBillingCurrencyFromUSD(row.profit_usd)}
              </TableCell>
              <TableCell className='text-right'>
                {formatNumber(row.request_count)}
              </TableCell>
              <TableCell className='text-right'>
                {new Intl.NumberFormat(undefined, {
                  style: 'percent',
                  maximumFractionDigits: 1,
                }).format(coverage)}
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

export function CostProfitDashboard() {
  const { t } = useTranslation()
  const { customization } = useThemeCustomization()
  const { resolvedTheme, themeReady } = useChartTheme()
  const initialRange = rangeForPreset('30d')
  const [preset, setPreset] = useState<Preset>('30d')
  const [startDate, setStartDate] = useState(
    dayjs(initialRange.start).format('YYYY-MM-DD')
  )
  const [endDate, setEndDate] = useState(
    dayjs(initialRange.end).format('YYYY-MM-DD')
  )
  const [dimension, setDimension] = useState<Dimension>('channel')

  const queryRange = useMemo(() => {
    const start = dayjs(startDate).startOf('day')
    const end = dayjs(endDate).endOf('day')
    const valid =
      start.isValid() &&
      end.isValid() &&
      !end.isBefore(start) &&
      end.diff(start, 'day') < 90
    return {
      start_timestamp: start.unix(),
      end_timestamp: end.unix(),
      valid,
    }
  }, [endDate, startDate])

  const reportQuery = useQuery({
    queryKey: [
      'dashboard',
      'financial',
      queryRange.start_timestamp,
      queryRange.end_timestamp,
    ],
    queryFn: async () => {
      const result = await getChannelFinancialReport({
        start_timestamp: queryRange.start_timestamp,
        end_timestamp: queryRange.end_timestamp,
      })
      if (!result.success) {
        throw new Error(result.message || t('Failed to load financial data'))
      }
      return result.data ?? null
    },
    enabled: queryRange.valid,
    staleTime: 60_000,
  })

  const report = reportQuery.data as ChannelFinancialReport | null | undefined
  const summary = report?.summary
  const totalCoverageEvents =
    (summary?.covered_count ?? 0) + (summary?.uncovered_count ?? 0)
  const coverage =
    totalCoverageEvents > 0
      ? (summary?.covered_count ?? 0) / totalCoverageEvents
      : 0

  const chartSpec = useMemo(() => {
    const values = (report?.trend ?? []).flatMap((point) => [
      {
        date: point.date.slice(5),
        metric: t('Sales'),
        value: point.revenue_usd,
      },
      { date: point.date.slice(5), metric: t('Cost'), value: point.cost_usd },
      {
        date: point.date.slice(5),
        metric: t('Profit'),
        value: point.profit_usd,
      },
    ])
    return {
      type: 'line' as const,
      data: [{ id: 'financial-trend', values }],
      xField: 'date',
      yField: 'value',
      seriesField: 'metric',
      color: ['#2563eb', '#dc2626', '#059669'],
      smooth: true,
      point: { visible: false },
      line: { style: { lineWidth: 2 } },
      axes: [
        {
          orient: 'left',
          label: {
            formatMethod: (value: number) =>
              formatBillingCurrencyFromUSD(value, {
                compact: true,
                digitsLarge: 1,
                digitsSmall: 2,
              }),
          },
          grid: { visible: true, style: { lineDash: [3, 3] } },
        },
        { orient: 'bottom' },
      ],
      legends: { visible: true, orient: 'top', position: 'start' },
    }
  }, [report?.trend, t])

  const setPresetRange = (next: Preset) => {
    setPreset(next)
    if (next === 'custom') return
    const range = rangeForPreset(next)
    setStartDate(dayjs(range.start).format('YYYY-MM-DD'))
    setEndDate(dayjs(range.end).format('YYYY-MM-DD'))
  }

  const loading = reportQuery.isLoading
  const profit = summary?.profit_usd ?? 0
  let trendContent = <Skeleton className='h-full w-full' />
  if (!loading && themeReady) {
    trendContent =
      (report?.trend.length ?? 0) === 0 ? (
        <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
          {t('No data available')}
        </div>
      ) : (
        <VChart
          spec={{
            ...chartSpec,
            theme: resolvedTheme === 'dark' ? 'dark' : 'light',
            background: 'transparent',
          }}
          option={VCHART_OPTION}
          key={`${startDate}-${endDate}-${resolvedTheme}-${customization.preset}`}
        />
      )
  }

  return (
    <div className='flex min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <ToggleGroup
          value={[preset]}
          onValueChange={(value) => {
            const next = value[0] as Preset | undefined
            if (next) setPresetRange(next)
          }}
          variant='outline'
          size='sm'
          spacing={0}
          className='max-w-full overflow-x-auto'
          aria-label={t('Date Range')}
        >
          {(Object.keys(PRESET_LABELS) as Preset[]).map((item) => (
            <ToggleGroupItem
              key={item}
              value={item}
              aria-label={t(PRESET_LABELS[item])}
              className='h-8 px-3 text-xs'
            >
              {t(PRESET_LABELS[item])}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className='flex flex-wrap items-center gap-2'>
          <CalendarDays className='text-muted-foreground size-4' />
          <Input
            type='date'
            value={startDate}
            max={endDate}
            onChange={(event) => {
              setPreset('custom')
              setStartDate(event.target.value)
            }}
            className='w-auto'
            aria-label={t('Start Date')}
          />
          <span className='text-muted-foreground text-xs'>{t('to')}</span>
          <Input
            type='date'
            value={endDate}
            min={startDate}
            max={dayjs().format('YYYY-MM-DD')}
            onChange={(event) => {
              setPreset('custom')
              setEndDate(event.target.value)
            }}
            className='w-auto'
            aria-label={t('End Date')}
          />
          <Button
            variant='outline'
            size='icon-sm'
            onClick={() => void reportQuery.refetch()}
            disabled={!queryRange.valid || reportQuery.isFetching}
            aria-label={t('Refresh')}
          >
            <RotateCw
              className={cn(reportQuery.isFetching && 'animate-spin')}
            />
          </Button>
        </div>
      </div>

      {!queryRange.valid ? (
        <div className='border-destructive/40 bg-destructive/5 text-destructive flex items-center gap-2 rounded-lg border px-3 py-2 text-sm'>
          <CircleAlert className='size-4 shrink-0' />
          {t('Select a valid date range of no more than 90 days.')}
        </div>
      ) : null}

      {reportQuery.isError ? (
        <div className='border-destructive/40 bg-destructive/5 text-destructive flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm'>
          <span>{reportQuery.error.message}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void reportQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      ) : null}

      <div className='overflow-hidden rounded-lg border'>
        <div className='grid sm:grid-cols-2 xl:grid-cols-5'>
          <StatBlock
            title={t('Sales')}
            value={
              loading
                ? '...'
                : formatBillingCurrencyFromUSD(summary?.revenue_usd ?? 0)
            }
            note={t('Net user charges in the selected range')}
            icon={BadgeDollarSign}
          />
          <StatBlock
            title={t('Cost')}
            value={
              loading
                ? '...'
                : formatBillingCurrencyFromUSD(summary?.cost_usd ?? 0)
            }
            note={t('Model base cost multiplied by channel rate')}
            icon={Landmark}
          />
          <StatBlock
            title={t('Profit')}
            value={loading ? '...' : formatBillingCurrencyFromUSD(profit)}
            note={t('Sales minus channel cost')}
            icon={TrendingUp}
            tone={revenueTone(profit)}
          />
          <StatBlock
            title={t('Profit Margin')}
            value={
              loading
                ? '...'
                : new Intl.NumberFormat(undefined, {
                    style: 'percent',
                    maximumFractionDigits: 1,
                  }).format(summary?.profit_margin ?? 0)
            }
            note={t('Profit as a share of net revenue')}
            icon={Percent}
          />
          <StatBlock
            title={t('Requests')}
            value={loading ? '...' : formatNumber(summary?.request_count ?? 0)}
            note={t('{{coverage}} data coverage', {
              coverage: new Intl.NumberFormat(undefined, {
                style: 'percent',
                maximumFractionDigits: 1,
              }).format(coverage),
            })}
            icon={ReceiptText}
          />
        </div>
      </div>

      {(summary?.estimated_count ?? 0) > 0 ||
      (summary?.uncovered_count ?? 0) > 0 ? (
        <div className='bg-muted/40 flex items-start gap-2 rounded-lg border px-3 py-2 text-sm'>
          <CircleAlert className='text-muted-foreground mt-0.5 size-4 shrink-0' />
          <div>
            <div className='font-medium'>{t('Financial data coverage')}</div>
            <div className='text-muted-foreground text-xs'>
              {t(
                '{{estimated}} historical records are estimated and {{uncovered}} records have no reliable cost estimate.',
                {
                  estimated: formatNumber(summary?.estimated_count ?? 0),
                  uncovered: formatNumber(summary?.uncovered_count ?? 0),
                }
              )}
            </div>
          </div>
        </div>
      ) : null}

      <div className='overflow-hidden rounded-lg border'>
        <div className='border-b px-4 py-3'>
          <div className='text-sm font-semibold'>{t('Financial Trend')}</div>
          <div className='text-muted-foreground mt-0.5 text-xs'>
            {t('Daily revenue, cost, and profit in the selected range')}
          </div>
        </div>
        <div className='h-80 p-3'>{trendContent}</div>
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3'>
          <div>
            <div className='text-sm font-semibold'>
              {t('Financial Breakdown')}
            </div>
            <div className='text-muted-foreground mt-0.5 text-xs'>
              {t('Compare revenue, cost, profit, and coverage')}
            </div>
          </div>
          <ToggleGroup
            value={[dimension]}
            onValueChange={(value) => {
              const next = value[0] as Dimension | undefined
              if (next) setDimension(next)
            }}
            variant='outline'
            size='sm'
            spacing={0}
            aria-label={t('Breakdown Dimension')}
          >
            {(['channel', 'model'] as Dimension[]).map((item) => (
              <ToggleGroupItem
                key={item}
                value={item}
                className='h-8 px-3 text-xs'
              >
                {item === 'channel' ? t('Channels') : t('Models')}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>
        <DimensionTable
          dimension={dimension}
          rows={
            dimension === 'channel'
              ? (report?.by_channel ?? [])
              : (report?.by_model ?? [])
          }
          loading={loading}
        />
      </div>
    </div>
  )
}
