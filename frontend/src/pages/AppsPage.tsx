import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type TumKurulum = {
  domain_id: number; alan_adi: string; tur: string; tur_adi: string; dizin: string
  surum: string; son_surum: string; durum: 'guncel' | 'eski' | 'bilinmiyor'
  kurulum_tarihi: string; site_url: string; admin_url: string
}

export default function AppsPage() {
  const { t } = useTranslation(['AppsPage', 'common'])
  const [tum, setTum] = useState<TumKurulum[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)

  function listele() {
    setYuk(true)
    api.get<TumKurulum[]>('/apps/tumu')
      .then(r => setTum(r.data || []))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(() => { listele() }, [])

  const eskiler = useMemo(() => tum.filter(tk => tk.durum === 'eski'), [tum])

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('AppsPage:breadcrumb_title') }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">🧩</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('AppsPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">{t('AppsPage:subtitle')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {!yuk && eskiler.length > 0 && (
        <div className="mb-4 px-4 py-3 rounded-2xl border border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 flex items-start gap-3">
          <span className="text-lg leading-none">⚠️</span>
          <div className="text-sm text-amber-800 dark:text-amber-200">
            <strong>{t('AppsPage:update_warning_title', { count: eskiler.length })}</strong> {t('AppsPage:update_warning_desc')}
          </div>
        </div>
      )}

      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('AppsPage:installed_title')} {!yuk && <span className="text-slate-400 font-normal">· {tum.length}</span>}</h3>
          <button onClick={listele} disabled={yuk} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">{t('AppsPage:refresh')}</button>
        </div>
        <div className="lg:overflow-x-auto">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700/60`}>
              <tr>
                <th className={T.baslik}>{t('AppsPage:col_domain')}</th>
                <th className={T.baslik}>{t('AppsPage:col_app')}</th>
                <th className={T.baslik}>{t('AppsPage:col_path')}</th>
                <th className={T.baslik}>{t('AppsPage:col_version')}</th>
                <th className={T.baslik}>{t('AppsPage:col_status')}</th>
                <th className={`${T.baslik} text-right`}>{t('AppsPage:col_actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700/60`}>
              {yuk ? (
                <tr><td colSpan={6} className={T.hucreDurum}>{t('AppsPage:scanning')}</td></tr>
              ) : tum.length === 0 ? (
                <tr><td colSpan={6} className={T.hucreDurum}>
                  <div className="text-2xl mb-1">🧩</div>
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('AppsPage:no_installations')}</p>
                </td></tr>
              ) : (
                tum.map(tk => {
                  const key = tk.domain_id + tk.tur + tk.dizin
                  const eski = tk.durum === 'eski'
                  return (
                    <tr key={key} className={`${T.satir} ${eski ? 'lg:bg-amber-50/50 dark:lg:bg-amber-900/10' : 'lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/40'}`}>
                      <td className={T.hucreBaslik}>
                        <a href={tk.site_url} target="_blank" rel="noreferrer" className="font-medium text-slate-800 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400">{tk.alan_adi}</a>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_app')}>
                        <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 font-medium">{tk.tur_adi}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_path')}>
                        <span className="font-mono text-xs text-slate-500 dark:text-slate-400 whitespace-nowrap">{tk.dizin}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_version')}>
                        <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 font-mono font-semibold">{tk.surum ? `v${tk.surum}` : '—'}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('AppsPage:col_status')}>
                        {tk.durum === 'eski' ? (
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-200 font-medium">{t('AppsPage:status_update_to', { version: tk.son_surum })}</span>
                        ) : tk.durum === 'guncel' ? (
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300 font-medium">{t('AppsPage:status_current')}</span>
                        ) : (
                          <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400 font-medium">{t('AppsPage:status_unknown')}</span>
                        )}
                      </td>
                      <td className={T.hucreAksiyon}>
                        <div className="flex items-center flex-wrap gap-1.5 lg:justify-end">
                          <a href={tk.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('AppsPage:admin_link')}</a>
                          <a href={`/abonelikler/${tk.domain_id}/uygulamalar`} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('AppsPage:manage_link')}</a>
                        </div>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
