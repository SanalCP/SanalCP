import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import type { CollectorStatus, Delivery, DeliveryDetail, DeliveryEventStatus, DeliveryList } from '@/lib/mailDelivery'
import { useAuth } from '@/store/auth'

const inputClass = 'w-full px-3 py-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg text-sm'
const buttonClass = 'px-3 py-2 text-sm border border-slate-300 dark:border-slate-600 rounded-lg disabled:opacity-50'
const emptyFilters = { sender: '', recipient: '', status: '', queue_id: '', from: '', to: '' }
type Filters = typeof emptyFilters

function timestamp(value: string | null, language: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString(language)
}

function Status({ status }: { status: DeliveryEventStatus }) {
  const { t } = useTranslation('MailDeliveryTracking')
  const color = status === 'delivered'
    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    : status === 'bounced'
      ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
      : status === 'deferred' || status === 'forwarded'
        ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
        : 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
  return <span className={`inline-block px-2 py-1 rounded text-xs ${color}`}>{t(`status.${status}`)}</span>
}

function CollectorNotice({ collector }: { collector: CollectorStatus }) {
  const { t, i18n } = useTranslation('MailDeliveryTracking')
  return <div className="space-y-2 text-xs" aria-live="polite">
    {(collector.warning === 'source_unavailable' || collector.state === 'error') &&
      <p role="status" className="p-3 rounded-lg bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">{t('source_unavailable')}</p>}
    {collector.state === 'starting' && collector.warning !== 'source_unavailable' &&
      <p role="status" className="p-3 rounded-lg bg-slate-100 text-slate-600 dark:bg-slate-900 dark:text-slate-300">{t('source_starting')}</p>}
    {collector.warning === 'history_gap' &&
      <p role="status" className="p-3 rounded-lg bg-amber-50 text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{t('history_gap')}</p>}
    <p className="text-slate-500">{t('last_collected', { at: timestamp(collector.last_collected_at, i18n.language) })}</p>
  </div>
}

function Result({ delivery }: { delivery: Pick<Delivery, 'relay' | 'dsn' | 'response'> }) {
  const { t } = useTranslation('MailDeliveryTracking')
  return <dl className="space-y-2 text-sm min-w-0">
    <div><dt className="text-xs text-slate-500">{t('relay')}</dt><dd className="break-all font-mono">{delivery.relay || '—'}</dd></div>
    <div><dt className="text-xs text-slate-500">{t('dsn')}</dt><dd className="break-all font-mono">{delivery.dsn || '—'}</dd></div>
    <div><dt className="text-xs text-slate-500">{t('response')}</dt><dd className="whitespace-pre-wrap break-words [overflow-wrap:anywhere] font-mono text-xs">{delivery.response || '—'}</dd></div>
  </dl>
}

function DeliveryDetails({ endpoint, id, onClose, onRefresh }: { endpoint: string; id: number; onClose: () => void; onRefresh: () => void }) {
  const { t, i18n } = useTranslation('MailDeliveryTracking')
  const [data, setData] = useState<DeliveryDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    let active = true
    api.get<DeliveryDetail>(`${endpoint}/${id}`, { signal: controller.signal })
      .then(response => { if (active) setData(response.data) })
      .catch(reason => { if (active) setError(apiHata(reason, t('detail_failed'))) })
    return () => { active = false; controller.abort() }
  }, [endpoint, id, t])
  const delivery = data?.delivery
  const events = data?.events ?? []
  return <section aria-label={t('detail_title')} className="p-5 border-t border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/40">
    <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
      <h3 className="font-semibold text-sm">{t('detail_title')}</h3>
      <div className="flex gap-2">
        <button type="button" className={buttonClass} onClick={onRefresh}>{t('refresh_detail')}</button>
        <button type="button" className={buttonClass} onClick={onClose}>{t('close')}</button>
      </div>
    </div>
    {error ? <p role="alert" className="text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap break-words">{error}</p> : !data || !delivery ?
      <p role="status" className="text-sm text-slate-500">{t('detail_loading')}</p> : <>
        <CollectorNotice collector={data.collector} />
        <dl className="grid sm:grid-cols-2 gap-3 my-4 text-sm">
          {([
            ['queue_id', delivery.queue_id], ['message_id', delivery.message_id],
            ['sender', delivery.sender || t('null_sender')], ['recipient', delivery.recipient],
            ['original_recipient', delivery.original_recipient || '—'],
            ['first_seen', timestamp(delivery.first_seen_at, i18n.language)],
            ['last_seen', timestamp(delivery.last_seen_at, i18n.language)],
          ] as const).map(([label, value]) => <div key={label}><dt className="text-xs text-slate-500">{t(label)}</dt><dd className="break-all">{value}</dd></div>)}
        </dl>
        <h4 className="text-sm font-semibold mb-2">{t('latest_result')}</h4>
        <div className="mb-3"><Status status={delivery.status} /></div>
        {delivery.status === 'forwarded' && <p className="text-xs text-amber-700 dark:text-amber-300 mb-3">{t('forwarded_hint')}</p>}
        <Result delivery={delivery} />
        <h4 className="text-sm font-semibold mt-5 mb-3">{t('timeline')}</h4>
        {data.events_truncated && <p role="status" className="text-xs text-amber-700 dark:text-amber-300 mb-3">{t('timeline_truncated')}</p>}
        {events.length === 0 ? <p className="text-sm text-slate-500">{t('no_events')}</p> :
          <ol className="space-y-3">
            {events.map(event => <li key={event.id} className="border border-slate-200 dark:border-slate-700 rounded-lg p-3">
              <div className="flex flex-wrap items-center gap-3 mb-3"><time dateTime={event.at} className="text-xs text-slate-500">{timestamp(event.at, i18n.language)}</time><Status status={event.status} />
                {event.delay_seconds != null && <span className="text-xs text-slate-500">{t('delay', { seconds: event.delay_seconds })}</span>}
              </div>
              <Result delivery={event} />
            </li>)}
          </ol>}
      </>}
  </section>
}

