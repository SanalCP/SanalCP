import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { AxiosProgressEvent } from 'axios'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Inventory = {
  provider: string
  username: string
  primary_domain: string
  archive_root: string
  entry_count: number
  expanded_bytes: number
  web_files: number
  web_bytes: number
  databases: string[]
  dns_zones: string[]
  mail_files: number
  mailboxes: string[]
  alias_count: number
  cron_present: boolean
  cron_jobs: { minute: string; hour: string; day: string; month: string; weekday: string; command: string; comment?: string }[]
  ssl_certs: number
  warnings: string[]
}
type Customer = { id: number; ad: string; eposta: string; plan_id?: number | null }
type Plan = { id: number; ad: string; php_surum?: string }
type ImportResult = {
  domain_id: number; domain: string; system_user: string; web_files: number
  databases: { source: string; target: string; user: string }[]
  mailboxes: { email: string; password: string; password_preserved?: boolean }[]
  aliases: number
  cron_jobs: number
  ssl_imported: boolean
  ssl_expires?: string
  credentials?: { ftp?: string; db?: string }; skipped: string[]
}
type RemoteSite = {
  domain: string; hesap: string; php_version?: string; application?: string
  target_exists?: boolean; transferable: boolean; check_status: 'compatible' | 'incompatible'
  source_modules?: string[]; missing_modules?: string[]; blockers?: string[]; warnings?: string[]
}
type RemoteInventory = { provider: string; surum: string; domainler: string[]; siteler: RemoteSite[] }
type RemoteJob = { id: number; domain: string; durum: string; ilerleme: number; mesaj: string; target_domain_id?: number; source_http_status?: number; target_http_status?: number }
type HostKeyInfo = { host: string; port: number; fingerprints: string[] }

