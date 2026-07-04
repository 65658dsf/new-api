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
import { memo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Bell, Megaphone, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getAnnouncementColorClass } from '@/lib/colors'
import { formatDateTimeObject } from '@/lib/time'
import { cn } from '@/lib/utils'
import { getNotice } from '@/lib/api'
import { Markdown } from '@/components/ui/markdown'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useAnnouncements } from '@/features/dashboard/hooks/use-status-data'
import { getPreviewText } from '@/features/dashboard/lib'
import type { AnnouncementItem } from '@/features/dashboard/types'
import { PanelWrapper } from '../ui/panel-wrapper'
import { AnnouncementDetailModal } from './announcement-detail-dialog'

const AnnouncementStatusDot = memo(function AnnouncementStatusDot(props: {
  type?: string
}) {
  return (
    <span
      className={cn(
        'mt-1.5 inline-block size-2 shrink-0 rounded-full',
        getAnnouncementColorClass(props.type)
      )}
    />
  )
})

function NotificationSectionHeader(props: {
  icon: LucideIcon
  title: string
}) {
  const Icon = props.icon

  return (
    <div className='mb-3 flex shrink-0 items-center gap-2 text-sm font-semibold'>
      <Icon className='text-muted-foreground/60 size-4' />
      {props.title}
    </div>
  )
}

function NotificationSectionEmpty(props: { children: string }) {
  return (
    <div className='text-muted-foreground flex min-h-0 flex-1 items-center justify-center px-3 text-center text-sm'>
      {props.children}
    </div>
  )
}

function SystemNoticeSection(props: { notice: string }) {
  const { t } = useTranslation()

  return (
    <section className='border-border/60 flex min-h-0 flex-col border-b p-4 sm:p-5 xl:border-r xl:border-b-0'>
      <NotificationSectionHeader
        icon={Bell}
        title={t('System notifications')}
      />
      {props.notice ? (
        <ScrollArea className='min-h-0 flex-1 pr-3'>
          <Markdown className='text-sm prose-h1:text-lg prose-h2:text-base prose-h3:text-sm'>
            {props.notice}
          </Markdown>
        </ScrollArea>
      ) : (
        <NotificationSectionEmpty>
          {t('No system notifications')}
        </NotificationSectionEmpty>
      )}
    </section>
  )
}

function AnnouncementListSection(props: {
  list: AnnouncementItem[]
  onAnnouncementClick: (item: AnnouncementItem) => void
}) {
  const { t } = useTranslation()

  return (
    <section className='flex min-h-0 flex-col p-4 sm:p-5'>
      <NotificationSectionHeader icon={Megaphone} title={t('Announcements')} />
      {props.list.length > 0 ? (
        <ScrollArea className='min-h-0 flex-1 pr-3'>
          <div>
            {props.list.map((item: AnnouncementItem, idx: number) => {
              const key =
                item.id ??
                `${item.publishDate ?? 'undated'}-${item.type ?? 'default'}-${item.content}`
              return (
                <button
                  key={key}
                  type='button'
                  onClick={() => props.onAnnouncementClick(item)}
                  className={cn(
                    'group hover:bg-muted/40 w-full px-2.5 py-2 text-left transition-colors',
                    idx < props.list.length - 1 &&
                      'border-border/60 border-b'
                  )}
                >
                  <div className='flex items-start gap-2.5'>
                    <AnnouncementStatusDot type={item.type} />
                    <div className='flex min-w-0 flex-1 flex-col gap-1'>
                      <p className='line-clamp-1 text-sm font-medium'>
                        {getPreviewText(item.content)}
                      </p>
                      <div className='flex items-center justify-between gap-2'>
                        {item.publishDate && (
                          <time className='text-muted-foreground/60 min-w-0 truncate text-xs'>
                            {formatDateTimeObject(new Date(item.publishDate))}
                          </time>
                        )}
                        <span className='text-muted-foreground/40 shrink-0 text-xs opacity-0 transition-opacity group-hover:opacity-100'>
                          {t('Click for details')}
                        </span>
                      </div>
                    </div>
                  </div>
                </button>
              )
            })}
          </div>
        </ScrollArea>
      ) : (
        <NotificationSectionEmpty>
          {t('No system announcements')}
        </NotificationSectionEmpty>
      )}
    </section>
  )
}

export function AnnouncementsPanel() {
  const { t } = useTranslation()
  const { items: list, loading: announcementsLoading } = useAnnouncements()
  const { data: noticeResponse, isLoading: noticeLoading } = useQuery({
    queryKey: ['notice'],
    queryFn: getNotice,
    staleTime: 1000 * 60 * 5,
  })
  const [selectedAnnouncement, setSelectedAnnouncement] =
    useState<AnnouncementItem | null>(null)
  const [isDialogOpen, setIsDialogOpen] = useState(false)

  const noticeContent = noticeResponse?.success
    ? (noticeResponse.data || '').trim()
    : ''
  const loading = announcementsLoading || noticeLoading

  const handleAnnouncementClick = (item: AnnouncementItem) => {
    setSelectedAnnouncement(item)
    setIsDialogOpen(true)
  }

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <Bell className='text-muted-foreground/60 size-4' />
          {t('System notifications / announcements')}
        </span>
      }
      description={t('Latest platform updates and notices')}
      loading={loading}
      height='h-[34rem] xl:h-72'
      contentClassName='p-0'
    >
      <div className='grid h-[34rem] grid-cols-1 xl:h-72 xl:grid-cols-2'>
        <SystemNoticeSection notice={noticeContent} />
        <AnnouncementListSection
          list={list}
          onAnnouncementClick={handleAnnouncementClick}
        />
      </div>

      <AnnouncementDetailModal
        open={isDialogOpen}
        onOpenChange={setIsDialogOpen}
        announcement={selectedAnnouncement}
      />
    </PanelWrapper>
  )
}