function DeliveryResults({ endpoint, query, page, pageSize, onPage }: { endpoint: string; query: string; page: number; pageSize: number; onPage: (page: number) => void }) {
  const { t, i18n } = useTranslation('MailDeliveryTracking')
  const [data, setData] = useState<DeliveryList | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<number | null>(null)
  const [detailRevision, setDetailRevision] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    let active = true
    api.get<DeliveryList>(`${endpoint}?${query}`, { signal: controller.signal })
      .then(response => {
        if (!active) return
        const lastPage = Math.max(1, Math.ceil(response.data.total / pageSize))
        if (page > lastPage) { onPage(lastPage); return }
        setData(response.data)
      })
      .catch(reason => { if (active) setError(apiHata(reason, t('list_failed'))) })
    return () => { active = false; controller.abort() }
  }, [endpoint, query, page, pageSize, onPage, t])
  if (error) return <div role="alert" className="p-5 text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap break-words">{error}</div>
  if (!data) return <p role="status" className="p-5 text-sm text-slate-500">{t('loading')}</p>
  return <>
    <div className="px-5 pb-4"><CollectorNotice collector={data.collector} /></div>
    {data.items.length === 0 ? <p className="p-5 text-sm text-slate-500">{t('empty')}</p> :
      <div className="overflow-x-auto">
        <table className="w-full text-sm text-left">
          <thead className="bg-slate-50 dark:bg-slate-900/50 text-xs text-slate-500"><tr>
            {['last_seen', 'sender', 'recipient', 'queue_id', 'status_label', 'details'].map(label => <th key={label} scope="col" className="px-5 py-3">{t(label)}</th>)}
          </tr></thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-700">
            {data.items.map(delivery => <tr key={delivery.id} className={selected === delivery.id ? 'bg-sky-50 dark:bg-sky-900/20' : ''}>
              <td className="px-5 py-3 whitespace-nowrap">{timestamp(delivery.last_seen_at, i18n.language)}</td>
              <td className="px-5 py-3 break-all max-w-xs">{delivery.sender || t('null_sender')}</td>
              <td className="px-5 py-3 break-all max-w-xs">{delivery.recipient}</td>
              <td className="px-5 py-3 font-mono">{delivery.queue_id}</td>
              <td className="px-5 py-3"><Status status={delivery.status} /></td>
              <td className="px-5 py-3"><button type="button" className="text-brand-600 dark:text-brand-400 hover:underline" aria-expanded={selected === delivery.id} aria-label={t('open_details', { recipient: delivery.recipient, queue: delivery.queue_id })} onClick={() => { setSelected(delivery.id); setDetailRevision(0) }}>{t('details')}</button></td>
            </tr>)}
          </tbody>
        </table>
      </div>}
    <div className="flex flex-wrap items-center justify-between gap-3 p-5 border-t border-slate-200 dark:border-slate-700">
      <p className="text-xs text-slate-500">{t('pagination', { page, pages: Math.max(1, Math.ceil(data.total / pageSize)), total: data.total })}</p>
      <div className="flex gap-2"><button type="button" className={buttonClass} disabled={page <= 1} onClick={() => onPage(page - 1)}>{t('previous')}</button>
        <button type="button" className={buttonClass} disabled={page * pageSize >= data.total} onClick={() => onPage(page + 1)}>{t('next')}</button></div>
    </div>
    {selected !== null && <DeliveryDetails key={`${selected}:${detailRevision}`} endpoint={endpoint} id={selected} onClose={() => setSelected(null)} onRefresh={() => setDetailRevision(value => value + 1)} />}
  </>
}

