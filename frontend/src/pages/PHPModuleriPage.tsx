// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'

type Surum = { surum: string; ini_dir: string; service: string }
type Ext = { adi: string; aktif: boolean; ini_dosya: string }

const ZORUNLU = new Set([
  'core', 'date', 'standard', 'pdo', 'mysqlnd', 'phar', 'spl', 'reflection',
  'session', 'pcre', 'tokenizer', 'json', 'hash', 'random', 'libxml',
])

export default function PHPModuleriPage() {
  const { t } = useTranslation(['PHPModuleriPage', 'common'])
  const [surumler, setSurumler] = useState<Surum[]>([])
  const [aktifSurum, setAktifSurum] = useState('8.3')
  const [exts, setExts] = useState<Ext[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [filtre, setFiltre] = useState('')
  const [peclModal, setPeclModal] = useState(false)

  function yukle() {
    setYuk(true); setHata(null)
    api.get(`/php-extensions?surum=${aktifSurum}`)
      .then(r => {
        setExts(r.data.icerik || [])
        setSurumler(r.data.surumler || [])
      })
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [aktifSurum])

  async function toggle(e: Ext) {
    if (ZORUNLU.has(e.adi.toLowerCase())) {
      alert(t('PHPModuleriPage:core_module_alert'))
      return
    }
    const yeniAktif = !e.aktif
    try {
      await api.put('/php-extensions/toggle', {
        surum: aktifSurum,
        ini_dosya: e.ini_dosya,
        aktif: yeniAktif,
      })
      setBasari(t('PHPModuleriPage:toggle_success', {
        name: e.adi,
        state: t(yeniAktif ? 'PHPModuleriPage:toggle_state_enabled' : 'PHPModuleriPage:toggle_state_disabled'),
      }))
      setTimeout(() => setBasari(null), 3000)
      yukle()
    } catch (err) {
      setHata(apiHata(err, t('PHPModuleriPage:toggle_failed')))
    }
  }

  async function ioncubeKur() {
    if (!confirm(t('PHPModuleriPage:ioncube_install_confirm', { version: aktifSurum }))) return
    setYuk(true); setHata(null)
    try {
      const r = await api.post('/php-extensions/ioncube-kur', { surum: aktifSurum })
      const d = r.data
      setBasari(t('PHPModuleriPage:ioncube_install_success', {
        state: t(d.yuklendi ? 'PHPModuleriPage:ioncube_install_success_loaded' : 'PHPModuleriPage:ioncube_install_success_written'),
      }))
      setTimeout(() => setBasari(null), 5000)
      yukle()
    } catch (err) {
      setHata(apiHata(err, t('PHPModuleriPage:ioncube_install_failed')))
      setYuk(false)
    }
  }

  async function ioncubeKaldir() {
    if (!confirm(t('PHPModuleriPage:ioncube_remove_confirm', { version: aktifSurum }))) return
    setYuk(true); setHata(null)
    try {
      await api.post('/php-extensions/ioncube-kaldir', { surum: aktifSurum })
      setBasari(t('PHPModuleriPage:ioncube_remove_success'))
      setTimeout(() => setBasari(null), 3000)
      yukle()
    } catch (err) {
      setHata(apiHata(err, t('PHPModuleriPage:ioncube_remove_failed')))
      setYuk(false)
    }
  }

  async function peclKur(paket: string) {
    if (!paket.match(/^[a-zA-Z0-9_-]+$/)) {
      alert(t('PHPModuleriPage:pecl_invalid_name_alert')); return
    }
    if (!confirm(t('PHPModuleriPage:pecl_install_confirm', { package: paket, version: aktifSurum }))) return
    setPeclModal(false); setYuk(true)
    try {
      const r = await api.post('/php-extensions/pecl-install', { surum: aktifSurum, paket })
      setBasari(t('PHPModuleriPage:pecl_install_success', { package: paket }))
      console.log('PECL install output:', r.data.output)
      yukle()
    } catch (err) {
      setHata(apiHata(err, t('PHPModuleriPage:pecl_install_failed')))
      setYuk(false)
    }
  }

  const filtreli = filtre ? exts.filter(e => e.adi.toLowerCase().includes(filtre.toLowerCase())) : exts
  const aktifSayi = exts.filter(e => e.aktif).length
  const pasifSayi = exts.length - aktifSayi

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' },
        { etiket: t('PHPModuleriPage:breadcrumb.system_management') },
        { etiket: t('PHPModuleriPage:breadcrumb.php_modules') },
      ]} />

      <div className="flex items-center justify-between mb-1">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('PHPModuleriPage:title')}</h1>
        <div className="flex gap-2">
          <button onClick={() => {
              const ioncubeKurlu = exts.some(e => e.adi.toLowerCase().includes('ioncube'))
              if (ioncubeKurlu) ioncubeKaldir(); else ioncubeKur()
            }}
            className="px-4 py-2 bg-amber-600 hover:bg-amber-700 text-white text-sm rounded-md">
            {exts.some(e => e.adi.toLowerCase().includes('ioncube')) ? t('PHPModuleriPage:ioncube_remove_btn') : t('PHPModuleriPage:ioncube_install_btn')}
          </button>
          <button onClick={() => setPeclModal(true)}
            className="px-4 py-2 bg-slate-700 hover:bg-slate-800 text-white text-sm rounded-md">
            {t('PHPModuleriPage:pecl_install_btn')}
          </button>
        </div>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        {t('PHPModuleriPage:desc_prefix')} <strong>{t('PHPModuleriPage:desc_strong')}</strong> {t('PHPModuleriPage:desc_suffix')}
      </p>

      {/* Sürüm sekmesi */}
      <div className="flex gap-2 mb-4 border-b border-slate-200 dark:border-slate-700">
        {surumler.map(s => (
          <button key={s.surum} onClick={() => setAktifSurum(s.surum)}
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition ${
              aktifSurum === s.surum
                ? 'border-brand-500 text-brand-700 dark:text-brand-300'
                : 'border-transparent text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300 dark:text-slate-300'
            }`}>
            {t('PHPModuleriPage:version_tab', { version: s.surum })}
          </button>
        ))}
      </div>

      {/* Üst bar — sayaç + arama */}
      <div className="flex items-center justify-between mb-4 gap-3">
        <div className="flex items-center gap-3 text-sm">
          <span className="px-2.5 py-0.5 rounded-full bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300 font-medium text-xs">
            {t('PHPModuleriPage:active_count', { count: aktifSayi })}
          </span>
          <span className="px-2.5 py-0.5 rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 dark:text-slate-500 font-medium text-xs">
            {t('PHPModuleriPage:passive_count', { count: pasifSayi })}
          </span>
          <span className="text-slate-400 dark:text-slate-500 text-xs">{t('PHPModuleriPage:total_count', { count: exts.length })}</span>
        </div>
        <input
          type="text"
          value={filtre}
          onChange={e => setFiltre(e.target.value)}
          placeholder={t('PHPModuleriPage:search_placeholder')}
          className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm w-64 focus:border-brand-500 outline-none"
        />
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {yuk ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div> : (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
          {filtreli.map(e => {
            const zorunlu = ZORUNLU.has(e.adi.toLowerCase())
            return (
              <div key={e.ini_dosya}
                className={`flex items-center justify-between gap-2 px-3 py-2 rounded-md border ${
                  e.aktif
                    ? 'bg-emerald-50 dark:bg-emerald-900/20 border-emerald-200 dark:border-emerald-800'
                    : 'bg-slate-50 dark:bg-slate-900 border-slate-200 dark:border-slate-700'
                }`}>
                <div className="min-w-0 flex-1">
                  <div className="font-mono text-sm font-semibold text-slate-900 dark:text-slate-100 truncate">{e.adi}</div>
                  {zorunlu && <div className="text-[10px] text-slate-500 dark:text-slate-500">{t('PHPModuleriPage:core_module_label')}</div>}
                </div>
                <button
                  onClick={() => toggle(e)}
                  disabled={zorunlu}
                  className={`flex-shrink-0 relative inline-flex h-5 w-9 items-center rounded-full transition ${
                    e.aktif ? 'bg-emerald-500' : 'bg-slate-300'
                  } ${zorunlu ? 'opacity-40 cursor-not-allowed' : ''}`}
                  title={zorunlu ? t('PHPModuleriPage:core_module_title') : (e.aktif ? t('PHPModuleriPage:disable_tooltip') : t('PHPModuleriPage:enable_tooltip'))}
                >
                  <span className={`inline-block h-3 w-3 transform rounded-full bg-white dark:bg-slate-800 shadow transition ${e.aktif ? 'translate-x-5' : 'translate-x-1'}`} />
                </button>
              </div>
            )
          })}
        </div>
      )}

      {peclModal && (
        <div className="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onClick={() => setPeclModal(false)}>
          <div className="bg-white dark:bg-slate-800 rounded-2xl w-full max-w-md p-5 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-2">{t('PHPModuleriPage:pecl_modal_title')}</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-3">{t('PHPModuleriPage:pecl_modal_desc_prefix')} <code className="font-mono">mongodb, swoole, geoip, oauth, yaml, msgpack</code></p>
            <p className="text-xs text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded p-2 mb-3">
              {t('PHPModuleriPage:pecl_modal_warn_prefix', { version: aktifSurum })} <code className="font-mono">/etc/php.d/</code> {t('PHPModuleriPage:pecl_modal_warn_suffix')}
            </p>
            <input id="peclPaketAdi" type="text" autoFocus placeholder={t('PHPModuleriPage:pecl_modal_input_placeholder')}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded font-mono text-sm mb-3"
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  const v = (e.target as HTMLInputElement).value.trim()
                  if (v) peclKur(v)
                }
              }} />
            <div className="flex justify-end gap-2">
              <button onClick={() => setPeclModal(false)}
                className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-sm rounded">{t('common:cancel')}</button>
              <button onClick={() => {
                const v = (document.getElementById('peclPaketAdi') as HTMLInputElement)?.value?.trim()
                if (v) peclKur(v)
              }} className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded">{t('PHPModuleriPage:pecl_modal_install_btn')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}