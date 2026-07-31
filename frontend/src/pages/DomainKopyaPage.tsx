import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Kopya = { ad: string; boyut_mb: number; tarih: string }

export default function DomainKopyaPage() {
  const { t } = useTranslation(['DomainKopyaPage', 'common'])
  const { id } = useParams()
  const [liste, setListe] = useState<Kopya[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [ok, setOk] = useState<string | null>(null)
  const [olusturuyor, setOlusturuyor] = useState(false)

  function yukle() {
    if (!id) return
    api.get<Kopya[]>(`/domains/${id}/kopya`).then(r => setListe(r.data || [])).catch(e => setHata(apiHata(e))).finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function olustur() {
    setHata(null); setOk(null); setOlusturuyor(true)
    try {
      const { data } = await api.post(`/domains/${id}/kopya`, {})
      setOk(t('DomainKopyaPage:created', { ad: data.ad, mb: data.boyut_mb }))
      yukle()
    } catch (e) { setHata(apiHata(e, t('DomainKopyaPage:create_failed'))) }
    finally { setOlusturuyor(false) }
  }

  async function sil(k: Kopya) {
    if (!confirm(t('DomainKopyaPage:confirm_delete', { ad: k.ad, mb: k.boyut_mb }))) return
    setHata(null); setOk(null)
    try { await api.delete(`/domains/${id}/kopya/${k.ad}`); yukle() }
    catch (e) { setHata(apiHata(e, t('DomainKopyaPage:delete_failed'))) }
  }

  return (
    <div className="px-6 py-5">
      <div>
        <Breadcrumb items={[
          { etiket: t('common:home'), href: '/' },
          { etiket: t('common:domain'), href: '/domainler' },
          { etiket: t('DomainKopyaPage:title') },
        ]} />
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainKopyaPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
          {t('DomainKopyaPage:desc')}
        </p>

        {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
        {ok && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{ok}</div>}

        <div className="bg-amber-50 dark:bg-amber-900/10 border border-amber-200 dark:border-amber-800/40 rounded-2xl p-4 mb-4 text-xs text-amber-800 dark:text-amber-300">
          {t('DomainKopyaPage:info_box')}
        </div>

        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-5 shadow-sm flex items-center justify-between">
          <div className="text-sm text-slate-600 dark:text-slate-300">{t('DomainKopyaPage:create_new_desc')}</div>
          <button onClick={olustur} disabled={olusturuyor}
            className="px-4 py-2 text-sm font-medium bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-lg disabled:opacity-50">
            {olusturuyor ? t('DomainKopyaPage:creating') : t('DomainKopyaPage:create_button')}
          </button>
        </div>

        <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 shadow-sm">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainKopyaPage:existing_copies')}</h3>
          {yuk ? (
            <div className="text-sm text-slate-400">{t('common:loading')}</div>
          ) : liste.length === 0 ? (
            <div className="text-center py-8">
              <div className="text-3xl mb-2">📁</div>
              <p className="text-sm text-slate-500 dark:text-slate-400">{t('DomainKopyaPage:no_copies')}</p>
            </div>
          ) : (
            <ul className="divide-y divide-slate-50 dark:divide-slate-700/50">
              {liste.map(k => (
                <li key={k.ad} className="flex items-center justify-between py-2.5">
                  <div>
                    <div className="font-mono text-sm text-slate-700 dark:text-slate-200">{k.ad}</div>
                    <div className="text-xs text-slate-400">{k.tarih} · {k.boyut_mb} MB</div>
                  </div>
                  <button onClick={() => sil(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('common:delete')}</button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="mt-4"><Link to={`/abonelikler/${id}`} className="text-sm text-brand-600 dark:text-brand-400">{t('DomainKopyaPage:back_to_subscription')}</Link></div>
      </div>
    </div>
  )
}
