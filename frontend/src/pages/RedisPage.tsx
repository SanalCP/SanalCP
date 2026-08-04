import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Durum = {
  aktif: boolean
  host: string
  port: number
  kullanici: string
  parola?: string
  prefix: string
  wp_snippet?: string
  wp_baglandi?: number
}

export default function RedisPage() {
  const { t } = useTranslation(['RedisPage', 'common'])
  const { id } = useParams()
  const [d, setD] = useState<Durum | null>(null)
  const [yuk, setYuk] = useState(true)
  const [mesgul, setMesgul] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [kopyalandi, setKopyalandi] = useState<string | null>(null)

  function yukle() {
    setYuk(true)
    api.get<Durum>(`/domains/${id}/redis`)
      .then(r => setD(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [id])

  async function ac() {
    setHata(null); setBasari(null); setMesgul(true)
    try {
      const { data } = await api.post<Durum>(`/domains/${id}/redis`, {})
      setD(data)
      setBasari(data.wp_baglandi && data.wp_baglandi > 0
        ? t('RedisPage:wp_connected_msg', { count: data.wp_baglandi })
        : t('RedisPage:no_wp_msg'))
    } catch (e) { setHata(apiHata(e, t('RedisPage:enable_failed'))) }
    finally { setMesgul(false) }
  }
  async function kapat() {
    if (!confirm(t('RedisPage:confirm_disable'))) return
    setHata(null); setBasari(null); setMesgul(true)
    try {
      await api.delete(`/domains/${id}/redis`)
      yukle()
      setBasari(t('RedisPage:disabled'))
    } catch (e) { setHata(apiHata(e, t('RedisPage:disable_failed'))) }
    finally { setMesgul(false) }
  }

  function kopyala(metin: string, etiket: string) {
    navigator.clipboard?.writeText(metin)
    setKopyalandi(etiket)
    setTimeout(() => setKopyalandi(null), 1500)
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('common:domain'), href: '/domainler' }, { etiket: t('RedisPage:breadcrumb_title') }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">⚡</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('RedisPage:title')}</h1>
        {d && (
          <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${d.aktif
            ? 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300'
            : 'bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400'}`}>
            {d.aktif ? t('RedisPage:active_badge') : t('RedisPage:inactive_badge')}
          </span>
        )}
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-4">
        {t('RedisPage:subtitle')}
      </p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('common:loading')}</div>
      ) : !d?.aktif ? (
        <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-6 text-center">
          <div className="text-3xl mb-2">⚡</div>
          <p className="text-sm text-slate-600 dark:text-slate-300 mb-1">{t('RedisPage:inactive_card_title')}</p>
          <p className="text-xs text-slate-400 mb-4">{t('RedisPage:inactive_card_desc')}</p>
          <div className="flex justify-center mb-4">
            <div className="inline-flex items-start gap-2 text-left px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
              <span className="text-amber-500 dark:text-amber-400 text-sm leading-none mt-0.5">⚠</span>
              <span className="text-xs text-amber-800 dark:text-amber-300">{t('RedisPage:resource_warning')}</span>
            </div>
          </div>
          <button onClick={ac} disabled={mesgul}
            className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-lg disabled:opacity-50">
            {mesgul ? t('RedisPage:enabling') : t('RedisPage:enable_button')}
          </button>
        </div>
      ) : (
        <>
          {/* Bağlantı bilgisi */}
          <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-4">
            <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60 flex items-center justify-between">
              <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('RedisPage:connection_info_title')}</h3>
              <button onClick={kapat} disabled={mesgul}
                className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">
                {t('RedisPage:close')}
              </button>
            </div>
            <div className="divide-y divide-slate-100 dark:divide-slate-700/60">
              <SatirKopya etiket={t('RedisPage:row_server')} deger={`${d.host}:${d.port}`} onKopya={kopyala} kopyalandi={kopyalandi} t={t} />
              <SatirKopya etiket={t('RedisPage:row_user')} deger={d.kullanici} onKopya={kopyala} kopyalandi={kopyalandi} t={t} />
              <SatirKopya etiket={t('RedisPage:row_password')} deger={d.parola || ''} gizli onKopya={kopyala} kopyalandi={kopyalandi} t={t} />
              <SatirKopya etiket={t('RedisPage:row_prefix')} deger={d.prefix} onKopya={kopyala} kopyalandi={kopyalandi} t={t} />
            </div>
          </div>

          {/* WordPress snippet */}
          {d.wp_snippet && (
            <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden">
              <div className="px-4 py-3 border-b border-slate-100 dark:border-slate-700/60 flex items-center justify-between">
                <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('RedisPage:wp_title')}</h3>
                <button onClick={() => kopyala(d.wp_snippet!, 'wp')}
                  className="text-xs px-2.5 py-1 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md">
                  {kopyalandi === 'wp' ? t('RedisPage:wp_copied_btn') : t('RedisPage:wp_copy_btn')}
                </button>
              </div>
              <div className="p-4">
                <p className="text-xs text-slate-500 dark:text-slate-400 mb-2">
                  {t('RedisPage:wp_step1')} {t('RedisPage:wp_step2')}
                </p>
                <pre className="text-[11px] font-mono bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-lg p-3 overflow-x-auto text-slate-700 dark:text-slate-200 whitespace-pre">{d.wp_snippet}</pre>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function SatirKopya({ etiket, deger, gizli, onKopya, kopyalandi, t }: {
  etiket: string; deger: string; gizli?: boolean
  onKopya: (m: string, e: string) => void; kopyalandi: string | null
  t: (k: string) => string
}) {
  const [goster, setGoster] = useState(false)
  const gorunen = gizli && !goster ? '•'.repeat(Math.min(deger.length, 20)) : deger
  return (
    <div className="flex items-center gap-3 px-4 py-2.5">
      <span className="text-xs text-slate-500 dark:text-slate-400 w-28 shrink-0">{etiket}</span>
      <span className="flex-1 font-mono text-xs text-slate-800 dark:text-slate-200 truncate">{gorunen}</span>
      {gizli && (
        <button onClick={() => setGoster(g => !g)} className="text-xs text-slate-400 hover:text-slate-600 dark:hover:text-slate-200">
          {goster ? t('RedisPage:hide') : t('RedisPage:show')}
        </button>
      )}
      <button onClick={() => onKopya(deger, etiket)}
        className="text-xs px-2 py-0.5 border border-slate-200 dark:border-slate-700 rounded text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">
        {kopyalandi === etiket ? t('RedisPage:copied_btn') : t('RedisPage:copy_btn')}
      </button>
    </div>
  )
}