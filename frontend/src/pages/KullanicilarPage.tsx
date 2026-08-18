// Panel hesapları (admin / bayi / müşteri) — /users CRUD'un arayüzü.
//
// Kapsam sunucu tarafında zorlanır (bkz. internal/users): bayi yalnız kendi
// altındaki hesapları görür ve yalnız müşteri hesabı açabilir. Buradaki rol
// kısıtları o kuralların arayüz yansımasıdır, güvenlik sınırı değildir.
import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import ListToolbar from '@/components/ListToolbar'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'
import { T } from '@/lib/tablo'

type Kullanici = {
  id: number
  kullanici_adi: string
  eposta: string
  ad_soyad: string
  rol: 'admin' | 'reseller' | 'user'
  durum: 'active' | 'suspended'
  bayi_id: number | null
  iki_fa: boolean
  parolasiz: boolean
  son_giris: string
  son_giris_ip: string
  olusturma: string
}

const ROL_STIL: Record<string, string> = {
  admin: 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-300',
  reseller: 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-300',
  user: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400',
}

type YeniHesap = { kullanici_adi: string; parola: string; eposta: string; ad_soyad: string; rol: string }
const BOS: YeniHesap = { kullanici_adi: '', parola: '', eposta: '', ad_soyad: '', rol: 'user' }

type BayiLimit = {
  user_id: number
  paket_id: number | null
  paket_ad: string
  max_customer: number
  max_domain: number
  disk_kota_mb: number
  trafik_kota_mb: number
  izinli_planlar: number[] | null
  fazla_satis: boolean
  tanimli: boolean
  mevcut_customer: number
  mevcut_domain: number
  mevcut_disk_mb: number
  mevcut_trafik_mb: number
}

type BayiPaketOzet = { id: number; ad: string; max_customer: number; max_domain: number; disk_kota_mb: number; trafik_kota_mb: number; fazla_satis: boolean }
type HizmetPlanOzet = { id: number; ad: string }

const ROOT_ONERI_KAPALI_KEY = 'sanalcp.kullanicilar.root-oneri-kapali'

