import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import { T } from '@/lib/tablo'

type Domain = { id: number; alan_adi: string }
type Sonuc = { site_url: string; admin_url: string; admin_kullanici: string; admin_parola: string; surum: string }
type TumKurulum = {
  domain_id: number; alan_adi: string; dizin: string; surum: string
  son_surum: string; durum: 'guncel' | 'eski' | 'bilinmiyor'; kurulum_tarihi: string
  site_url: string; admin_url: string
}

export default function WordPressPage() {
  const { t } = useTranslation(['WordPressPage', 'common'])
  const [domainler, setDomainler] = useState<Domain[]>([])
  const [domainId, setDomainId] = useState<number | null>(null)
  const [tum, setTum] = useState<TumKurulum[]>([])
  const [tumYuk, setTumYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [kuruyor, setKuruyor] = useState(false)
  const [sonuc, setSonuc] = useState<Sonuc | null>(null)
  const [mesgul, setMesgul] = useState<string | null>(null)

  const [altDizin, setAltDizin] = useState('')
  const [baslik, setBaslik] = useState('')
  const [adminK, setAdminK] = useState('admin')
  const [adminE, setAdminE] = useState('')

  useEffect(() => {
    api.get<Domain[]>('/domains').then(r => {
      setDomainler(r.data || [])
      if (r.data?.length) setDomainId(r.data[0].id)
    }).catch(e => setHata(apiHata(e)))
    tumListele()
  }, [])

  function tumListele() {
    setTumYuk(true)
    api.get<TumKurulum[]>('/wordpress/tumu')
      .then(r => setTum(r.data || []))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setTumYuk(false))
  }

  async function kur(e: React.FormEvent) {
    e.preventDefault()
    if (!domainId) return
    setHata(null); setSonuc(null); setKuruyor(true)
    try {
      const { data } = await api.post<Sonuc>(`/domains/${domainId}/wordpress`, {
        alt_dizin: altDizin.trim(), site_basligi: baslik.trim(), admin_kullanici: adminK.trim(), admin_email: adminE.trim(),
      })
      setSonuc(data); setBaslik(''); setAltDizin('')
      tumListele()
    } catch (err) { setHata(apiHata(err, t('WordPressPage:install_failed'))) }
    finally { setKuruyor(false) }
  }

  async function guncelle(tk: TumKurulum) {
    const key = tk.domain_id + tk.dizin
    setMesgul(key); setHata(null)
    try { await api.post(`/domains/${tk.domain_id}/wordpress/guncelle`, { dizin: tk.dizin }); tumListele() }
    catch (err) { setHata(apiHata(err, t('WordPressPage:update_failed'))) }
    finally { setMesgul(null) }
  }

  async function sil(tk: TumKurulum) {
    if (tk.dizin.includes('kök')) { alert(t('WordPressPage:root_cant_delete')); return }
    if (!confirm(t('WordPressPage:confirm_delete', { yol: `${tk.alan_adi}${tk.dizin}` }))) return
    const key = tk.domain_id + tk.dizin
    setMesgul(key); setHata(null)
    try {
      await api.delete(`/domains/${tk.domain_id}/wordpress`, { data: { dizin: tk.dizin, db_sil: true } })
      tumListele()
    } catch (err) { setHata(apiHata(err, t('WordPressPage:delete_failed'))) }
    finally { setMesgul(null) }
  }

  const sel = domainler.find(d => d.id === domainId)
  const eskiler = useMemo(() => tum.filter(tk => tk.durum === 'eski'), [tum])

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('WordPressPage:breadcrumb_title') }]} />
      <div className="flex items-center gap-3 mb-1">
        <span className="text-2xl">📝</span>
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('WordPressPage:title')}</h1>
      </div>
      <p className="text-sm text-slate-500 dark:text-slate-400 mb-5">{t('WordPressPage:subtitle')}</p>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {/* Güvenlik uyarı bandı */}
      {!tumYuk && eskiler.length > 0 && (
        <div className="mb-4 px-4 py-3 rounded-2xl border border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 flex items-start gap-3">
          <span className="text-lg leading-none">⚠️</span>
          <div className="text-sm text-amber-800 dark:text-amber-200">
            <strong>{t('WordPressPage:update_warning_title', { count: eskiler.length })}</strong> {t('WordPressPage:update_warning_desc')}
            <div className="mt-1 text-xs text-amber-700 dark:text-amber-300 font-mono">
              {eskiler.map(e => `${e.alan_adi}${e.dizin === '/ (kök)' ? '' : e.dizin}`).join(' · ')}
            </div>
          </div>
        </div>
      )}

      {/* Kurulum sonucu — kimlik bilgileri (bir kez) */}
      {sonuc && (
        <div className="mb-4 rounded-2xl border border-emerald-200 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/15 p-4">
          <div className="flex items-center gap-2 text-sm font-semibold text-emerald-700 dark:text-emerald-300 mb-2">
            {t('WordPressPage:installed_ok', { version: sonuc.surum })}
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
            <Bilgi et={t('WordPressPage:result_site')} v={sonuc.site_url} link />
            <Bilgi et={t('WordPressPage:result_admin')} v={sonuc.admin_url} link />
            <Bilgi et={t('WordPressPage:result_user')} v={sonuc.admin_kullanici} mono />
            <Bilgi et={t('WordPressPage:result_password')} v={sonuc.admin_parola} mono />
          </div>
          <p className="text-[11px] text-amber-700 dark:text-amber-400 mt-2">{t('WordPressPage:password_warning')}</p>
        </div>
      )}

      {/* Geniş tablo: tüm kurulumlar */}
      <div className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl overflow-hidden mb-6">
        <div className="flex items-center justify-between px-4 py-3 border-b border-slate-100 dark:border-slate-700/60">
          <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{t('WordPressPage:installed_title')} {!tumYuk && <span className="text-slate-400 font-normal">· {tum.length}</span>}</h3>
          <button onClick={tumListele} disabled={tumYuk} className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-50">{t('WordPressPage:refresh')}</button>
        </div>
        <div className="lg:overflow-x-auto">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/50 border-b border-slate-200 dark:border-slate-700/60`}>
              <tr>
                <th className={T.baslik}>{t('WordPressPage:col_domain')}</th>
                <th className={T.baslik}>{t('WordPressPage:col_path')}</th>
                <th className={T.baslik}>{t('WordPressPage:col_version')}</th>
                <th className={T.baslik}>{t('WordPressPage:col_status')}</th>
                <th className={T.baslik}>{t('WordPressPage:col_installed')}</th>
                <th className={`${T.baslik} text-right`}>{t('WordPressPage:col_actions')}</th>
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-700/60`}>
              {tumYuk ? (
                <tr><td colSpan={6} className={T.hucreDurum}>{t('WordPressPage:scanning')}</td></tr>
              ) : tum.length === 0 ? (
                <tr><td colSpan={6} className={T.hucreDurum}>
                  <div className="text-2xl mb-1">📝</div>
                  <p className="text-sm text-slate-500 dark:text-slate-400">{t('WordPressPage:no_installations')}</p>
                  <p className="text-xs text-slate-400 mt-1">{t('WordPressPage:no_installations_hint')}</p>
                </td></tr>
              ) : (
                tum.map(tk => {
                  const key = tk.domain_id + tk.dizin
                  const eski = tk.durum === 'eski'
                  return (
                    <tr key={key} className={`${T.satir} ${eski ? 'lg:bg-amber-50/50 dark:lg:bg-amber-900/10' : 'lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800/40'}`}>
                      <td className={T.hucreBaslik}>
                        <a href={tk.site_url} target="_blank" rel="noreferrer" className="font-medium text-slate-800 dark:text-slate-100 hover:text-brand-600 dark:hover:text-brand-400">{tk.alan_adi}</a>
                      </td>
                      <td className={T.hucre} data-etiket={t('WordPressPage:col_path')}>
                        <span className="font-mono text-xs text-slate-500 dark:text-slate-400 whitespace-nowrap">{tk.dizin}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('WordPressPage:col_version')}>
                        <span className="text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 font-mono font-semibold">{tk.surum ? `${t('WordPressPage:v_prefix')}${tk.surum}` : t('WordPressPage:no_version')}</span>
                      </td>
                      <td className={T.hucre} data-etiket={t('WordPressPage:col_status')}><DurumRozet tk={tk} /></td>
                      <td className={T.hucre} data-etiket={t('WordPressPage:col_installed')}>
                        <span className="text-xs text-slate-500 dark:text-slate-400 font-mono whitespace-nowrap">{tk.kurulum_tarihi || t('WordPressPage:no_version')}</span>
                      </td>
                      <td className={T.hucreAksiyon}>
                        <div className="flex items-center flex-wrap gap-1.5 lg:justify-end">
                          <a href={tk.admin_url} target="_blank" rel="noreferrer" className="text-xs px-2.5 py-1 border border-slate-200 dark:border-slate-700 rounded-md text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700">{t('WordPressPage:admin_link')}</a>
                          <button disabled={!!mesgul} onClick={() => guncelle(tk)}
                            className={`text-xs px-2.5 py-1 rounded-md disabled:opacity-50 ${eski ? 'bg-amber-500 hover:bg-amber-600 text-white' : 'border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'}`}>
                            {mesgul === key ? '…' : eski ? t('WordPressPage:update_to_link', { version: tk.son_surum }) : t('WordPressPage:update_link')}
                          </button>
                          {!tk.dizin.includes('kök') && (
                            <button disabled={!!mesgul} onClick={() => sil(tk)} className="text-xs px-2.5 py-1 border border-red-300 dark:border-red-800 text-red-600 dark:text-red-400 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50">{t('WordPressPage:delete')}</button>
                          )}
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

      {/* Yeni kurulum */}
      <form onSubmit={kur} className="bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 rounded-2xl p-4 max-w-2xl">
        <h3 className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold mb-3">{t('WordPressPage:new_install_title')}</h3>
        <div className="mb-3">
          <label className="ta-label-sm">{t('WordPressPage:domain_label')}</label>
          <select value={domainId ?? ''} onChange={e => setDomainId(Number(e.target.value))}
            className="ta-input ta-input-sm w-full sm:w-80">
            {domainler.map(d => <option key={d.id} value={d.id}>{d.alan_adi}</option>)}
          </select>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Alan et={t('WordPressPage:site_title_label')} v={baslik} set={setBaslik} zorunlu ph={t('WordPressPage:site_title_placeholder')} />
          <Alan et={t('WordPressPage:subdir_label')} v={altDizin} set={setAltDizin} ph={t('WordPressPage:subdir_placeholder')} mono />
          <Alan et={t('WordPressPage:admin_user_label')} v={adminK} set={setAdminK} zorunlu mono />
          <Alan et={t('WordPressPage:admin_email_label')} v={adminE} set={setAdminE} zorunlu type="email" ph={t('WordPressPage:admin_email_placeholder')} />
        </div>
        <button disabled={kuruyor || !domainId} className="ta-primary-button mt-3 w-full sm:w-auto">
          {kuruyor ? t('WordPressPage:installing_button') : (sel ? t('WordPressPage:install_button_with_domain', { domain: sel.alan_adi }) : t('WordPressPage:install_button'))}
        </button>
      </form>
    </div>
  )
}

function DurumRozet({ tk }: { tk: TumKurulum }) {
  const { t } = useTranslation('WordPressPage')
  if (tk.durum === 'eski') {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/40 text-amber-800 dark:text-amber-200 font-medium">
        <span className="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
        {tk.son_surum ? t('WordPressPage:status_update_to', { version: tk.son_surum }) : t('WordPressPage:status_update_available')}
      </span>
    )
  }
  if (tk.durum === 'guncel') {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300 font-medium">
        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
        {t('WordPressPage:status_current')}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400 font-medium">
      {t('WordPressPage:status_unknown')}
    </span>
  )
}

function Bilgi({ et, v, mono, link }: { et: string; v: string; mono?: boolean; link?: boolean }) {
  return (
    <div className="flex items-baseline gap-1.5 min-w-0">
      <span className="text-[11px] uppercase tracking-wide text-slate-400 font-semibold shrink-0">{et}</span>
      {link ? <a href={v} target="_blank" rel="noreferrer" className="text-xs text-brand-600 dark:text-brand-400 hover:underline truncate font-mono">{v}</a>
        : <span className={`text-xs text-slate-800 dark:text-slate-100 truncate ${mono ? 'font-mono' : ''}`}>{v}</span>}
    </div>
  )
}

function Alan({ et, v, set, zorunlu, ph, mono, type }: { et: string; v: string; set: (s: string) => void; zorunlu?: boolean; ph?: string; mono?: boolean; type?: string }) {
  return (
    <label className="block">
      <span className="ta-label-sm">{et}</span>
      <input value={v} onChange={e => set(e.target.value)} required={zorunlu} placeholder={ph} type={type || 'text'}
        className={`ta-input ta-input-sm w-full ${mono ? 'font-mono' : ''}`} />
    </label>
  )
}
