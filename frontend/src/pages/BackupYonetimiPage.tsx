import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type OzetSatir = { domain_id: number; alan_adi: string; sayi: number; toplam_b: number; son_yedek: string }
type Ozet = { domainler: OzetSatir[]; toplam_boyut_b: number; toplam_yedek: number; hedef_sayisi: number; zamanlama: string }

export default function BackupYonetimiPage() {
  const { t } = useTranslation(['BackupYonetimiPage', 'common'])
  const [o, setO] = useState<Ozet | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [yedekliyor, setYedekliyor] = useState(false)

  function yukle() {
    setYuk(true)
    api.get<Ozet>('/admin/backups/ozet')
      .then(r => setO(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [])

  async function simdiYedekle() {
    setHata(null); setBasari(null); setYedekliyor(true)
    try {
      await api.post('/admin/backups/tick')
      setBasari(t('BackupYonetimiPage:success_triggered'))
    } catch (e) { setHata(apiHata(e, t('BackupYonetimiPage:error_trigger_failed'))) }
    finally { setYedekliyor(false) }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('BackupYonetimiPage:breadcrumb.tools_settings'), href: '/araclar-ayarlar' },
        { etiket: t('BackupYonetimiPage:title') },
      ]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">💾</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('BackupYonetimiPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">{t('BackupYonetimiPage:subtitle')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {/* KPI */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        <Kpi et={t('BackupYonetimiPage:kpi.total_size')} v={o ? fmtByte(o.toplam_boyut_b) : '—'} renk="sky" ikon="💽" />
        <Kpi et={t('BackupYonetimiPage:kpi.total_backups')} v={o ? String(o.toplam_yedek) : '—'} renk="violet" ikon="📦" />
        <Kpi et={t('BackupYonetimiPage:kpi.domain_count')} v={o ? String(o.domainler.length) : '—'} renk="teal" ikon="🌐" />
        <Kpi et={t('BackupYonetimiPage:kpi.active_remote_target')} v={o ? String(o.hedef_sayisi) : '—'} renk="emerald" ikon="☁️" alt={t('BackupYonetimiPage:kpi.s3_sftp')} />
      </div>

      {/* Zamanlama + eylem */}
      <div className="mb-5 flex flex-wrap items-center gap-3 px-4 py-3 rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60">
        <span className="text-sm text-slate-600 dark:text-slate-300">
          {t('BackupYonetimiPage:schedule.label')} <strong>{o?.zamanlama || t('BackupYonetimiPage:schedule.default_time')}</strong> · {t('BackupYonetimiPage:schedule.retention')}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button onClick={simdiYedekle} disabled={yedekliyor}
            className="px-3.5 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
            {yedekliyor ? t('BackupYonetimiPage:buttons.triggering') : t('BackupYonetimiPage:buttons.backup_now')}
          </button>
          <button onClick={yukle} disabled={yuk} className="px-3 py-2 text-sm border border-slate-200 dark:border-slate-700 rounded-lg text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">↻ {t('common:refresh')}</button>
        </div>
      </div>

      {/* Tablo */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('BackupYonetimiPage:table.title')}</h3>
        </div>
        <div className="lg:overflow-x-auto">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700/60`}>
              <tr>
                <th className={T.baslik}>{t('common:domain')}</th>
                <th className={`${T.baslik} text-right`}>{t('BackupYonetimiPage:table.backup_count')}</th>
                <th className={`${T.baslik} text-right`}>{t('BackupYonetimiPage:table.total_size')}</th>
                <th className={T.baslik}>{t('BackupYonetimiPage:table.last_backup')}</th>
                <th className={`${T.baslik} text-right`}>{t('common:actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700/60`}>
              {yuk ? (
                <tr><td colSpan={5} className={T.hucreDurum}>{t('common:loading')}</td></tr>
              ) : !o || o.domainler.length === 0 ? (
                <tr><td colSpan={5} className={T.hucreDurum}>{t('BackupYonetimiPage:table.no_domain')}</td></tr>
              ) : (
                o.domainler.map(d => (
                  <tr key={d.domain_id} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/40`}>
                    <td className={T.hucreBaslik}>{d.alan_adi}</td>
                    <td className={T.hucre} data-etiket={t('BackupYonetimiPage:table.backup_count')}><span className="font-mono text-xs text-slate-600 dark:text-slate-300">{d.sayi}</span></td>
                    <td className={T.hucre} data-etiket={t('BackupYonetimiPage:table.total_size')}><span className="font-mono text-xs text-slate-600 dark:text-slate-300">{d.sayi ? fmtByte(d.toplam_b) : '—'}</span></td>
                    <td className={T.hucre} data-etiket={t('BackupYonetimiPage:table.last_backup')}><span className="font-mono text-xs text-slate-500 dark:text-slate-400">{d.son_yedek || <span className="text-slate-400">{t('BackupYonetimiPage:table.never')}</span>}</span></td>
                    <td className={T.hucreAksiyon}>
                      <Link to={`/abonelikler/${d.domain_id}/yedekler`} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-brand-600 dark:text-brand-400 hover:bg-slate-50 dark:hover:bg-slate-700">{t('BackupYonetimiPage:table.manage')}</Link>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
      <p className="text-xs text-slate-400 dark:text-slate-500 mt-3">
        {t('BackupYonetimiPage:info_note_before')}<span className="font-mono">/var/backups/sanalcp/&lt;domain&gt;/</span>{t('BackupYonetimiPage:info_note_after', { manage: t('BackupYonetimiPage:table.manage_text') })}
      </p>
    </div>
  )
}

function Kpi({ et, v, renk, ikon, alt }: { et: string; v: string; renk: string; ikon: string; alt?: string }) {
  const c: Record<string, string> = {
    sky: 'text-sky-600 dark:text-sky-400', violet: 'text-violet-600 dark:text-violet-400',
    teal: 'text-teal-600 dark:text-teal-400', emerald: 'text-emerald-600 dark:text-emerald-400',
  }
  return (
    <div className="rounded-2xl border border-slate-200 dark:border-slate-700/60 bg-white dark:bg-slate-800/60 p-4">
      <div className="flex items-center gap-2 text-[11px] uppercase tracking-wide text-slate-400 font-semibold">{ikon} {et}</div>
      <div className={`text-2xl font-semibold mt-1 ${c[renk] || 'text-slate-700 dark:text-slate-200'}`}>{v}</div>
      {alt && <div className="text-[11px] text-slate-400 mt-0.5">{alt}</div>}
    </div>
  )
}

function fmtByte(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}