export default function KullanicilarPage() {
  const { t } = useTranslation(['KullanicilarPage', 'common'])
  const ROL_ETIKET: Record<string, string> = {
    admin: t('KullanicilarPage:roles.admin'),
    reseller: t('KullanicilarPage:roles.reseller'),
    user: t('KullanicilarPage:roles.user'),
  }
  const [aramaParam] = useSearchParams()
  const benimRolum = useAuth((s) => s.kullanici?.rol)
  const benimID = useAuth((s) => s.kullanici?.id)
  const benimAdim = useAuth((s) => s.kullanici?.adi)
  const adminMiyim = benimRolum === 'admin'

  // root ile çalışan yöneticiye kendi panel hesabını açmasını öner. root'un
  // parolası /etc/shadow'dadır (kurtarma yolu, bkz. internal/auth/parola.go);
  // günlük kullanım için root'tan bağımsız, bcrypt parolalı bir yönetici
  // hesabı daha güvenlidir. Kapatma kalıcıdır.
  const [rootOneriKapali, setRootOneriKapali] = useState(
    () => localStorage.getItem(ROOT_ONERI_KAPALI_KEY) === '1'
  )
  const rootOneriGoster = adminMiyim && benimAdim === 'root' && !rootOneriKapali

  const [liste, setListe] = useState<Kullanici[]>([])
  const [yukleniyor, setYukleniyor] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [aranan, setAranan] = useState(() => aramaParam.get('arama') || '')

  useEffect(() => { setAranan(aramaParam.get('arama') || '') }, [aramaParam])

  const [yeni, setYeni] = useState<YeniHesap | null>(null)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [parolaHedef, setParolaHedef] = useState<Kullanici | null>(null)
  const [yeniParola, setYeniParola] = useState('')
  const [silinecek, setSilinecek] = useState<Kullanici | null>(null)
  const [limitHedef, setLimitHedef] = useState<Kullanici | null>(null)
  const [limit, setLimit] = useState<BayiLimit | null>(null)
  const [limitYukleniyor, setLimitYukleniyor] = useState(false)
  const [bayiPaketleri, setBayiPaketleri] = useState<BayiPaketOzet[]>([])
  const [hizmetPlanlari, setHizmetPlanlari] = useState<HizmetPlanOzet[]>([])

  async function getir() {
    setYukleniyor(true)
    try {
      const r = await api.get<Kullanici[]>('/users')
      setListe(Array.isArray(r.data) ? r.data : [])
      setHata(null)
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.load')))
    } finally {
      setYukleniyor(false)
    }
  }
  useEffect(() => { getir() }, [])

  const suzulmus = useMemo(() => {
    const t = aranan.trim().toLowerCase()
    if (!t) return liste
    return liste.filter((k) => `${k.kullanici_adi} ${k.eposta} ${k.ad_soyad}`.toLowerCase().includes(t))
  }, [liste, aranan])

  async function olustur() {
    if (!yeni) return
    setKaydediliyor(true)
    setHata(null)
    try {
      await api.post('/users', yeni)
      setBasari(t('KullanicilarPage:success.created', { name: yeni.kullanici_adi }))
      setYeni(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.create')))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function parolaSifirla() {
    if (!parolaHedef) return
    setKaydediliyor(true)
    setHata(null)
    try {
      await api.post(`/users/${parolaHedef.id}/parola`, { yeni: yeniParola })
      setBasari(t('KullanicilarPage:success.password_updated', { name: parolaHedef.kullanici_adi }))
      setParolaHedef(null)
      setYeniParola('')
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.password_reset')))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function limitAc(k: Kullanici) {
    setLimitHedef(k)
    setLimit(null)
    setLimitYukleniyor(true)
    setHata(null)
    if (bayiPaketleri.length === 0) {
      api.get<BayiPaketOzet[]>('/bayi-paketleri').then(r => setBayiPaketleri(r.data || [])).catch(() => {})
    }
    if (hizmetPlanlari.length === 0) {
      api.get<HizmetPlanOzet[]>('/plans').then(r => setHizmetPlanlari(r.data || [])).catch(() => {})
    }
    try {
      const r = await api.get<BayiLimit>(`/users/${k.id}/limitler`)
      setLimit(r.data)
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.limits_load')))
      setLimitHedef(null)
    } finally {
      setLimitYukleniyor(false)
    }
  }

  function paketUygula(paketID: number) {
    if (!limit) return
    if (paketID === 0) {
      setLimit({ ...limit, paket_id: null, paket_ad: '' })
      return
    }
    const p = bayiPaketleri.find(x => x.id === paketID)
    if (!p) return
    setLimit({
      ...limit,
      paket_id: p.id,
      paket_ad: p.ad,
      max_customer: p.max_customer,
      max_domain: p.max_domain,
      disk_kota_mb: p.disk_kota_mb,
      trafik_kota_mb: p.trafik_kota_mb,
      fazla_satis: p.fazla_satis,
    })
  }

  function izinliPlanToggle(planID: number) {
    if (!limit) return
    const mevcut = limit.izinli_planlar || []
    const yeni = mevcut.includes(planID) ? mevcut.filter(id => id !== planID) : [...mevcut, planID]
    setLimit({ ...limit, izinli_planlar: yeni })
  }

  async function limitKaydet() {
    if (!limitHedef || !limit) return
    setKaydediliyor(true)
    setHata(null)
    try {
      await api.put(`/users/${limitHedef.id}/limitler`, {
        paket_id: limit.paket_id || 0,
        max_customer: limit.max_customer,
        max_domain: limit.max_domain,
        disk_kota_mb: limit.disk_kota_mb,
        trafik_kota_mb: limit.trafik_kota_mb,
        izinli_planlar: limit.izinli_planlar || [],
        fazla_satis: limit.fazla_satis,
      })
      setBasari(t('KullanicilarPage:success.limits_updated', { name: limitHedef.kullanici_adi }))
      setLimitHedef(null)
      setLimit(null)
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.limits_save')))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function durumDegistir(k: Kullanici) {
    const hedef = k.durum === 'active' ? 'suspended' : 'active'
    setHata(null)
    try {
      await api.post(`/users/${k.id}/durum`, { durum: hedef })
      setBasari(hedef === 'active'
        ? t('KullanicilarPage:success.activated', { name: k.kullanici_adi })
        : t('KullanicilarPage:success.suspended', { name: k.kullanici_adi }))
      await getir()
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.status_change')))
    }
  }

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/users/${silinecek.id}`)
      setBasari(t('KullanicilarPage:success.deleted', { name: silinecek.kullanici_adi }))
      setSilinecek(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, t('KullanicilarPage:error.delete')))
      setSilinecek(null)
    }
  }

  // root ve kendi hesabın üzerinde yıkıcı işlem yok — sunucu da reddeder,
  // düğmeyi göstermemek gereksiz hata mesajını önler.
  const korumali = (k: Kullanici) => k.id === 1 || k.id === benimID

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('KullanicilarPage:breadcrumb') }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('KullanicilarPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {adminMiyim
            ? t('KullanicilarPage:subtitle_admin')
            : t('KullanicilarPage:subtitle_reseller')}
        </p>
      </div>

      {rootOneriGoster && (
        <div className="mb-4 flex items-start gap-3 rounded-xl border border-brand-200 bg-brand-50 px-4 py-3 dark:border-brand-800/60 dark:bg-brand-900/20">
          <svg className="mt-0.5 h-5 w-5 shrink-0 text-brand-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z" />
          </svg>
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-brand-900 dark:text-brand-100">
              {t('KullanicilarPage:root_oneri.baslik')}
            </p>
            <p className="mt-0.5 text-xs text-brand-800 dark:text-brand-200/90">
              {t('KullanicilarPage:root_oneri.aciklama')}
            </p>
            <button
              type="button"
              onClick={() => setYeni({ ...BOS, rol: 'admin' })}
              className="mt-2 rounded-lg bg-brand-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-brand-700"
            >
              {t('KullanicilarPage:root_oneri.buton')}
            </button>
          </div>
          <button
            type="button"
            onClick={() => {
              localStorage.setItem(ROOT_ONERI_KAPALI_KEY, '1')
              setRootOneriKapali(true)
            }}
            className="shrink-0 -m-1 rounded-md p-1 text-brand-500 transition hover:bg-brand-100 hover:text-brand-700 dark:hover:bg-brand-900/30"
            aria-label={t('KullanicilarPage:root_oneri.kapat_aria')}
          >
            <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      )}

      <ListToolbar
        birincil={{ etiket: adminMiyim ? t('KullanicilarPage:new_account') : t('KullanicilarPage:new_customer'), onClick: () => setYeni({ ...BOS, rol: adminMiyim ? 'reseller' : 'user' }) }}
        aranan={aranan}
        arananSetter={setAranan}
      />

      {hata && <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{hata}</div>}
      {basari && <div className="mb-4 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-sm">{basari}</div>}

      {yukleniyor ? (
        <div className="py-16 text-center text-sm text-slate-400">{t('common:loading')}</div>
      ) : liste.length === 0 ? (
        <EmptyState
          baslik={adminMiyim ? t('KullanicilarPage:empty.title_admin') : t('KullanicilarPage:empty.title_reseller')}
          aciklama={t('KullanicilarPage:empty.desc')}
          buton={{ etiket: t('KullanicilarPage:new_account'), onClick: () => setYeni({ ...BOS, rol: adminMiyim ? 'reseller' : 'user' }) }}
        />
      ) : suzulmus.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('KullanicilarPage:empty.no_search_results')}</div>
      ) : (
        <div className="lg:overflow-x-auto lg:rounded-xl lg:border lg:border-slate-200 dark:lg:border-slate-800">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/60`}>
              <tr>
                {[t('KullanicilarPage:table.user'), t('KullanicilarPage:table.full_name'), t('KullanicilarPage:table.role'), t('KullanicilarPage:table.status'), t('KullanicilarPage:table.twofa'), t('KullanicilarPage:table.last_login'), ''].map((b, i) => (
                  <th key={i} className={`${T.baslik} whitespace-nowrap`}>{b}</th>
                ))}
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800 lg:bg-white dark:lg:bg-slate-950`}>
              {suzulmus.map((k) => (
                <tr key={k.id} className={T.satir}>
                  <td className={T.hucreBaslik}>
                    <span className="font-mono text-slate-900 dark:text-slate-100">{k.kullanici_adi}</span>
                    {k.id === 1 && <span className="ml-1.5 text-[10px] text-slate-400">{t('KullanicilarPage:table.system_tag')}</span>}
                  </td>
                  <td className={T.hucre} data-etiket={t('KullanicilarPage:table.full_name')}><span className="text-slate-600 dark:text-slate-400">{k.ad_soyad || '—'}</span></td>
                  <td className={T.hucre} data-etiket={t('KullanicilarPage:table.role')}>
                    <span className={`px-2 py-0.5 rounded text-xs ${ROL_STIL[k.rol]}`}>{ROL_ETIKET[k.rol] ?? k.rol}</span>
                  </td>
                  <td className={T.hucre} data-etiket={t('KullanicilarPage:table.status')}>
                    {k.durum === 'active'
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('KullanicilarPage:table.active')}</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{t('KullanicilarPage:table.suspended')}</span>}
                    {/* Parolası olmayan hesap giriş yapamaz — "aktif" görünüp
                        çalışmadığı için ayrı bir uyarı rozeti hak ediyor. */}
                    {k.parolasiz && (
                      <span
                        title={t('KullanicilarPage:table.no_password_title')}
                        className="ml-1.5 px-2 py-0.5 rounded text-xs bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300"
                      >{t('KullanicilarPage:table.no_password')}</span>
                    )}
                  </td>
                  <td className={T.hucre} data-etiket={t('KullanicilarPage:table.twofa')}>
                    {k.iki_fa ? <span className="text-emerald-600 dark:text-emerald-400">{t('KullanicilarPage:table.twofa_on')}</span> : <span className="text-slate-400">{t('KullanicilarPage:table.twofa_off')}</span>}
                  </td>
                  <td className={`${T.hucre} text-xs text-slate-500`} data-etiket={t('KullanicilarPage:table.last_login')}>
                    {k.son_giris || '—'}
                    {k.son_giris_ip && <span className="ml-1 opacity-60">({k.son_giris_ip})</span>}
                  </td>
                  <td className={T.hucreAksiyon}>
                    {k.id === 1 ? (
                      <span className="text-xs text-slate-400">{t('KullanicilarPage:table.system_account')}</span>
                    ) : (
                      <>
                        <button onClick={() => { setParolaHedef(k); setYeniParola('') }} className="text-xs text-brand-600 dark:text-brand-400 hover:underline mr-3">
                          {t('KullanicilarPage:table.password_btn')}
                        </button>
                        {/* Kota yalnız bayilerde anlamlı ve yalnız admin yönetir. */}
                        {adminMiyim && k.rol === 'reseller' && (
                          <button onClick={() => limitAc(k)} className="text-xs text-sky-600 dark:text-sky-400 hover:underline mr-3">
                            {t('KullanicilarPage:table.limits_btn')}
                          </button>
                        )}
                        {!korumali(k) && (
                          <>
                            <button onClick={() => durumDegistir(k)} className="text-xs text-amber-600 dark:text-amber-400 hover:underline mr-3">
                              {k.durum === 'active' ? t('KullanicilarPage:table.suspend') : t('KullanicilarPage:table.activate')}
                            </button>
                            <button onClick={() => setSilinecek(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">
                              {t('common:delete')}
                            </button>
                          </>
                        )}
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Yeni hesap */}
      <Modal acik={yeni !== null} baslik={adminMiyim ? t('KullanicilarPage:modal_new.title_admin') : t('KullanicilarPage:modal_new.title_reseller')} onKapat={() => setYeni(null)}>
        {yeni && (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('KullanicilarPage:modal_new.username')}</label>
              <input
                value={yeni.kullanici_adi}
                onChange={(e) => setYeni({ ...yeni, kullanici_adi: e.target.value })}
                placeholder={t('KullanicilarPage:modal_new.username_placeholder')}
                className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
              <p className="mt-1 text-[11px] text-slate-400">{t('KullanicilarPage:modal_new.username_hint')}</p>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('KullanicilarPage:modal_new.password')}</label>
              <input
                type="text"
                value={yeni.parola}
                onChange={(e) => setYeni({ ...yeni, parola: e.target.value })}
                className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
              <p className="mt-1 text-[11px] text-slate-400">{t('KullanicilarPage:modal_new.password_hint')}</p>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('KullanicilarPage:modal_new.role')}</label>
                <select
                  value={yeni.rol}
                  onChange={(e) => setYeni({ ...yeni, rol: e.target.value })}
                  disabled={!adminMiyim}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 disabled:opacity-60 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  {adminMiyim && <option value="admin">{t('KullanicilarPage:roles.admin')}</option>}
                  {adminMiyim && <option value="reseller">{t('KullanicilarPage:roles.reseller')}</option>}
                  <option value="user">{t('KullanicilarPage:roles.user')}</option>
                </select>
                {!adminMiyim && <p className="mt-1 text-[11px] text-slate-400">{t('KullanicilarPage:modal_new.role_hint_reseller')}</p>}
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('common:email')}</label>
                <input
                  type="email"
                  value={yeni.eposta}
                  onChange={(e) => setYeni({ ...yeni, eposta: e.target.value })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('KullanicilarPage:modal_new.full_name')}</label>
              <input
                value={yeni.ad_soyad}
                onChange={(e) => setYeni({ ...yeni, ad_soyad: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setYeni(null)} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">{t('common:giveUp')}</button>
              <button onClick={olustur} disabled={kaydediliyor} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {kaydediliyor ? t('common:creating') : t('common:create')}
              </button>
            </div>
          </div>
        )}
      </Modal>

      {/* Parola sıfırlama */}
      <Modal acik={parolaHedef !== null} baslik={t('KullanicilarPage:modal_password.title')} onKapat={() => setParolaHedef(null)}>
        {parolaHedef && (
          <div className="space-y-3">
            <p className="text-sm text-slate-600 dark:text-slate-400">
              <span className="font-mono">{parolaHedef.kullanici_adi}</span> {t('KullanicilarPage:modal_password.desc_suffix')}
              {parolaHedef.rol === 'user' && (
                <span className="block mt-1.5 text-xs text-slate-500">
                  {t('KullanicilarPage:modal_password.user_hint')}
                </span>
              )}
            </p>
            <input
              type="text"
              value={yeniParola}
              onChange={(e) => setYeniParola(e.target.value)}
              placeholder={t('KullanicilarPage:modal_password.placeholder')}
              className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
            />
            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setParolaHedef(null)} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">{t('common:giveUp')}</button>
              <button onClick={parolaSifirla} disabled={kaydediliyor || yeniParola.length < 8} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {kaydediliyor ? t('common:saving') : t('KullanicilarPage:modal_password.submit')}
              </button>
            </div>
          </div>
        )}
      </Modal>

      {/* Bayi limitleri */}
      <Modal acik={limitHedef !== null} baslik={t('KullanicilarPage:modal_limits.title')} onKapat={() => { setLimitHedef(null); setLimit(null) }}>
        {limitYukleniyor ? (
          <div className="py-8 text-center text-sm text-slate-400">{t('common:loading')}</div>
        ) : limit && limitHedef ? (
          <div className="space-y-4">
            <p className="text-sm text-slate-600 dark:text-slate-400">
              <span className="font-mono">{limitHedef.kullanici_adi}</span> {t('KullanicilarPage:modal_limits.desc_suffix')}
              <span className="block mt-1 text-xs text-slate-500">
                <strong>{t('KullanicilarPage:modal_limits.zero_unlimited')}</strong> {t('KullanicilarPage:modal_limits.zero_hint')}
              </span>
            </p>

            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                {t('KullanicilarPage:modal_limits.package')}
              </label>
              <select
                value={limit.paket_id ?? 0}
                onChange={(e) => paketUygula(Number(e.target.value))}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              >
                <option value={0}>{t('KullanicilarPage:modal_limits.manual_option')}</option>
                {bayiPaketleri.map(p => <option key={p.id} value={p.id}>{p.ad}</option>)}
              </select>
              {limit.paket_id ? (
                <p className="mt-1 text-[11px] text-slate-400">
                  {t('KullanicilarPage:modal_limits.package_filled_hint', { name: limit.paket_ad })}
                </p>
              ) : (
                <p className="mt-1 text-[11px] text-slate-400">{t('KullanicilarPage:modal_limits.package_manual_hint')}</p>
              )}
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  {t('KullanicilarPage:modal_limits.max_customer')}
                </label>
                <input
                  type="number"
                  min={0}
                  disabled={!!limit.paket_id}
                  value={limit.max_customer}
                  onChange={(e) => setLimit({ ...limit, max_customer: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500 disabled:opacity-60"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  {t('KullanicilarPage:modal_limits.in_use', { n: limit.mevcut_customer })}
                  {limit.max_customer > 0 && limit.mevcut_customer > limit.max_customer && (
                    <span className="text-amber-600 dark:text-amber-400"> {t('KullanicilarPage:modal_limits.below_usage')}</span>
                  )}
                </p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  {t('KullanicilarPage:modal_limits.max_domain')}
                </label>
                <input
                  type="number"
                  min={0}
                  disabled={!!limit.paket_id}
                  value={limit.max_domain}
                  onChange={(e) => setLimit({ ...limit, max_domain: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500 disabled:opacity-60"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  {t('KullanicilarPage:modal_limits.in_use', { n: limit.mevcut_domain })}
                  {limit.max_domain > 0 && limit.mevcut_domain > limit.max_domain && (
                    <span className="text-amber-600 dark:text-amber-400"> {t('KullanicilarPage:modal_limits.below_usage')}</span>
                  )}
                </p>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  {t('KullanicilarPage:modal_limits.disk_quota')}
                </label>
                <input
                  type="number"
                  min={0}
                  disabled={!!limit.paket_id}
                  value={limit.disk_kota_mb}
                  onChange={(e) => setLimit({ ...limit, disk_kota_mb: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500 disabled:opacity-60"
                />
                <p className="mt-1 text-[11px] text-slate-400">{t('KullanicilarPage:modal_limits.in_use_mb', { n: limit.mevcut_disk_mb })}</p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  {t('KullanicilarPage:modal_limits.traffic_quota')}
                </label>
                <input
                  type="number"
                  min={0}
                  disabled={!!limit.paket_id}
                  value={limit.trafik_kota_mb}
                  onChange={(e) => setLimit({ ...limit, trafik_kota_mb: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500 disabled:opacity-60"
                />
                <p className="mt-1 text-[11px] text-slate-400">{t('KullanicilarPage:modal_limits.in_use_mb', { n: limit.mevcut_trafik_mb })}</p>
              </div>
            </div>

            <label className={`flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 ${limit.paket_id ? 'opacity-60' : 'cursor-pointer'}`}>
              <input
                type="checkbox"
                disabled={!!limit.paket_id}
                checked={limit.fazla_satis}
                onChange={(e) => setLimit({ ...limit, fazla_satis: e.target.checked })}
                className="rounded"
              />
              {t('KullanicilarPage:modal_limits.overselling')}
            </label>
            <p className="text-[11px] text-slate-400 -mt-2">
              {t('KullanicilarPage:modal_limits.overselling_hint')}
            </p>

            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                {t('KullanicilarPage:modal_limits.plans_label')}
              </label>
              {hizmetPlanlari.length === 0 ? (
                <p className="text-[11px] text-slate-400">{t('KullanicilarPage:modal_limits.plans_empty')}</p>
              ) : (
                <div className="max-h-36 overflow-y-auto space-y-1 rounded-lg border border-slate-200 dark:border-slate-800 p-2">
                  {hizmetPlanlari.map(p => (
                    <label key={p.id} className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
                      <input
                        type="checkbox"
                        className="rounded"
                        checked={(limit.izinli_planlar || []).includes(p.id)}
                        onChange={() => izinliPlanToggle(p.id)}
                      />
                      {p.ad}
                    </label>
                  ))}
                </div>
              )}
              <p className="mt-1 text-[11px] text-slate-400">
                {t('KullanicilarPage:modal_limits.plans_hint')}
              </p>
            </div>

            {!limit.tanimli && (
              <div className="px-3 py-2 rounded-lg bg-slate-50 dark:bg-slate-900 text-xs text-slate-500 dark:text-slate-400">
                {t('KullanicilarPage:modal_limits.no_limit_defined')}
              </div>
            )}

            <p className="text-[11px] text-slate-400">
              {t('KullanicilarPage:modal_limits.footer_hint')}
            </p>

            <div className="flex justify-end gap-2 pt-1">
              <button onClick={() => { setLimitHedef(null); setLimit(null) }} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">
                {t('common:giveUp')}
              </button>
              <button onClick={limitKaydet} disabled={kaydediliyor} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {kaydediliyor ? t('common:saving') : t('common:save')}
              </button>
            </div>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        acik={silinecek !== null}
        baslik={t('KullanicilarPage:confirm_delete.title')}
        mesaj={t('KullanicilarPage:confirm_delete.message', {
          name: silinecek?.kullanici_adi ?? '',
          extra: silinecek?.rol === 'reseller' ? t('KullanicilarPage:confirm_delete.message_reseller_extra') : '',
        })}
        onayMetni={t('KullanicilarPage:confirm_delete.confirm')}
        tehlikeli
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}
