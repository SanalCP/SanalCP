// sanal-dark-swept
// sanal-dark-swept-v2
import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import CodeMirror from '@uiw/react-codemirror'
import { oneDark } from '@codemirror/theme-one-dark'

type Ayarlar = {
  hdr_x_content_type: boolean
  hdr_x_xss: boolean
  hdr_referrer: boolean
  hdr_permissions: boolean
  hdr_csp_upgrade: boolean
  hdr_hsts: boolean
  hsts_max_age: number
  hsts_subdomains: boolean
  hsts_preload: boolean
  fastcgi_cache: boolean
  fastcgi_cache_dakika: number
  browser_cache: boolean
  browser_cache_gun: number
  ek_direktifler: string
}

type Yanit = { alan_adi: string; ayarlar: Ayarlar }
type VhostOzelYanit = { ozel: boolean; icerik: string; alan_adi: string }

function backendBilgi(t: TFunction): Record<string, { ad: string; ikon: string; aciklama: string; renk: string }> {
  return {
    'php-fpm': {
      ad: t('DomainWebSunucuPage:backend_phpfpm'),
      ikon: '⚡',
      aciklama: t('DomainWebSunucuPage:backend_phpfpm_desc'),
      renk: 'emerald',
    },
    'apache': {
      ad: t('DomainWebSunucuPage:backend_apache'),
      ikon: '🪶',
      aciklama: t('DomainWebSunucuPage:backend_apache_desc'),
      renk: 'indigo',
    },
    'static': {
      ad: t('DomainWebSunucuPage:backend_static'),
      ikon: '📄',
      aciklama: t('DomainWebSunucuPage:backend_static_desc'),
      renk: 'slate',
    },
  }
}

function headerList(t: TFunction) {
  return [
    { key: 'hdr_x_content_type', etiket: t('DomainWebSunucuPage:header_x_content_type'), deger: 'nosniff',
      aciklama: t('DomainWebSunucuPage:header_x_content_type_desc') },
    { key: 'hdr_x_xss', etiket: t('DomainWebSunucuPage:header_x_xss'), deger: '1; mode=block',
      aciklama: t('DomainWebSunucuPage:header_x_xss_desc') },
    { key: 'hdr_referrer', etiket: t('DomainWebSunucuPage:header_referrer'), deger: 'strict-origin-when-cross-origin',
      aciklama: t('DomainWebSunucuPage:header_referrer_desc') },
    { key: 'hdr_permissions', etiket: t('DomainWebSunucuPage:header_permissions'), deger: 'geolocation=(), microphone=(), camera=(), interest-cohort=()',
      aciklama: t('DomainWebSunucuPage:header_permissions_desc') },
    { key: 'hdr_csp_upgrade', etiket: t('DomainWebSunucuPage:header_csp_upgrade'), deger: 'CSP: upgrade-insecure-requests',
      aciklama: t('DomainWebSunucuPage:header_csp_upgrade_desc') },
  ] as const
}

