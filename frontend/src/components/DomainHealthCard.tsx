import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { useDomainFeatures } from '@/lib/domainFeaturesContext'
import { kotaDurumu } from '@/lib/mailQuota'

type Status = 'ok' | 'uyari' | 'hata' | 'unknown' | 'loading'
type Check = { id: string; status: Status; message: string; values?: Record<string, string | number>; path: string }
type WebHealth = { erisilebilir: boolean; durum_kodu: number; yanit_suresi_ms: number }
type Certificate = { aktif: boolean; kaynak: string; bitis_iso?: string }
type DNSCheck = { anahtar: string; durum: 'ok' | 'uyari' | 'hata' }
type Backup = { olusturma: string; boyut_b: number; dogrulama_durum: string }
type Schedule = { freq: 'none' | 'daily' | 'weekly' | 'monthly' }
type Mailbox = { quota_bytes: number; used_bytes: number | null }

const colors: Record<Status, string> = {
  ok: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
  uyari: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
  hata: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  unknown: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
  loading: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
}

function dnsResult(id: string, checks: DNSCheck[], path: string): Check {
  if (!checks.length) return { id, status: 'unknown', message: 'unavailable', path }
  const errors = checks.filter(c => c.durum === 'hata').length
  const warnings = checks.filter(c => c.durum === 'uyari').length
  return { id, status: errors ? 'hata' : warnings ? 'uyari' : 'ok',
    message: errors || warnings ? 'dns_issues' : 'dns_ok', values: { errors, warnings }, path }
}

export default function DomainHealthCard({ domainId, suspended = false }: { domainId: number; suspended?: boolean }) {
  // Remount on domain or permission changes so previous results never appear in another scope.
  const { ready, failed, allows } = useDomainFeatures()
  const dns = ready && !failed && allows('dns')
  const mail = ready && !failed && allows('mail')
  const backups = ready && !failed && allows('backups')
  if (!ready || failed) return null
  return <HealthChecks key={`${domainId}:${dns}:${mail}:${backups}:${suspended}`} domainId={domainId}
    dns={dns} mail={mail} backups={backups} suspended={suspended} />
}