export default function HesapAktarimiPage() {
  const { t } = useTranslation(['HesapAktarimiPage', 'common'])
  const [dosya, setDosya] = useState<File | null>(null)
  const [envanter, setEnvanter] = useState<Inventory | null>(null)
  const [hata, setHata] = useState<string | null>(null)
  const [yukleniyor, setYukleniyor] = useState(false)
  const [ilerleme, setIlerleme] = useState(0)
  const [customers, setCustomers] = useState<Customer[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [customerID, setCustomerID] = useState('')
  const [planID, setPlanID] = useState('')
  const [domain, setDomain] = useState('')
  const [phpVersion, setPHPVersion] = useState('8.3')
  const [aktariliyor, setAktariliyor] = useState(false)
  const [sonuc, setSonuc] = useState<ImportResult | null>(null)
  const [sshHost, setSSHHost] = useState('')
  const [sshPort, setSSHPort] = useState(22)
  const [sshKesif, setSSHKesif] = useState<RemoteInventory | null>(null)
  const [sshBusy, setSSHBusy] = useState(false)
  const [sshSecili, setSSHSecili] = useState<string[]>([])
  const [sshJobs, setSSHJobs] = useState<RemoteJob[]>([])
  const [sshGecmis, setSSHGecmis] = useState<RemoteJob[]>([])
  const [sshPublicKey, setSSHPublicKey] = useState('')
  const [sshHostKey, setSSHHostKey] = useState<HostKeyInfo | null>(null)
  const [sshGuvenildi, setSSHGuvenildi] = useState(false)

  useEffect(() => {
    Promise.all([
      api.get<Customer[]>('/customers'),
      api.get<Plan[]>('/plans'),
    ]).then(([c, p]) => {
      setCustomers(c.data)
      setPlans(p.data)
    }).catch(() => { /* seçim listesi boş kalır; API hatası importta gösterilir */ })
  }, [])

  useEffect(() => { api.get<RemoteJob[]>('/admin/transfers/remote/jobs').then(r => setSSHGecmis(r.data)).catch(() => {}) }, [])

  async function sshHazirla() {
    if (!sshHost) return
    setSSHBusy(true); setHata(null); setSSHGuvenildi(false); setSSHKesif(null)
    try {
      const [key, hostKey] = await Promise.all([
        api.post<{ public_key: string }>('/admin/transfers/remote/access-key'),
        api.post<HostKeyInfo>('/admin/transfers/remote/host-key/scan', { host: sshHost, port: sshPort }),
      ])
      setSSHPublicKey(key.data.public_key); setSSHHostKey(hostKey.data)
    } catch (e) { setHata(apiHata(e, t('HesapAktarimiPage:remote.prepare_failed'))) }
    finally { setSSHBusy(false) }
  }

  async function sshHostKeyOnayla() {
    if (!sshHostKey) return
    setSSHBusy(true); setHata(null)
    try {
      await api.post('/admin/transfers/remote/host-key/trust', {
        host: sshHostKey.host, port: sshHostKey.port, fingerprints: sshHostKey.fingerprints,
      })
      setSSHGuvenildi(true)
    } catch (e) { setHata(apiHata(e, t('HesapAktarimiPage:remote.trust_failed'))) }
    finally { setSSHBusy(false) }
  }

  async function analizEt() {
    if (!dosya) return
    setHata(null); setEnvanter(null); setYukleniyor(true); setIlerleme(0)
    const form = new FormData()
    form.append('archive', dosya)
    try {
      const r = await api.post<Inventory>('/admin/transfers/analyze', form, {
        timeout: 0,
        onUploadProgress: (e: AxiosProgressEvent) => {
          if (e.total) setIlerleme(Math.round((e.loaded / e.total) * 100))
        },
      })
      setEnvanter(r.data)
      setDomain(r.data.primary_domain || '')
    } catch (e) {
      setHata(apiHata(e, t('HesapAktarimiPage:error.analyze_failed')))
    } finally {
      setYukleniyor(false)
    }
  }

  async function uzakKesfet() {
    setSSHBusy(true); setHata(null); setSSHKesif(null)
    try {
      const r = await api.post<RemoteInventory>('/admin/transfers/remote/discover', { host: sshHost, port: sshPort, kullanici: 'root' })
      setSSHKesif(r.data); setSSHSecili([]); setSSHJobs([])
    } catch (e) { setHata(apiHata(e, t('HesapAktarimiPage:remote.failed'))) }
    finally { setSSHBusy(false) }
  }

  async function uzakAktar() {
    if (!sshKesif || sshSecili.length === 0 || !customerID) return
    setSSHBusy(true); setHata(null)
    try {
      const seciliSiteler = sshKesif.siteler.filter(s => sshSecili.includes(s.domain))
      const baslatmalar = await Promise.allSettled(seciliSiteler.map(async site => {
        const r = await api.post<{ job_id: number }>('/admin/transfers/remote/start', {
          host: sshHost, port: sshPort, provider: sshKesif.provider,
          hesap: site.hesap, domain: site.domain, customer_id: Number(customerID),
          plan_id: planID ? Number(planID) : null, php_version: site.php_version || phpVersion,
        })
        return { id: r.data.job_id, domain: site.domain, durum: 'queued', ilerleme: 0, mesaj: t('HesapAktarimiPage:remote.queued') } satisfies RemoteJob
      }))
      const sonuclar = baslatmalar.filter((s): s is PromiseFulfilledResult<RemoteJob> => s.status === 'fulfilled').map(s => s.value)
      setSSHJobs(sonuclar)
      const basarisiz = baslatmalar.length - sonuclar.length
      if (basarisiz > 0) setHata(t('HesapAktarimiPage:remote.partial_start_failed', { count: basarisiz }))
    } catch (e) { setHata(apiHata(e, t('HesapAktarimiPage:remote.start_failed'))) }
    finally { setSSHBusy(false) }
  }

  useEffect(() => {
    if (sshJobs.length === 0 || sshJobs.every(j => ['success', 'failed'].includes(j.durum))) return
    const timer = window.setInterval(() => {
      const devamEden = sshJobs.filter(j => !['success', 'failed'].includes(j.durum))
      Promise.all(devamEden.map(j => api.get<RemoteJob>(`/admin/transfers/remote/jobs/${j.id}`)))
        .then(yanitlar => {
          const guncel = new Map(yanitlar.map(r => [r.data.id, r.data]))
          setSSHJobs(eski => eski.map(j => guncel.get(j.id) || j))
        }).catch(() => {})
    }, 2000)
    return () => window.clearInterval(timer)
  }, [sshJobs])

  async function iceAktar() {
    if (!dosya || !envanter || !customerID || !domain) return
    setHata(null); setSonuc(null); setAktariliyor(true); setIlerleme(0)
    const form = new FormData()
    form.append('archive', dosya)
    form.append('customer_id', customerID)
    form.append('domain', domain)
    form.append('php_version', phpVersion)
    if (planID) form.append('plan_id', planID)
    try {
      const r = await api.post<ImportResult>('/admin/transfers/import', form, {
        timeout: 0,
        onUploadProgress: e => {
          if (e.total) setIlerleme(Math.round((e.loaded / e.total) * 100))
        },
      })
      setSonuc(r.data)
    } catch (e) {
      setHata(apiHata(e, t('HesapAktarimiPage:error.import_failed')))
    } finally {
      setAktariliyor(false)
    }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('HesapAktarimiPage:breadcrumb.admin') },
        { etiket: t('HesapAktarimiPage:breadcrumb.current') },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🚚</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('HesapAktarimiPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">
        {t('HesapAktarimiPage:subtitle')}
      </p>

      {hata && <div className="mb-4 px-4 py-3 rounded-xl border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-950/30 text-sm text-red-700 dark:text-red-300">{hata}</div>}

      <div className="rounded-2xl border border-brand-200 dark:border-brand-800 bg-brand-50/40 dark:bg-brand-950/10 p-5 mb-5">
        <h2 className="text-base font-semibold text-slate-800 dark:text-slate-100">{t('HesapAktarimiPage:remote.title')}</h2>
        <p className="mt-1 text-xs text-slate-500">{t('HesapAktarimiPage:remote.desc')}</p>
        <div className="mt-3 flex flex-col gap-2 sm:flex-row">
          <input value={sshHost} onChange={e => { setSSHHost(e.target.value); setSSHHostKey(null); setSSHGuvenildi(false); setSSHKesif(null) }} placeholder="old-server.example.com" className={`${inputClass} flex-1`} />
          <input type="number" min={1} max={65535} value={sshPort} onChange={e => { setSSHPort(Number(e.target.value)); setSSHHostKey(null); setSSHGuvenildi(false); setSSHKesif(null) }} className={`${inputClass} sm:w-28`} />
          <button onClick={() => void sshHazirla()} disabled={!sshHost || sshBusy} className="rounded-lg bg-brand-600 px-5 py-2.5 text-sm font-medium text-white disabled:opacity-50">{sshBusy ? t('HesapAktarimiPage:remote.preparing') : t('HesapAktarimiPage:remote.prepare')}</button>
        </div>
        {sshPublicKey && sshHostKey && <div className="mt-3 space-y-3 rounded-xl border border-amber-200 bg-amber-50 p-3 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/20 dark:text-amber-200">
          <div><strong>{t('HesapAktarimiPage:remote.public_key_title')}</strong><p className="mt-1">{t('HesapAktarimiPage:remote.public_key_desc')}</p></div>
          <div className="flex gap-2"><code className="min-w-0 flex-1 overflow-x-auto rounded bg-slate-900 p-2 text-[11px] text-slate-100">{sshPublicKey}</code><button type="button" onClick={() => navigator.clipboard?.writeText(sshPublicKey)} className="rounded border border-amber-300 px-3">{t('common:copy')}</button></div>
          <div><strong>{t('HesapAktarimiPage:remote.host_fingerprint')}</strong>{sshHostKey.fingerprints.map(fp => <code key={fp} className="mt-1 block font-mono text-[11px]">{fp}</code>)}</div>
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => void sshHostKeyOnayla()} disabled={sshBusy || sshGuvenildi} className="rounded-lg bg-amber-600 px-4 py-2 font-medium text-white disabled:opacity-50">{sshGuvenildi ? t('HesapAktarimiPage:remote.trusted') : t('HesapAktarimiPage:remote.trust')}</button>
            <button type="button" onClick={() => void uzakKesfet()} disabled={sshBusy || !sshGuvenildi} className="rounded-lg bg-emerald-600 px-4 py-2 font-medium text-white disabled:opacity-50">{sshBusy ? t('HesapAktarimiPage:remote.discovering') : t('HesapAktarimiPage:remote.discover')}</button>
          </div>
        </div>}
        {sshKesif && <div className="mt-3 rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950/20 dark:text-emerald-300">
          <strong>{sshKesif.provider}</strong>{sshKesif.surum && ` · ${sshKesif.surum}`}
          <div className="mt-1 text-xs">{t('HesapAktarimiPage:remote.domains', { count: sshKesif.domainler.length })}: {sshKesif.domainler.join(', ') || '—'}</div>
          {['sanalcp', 'cpanel', 'plesk', 'directadmin'].includes(sshKesif.provider) && <div className="mt-3 space-y-3">
            <div className="flex items-center justify-between text-xs font-medium">
              <span>{t('HesapAktarimiPage:remote.select_domains')}</span>
              <button type="button" className="text-brand-700 dark:text-brand-300" onClick={() => { const uygun = sshKesif.siteler.filter(s => s.transferable && !s.target_exists).map(s => s.domain); setSSHSecili(sshSecili.length === uygun.length ? [] : uygun) }}>
                {sshSecili.length > 0 && sshSecili.length === sshKesif.siteler.filter(s => s.transferable && !s.target_exists).length ? t('HesapAktarimiPage:remote.select_none') : t('HesapAktarimiPage:remote.select_all')}
              </button>
            </div>
            <div className="max-h-56 overflow-y-auto rounded-lg border border-emerald-200 bg-white/70 p-2 dark:border-emerald-800 dark:bg-slate-900/40">
              {sshKesif.siteler.map(site => <label key={site.domain} className={`block rounded px-2 py-1.5 ${!site.transferable ? 'cursor-not-allowed bg-red-50/70 dark:bg-red-950/20' : 'cursor-pointer hover:bg-emerald-100/60 dark:hover:bg-emerald-900/30'}`}>
                <span className="flex items-center gap-2">
                  <input type="checkbox" disabled={!site.transferable} checked={sshSecili.includes(site.domain)} onChange={e => setSSHSecili(eski => e.target.checked ? [...eski, site.domain] : eski.filter(d => d !== site.domain))} />
                  <span className="font-mono text-xs">{site.domain}</span>
                  <span className="ml-auto text-[11px] opacity-70">{site.hesap}{site.php_version ? ` · PHP ${site.php_version}` : ''}{site.application && site.application !== 'unknown' ? ` · ${site.application}` : ''}</span>
                  <span className={`text-[10px] font-semibold ${site.transferable ? 'text-emerald-700' : 'text-red-600'}`}>{site.transferable ? t('HesapAktarimiPage:remote.compatible') : t('HesapAktarimiPage:remote.incompatible')}</span>
                </span>
                {!!site.blockers?.length && <span className="mt-1 block pl-6 text-[11px] text-red-700 dark:text-red-300">{site.blockers.join(' · ')}</span>}
              </label>)}
            </div>
            <div className="grid gap-2 sm:grid-cols-3">
              <select value={customerID} onChange={e => setCustomerID(e.target.value)} className={inputClass}><option value="">{t('HesapAktarimiPage:target_section.select_customer')}</option>{customers.map(c => <option key={c.id} value={c.id}>{c.ad}</option>)}</select>
              <select value={planID} onChange={e => setPlanID(e.target.value)} className={inputClass}><option value="">{t('HesapAktarimiPage:target_section.default_plan')}</option>{plans.map(p => <option key={p.id} value={p.id}>{p.ad}</option>)}</select>
              <button onClick={() => void uzakAktar()} disabled={sshSecili.length === 0 || !customerID || sshBusy} className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">{t('HesapAktarimiPage:remote.start_selected', { count: sshSecili.length })}</button>
            </div>
          </div>}
        </div>}
        {sshJobs.map(sshJob => <div key={sshJob.id} className={`mt-3 rounded-xl border p-3 text-sm ${sshJob.durum === 'failed' ? 'border-red-200 bg-red-50 text-red-700' : sshJob.durum === 'success' ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-sky-200 bg-sky-50 text-sky-700'}`}>
          <div className="flex justify-between"><strong>{sshJob.domain} · #{sshJob.id}</strong><span>{sshJob.ilerleme}% · {sshJob.durum}</span></div>
          <div className="mt-1 text-xs">{sshJob.mesaj}</div><div className="mt-2 h-1.5 rounded bg-white"><div className="h-full rounded bg-brand-600" style={{ width: `${sshJob.ilerleme}%` }} /></div>
          {(sshJob.source_http_status || sshJob.target_http_status) && <div className="mt-2 text-xs">HTTP: {sshJob.source_http_status || '—'} → {sshJob.target_http_status || '—'}</div>}
          {sshJob.target_domain_id && <Link className="mt-2 inline-block font-medium text-brand-700" to={`/abonelikler/${sshJob.target_domain_id}`}>{t('HesapAktarimiPage:result.manage_domain')}</Link>}
        </div>)}
        {sshGecmis.length > 0 && <details className="mt-3 text-xs text-slate-600 dark:text-slate-300"><summary>{t('HesapAktarimiPage:remote.history')}</summary><div className="mt-2 space-y-1">{sshGecmis.map(j => <div key={j.id} className="flex justify-between rounded bg-white/70 p-2 dark:bg-slate-900/50"><span>#{j.id} · {j.domain}</span><span>{j.ilerleme}% · {j.durum}</span></div>)}</div></details>}
      </div>

      <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-5 mb-5">
        <div className="flex flex-col lg:flex-row lg:items-end gap-4">
          <label className="flex-1">
            <span className="block text-sm font-medium text-slate-700 dark:text-slate-200 mb-2">{t('HesapAktarimiPage:upload.file_label')}</span>
            <input type="file" accept=".tar.gz,.tgz,application/gzip"
              onChange={e => { setDosya(e.target.files?.[0] || null); setEnvanter(null) }}
              className="block w-full text-sm text-slate-600 dark:text-slate-300 file:mr-4 file:rounded-lg file:border-0 file:bg-brand-50 dark:file:bg-brand-950/40 file:px-4 file:py-2.5 file:text-sm file:font-medium file:text-brand-700 dark:file:text-brand-300 hover:file:bg-brand-100" />
          </label>
          <button onClick={analizEt} disabled={!dosya || yukleniyor}
            className="px-5 py-2.5 rounded-lg bg-brand-600 hover:bg-brand-700 text-white text-sm font-medium disabled:opacity-50">
            {yukleniyor ? t('HesapAktarimiPage:upload.uploading', { progress: ilerleme }) : t('HesapAktarimiPage:upload.analyze_button')}
          </button>
        </div>
        {dosya && <div className="mt-3 text-xs text-slate-400">{dosya.name} · {fmtByte(dosya.size)}</div>}
        {yukleniyor && <div className="mt-3 h-1.5 rounded-full bg-slate-100 dark:bg-slate-700 overflow-hidden"><div className="h-full bg-brand-600 transition-all" style={{ width: `${ilerleme}%` }} /></div>}
      </div>

      {envanter && (
        <>
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
            <Kpi etiket={t('HesapAktarimiPage:kpi.primary_domain')} deger={envanter.primary_domain || t('HesapAktarimiPage:kpi.undetermined')} />
            <Kpi etiket={t('HesapAktarimiPage:kpi.web_files')} deger={envanter.web_files.toLocaleString('tr-TR')} alt={fmtByte(envanter.web_bytes)} />
            <Kpi etiket={t('HesapAktarimiPage:kpi.databases')} deger={String(envanter.databases.length)} />
            <Kpi etiket={t('HesapAktarimiPage:kpi.mail_data')} deger={envanter.mail_files ? `${envanter.mail_files.toLocaleString('tr-TR')} ${t('HesapAktarimiPage:kpi.files_suffix')}` : t('HesapAktarimiPage:kpi.none')} />
          </div>

          {envanter.warnings.length > 0 && (
            <div className="mb-5 rounded-xl border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/20 px-4 py-3">
              {envanter.warnings.map(w => <div key={w} className="text-sm text-amber-800 dark:text-amber-300">⚠ {w}</div>)}
            </div>
          )}

          <div className="grid lg:grid-cols-2 gap-4">
            <Detail title={t('HesapAktarimiPage:detail.title')} rows={[
              [t('HesapAktarimiPage:detail.source_panel'), t('HesapAktarimiPage:detail.source_panel_value')],
              [t('HesapAktarimiPage:detail.user'), envanter.username || '—'],
              [t('HesapAktarimiPage:detail.archive_root'), envanter.archive_root || '—'],
              [t('HesapAktarimiPage:detail.total_entries'), envanter.entry_count.toLocaleString('tr-TR')],
              [t('HesapAktarimiPage:detail.expanded_size'), fmtByte(envanter.expanded_bytes)],
              [t('HesapAktarimiPage:detail.cron_jobs'), String(envanter.cron_jobs.length)],
              [t('HesapAktarimiPage:detail.ssl_files'), String(envanter.ssl_certs)],
              [t('HesapAktarimiPage:detail.mailboxes'), String(envanter.mailboxes.length)],
              [t('HesapAktarimiPage:detail.aliases'), String(envanter.alias_count)],
            ]} />
            <div className="space-y-4">
              <List title={t('HesapAktarimiPage:lists.databases')} values={envanter.databases} emptyLabel={t('HesapAktarimiPage:lists.empty')} />
              <List title={t('HesapAktarimiPage:lists.dns_zones')} values={envanter.dns_zones} emptyLabel={t('HesapAktarimiPage:lists.empty')} />
              <List title={t('HesapAktarimiPage:lists.cron_jobs')} values={envanter.cron_jobs.map(c => `${c.minute} ${c.hour} ${c.day} ${c.month} ${c.weekday}  ${c.command}`)} emptyLabel={t('HesapAktarimiPage:lists.empty')} />
            </div>
          </div>

          <div className="mt-5 rounded-xl border border-sky-200 dark:border-sky-800 bg-sky-50 dark:bg-sky-950/20 px-4 py-3 text-sm text-sky-800 dark:text-sky-300">
            {t('HesapAktarimiPage:info_note')}
          </div>

          <div className="mt-5 rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-5">
            <h2 className="text-base font-semibold text-slate-800 dark:text-slate-100 mb-4">{t('HesapAktarimiPage:target_section.title')}</h2>
            <div className="grid md:grid-cols-2 gap-4">
              <Field label={t('HesapAktarimiPage:target_section.target_customer')}>
                <select value={customerID} onChange={e => setCustomerID(e.target.value)} className={inputClass}>
                  <option value="">{t('HesapAktarimiPage:target_section.select_customer')}</option>
                  {customers.map(c => <option key={c.id} value={c.id}>{c.ad} — {c.eposta}</option>)}
                </select>
              </Field>
              <Field label={t('HesapAktarimiPage:target_section.service_plan')}>
                <select value={planID} onChange={e => setPlanID(e.target.value)} className={inputClass}>
                  <option value="">{t('HesapAktarimiPage:target_section.default_plan')}</option>
                  {plans.map(p => <option key={p.id} value={p.id}>{p.ad}</option>)}
                </select>
              </Field>
              <Field label={t('HesapAktarimiPage:target_section.primary_domain')}>
                <input value={domain} onChange={e => setDomain(e.target.value.toLowerCase())} className={inputClass} />
              </Field>
              <Field label={t('HesapAktarimiPage:target_section.php_version')}>
                <select value={phpVersion} onChange={e => setPHPVersion(e.target.value)} className={inputClass}>
                  {['7.4', '8.2', '8.3', '8.4', '8.5'].map(v => <option key={v}>{v}</option>)}
                </select>
              </Field>
            </div>
            <button onClick={iceAktar}
              disabled={aktariliyor || !customerID || !domain}
              className="mt-5 px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-white text-sm font-medium disabled:opacity-50">
              {aktariliyor ? t('HesapAktarimiPage:target_section.importing', { progress: ilerleme }) : t('HesapAktarimiPage:target_section.import_button')}
            </button>
          </div>

          {sonuc && <div className="mt-5 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-950/20 p-5">
            <h2 className="font-semibold text-emerald-800 dark:text-emerald-200">{t('HesapAktarimiPage:result.success_title')}</h2>
            <p className="mt-1 text-sm text-emerald-700 dark:text-emerald-300">{t('HesapAktarimiPage:result.summary', { domain: sonuc.domain, files: sonuc.web_files, databases: sonuc.databases.length })}</p>
            <p className="mt-1 text-xs text-emerald-700 dark:text-emerald-300">{t('HesapAktarimiPage:result.cron_imported', { count: sonuc.cron_jobs })}</p>
            {sonuc.ssl_imported && <p className="mt-1 text-xs text-emerald-700 dark:text-emerald-300">{t('HesapAktarimiPage:result.ssl_imported', { expires: sonuc.ssl_expires })}</p>}
            {sonuc.databases.map(d => <p key={d.target} className="mt-1 text-xs font-mono text-emerald-700 dark:text-emerald-300">{d.source} → {d.target}</p>)}
            {sonuc.mailboxes.length > 0 && <div className="mt-3 rounded-lg border border-amber-200 dark:border-amber-800 bg-white/60 dark:bg-slate-900/40 p-3">
              <p className="text-xs font-semibold text-amber-800 dark:text-amber-200 mb-2">{t('HesapAktarimiPage:result.mailbox_passwords_title')}</p>
              {sonuc.mailboxes.map(m => <p key={m.email} className="text-xs font-mono text-amber-800 dark:text-amber-200">{m.email}: {m.password_preserved ? t('HesapAktarimiPage:result.password_preserved') : m.password}</p>)}
              <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">{t('HesapAktarimiPage:result.aliases_imported', { count: sonuc.aliases })}</p>
            </div>}
            {sonuc.skipped?.map(s => <p key={s} className="mt-1 text-xs text-amber-700 dark:text-amber-300">{t('HesapAktarimiPage:result.skipped_prefix')}{s}</p>)}
            <Link to={`/abonelikler/${sonuc.domain_id}`} className="inline-block mt-3 text-sm font-medium text-brand-700 dark:text-brand-300">{t('HesapAktarimiPage:result.manage_domain')}</Link>
          </div>}
        </>
      )}
    </div>
  )
}

const inputClass = 'ta-input w-full'
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label><span className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1.5">{label}</span>{children}</label>
}

function Kpi({ etiket, deger, alt }: { etiket: string; deger: string; alt?: string }) {
  return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-4">
    <div className="text-[11px] uppercase tracking-wide font-semibold text-slate-400">{etiket}</div>
    <div className="mt-1 text-lg font-semibold text-slate-800 dark:text-slate-100 truncate">{deger}</div>
    {alt && <div className="text-xs text-slate-400">{alt}</div>}
  </div>
}

function Detail({ title, rows }: { title: string; rows: string[][] }) {
  return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-4">
    <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">{title}</h2>
    <dl className="divide-y divide-slate-100 dark:divide-slate-700/60">
      {rows.map(([k, v]) => <div key={k} className="flex justify-between gap-4 py-2 text-sm"><dt className="text-slate-500">{k}</dt><dd className="text-slate-800 dark:text-slate-200 font-mono text-xs text-right">{v}</dd></div>)}
    </dl>
  </div>
}

function List({ title, values, emptyLabel }: { title: string; values: string[]; emptyLabel: string }) {
  return <div className="rounded-2xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800/60 p-4">
    <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-2">{title} <span className="text-slate-400">({values.length})</span></h2>
    {values.length ? <div className="flex flex-wrap gap-1.5">{values.map(v => <span key={v} className="px-2 py-1 rounded-md bg-slate-100 dark:bg-slate-700 text-xs font-mono text-slate-700 dark:text-slate-200">{v}</span>)}</div> : <p className="text-xs text-slate-400">{emptyLabel}</p>}
  </div>
}

function fmtByte(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`
  return `${(b / 1024 ** 3).toFixed(2)} GB`
}