function ScopedTracking({ endpoint }: { endpoint: string }) {
  const { t } = useTranslation('MailDeliveryTracking')
  const [filters, setFilters] = useState<Filters>(emptyFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(25)
  const [revision, setRevision] = useState(0)
  const query = new URLSearchParams({ ...filters, page: String(page), page_size: String(pageSize) }).toString()
  const invalidDates = Boolean(filters.from && filters.to && filters.from > filters.to)
  function filterChange(field: keyof Filters, value: string) {
    setFilters(current => ({ ...current, [field]: value }))
    setPage(1)
  }
  return <section aria-label={t('title')} className="bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-100 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
    <div className="p-5">
      <div className="flex flex-wrap items-center justify-between gap-3 mb-2"><h2 className="text-sm font-semibold">{t('title')}</h2>
        <button type="button" className={buttonClass} onClick={() => setRevision(value => value + 1)}>{t('refresh')}</button></div>
      <p className="text-xs text-slate-500 mb-2">{t('subtitle')}</p>
      <p className="text-xs text-slate-500 mb-4">{t('handoff_hint')}</p>
      <div className="grid sm:grid-cols-2 xl:grid-cols-3 gap-3">
        {(['sender', 'recipient', 'queue_id'] as const).map(field => <label key={field} className="text-xs text-slate-500">{t(field)}
          <input type="text" value={filters[field]} maxLength={field === 'queue_id' ? 64 : 320} className={`${inputClass} mt-1 text-slate-900 dark:text-slate-100`} onChange={event => filterChange(field, event.target.value)} />
        </label>)}
        <label className="text-xs text-slate-500">{t('status_label')}<select value={filters.status} className={`${inputClass} mt-1 text-slate-900 dark:text-slate-100`} onChange={event => filterChange('status', event.target.value)}>
          <option value="">{t('all_statuses')}</option>
          {(['pending', 'deferred', 'delivered', 'bounced', 'forwarded'] as const).map(status => <option key={status} value={status}>{t(`status.${status}`)}</option>)}
        </select></label>
        {(['from', 'to'] as const).map(field => <label key={field} className="text-xs text-slate-500">{t(field)}
          <input type="date" value={filters[field]} className={`${inputClass} mt-1 text-slate-900 dark:text-slate-100`} onChange={event => filterChange(field, event.target.value)} />
        </label>)}
      </div>
      <div className="flex flex-wrap items-center gap-3 mt-3">
        <button type="button" className={buttonClass} onClick={() => { setFilters(emptyFilters); setPage(1) }}>{t('clear_filters')}</button>
        <label className="text-xs text-slate-500">{t('page_size')}<select value={pageSize} className="ml-2 p-2 border border-slate-300 dark:border-slate-600 dark:bg-slate-900 rounded-lg" onChange={event => { setPageSize(Number(event.target.value)); setPage(1) }}>
          {[25, 50, 100].map(size => <option key={size} value={size}>{size}</option>)}
        </select></label>
        <span className="text-xs text-slate-500">{t('date_hint')}</span>
      </div>
    </div>
    {invalidDates ? <p role="alert" className="px-5 pb-5 text-sm text-red-700 dark:text-red-300">{t('invalid_dates')}</p> :
      <DeliveryResults key={`${query}:${revision}`} endpoint={endpoint} query={query} page={page} pageSize={pageSize} onPage={setPage} />}
  </section>
}

export default function MailDeliveryTracking({ domainID }: { domainID?: string }) {
  const identity = useAuth(state => state.kullanici)
  const endpoint = domainID === undefined ? '/mail/deliveries' : `/domains/${encodeURIComponent(domainID)}/mail/deliveries`
  // A scope change remounts all filters, selection and pending requests before painting another tenant's view.
  return <ScopedTracking key={`${endpoint}:${identity?.id}:${identity?.rol}`} endpoint={endpoint} />
}
