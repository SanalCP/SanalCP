// sanal-dark-swept
// sanal-dark-swept-v2
import { modalOnay, modalUyari } from '@/lib/dialog'
import { useCallback, useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
import Modal from '@/components/Modal'
import { T } from '@/lib/tablo'

type Domain = { id: number; alan_adi: string; sistem_kullanici: string }
type Yedek = { id: number; domain_id: number; tip: string; dosya: string; boyut_b: number; notlar: string; olusturma: string; uzak_durum?: string; uzak_hata?: string; dogrulama_durum: string; dogrulama_hata?: string; dogrulama_sha256?: string; dogrulama_zamani?: string }
type DB = { db_name: string }
type RestoreScope = 'full' | 'files' | 'file' | 'database' | 'email'
type Schedule = {
  freq: 'none' | 'daily' | 'weekly' | 'monthly'; hour: number
  retention: number          // kaç OTOMATİK yedek tutulacak
  manuel_retention: number   // kaç MANUEL yedek tutulacak — 0 = sınırsız
  last_backup_at?: string
}
const bosSchedule: Schedule = { freq: 'none', hour: 3, retention: 7, manuel_retention: 0 }
type Destination = {
  yok?: boolean
  id?: number; tip?: DestTip; host?: string; port?: number
  kullanici?: string; uzak_dizin?: string; aktif?: boolean
  bucket?: string; region?: string; endpoint?: string; path_style?: boolean
  son_yukleme?: string; son_durum?: string; son_hata?: string
}
type DestTip = 'ftp' | 'sftp' | 's3' | 'b2'
const bosDestForm = {
  tip: 'sftp' as DestTip, host: '', port: 22, kullanici: '', parola: '',
  uzak_dizin: '/', bucket: '', region: 'us-east-1', endpoint: '',
  path_style: true, aktif: true,
}

export default function DomainBackupsPage() {
  const { t } = useTranslation(['DomainBackupsPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [yedekler, setYedekler] = useState<Yedek[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [isleniyor, setIsleniyor] = useState(false)
  const [silinecek, setSilinecek] = useState<Yedek | null>(null)
  const [geriYukle, setGeriYukle] = useState<Yedek | null>(null)
  const [restoreScope, setRestoreScope] = useState<RestoreScope>('full')
  const [restorePath, setRestorePath] = useState('public_html/index.php')
  const [restoreDatabase, setRestoreDatabase] = useState('')
  const [databases, setDatabases] = useState<DB[]>([])

  const [sched, setSched] = useState<Schedule>(bosSchedule)
  const [schedKayit, setSchedKayit] = useState(false)

  const [dest, setDest] = useState<Destination>({ yok: true })
  const [destForm, setDestForm] = useState({ ...bosDestForm })
  const [destKayit, setDestKayit] = useState(false)
  const [destTest, setDestTest] = useState<{ ok: boolean; hata?: string } | null>(null)

  const yukle = useCallback(() => {
    if (!id) return
    setYuk(true)
    Promise.all([
      api.get<Yedek[]>(`/domains/${id}/backups`),
      api.get<Schedule>(`/domains/${id}/backup-schedule`).catch(() => ({ data: bosSchedule })),
      api.get<Destination>(`/domains/${id}/backup-destination`).catch(() => ({ data: { yok: true } as Destination })),
      api.get<DB[]>(`/domains/${id}/databases`).catch(() => ({ data: [] as DB[] })),
    ]).then(([y, s, d, dbs]) => {
      setYedekler(y.data)
      setDatabases(dbs.data)
      if (dbs.data.length) setRestoreDatabase(mevcut => mevcut || dbs.data[0].db_name)
      setSched(s.data)
      setDest(d.data)
      if (!d.data.yok) {
        setDestForm({
          tip: (d.data.tip || 'sftp') as DestTip,
          host: d.data.host || '',
          port: d.data.port || (d.data.tip === 'ftp' ? 21 : 22),
          kullanici: d.data.kullanici || '',
          parola: '',  // güvenlik: boş bırak, kullanıcı isterse yeniden girer
          uzak_dizin: d.data.uzak_dizin || '/',
          bucket: d.data.bucket || '',
          region: d.data.region || 'us-east-1',
          endpoint: d.data.endpoint || '',
          path_style: d.data.path_style ?? true,
          aktif: !!d.data.aktif,
        })
      }
    })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }, [id])

  async function destKaydet() {
    setDestKayit(true); setHata(null); setBasari(null); setDestTest(null)
    try {
      const r = await api.put<Destination>(`/domains/${id}/backup-destination`, destForm)
      setDest(r.data)
      setBasari(t('DomainBackupsPage:destination.saved'))
      setTimeout(() => setBasari(null), 4000)
    } catch (e) {
      setHata(apiHata(e, t('DomainBackupsPage:destination.save_failed')))
    } finally {
      setDestKayit(false)
    }
  }

  async function destBaglantiTesti() {
    setDestKayit(true); setDestTest(null)
    try {
      const r = await api.post<{ ok: boolean; hata?: string }>(`/domains/${id}/backup-destination/test`, destForm)
      setDestTest(r.data)
      setTimeout(() => setDestTest(null), 8000)
    } catch (e) {
      setDestTest({ ok: false, hata: apiHata(e) })
    } finally {
      setDestKayit(false)
    }
  }

  async function destSil() {
    if (!(await modalOnay(t('DomainBackupsPage:destination.delete_confirm')))) return
    setDestKayit(true)
    try {
      await api.delete(`/domains/${id}/backup-destination`)
      setDest({ yok: true })
      setDestForm({ ...bosDestForm })
      setBasari(t('DomainBackupsPage:destination.deleted'))
      setTimeout(() => setBasari(null), 4000)
    } catch (e) {
      setHata(apiHata(e))
    } finally {
      setDestKayit(false)
    }
  }
  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    yukle()
  }, [id, yukle])

  async function scheduleKaydet(yeni: Schedule) {
    setSchedKayit(true); setHata(null); setBasari(null)
    try {
      const r = await api.put<{ schedule: Schedule }>(`/domains/${id}/backup-schedule`, yeni)
      setSched(r.data.schedule)
      const freqAd = yeni.freq === 'daily' ? t('DomainBackupsPage:freq.daily') : yeni.freq === 'weekly' ? t('DomainBackupsPage:freq.weekly') : t('DomainBackupsPage:freq.monthly')
      setBasari(yeni.freq === 'none'
        ? t('DomainBackupsPage:schedule.saved_off')
        : t('DomainBackupsPage:schedule.saved_on', { freq: freqAd, hour: String(yeni.hour).padStart(2,'0'), retention: yeni.retention }))
      setTimeout(() => setBasari(null), 5000)
    } catch (e) {
      setHata(apiHata(e, t('DomainBackupsPage:schedule.save_failed')))
    } finally {
      setSchedKayit(false)
    }
  }

  async function olustur() {
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      await api.post(`/domains/${id}/backups`)
      setBasari(t('DomainBackupsPage:created'))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainBackupsPage:create_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/domains/${id}/backups/${silinecek.id}`)
      setSilinecek(null); yukle()
    } catch (e) {
      await modalUyari(apiHata(e))
    }
  }

  async function restore() {
    if (!geriYukle) return
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      const { data } = await api.post(`/domains/${id}/backups/${geriYukle.id}/geriyukle`, {
        scope: restoreScope,
        path: restoreScope === 'file' ? restorePath : '',
        database: restoreScope === 'database' ? restoreDatabase : '',
      })
      setBasari(t('DomainBackupsPage:restore_modal.restored', { domain: data.alan_adi, result: data.sonuc || '' }))
      setGeriYukle(null)
    } catch (e) {
      setHata(apiHata(e, t('DomainBackupsPage:restore_modal.restore_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  async function dogrula(y: Yedek) {
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      await api.post(`/domains/${id}/backups/${y.id}/dogrula`)
      setBasari(t('DomainBackupsPage:verification.completed'))
      yukle()
    } catch (e) { setHata(apiHata(e, t('DomainBackupsPage:verification.failed'))) }
    finally { setIsleniyor(false) }
  }

  function indir(y: Yedek) {
    // Oturum HttpOnly çerezde; credentials olmadan tarayıcı çerezi eklemez.
    fetch(`/api/v1/domains/${id}/backups/${y.id}/indir`, { credentials: 'include' })
      .then(r => r.blob())
      .then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = y.dosya
        a.click()
      })
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('DomainBackupsPage:breadcrumb.domains'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainBackupsPage:breadcrumb.backups') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainBackupsPage:title')}</h1>
      {domain && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        <Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">{domain.alan_adi}</Link>
        {' · '}{t('DomainBackupsPage:subtitle.base')} · {sched.freq === 'none'
          ? t('DomainBackupsPage:subtitle.auto_off')
          : t('DomainBackupsPage:subtitle.auto_on', { freq: sched.freq === 'daily' ? t('DomainBackupsPage:freq.daily') : sched.freq === 'weekly' ? t('DomainBackupsPage:freq.weekly') : t('DomainBackupsPage:freq.monthly'), hour: String(sched.hour).padStart(2,'0'), retention: sched.retention })}
      </p>}

      {/* Otomatik Yedek Planı */}
      <div className="mb-5 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainBackupsPage:schedule.title')}</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
              {t('DomainBackupsPage:schedule.desc')}
            </p>
          </div>
          {sched.last_backup_at && (
            <div className="text-xs text-slate-500 dark:text-slate-500">{t('DomainBackupsPage:schedule.last_backup')} <span className="font-mono">{sched.last_backup_at.replace('T',' ').replace('Z','')}</span></div>
          )}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-4 gap-3">
          {(['none','daily','weekly','monthly'] as const).map(f => {
            const aktif = sched.freq === f
            const meta: Record<string,{ad:string;ikon:string;aciklama:string;renk:string}> = {
              none: { ad: t('DomainBackupsPage:schedule.options.none.name'), ikon:'⏸', aciklama: t('DomainBackupsPage:schedule.options.none.desc'), renk:'slate' },
              daily: { ad: t('DomainBackupsPage:schedule.options.daily.name'), ikon:'🌙', aciklama: t('DomainBackupsPage:schedule.options.daily.desc'), renk:'emerald' },
              weekly: { ad: t('DomainBackupsPage:schedule.options.weekly.name'), ikon:'📅', aciklama: t('DomainBackupsPage:schedule.options.weekly.desc'), renk:'indigo' },
              monthly: { ad: t('DomainBackupsPage:schedule.options.monthly.name'), ikon:'🗓️', aciklama: t('DomainBackupsPage:schedule.options.monthly.desc'), renk:'indigo' },
            }
            const m = meta[f]
            const renk: Record<string,string> = {
              slate:   aktif ? 'border-slate-500 bg-slate-100 dark:bg-slate-800 ring-2 ring-slate-400/20'      : 'border-slate-200 dark:border-slate-700 hover:border-slate-400 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800',
              emerald: aktif ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-2 ring-emerald-500/20': 'border-slate-200 dark:border-slate-700 hover:border-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 dark:bg-emerald-900/20',
              indigo:  aktif ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20 ring-2 ring-indigo-500/20'   : 'border-slate-200 dark:border-slate-700 hover:border-indigo-300 hover:bg-indigo-50 dark:bg-indigo-900/20',
            }
            return (
              <button key={f} type="button" disabled={schedKayit || aktif}
                onClick={() => scheduleKaydet({ ...sched, freq: f })}
                className={`text-left p-3 border rounded-lg transition disabled:cursor-default ${renk[m.renk]}`}>
                <div className="flex items-center justify-between mb-1">
                  <span className="text-base leading-none">{m.ikon}</span>
                  {aktif && <span className="text-[10px] uppercase tracking-wider font-semibold text-emerald-700 dark:text-emerald-300">{t('DomainBackupsPage:schedule.active_badge')}</span>}
                </div>
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{m.ad}</div>
                <div className="text-[11px] text-slate-600 dark:text-slate-400 mt-1 leading-snug">{m.aciklama}</div>
              </button>
            )
          })}
        </div>

        {sched.freq !== 'none' && (
          <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <label className="block">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainBackupsPage:schedule.hour_label')}</span>
              <select
                value={sched.hour}
                onChange={e => scheduleKaydet({ ...sched, hour: Number(e.target.value) })}
                disabled={schedKayit}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm bg-white dark:bg-slate-800">
                {Array.from({length:24},(_,i)=>i).map(h =>
                  <option key={h} value={h}>{String(h).padStart(2,'0')}:00</option>
                )}
              </select>
            </label>
            <label className="block">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainBackupsPage:schedule.retention_label')}</span>
              <input type="number" min={1} max={90} value={sched.retention}
                onChange={e => setSched(s => ({...s, retention: Math.max(1, Math.min(90, Number(e.target.value)||1))}))}
                onBlur={() => scheduleKaydet(sched)}
                disabled={schedKayit}
                className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
              <span className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5 block">{t('DomainBackupsPage:schedule.retention_hint')}</span>
            </label>
          </div>
        )}

        {/* Manuel saklama: freq'ten BAĞIMSIZ gösterilir — otomatik yedek kapalı
            olsa da kullanıcı elle yedek alabilir, sınır orada da geçerlidir. */}
        <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-700">
          <label className="block sm:w-1/2">
            <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainBackupsPage:schedule.manual_retention_label')}</span>
            <input type="number" min={0} max={90} value={sched.manuel_retention}
              onChange={e => setSched(s => ({...s, manuel_retention: Math.max(0, Math.min(90, Number(e.target.value)||0))}))}
              onBlur={() => scheduleKaydet(sched)}
              disabled={schedKayit}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
            <span className="text-[10px] text-slate-500 dark:text-slate-500 mt-0.5 block">
              {sched.manuel_retention === 0
                ? t('DomainBackupsPage:schedule.manual_retention_hint_unlimited')
                : t('DomainBackupsPage:schedule.manual_retention_hint', { n: sched.manuel_retention })}
            </span>
          </label>
        </div>
      </div>

      {/* Uzak Yedek Hedefi */}
      <div className="mb-5 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainBackupsPage:destination.title')}</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
              {t('DomainBackupsPage:destination.desc')}
            </p>
          </div>
          {!dest.yok && dest.son_durum && (
            <span className={`text-[10px] uppercase tracking-wider font-semibold px-2 py-1 rounded ${
              dest.son_durum === 'basarili' ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' :
              dest.son_durum === 'hata' ? 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300' :
              'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400'
            }`}>{dest.son_durum === 'basarili' ? t('DomainBackupsPage:destination.status_ok') : dest.son_durum === 'hata' ? t('DomainBackupsPage:destination.status_err') : dest.son_durum}</span>
          )}
        </div>

        {!dest.yok && dest.son_yukleme && (
          <div className="mb-3 text-xs text-slate-500 dark:text-slate-500">
            {t('DomainBackupsPage:destination.last_upload')} <span className="font-mono">{dest.son_yukleme}</span>
            {dest.son_durum === 'hata' && dest.son_hata && (
              <div className="mt-1 text-[11px] text-red-700 dark:text-red-300 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded p-2 font-mono whitespace-pre-wrap">{dest.son_hata}</div>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-6 gap-3 mb-3">
          <div className="sm:col-span-6">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainBackupsPage:destination.protocol_label')}</label>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
              {(['sftp','ftp','s3','b2'] as const).map(tip => {
                const aktif = destForm.tip === tip
                const ad: Record<DestTip,string> = {
                  sftp: t('DomainBackupsPage:destination.providers.sftp'), ftp: t('DomainBackupsPage:destination.providers.ftp'), s3: t('DomainBackupsPage:destination.providers.s3'), b2: t('DomainBackupsPage:destination.providers.b2'),
                }
                return (
                  <button key={tip} type="button"
                    onClick={() => setDestForm(f => ({
                      ...f, tip: tip,
                      port: tip === 'sftp' ? 22 : tip === 'ftp' ? 21 : 443,
                      region: tip === 'b2' && f.region === 'us-east-1' ? '' : f.region,
                    }))}
                    className={`flex-1 text-xs px-3 py-2 rounded border ${aktif ? 'border-brand-500 bg-brand-50 dark:bg-brand-900/20 text-brand-700 dark:text-brand-300 font-semibold' : 'border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
                    {ad[tip]}
                  </button>
                )
              })}
            </div>
          </div>
          {(destForm.tip === 'ftp' || destForm.tip === 'sftp') ? <>
            <div className="sm:col-span-5">
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainBackupsPage:destination.host_label')}</label>
              <input type="text" value={destForm.host} placeholder={t('DomainBackupsPage:destination.host_placeholder')}
                onChange={e => setDestForm(f => ({...f, host: e.target.value}))}
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
            </div>
            <div className="sm:col-span-1">
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainBackupsPage:destination.port_label')}</label>
              <input type="number" value={destForm.port}
                onChange={e => setDestForm(f => ({...f, port: Number(e.target.value)||0}))}
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
            </div>
          </> : <>
            <div className="sm:col-span-3">
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainBackupsPage:destination.bucket_label')}</label>
              <input type="text" value={destForm.bucket} placeholder={t('DomainBackupsPage:destination.bucket_placeholder')}
                onChange={e => setDestForm(f => ({...f, bucket: e.target.value.trim()}))}
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
            </div>
            <div className="sm:col-span-3">
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainBackupsPage:destination.region_label')}</label>
              <input type="text" value={destForm.region}
                placeholder={destForm.tip === 'b2' ? t('DomainBackupsPage:destination.region_placeholder_b2') : t('DomainBackupsPage:destination.region_placeholder_s3')}
                onChange={e => setDestForm(f => ({...f, region: e.target.value.trim()}))}
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
            </div>
            <div className="sm:col-span-6">
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">
                {t('DomainBackupsPage:destination.endpoint_label')} {destForm.tip === 's3' && <span className="text-[10px] text-slate-400">{t('DomainBackupsPage:destination.endpoint_hint_s3')}</span>}
              </label>
              <input type="url" value={destForm.endpoint}
                placeholder={destForm.tip === 'b2' ? t('DomainBackupsPage:destination.endpoint_placeholder_b2') : t('DomainBackupsPage:destination.endpoint_placeholder_s3')}
                onChange={e => setDestForm(f => ({...f, endpoint: e.target.value.trim()}))}
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
            </div>
          </>}
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{destForm.tip === 's3' || destForm.tip === 'b2' ? t('DomainBackupsPage:destination.access_key_label') : t('DomainBackupsPage:destination.user_label')}</label>
            <input type="text" value={destForm.kullanici} autoComplete="off"
              onChange={e => setDestForm(f => ({...f, kullanici: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{destForm.tip === 's3' || destForm.tip === 'b2' ? t('DomainBackupsPage:destination.secret_key_label') : t('DomainBackupsPage:destination.password_label')} {!dest.yok && <span className="text-[10px] text-slate-400 dark:text-slate-500">{t('DomainBackupsPage:destination.password_hint_existing')}</span>}</label>
            <input type="password" value={destForm.parola} autoComplete="new-password"
              onChange={e => setDestForm(f => ({...f, parola: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{destForm.tip === 's3' || destForm.tip === 'b2' ? t('DomainBackupsPage:destination.prefix_label') : t('DomainBackupsPage:destination.remote_dir_label')}</label>
            <input type="text" value={destForm.uzak_dizin}
              onChange={e => setDestForm(f => ({...f, uzak_dizin: e.target.value}))}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono"/>
          </div>
        </div>

        <div className="flex items-center justify-between flex-wrap gap-3">
          <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
            <input type="checkbox" checked={destForm.aktif}
              onChange={e => setDestForm(f => ({...f, aktif: e.target.checked}))}
              className="cursor-pointer"/>
            {t('DomainBackupsPage:destination.active_checkbox')}
          </label>
          <div className="flex items-center gap-2">
            {destTest && (
              <span className={`text-xs px-2 py-1 rounded font-medium ${destTest.ok ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300' : 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300'}`}>
                {destTest.ok ? t('DomainBackupsPage:destination.test_ok') : t('DomainBackupsPage:destination.test_err_prefix') + (destTest.hata?.slice(0, 80) || t('DomainBackupsPage:destination.test_err_fallback'))}
              </span>
            )}
            <button type="button" onClick={destBaglantiTesti} disabled={destKayit || !destForm.kullanici || ((destForm.tip === 's3' || destForm.tip === 'b2') ? !destForm.bucket || (destForm.tip === 'b2' && !destForm.endpoint) : !destForm.host)}
              className="text-xs px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 disabled:opacity-50">
              {destKayit ? t('DomainBackupsPage:destination.testing') : t('DomainBackupsPage:destination.test_button')}
            </button>
            <button type="button" onClick={destKaydet} disabled={destKayit || !destForm.kullanici || ((destForm.tip === 's3' || destForm.tip === 'b2') ? !destForm.bucket || (destForm.tip === 'b2' && !destForm.endpoint) : !destForm.host)}
              className="text-xs px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 rounded font-medium">
              {t('common:save')}
            </button>
            {!dest.yok && (
              <button type="button" onClick={destSil} disabled={destKayit}
                className="text-xs px-3 py-1.5 border border-red-300 dark:border-red-700 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 rounded">
                {t('DomainBackupsPage:destination.delete_button')}
              </button>
            )}
          </div>
        </div>
      </div>

      <div className="flex items-center gap-2 mb-4">
        <button onClick={olustur} disabled={isleniyor} className="px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
          {isleniyor ? t('DomainBackupsPage:creating') : t('DomainBackupsPage:create_button')}
        </button>
        <button onClick={yukle} className="px-3 py-2 bg-white hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md">↻ {t('common:refresh')}</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{t('DomainBackupsPage:count', { count: yedekler.length })}</span>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {yuk ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div> :
         yedekler.length === 0 ? <div className="py-16 text-center text-sm text-slate-500 dark:text-slate-500">{t('DomainBackupsPage:empty')}</div> :
        <table className={T.tablo}>
          <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700`}>
            <tr>
              <th className={T.baslik}>{t('DomainBackupsPage:table.file')}</th>
              <th className={T.baslik}>{t('DomainBackupsPage:table.type')}</th>
              <th className={T.baslik}>{t('DomainBackupsPage:table.remote_copy')}</th>
              <th className={T.baslik}>{t('DomainBackupsPage:table.verification')}</th>
              <th className={T.baslik}>{t('DomainBackupsPage:table.size')}</th>
              <th className={T.baslik}>{t('DomainBackupsPage:table.created')}</th>
              <th className={`${T.baslik} text-right`}>{t('common:actions')}</th>
            </tr>
          </thead>
          <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
            {yedekler.map(y => (
              <tr key={y.id} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800`}>
                <td className={T.hucreBaslik}><span className="font-mono lg:text-sm text-base break-all">{y.dosya}</span></td>
                <td className={T.hucre} data-etiket={t('DomainBackupsPage:table.type')}>
                  <span className={`text-xs px-1.5 py-0.5 rounded uppercase tracking-wider font-semibold ${
                    y.tip === 'planli' ? 'bg-sky-100 text-sky-700' : 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400'
                  }`}>{y.tip === 'planli' ? t('DomainBackupsPage:type_scheduled') : y.tip}</span>
                </td>
                <td className={T.hucre} data-etiket={t('DomainBackupsPage:table.remote_copy')}>
                  {!y.uzak_durum ? <span className="text-xs text-slate-400">—</span> :
                   y.uzak_durum === 'basarili' ? <span className="text-xs text-emerald-600 dark:text-emerald-400">{t('DomainBackupsPage:remote_status.ok')}</span> :
                   y.uzak_durum === 'yukleniyor' ? <span className="text-xs text-sky-600 dark:text-sky-400">{t('DomainBackupsPage:remote_status.uploading')}</span> :
                   <span title={y.uzak_hata} className="text-xs text-red-600 dark:text-red-400">{t('DomainBackupsPage:remote_status.error')}</span>}
                </td>
                <td className={T.hucre} data-etiket={t('DomainBackupsPage:table.verification')}>
                  <div title={y.dogrulama_hata || y.dogrulama_sha256} className="text-xs">
                    {y.dogrulama_durum === 'dogrulandi' ? <span className="text-emerald-600 dark:text-emerald-400">✓ {t('DomainBackupsPage:verification.verified')}</span> :
                     y.dogrulama_durum === 'basarisiz' ? <span className="text-red-600 dark:text-red-400">✗ {t('DomainBackupsPage:verification.failed_short')}</span> :
                     y.dogrulama_durum === 'dogrulaniyor' ? <span className="text-sky-600 dark:text-sky-400">◌ {t('DomainBackupsPage:verification.verifying')}</span> :
                     <span className="text-amber-600 dark:text-amber-400">○ {t('DomainBackupsPage:verification.pending')}</span>}
                    {y.dogrulama_zamani && <div className="text-[10px] text-slate-400 mt-0.5">{y.dogrulama_zamani}</div>}
                  </div>
                </td>
                <td className={T.hucre} data-etiket={t('DomainBackupsPage:table.size')}><span className="font-mono text-sm text-slate-600 dark:text-slate-400">{formatBoyut(y.boyut_b)}</span></td>
                <td className={T.hucre} data-etiket={t('DomainBackupsPage:table.created')}><span className="text-sm text-slate-600 dark:text-slate-400">{y.olusturma}</span></td>
                <td className={T.hucreAksiyon}>
                  <button disabled={isleniyor} onClick={() => dogrula(y)} className="text-sm text-emerald-700 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 px-2 py-1 rounded disabled:opacity-50">{t('DomainBackupsPage:verification.button')}</button>
                  <button onClick={() => indir(y)} className="text-sm text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 dark:bg-brand-900/20 px-2 py-1 rounded">{t('DomainBackupsPage:download')}</button>
                  <button onClick={() => setGeriYukle(y)} className="text-sm text-amber-700 dark:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-900/30 dark:bg-amber-900/20 px-2 py-1 rounded">{t('DomainBackupsPage:restore_button')}</button>
                  <button onClick={() => setSilinecek(y)} className="text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 px-2 py-1 rounded">{t('common:delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>}
      </div>

      <ConfirmDialog
        acik={!!silinecek}
        baslik={t('DomainBackupsPage:delete_confirm_title')}
        mesaj={t('DomainBackupsPage:delete_confirm_message', { file: silinecek?.dosya })}
        tehlikeli onayMetni={t('DomainBackupsPage:delete_confirm_ok')}
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />

      {geriYukle && (
        <Modal acik baslik={t('DomainBackupsPage:restore_modal.title')} onKapat={() => { if (!isleniyor) setGeriYukle(null) }} genislik="lg" kapatEtiketi={t('common:cancel')}>
            <p className="mt-1 text-xs text-slate-500 font-mono break-all">{geriYukle.dosya}</p>
            <label className="block mt-4 text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainBackupsPage:restore_modal.scope_label')}</label>
            <select value={restoreScope} onChange={e => setRestoreScope(e.target.value as RestoreScope)}
              className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-900 text-sm">
              <option value="full">{t('DomainBackupsPage:restore_modal.scope_full')}</option>
              <option value="files">{t('DomainBackupsPage:restore_modal.scope_files')}</option>
              <option value="file">{t('DomainBackupsPage:restore_modal.scope_file')}</option>
              <option value="database">{t('DomainBackupsPage:restore_modal.scope_database')}</option>
              <option value="email">{t('DomainBackupsPage:restore_modal.scope_email')}</option>
            </select>
            {restoreScope === 'file' && (
              <label className="block mt-3">
                <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainBackupsPage:restore_modal.path_label')}</span>
                <input value={restorePath} onChange={e => setRestorePath(e.target.value)}
                  placeholder="public_html/index.php"
                  className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm"/>
              </label>
            )}
            {restoreScope === 'database' && (
              <label className="block mt-3">
                <span className="text-xs font-medium text-slate-600 dark:text-slate-400">{t('DomainBackupsPage:restore_modal.database_label')}</span>
                <select value={restoreDatabase} onChange={e => setRestoreDatabase(e.target.value)}
                  className="mt-1 w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-900 font-mono text-sm">
                  {!databases.length && <option value="">{t('DomainBackupsPage:restore_modal.database_not_found')}</option>}
                  {databases.map(db => <option key={db.db_name} value={db.db_name}>{db.db_name}</option>)}
                </select>
              </label>
            )}
            <div className="mt-4 p-3 rounded bg-amber-50 dark:bg-amber-900/20 text-xs text-amber-800 dark:text-amber-200">
              {t('DomainBackupsPage:restore_modal.warning')}
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <button onClick={() => setGeriYukle(null)} disabled={isleniyor}
                className="ta-secondary-button">{t('common:cancel')}</button>
              <button onClick={restore}
                disabled={isleniyor || (restoreScope === 'file' && !restorePath.trim()) || (restoreScope === 'database' && !restoreDatabase)}
                className="px-3 py-2 text-sm bg-red-600 hover:bg-red-700 text-white rounded disabled:opacity-50">
                {isleniyor ? t('DomainBackupsPage:restore_modal.restoring') : t('DomainBackupsPage:restore_modal.restore')}
              </button>
            </div>
        </Modal>
      )}
    </div>
  )
}

function formatBoyut(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(0)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}