function HealthChecks({ domainId, dns, mail, backups, suspended }: {
  domainId: number; dns: boolean; mail: boolean; backups: boolean; suspended: boolean
}) {
  const { t, i18n } = useTranslation('DomainHealthCard')
  const [revision, setRevision] = useState(0)
  const [checks, setChecks] = useState<Check[]>([])
  const [loading, setLoading] = useState(true)
  const [checkedAt, setCheckedAt] = useState<Date | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    const get = async <T,>(path: string): Promise<T> => (await api.get<T>(`/domains/${domainId}/${path}`, { signal: controller.signal })).data
    const initial = [
      ...(!suspended ? [{ id: 'web', path: 'gunlukler' }] : []),
      { id: 'ssl', path: 'ssl' },
      ...(dns ? [{ id: 'dns', path: 'dns' }] : []),
      ...(mail ? [{ id: 'mail', path: 'mail' }] : []),
      ...(backups ? [{ id: 'backups', path: 'yedekler' }] : []),
      { id: 'disk', path: '' },
    ].map(row => ({ ...row, status: 'loading' as const, message: 'checking' }))
    setChecks(initial)
    setLoading(true)
    setCheckedAt(null)
    const update = (check: Check) => {
      if (!controller.signal.aborted) setChecks(rows => rows.map(row => row.id === check.id ? check : row))
    }
    const run = async (id: string, path: string, task: () => Promise<Check | null>) => {
      try {
        const result = await task()
        if (controller.signal.aborted) return
        if (result) update(result)
        else setChecks(rows => rows.filter(row => row.id !== id))
      } catch {
        update({ id, path, status: 'unknown', message: 'unavailable' })
      }
    }
    // Share the DNS request between web DNS and active mail checks.
    const dnsData = dns ? get<{ kontroller: DNSCheck[] }>('dns/dogrula') : null
    // Attach a rejection handler immediately, including while mail status loads.
    void dnsData?.catch(() => {})
    const tasks = [
      ...(!suspended ? [run('web', 'gunlukler', async () => {
        const web = await get<WebHealth>('health')
        return { id: 'web', path: 'gunlukler',
          status: !web.erisilebilir || web.durum_kodu >= 500 ? 'hata' : web.durum_kodu >= 400 ? 'uyari' : 'ok',
          message: !web.erisilebilir ? 'web_unreachable' : 'web_response',
          values: { code: web.durum_kodu, ms: Math.round(web.yanit_suresi_ms) } }
      })] : []),
      run('ssl', 'ssl', async () => {
        const cert = await get<Certificate>('ssl')
        const expiry = Date.parse(cert.bitis_iso || '')
        if (!cert.aktif) return { id: 'ssl', path: 'ssl', status: 'uyari', message: 'ssl_missing' }
        if (!Number.isFinite(expiry)) return { id: 'ssl', path: 'ssl', status: 'unknown', message: 'unavailable' }
        const days = Math.ceil((expiry - Date.now()) / 86_400_000)
        return { id: 'ssl', path: 'ssl', status: days <= 0 ? 'hata' : days <= 14 || cert.kaynak === 'self-signed' ? 'uyari' : 'ok',
          message: days <= 0 ? 'ssl_expired' : cert.kaynak === 'self-signed' ? 'ssl_self_signed' : 'ssl_days', values: { days } }
      }),
      ...(dnsData ? [run('dns', 'dns', async () => dnsResult('dns',
        (await dnsData).kontroller.filter(c => ['ns', 'a'].includes(c.anahtar)), 'dns'))] : []),
      ...(mail ? [run('mail', 'mail', async () => {
        const status = await get<{ etkin: boolean }>('mail/durum')
        if (!status.etkin) return null
        const [boxes, records] = await Promise.all([
          get<Mailbox[]>('mail'),
          dnsData ? dnsData.then(data => data.kontroller.filter(c => ['mail_a', 'mx', 'spf', 'dkim', 'dmarc'].includes(c.anahtar))).catch(() => null) : null,
        ])
        const full = boxes.filter(b => kotaDurumu(b.used_bytes, b.quota_bytes).durum === 'asildi').length
        const near = boxes.filter(b => kotaDurumu(b.used_bytes, b.quota_bytes).durum === 'yaklasti').length
        if (full || near) return { id: 'mail', path: 'mail', status: full ? 'hata' : 'uyari', message: 'mail_quota', values: { full, near } }
        if (dnsData) return records ? dnsResult('mail', records, 'mail') : { id: 'mail', path: 'mail', status: 'unknown', message: 'mail_dns_unknown' }
        return { id: 'mail', path: 'mail', status: 'ok', message: 'mail_active', values: { count: boxes.length } }
      })] : []),
      ...(backups ? [run('backups', 'yedekler', async () => {
        const [items, schedule] = await Promise.all([get<Backup[]>('backups'), get<Schedule>('backup-schedule')])
        if (schedule.freq === 'none' && !items.length) return null
        const verified = items.filter(b => b.boyut_b > 0 && b.dogrulama_durum === 'dogrulandi')
          .sort((a, b) => b.olusturma.localeCompare(a.olusturma))
        if (!verified.length) return { id: 'backups', path: 'yedekler', status: 'uyari', message: 'backup_missing' }
        const last = verified[0].olusturma
        // SQL dates have no timezone; compare using server-reported wall dates, with a grace day.
        const date = Date.parse(last.replace(' ', 'T'))
        if (!Number.isFinite(date)) return { id: 'backups', path: 'yedekler', status: 'unknown', message: 'unavailable' }
        const age = Math.floor((Date.now() - date) / 86_400_000)
        const maxAge = { none: Infinity, daily: 2, weekly: 8, monthly: 32 }[schedule.freq]
        const latestFailed = items[0]?.dogrulama_durum === 'basarisiz'
        return { id: 'backups', path: 'yedekler', status: latestFailed ? 'hata' : age >= maxAge ? 'uyari' : 'ok',
          message: latestFailed ? 'backup_failed' : age >= maxAge ? 'backup_stale' : 'backup_last', values: { date: last } }
      })] : []),
      run('disk', '', async () => {
        const data = await get<{ disk_mb: { kullanim: number; limit: number } }>('kaynak')
        const { kullanim: used, limit } = data.disk_mb
        const quota = kotaDurumu(used, limit)
        return { id: 'disk', path: '', status: quota.durum === 'asildi' ? 'hata' : quota.durum === 'yaklasti' ? 'uyari' : 'ok',
          message: limit > 0 ? 'disk_usage' : 'disk_unlimited', values: { used, limit, percent: quota.yuzde } }
      }),
    ]
    void Promise.all(tasks).then(() => {
      if (!controller.signal.aborted) { setLoading(false); setCheckedAt(new Date()) }
    })
    return () => controller.abort()
  }, [domainId, dns, mail, backups, suspended, revision])

  const issues = checks.filter(c => c.status === 'hata' || c.status === 'uyari').length
  const unknown = checks.some(c => c.status === 'unknown')
  return (
    <section className="ta-card mb-6 overflow-hidden" aria-label={t('title')} aria-busy={loading}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 px-5 py-4 dark:border-slate-800">
        <div>
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('title')}</h2>
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400" role="status">
            {loading ? t('checking') : issues ? t('issues', { count: issues }) : unknown ? t('incomplete') : t('all_ok')}
          </p>
        </div>
        <button type="button" onClick={() => setRevision(n => n + 1)} disabled={loading}
          className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
          {t('refresh')}
        </button>
      </div>
      {suspended && <p className="px-5 pt-4 text-xs text-amber-700 dark:text-amber-300">{t('suspended')}</p>}
      <ul className="divide-y divide-slate-100 px-5 dark:divide-slate-800">
        {checks.map(check => (
          <li key={check.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3">
            <div className="min-w-0 flex-1 basis-40">
              <h3 className="text-sm font-medium text-slate-800 dark:text-slate-200">{t(`labels.${check.id}`)}</h3>
              <p className="mt-0.5 break-words text-xs text-slate-500 dark:text-slate-400">{t(check.message, check.values)}</p>
            </div>
            <span className={`rounded-md px-2 py-1 text-[11px] font-medium ${colors[check.status]}`}>{t(`statuses.${check.status}`)}</span>
            <Link to={`/abonelikler/${domainId}${check.path ? `/${check.path}` : '#kaynaklar'}`} aria-label={t('inspect_label', { name: t(`labels.${check.id}`) })}
              className="rounded text-xs font-medium text-brand-700 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand-500 dark:text-brand-300">{t('inspect')}</Link>
          </li>
        ))}
      </ul>
      {checkedAt && <p className="border-t border-slate-100 px-5 py-3 text-[11px] text-slate-500 dark:border-slate-800 dark:text-slate-400">
        {t('checked_at', { time: checkedAt.toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' }) })}
      </p>}
    </section>
  )
}