export default function DomainWebSunucuPage() {
  const { t } = useTranslation(['DomainWebSunucuPage', 'common'])
  const BACKEND_BILGI = backendBilgi(t)
  const HEADERS = headerList(t)
  const { id } = useParams()
  const [yanit, setYanit] = useState<Yanit | null>(null)
  const [a, setA] = useState<Ayarlar | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [isleniyor, setIsleniyor] = useState(false)

  const [backend, setBackend] = useState<string>('php-fpm')
  const [backendDegistiriliyor, setBackendDegistiriliyor] = useState(false)

  const [vhostOzel, setVhostOzel] = useState<VhostOzelYanit | null>(null)
  const [vhostOzelIcerikDuzenle, setVhostOzelIcerikDuzenle] = useState('')
  const [vhostOzelHata, setVhostOzelHata] = useState<string | null>(null)
  const [vhostOzelIsleniyor, setVhostOzelIsleniyor] = useState(false)
  const [vhostOzelKaydediliyor, setVhostOzelKaydediliyor] = useState(false)
  const vhostOzelKirli = vhostOzel !== null && vhostOzelIcerikDuzenle !== vhostOzel.icerik

  function yukle() {
    if (!id) return
    setYuk(true); setHata(null)
    Promise.all([
      api.get<Yanit>(`/domains/${id}/nginx-settings`),
      api.get<{backend: string}>(`/domains/${id}/web-backend`),
    ]).then(([y, b]) => {
      setYanit(y.data); setA(y.data.ayarlar)
      setBackend(b.data.backend)
    }).catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
    api.get<VhostOzelYanit>(`/domains/${id}/vhost-ozel`).then(v => {
      setVhostOzel(v.data)
      setVhostOzelIcerikDuzenle(v.data.icerik)
    }).catch(() => {})
  }
  useEffect(yukle, [id])

  async function vhostOzelKaydet() {
    setVhostOzelHata(null); setVhostOzelKaydediliyor(true)
    try {
      await api.put(`/domains/${id}/vhost-ozel`, { ozel: true, icerik: vhostOzelIcerikDuzenle })
      setVhostOzel(v => v ? { ...v, ozel: true, icerik: vhostOzelIcerikDuzenle } : v)
      setBasari(t('DomainWebSunucuPage:vhost_saved'))
      setTimeout(() => setBasari(null), 4000)
    } catch (e) {
      setVhostOzelHata(apiHata(e, t('DomainWebSunucuPage:vhost_save_failed')))
    } finally {
      setVhostOzelKaydediliyor(false)
    }
  }

  async function vhostOzelKapat() {
    if (!confirm('Bu dosyayı panelin standart yönetimine geri döndürmek istiyor musun?\n\nYaptığın düzenleme SİLİNMEZ — tekrar özel moda geçersen kaldığın yerden devam edersin.')) return
    setVhostOzelIsleniyor(true)
    try {
      await api.put(`/domains/${id}/vhost-ozel`, { ozel: false, icerik: vhostOzel?.icerik || '' })
      setBasari(t('DomainWebSunucuPage:vhost_saved'))
      setTimeout(() => setBasari(null), 4000)
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainWebSunucuPage:vhost_save_failed')))
    } finally {
      setVhostOzelIsleniyor(false)
    }
  }

  async function backendKaydet(yeni: string) {
    if (yeni === backend || backendDegistiriliyor) return
    setBackendDegistiriliyor(true); setHata(null); setBasari(null)
    try {
      await api.put(`/domains/${id}/web-backend`, { backend: yeni })
      setBackend(yeni)
      setBasari(t('DomainWebSunucuPage:saved'))
      setTimeout(() => setBasari(null), 4000)
    } catch (e) {
      setHata(apiHata(e, t('DomainWebSunucuPage:backend_change_failed')))
    } finally {
      setBackendDegistiriliyor(false)
    }
  }

  async function kaydet() {
    if (!a) return
    setIsleniyor(true); setHata(null); setBasari(null)
    try {
      await api.put(`/domains/${id}/nginx-settings`, { ayarlar: a })
      setBasari(t('DomainWebSunucuPage:saved'))
      yukle()
    } catch (e) {
      setHata(apiHata(e, t('DomainWebSunucuPage:save_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  function P<K extends keyof Ayarlar>(k: K, v: Ayarlar[K]) {
    if (!a) return
    setA({ ...a, [k]: v })
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('common:domain'), href: '/domainler' },
        { etiket: yanit?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainWebSunucuPage:breadcrumb_title') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainWebSunucuPage:title')}</h1>
      {yanit && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5">
        <Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">{yanit.alan_adi}</Link>
        {' · '}{t('DomainWebSunucuPage:subtitle')}
      </p>}

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300 whitespace-pre-wrap">{hata}</div>}
      {basari && <div className="mb-3 px-3 py-2 bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md text-sm text-emerald-700 dark:text-emerald-300">{basari}</div>}

      {/* Web Sunucu Yığını Seçici */}
      <div className="mb-6 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5">
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainWebSunucuPage:backend_title')}</h3>
            <p className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">
              {t('DomainWebSunucuPage:backend_desc')}
            </p>
          </div>
          {backendDegistiriliyor && <span className="text-xs text-slate-400 dark:text-slate-500">{t('DomainWebSunucuPage:applying')}</span>}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {(['php-fpm','apache','static'] as const).map(k => {
            const b = BACKEND_BILGI[k]
            const aktif = backend === k
            const renkler: Record<string, string> = {
              emerald: aktif ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-2 ring-emerald-500/20' : 'border-slate-200 dark:border-slate-700 hover:border-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-900/30 dark:bg-emerald-900/20',
              indigo:  aktif ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20 ring-2 ring-indigo-500/20'    : 'border-slate-200 dark:border-slate-700 hover:border-indigo-300 hover:bg-indigo-50 dark:hover:bg-indigo-900/20',
              slate:   aktif ? 'border-slate-500 bg-slate-100 dark:bg-slate-800 ring-2 ring-slate-400/20'      : 'border-slate-200 dark:border-slate-700 hover:border-slate-400 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800',
            }
            return (
              <button key={k} type="button"
                onClick={() => backendKaydet(k)}
                disabled={backendDegistiriliyor || aktif}
                className={`text-left p-4 border rounded-lg transition disabled:cursor-default ${renkler[b.renk]}`}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-lg leading-none">{b.ikon}</span>
                  {aktif && <span className="text-[10px] uppercase tracking-wider font-semibold text-emerald-700 dark:text-emerald-300">{t('DomainWebSunucuPage:active_label')}</span>}
                </div>
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{b.ad}</div>
                <div className="text-[11px] text-slate-600 dark:text-slate-400 mt-1.5 leading-snug">{b.aciklama}</div>
              </button>
            )
          })}
        </div>
      </div>

      <div className="mb-5 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md text-xs text-amber-800 dark:text-amber-200">
        {t('DomainWebSunucuPage:hsts_notice')}
      </div>

      {yuk || !a ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('DomainWebSunucuPage:loading')}</div> : (
        <>
          {vhostOzel?.ozel && (
            <div className="mb-5 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md text-xs text-amber-800 dark:text-amber-200">
              {t('DomainWebSunucuPage:vhost_warning')}
            </div>
          )}

          {/* Genel security headers */}
          <Kart baslik={t('DomainWebSunucuPage:security_headers_title')}>
            <div className="space-y-3">
              {HEADERS.map(h => (
                <SatirToggle
                  key={h.key}
                  etiket={h.etiket}
                  deger={h.deger}
                  aciklama={h.aciklama}
                  acik={a[h.key as keyof Ayarlar] as boolean}
                  onToggle={() => P(h.key as keyof Ayarlar, !a[h.key as keyof Ayarlar] as never)}
                />
              ))}
            </div>
          </Kart>

          {/* HSTS özel */}
          <Kart baslik={t('DomainWebSunucuPage:hsts_title')}>
            <SatirToggle
              etiket="Strict-Transport-Security"
              deger={t('DomainWebSunucuPage:hsts_value', {
                seconds: a.hsts_max_age,
                subdomains: a.hsts_subdomains ? t('DomainWebSunucuPage:hsts_subdomains_flag') : '',
                preload: a.hsts_preload ? t('DomainWebSunucuPage:hsts_preload_flag') : ''
              })}
              aciklama={t('DomainWebSunucuPage:hsts_desc')}
              acik={a.hdr_hsts}
              onToggle={() => P('hdr_hsts', !a.hdr_hsts)}
            />
            {a.hdr_hsts && (
              <div className="mt-3 pl-4 border-l-2 border-slate-200 dark:border-slate-700 space-y-2">
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainWebSunucuPage:max_age_label')}</label>
                  <select value={a.hsts_max_age} onChange={e => P('hsts_max_age', parseInt(e.target.value))}
                    className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono">
                    <option value={300}>{t('DomainWebSunucuPage:max_age_5min')}</option>
                    <option value={86400}>{t('DomainWebSunucuPage:max_age_1d')}</option>
                    <option value={604800}>{t('DomainWebSunucuPage:max_age_1w')}</option>
                    <option value={2592000}>{t('DomainWebSunucuPage:max_age_30d')}</option>
                    <option value={15768000}>{t('DomainWebSunucuPage:max_age_6mo')}</option>
                    <option value={31536000}>{t('DomainWebSunucuPage:max_age_1y')}</option>
                    <option value={63072000}>{t('DomainWebSunucuPage:max_age_2y')}</option>
                  </select>
                </div>
                <CheckboxRow
                  etiket={t('DomainWebSunucuPage:subdomains_label')}
                  aciklama={t('DomainWebSunucuPage:subdomains_desc')}
                  checked={a.hsts_subdomains}
                  onChange={v => P('hsts_subdomains', v)}
                />
                <CheckboxRow
                  etiket={t('DomainWebSunucuPage:preload_label')}
                  aciklama={t('DomainWebSunucuPage:preload_desc')}
                  checked={a.hsts_preload}
                  onChange={v => P('hsts_preload', v)}
                />
              </div>
            )}
          </Kart>

          {/* Performans Önbelleği */}
          <Kart baslik={t('DomainWebSunucuPage:cache_title')}>
            <SatirToggle
              etiket={t('DomainWebSunucuPage:fastcgi_cache_label')}
              deger={t('DomainWebSunucuPage:fastcgi_cache_value', { min: a.fastcgi_cache_dakika })}
              aciklama={t('DomainWebSunucuPage:fastcgi_cache_desc')}
              acik={a.fastcgi_cache}
              onToggle={() => P('fastcgi_cache', !a.fastcgi_cache)}
            />
            {a.fastcgi_cache && (
              <div className="mt-3 pl-4 border-l-2 border-slate-200 dark:border-slate-700">
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainWebSunucuPage:cache_minutes_label')}</label>
                <select value={a.fastcgi_cache_dakika} onChange={e => P('fastcgi_cache_dakika', parseInt(e.target.value))}
                  className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono">
                  <option value={5}>{t('DomainWebSunucuPage:cache_5min')}</option>
                  <option value={15}>{t('DomainWebSunucuPage:cache_15min')}</option>
                  <option value={60}>{t('DomainWebSunucuPage:cache_1h')}</option>
                  <option value={360}>{t('DomainWebSunucuPage:cache_6h')}</option>
                  <option value={1440}>{t('DomainWebSunucuPage:cache_1d')}</option>
                </select>
              </div>
            )}

            <div className="mt-4 pt-4 border-t border-slate-100 dark:border-slate-800">
              <SatirToggle
                etiket={t('DomainWebSunucuPage:browser_cache_label')}
                deger={t('DomainWebSunucuPage:browser_cache_value', { days: a.browser_cache_gun })}
                aciklama={t('DomainWebSunucuPage:browser_cache_desc')}
                acik={a.browser_cache}
                onToggle={() => P('browser_cache', !a.browser_cache)}
              />
              {a.browser_cache && (
                <div className="mt-3 pl-4 border-l-2 border-slate-200 dark:border-slate-700">
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainWebSunucuPage:cache_days_label')}</label>
                  <select value={a.browser_cache_gun} onChange={e => P('browser_cache_gun', parseInt(e.target.value))}
                    className="px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono">
                    <option value={1}>{t('DomainWebSunucuPage:cache_1d_short')}</option>
                    <option value={7}>{t('DomainWebSunucuPage:cache_7d')}</option>
                    <option value={30}>{t('DomainWebSunucuPage:cache_30d')}</option>
                    <option value={90}>{t('DomainWebSunucuPage:cache_7d')}</option>
                    <option value={365}>{t('DomainWebSunucuPage:cache_365d')}</option>
                  </select>
                </div>
              )}
            </div>
          </Kart>

          {vhostOzel && (
            <Kart baslik={t('DomainWebSunucuPage:vhost_ozel_title')}>
              <div className="mb-3 px-3 py-2 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md text-xs text-amber-800 dark:text-amber-200">
                Bu, nginx'in bu domain için şu an gerçekten sunduğu dosyadır. Doğrudan düzenleyip kaydedebilirsin — kaydettiğin an
                tüm vhost dosyası (HTTP→HTTPS yönlendirmesi ve Let's Encrypt doğrulama konumu dahil) senin sorumluluğuna geçer;
                yukarıdaki header/cache/ek-direktif ayarları ve panel bu dosyaya bir daha dokunmaz.{' '}
                <code className="font-mono">/.well-known/acme-challenge/</code> bloğunu kaldırırsan sertifika 90 gün sonra otomatik yenilenemez.
              </div>

              <div className="flex items-center justify-between gap-3 mb-3">
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  {vhostOzel.ozel ? '🟢 Özel vhost aktif — panel bu dosyaya dokunmuyor' : '⚪ Panel yönetiyor — aşağıda şu an aktif dosya gösteriliyor'}
                </div>
                <div className="flex items-center gap-2 flex-shrink-0">
                  {vhostOzelKirli && <span className="text-[10px] uppercase tracking-wider text-amber-600 dark:text-amber-400 bg-amber-500/15 px-1.5 py-0.5 rounded">{t('DomainWebSunucuPage:vhost_dirty')}</span>}
                  {vhostOzel.ozel && (
                    <button onClick={vhostOzelKapat} disabled={vhostOzelIsleniyor}
                      className="px-3 py-1.5 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 text-xs rounded-md">
                      {vhostOzelIsleniyor ? t('DomainWebSunucuPage:vhost_loading') : t('DomainWebSunucuPage:back_to_panel_manage')}
                    </button>
                  )}
                  <button onClick={vhostOzelKaydet} disabled={vhostOzelKaydediliyor || !vhostOzelKirli}
                    className="px-3.5 py-1.5 bg-emerald-600 hover:bg-emerald-700 disabled:opacity-40 text-white text-xs font-medium rounded-md">
                    {vhostOzelKaydediliyor ? t('DomainWebSunucuPage:vhost_loading') : '💾 ' + t('DomainWebSunucuPage:save')}
                  </button>
                </div>
              </div>

              {vhostOzelHata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-xs text-red-700 dark:text-red-300 whitespace-pre-wrap">{vhostOzelHata}</div>}

              <div className="rounded-lg overflow-hidden border border-slate-700">
                <CodeMirror
                  value={vhostOzelIcerikDuzenle}
                  height="480px"
                  theme={oneDark}
                  onChange={setVhostOzelIcerikDuzenle}
                  basicSetup={{
                    lineNumbers: true,
                    foldGutter: true,
                    highlightActiveLine: true,
                    highlightActiveLineGutter: true,
                    bracketMatching: true,
                    tabSize: 2,
                  }}
                  style={{ fontSize: '13px' }}
                />
              </div>
            </Kart>
          )}

          {/* Ek direktifler */}
          <Kart baslik={t('DomainWebSunucuPage:ek_title')}>
            <p className="text-xs text-slate-500 dark:text-slate-500 mb-2">
              Bu metin <code className="font-mono">server</code> bloğunun sonuna eklenir. Örn: <code className="font-mono">client_max_body_size 200m;</code>
            </p>
            <textarea value={a.ek_direktifler} onChange={e => P('ek_direktifler', e.target.value)}
              rows={6}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-xs font-mono"
              placeholder={t('DomainWebSunucuPage:ek_placeholder')} />
          </Kart>

          <div className="flex gap-3 mt-6">
            <button onClick={kaydet} disabled={isleniyor}
              className="px-6 py-2.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">
              {isleniyor ? t('DomainWebSunucuPage:applying') : '💾 ' + t('DomainWebSunucuPage:save')}
            </button>
            <button onClick={yukle} disabled={isleniyor}
              className="px-4 py-2.5 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 text-sm rounded-md">
              {t('DomainWebSunucuPage:vhost_revert')}
            </button>
          </div>
        </>
      )}
    </div>
  )
}

function Kart({ baslik, children }: { baslik: string; children: any }) {
  return (
    <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl p-5 mb-4">
      <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100 mb-3 pb-2 border-b border-slate-100 dark:border-slate-800">{baslik}</h3>
      {children}
    </div>
  )
}

function SatirToggle({ etiket, deger, aciklama, acik, onToggle }:
  { etiket: string; deger: string; aciklama: string; acik: boolean; onToggle: () => void }) {
  return (
    <div className="flex items-start gap-3 py-2 border-b border-slate-50 last:border-0">
      <button onClick={onToggle}
        className={`flex-shrink-0 mt-0.5 relative inline-flex h-6 w-11 items-center rounded-full transition ${
          acik ? 'bg-emerald-500' : 'bg-slate-300'
        }`}>
        <span className={`inline-block h-4 w-4 transform rounded-full bg-white dark:bg-slate-800 shadow transition ${acik ? 'translate-x-6' : 'translate-x-1'}`} />
      </button>
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline justify-between gap-2">
          <div className="font-mono text-sm font-semibold text-slate-900 dark:text-slate-100">{etiket}</div>
          <code className="text-xs font-mono text-slate-500 dark:text-slate-500 truncate">{deger}</code>
        </div>
        <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">{aciklama}</div>
      </div>
    </div>
  )
}

function CheckboxRow({ etiket, aciklama, checked, onChange }:
  { etiket: string; aciklama: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start gap-2 cursor-pointer">
      <input type="checkbox" checked={checked} onChange={e => onChange(e.target.checked)}
        className="mt-1 cursor-pointer" />
      <div>
        <div className="font-mono text-xs font-medium text-slate-900 dark:text-slate-100">{etiket}</div>
        <div className="text-xs text-slate-500 dark:text-slate-500">{aciklama}</div>
      </div>
    </label>
  )
}
