// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Surum = {
  surum: string; kod: string; kaynak: 'remi' | 'appstream'
  yuklu: boolean
  pool_dir?: string; sock_dir?: string; service?: string; php_bin?: string
  gercek_surum?: string; modul_sayi?: number; aciklama?: string
}

export default function PHPSurumleriPage() {
  const { t } = useTranslation(['PHPSurumleriPage', 'common'])
  const [surumler, setSurumler] = useState<Surum[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [isleniyor, setIsleniyor] = useState<string | null>(null)
  const [output, setOutput] = useState<{ baslik: string; output: string } | null>(null)
  const [filtre, setFiltre] = useState<'tumu' | 'yuklu' | 'yuklenebilir'>('tumu')

  function yukle() {
    setYuk(true)
    api.get<{ surumler: Surum[] }>('/php-surumler')
      .then(r => setSurumler(r.data.surumler || []))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [])

  async function kur(s: Surum) {
    if (!confirm(t('PHPSurumleriPage:install_confirm', { version: s.surum, source: s.kaynak }))) return
    const key = s.surum + ':' + s.kaynak
    setIsleniyor(key); setHata(null); setBasari(null)
    try {
      const r = await api.post('/php-surumler/kur', { surum: s.surum, kaynak: s.kaynak })
      setBasari(t('PHPSurumleriPage:install_done', { version: s.surum }))
      setOutput({ baslik: t('PHPSurumleriPage:install_title', { version: s.surum }), output: r.data.output || '' })
      setTimeout(() => setBasari(null), 4000)
      yukle()
    } catch (e) { setHata(apiHata(e, t('PHPSurumleriPage:install_failed'))) }
    finally { setIsleniyor(null) }
  }

  async function kaldir(s: Surum) {
    if (s.kaynak === 'appstream') {
      alert(t('PHPSurumleriPage:appstream_remove_blocked'))
      return
    }
    if (!confirm(t('PHPSurumleriPage:remove_confirm', { version: s.surum }))) return
    const key = s.surum + ':' + s.kaynak
    setIsleniyor(key); setHata(null); setBasari(null)
    try {
      const r = await api.post('/php-surumler/kaldir', { surum: s.surum, kaynak: s.kaynak })
      setBasari(t('PHPSurumleriPage:remove_done', { version: s.surum }))
      setOutput({ baslik: t('PHPSurumleriPage:remove_title', { version: s.surum }), output: r.data.output || '' })
      setTimeout(() => setBasari(null), 4000)
      yukle()
    } catch (e) { setHata(apiHata(e, t('PHPSurumleriPage:remove_failed'))) }
    finally { setIsleniyor(null) }
  }

  const filtreli = surumler.filter(s => {
    if (filtre === 'yuklu') return s.yuklu
    if (filtre === 'yuklenebilir') return !s.yuklu
    return true
  })
  const yukluSayi = surumler.filter(s => s.yuklu).length

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('PHPSurumleriPage:breadcrumb.tools_settings') },
        { etiket: t('PHPSurumleriPage:breadcrumb.php_versions') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('PHPSurumleriPage:title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        {t('PHPSurumleriPage:desc')}
      </p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {/* Filtre */}
      <div className="flex items-center gap-2 mb-4">
        <span className="text-sm text-slate-600 dark:text-slate-400 dark:text-slate-500 mr-2">{t('PHPSurumleriPage:filter_label')}</span>
        {(['tumu', 'yuklu', 'yuklenebilir'] as const).map(f => (
          <button key={f} onClick={() => setFiltre(f)}
            className={`px-3 py-1 text-sm rounded ${filtre === f ? 'bg-brand-600 text-white' : 'border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-400 dark:text-slate-500 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
            {f === 'tumu' ? t('PHPSurumleriPage:filter_all') : f === 'yuklu' ? t('PHPSurumleriPage:filter_installed', { count: yukluSayi }) : t('PHPSurumleriPage:filter_installable', { count: surumler.length - yukluSayi })}
          </button>
        ))}
      </div>

      {yuk ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div> : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {filtreli.map(s => {
            const key = s.surum + ':' + s.kaynak
            const meşgul = isleniyor === key
            return (
              <div key={key}
                className={`border rounded-2xl p-4 transition ${s.yuklu ? 'border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/20' : 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800'}`}>
                <div className="flex items-start justify-between mb-2">
                  <div>
                    <div className="text-lg font-mono font-bold text-slate-900 dark:text-slate-100">PHP {s.surum}</div>
                    <div className="flex items-center gap-1.5 mt-0.5">
                      <span className={`text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium ${
                        s.kaynak === 'appstream'
                          ? 'bg-sky-100 text-sky-700'
                          : 'bg-violet-100 dark:bg-violet-900/30 text-violet-700 dark:text-violet-300'
                      }`}>{s.kaynak}</span>
                      {s.yuklu && <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300">{t('PHPSurumleriPage:installed_badge')}</span>}
                      {parseInt(s.surum) < 8 && <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300">{t('PHPSurumleriPage:eol_badge')}</span>}
                    </div>
                  </div>
                </div>

                {s.aciklama && <div className="text-xs text-slate-500 dark:text-slate-500 mb-2">{s.aciklama}</div>}

                {s.yuklu && (
                  <div className="text-xs text-slate-600 dark:text-slate-400 dark:text-slate-500 space-y-0.5 mb-3 font-mono bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded p-2">
                    {s.gercek_surum && <div>{t('PHPSurumleriPage:version_label')} <span className="text-slate-900 dark:text-slate-100">{s.gercek_surum}</span></div>}
                    {s.modul_sayi !== undefined && <div>{t('PHPSurumleriPage:module_label')} <span className="text-slate-900 dark:text-slate-100">{s.modul_sayi}</span></div>}
                    {s.service && <div className="truncate">{t('PHPSurumleriPage:service_label')} <span className="text-slate-700 dark:text-slate-300">{s.service}</span></div>}
                  </div>
                )}

                {s.yuklu ? (
                  s.kaynak === 'appstream' ? (
                    <button disabled className="w-full px-3 py-1.5 bg-slate-100 dark:bg-slate-800 text-slate-400 dark:text-slate-500 text-sm rounded cursor-not-allowed">
                      {t('PHPSurumleriPage:fixed_default')}
                    </button>
                  ) : (
                    <button onClick={() => kaldir(s)} disabled={meşgul}
                      className="w-full px-3 py-1.5 bg-red-600 hover:bg-red-700 disabled:bg-slate-300 text-white text-sm rounded">
                      {meşgul ? t('PHPSurumleriPage:removing') : t('PHPSurumleriPage:remove')}
                    </button>
                  )
                ) : (
                  <button onClick={() => kur(s)} disabled={meşgul}
                    className="w-full px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded">
                    {meşgul ? t('PHPSurumleriPage:installing') : t('PHPSurumleriPage:install')}
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}

      {output && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setOutput(null)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full shadow-xl flex flex-col max-h-[80vh]" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 dark:border-slate-700">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{output.baslik}</h3>
              <button onClick={() => setOutput(null)} className="text-slate-400 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300">×</button>
            </div>
            <pre className="flex-1 overflow-auto p-3 bg-slate-900 text-slate-100 text-xs font-mono whitespace-pre-wrap">{output.output}</pre>
            <div className="px-4 py-2 border-t border-slate-200 dark:border-slate-700 text-right">
              <button onClick={() => setOutput(null)}
                className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded">{t('common:close')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}