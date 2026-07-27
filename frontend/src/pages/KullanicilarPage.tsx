// Panel hesapları (admin / bayi / müşteri) — /users CRUD'un arayüzü.
//
// Kapsam sunucu tarafında zorlanır (bkz. internal/users): bayi yalnız kendi
// altındaki hesapları görür ve yalnız müşteri hesabı açabilir. Buradaki rol
// kısıtları o kuralların arayüz yansımasıdır, güvenlik sınırı değildir.
import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, apiHata } from '@/lib/api'
import { useAuth } from '@/store/auth'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import ListToolbar from '@/components/ListToolbar'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

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

const ROL_ETIKET: Record<string, string> = {
  admin: 'Yönetici',
  reseller: 'Bayi',
  user: 'Müşteri',
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
  max_customer: number
  max_domain: number
  disk_kota_mb: number
  trafik_kota_mb: number
  tanimli: boolean
  mevcut_customer: number
  mevcut_domain: number
  mevcut_disk_mb: number
  mevcut_trafik_mb: number
}

export default function KullanicilarPage() {
  const [aramaParam] = useSearchParams()
  const benimRolum = useAuth((s) => s.kullanici?.rol)
  const benimID = useAuth((s) => s.kullanici?.id)
  const adminMiyim = benimRolum === 'admin'

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

  async function getir() {
    setYukleniyor(true)
    try {
      const r = await api.get<Kullanici[]>('/users')
      setListe(Array.isArray(r.data) ? r.data : [])
      setHata(null)
    } catch (e) {
      setHata(apiHata(e, 'Hesaplar alınamadı'))
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
      setBasari(`${yeni.kullanici_adi} hesabı oluşturuldu.`)
      setYeni(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, 'Hesap oluşturulamadı'))
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
      setBasari(`${parolaHedef.kullanici_adi} parolası güncellendi.`)
      setParolaHedef(null)
      setYeniParola('')
    } catch (e) {
      setHata(apiHata(e, 'Parola sıfırlanamadı'))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function limitAc(k: Kullanici) {
    setLimitHedef(k)
    setLimit(null)
    setLimitYukleniyor(true)
    setHata(null)
    try {
      const r = await api.get<BayiLimit>(`/users/${k.id}/limitler`)
      setLimit(r.data)
    } catch (e) {
      setHata(apiHata(e, 'Limitler okunamadı'))
      setLimitHedef(null)
    } finally {
      setLimitYukleniyor(false)
    }
  }

  async function limitKaydet() {
    if (!limitHedef || !limit) return
    setKaydediliyor(true)
    setHata(null)
    try {
      await api.put(`/users/${limitHedef.id}/limitler`, {
        max_customer: limit.max_customer,
        max_domain: limit.max_domain,
        disk_kota_mb: limit.disk_kota_mb,
        trafik_kota_mb: limit.trafik_kota_mb,
      })
      setBasari(`${limitHedef.kullanici_adi} limitleri güncellendi.`)
      setLimitHedef(null)
      setLimit(null)
    } catch (e) {
      setHata(apiHata(e, 'Limitler kaydedilemedi'))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function durumDegistir(k: Kullanici) {
    const hedef = k.durum === 'active' ? 'suspended' : 'active'
    setHata(null)
    try {
      await api.post(`/users/${k.id}/durum`, { durum: hedef })
      setBasari(`${k.kullanici_adi} ${hedef === 'active' ? 'etkinleştirildi' : 'askıya alındı'}.`)
      await getir()
    } catch (e) {
      setHata(apiHata(e, 'Durum değiştirilemedi'))
    }
  }

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/users/${silinecek.id}`)
      setBasari(`${silinecek.kullanici_adi} silindi.`)
      setSilinecek(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, 'Silinemedi'))
      setSilinecek(null)
    }
  }

  // root ve kendi hesabın üzerinde yıkıcı işlem yok — sunucu da reddeder,
  // düğmeyi göstermemek gereksiz hata mesajını önler.
  const korumali = (k: Kullanici) => k.id === 1 || k.id === benimID

  return (
    <div className="w-full max-w-[1600px] px-6 py-5">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Kullanıcılar' }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Kullanıcılar</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {adminMiyim
            ? 'Panel hesapları: yöneticiler, bayiler ve müşteriler.'
            : 'Açtığınız müşteri hesapları.'}
        </p>
      </div>

      <ListToolbar
        birincil={{ etiket: adminMiyim ? 'Yeni Hesap' : 'Yeni Müşteri', onClick: () => setYeni({ ...BOS, rol: adminMiyim ? 'reseller' : 'user' }) }}
        aranan={aranan}
        arananSetter={setAranan}
      />

      {hata && <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{hata}</div>}
      {basari && <div className="mb-4 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-sm">{basari}</div>}

      {yukleniyor ? (
        <div className="py-16 text-center text-sm text-slate-400">Yükleniyor…</div>
      ) : liste.length === 0 ? (
        <EmptyState
          baslik={adminMiyim ? 'Henüz başka hesap yok' : 'Henüz müşteri hesabınız yok'}
          aciklama="Yeni hesap oluşturarak başlayın."
          buton={{ etiket: 'Yeni Hesap', onClick: () => setYeni({ ...BOS, rol: adminMiyim ? 'reseller' : 'user' }) }}
        />
      ) : suzulmus.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">Aramayla eşleşen hesap yok.</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {['Kullanıcı', 'Ad Soyad', 'Rol', 'Durum', '2FA', 'Son Giriş', ''].map((b, i) => (
                  <th key={i} className="px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">{b}</th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {suzulmus.map((k) => (
                <tr key={k.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    <span className="font-mono text-slate-900 dark:text-slate-100">{k.kullanici_adi}</span>
                    {k.id === 1 && <span className="ml-1.5 text-[10px] text-slate-400">(sistem)</span>}
                  </td>
                  <td className="px-3 py-2.5 text-slate-600 dark:text-slate-400 whitespace-nowrap">{k.ad_soyad || '—'}</td>
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    <span className={`px-2 py-0.5 rounded text-xs ${ROL_STIL[k.rol]}`}>{ROL_ETIKET[k.rol] ?? k.rol}</span>
                  </td>
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    {k.durum === 'active'
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Aktif</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">Askıda</span>}
                    {/* Parolası olmayan hesap giriş yapamaz — "aktif" görünüp
                        çalışmadığı için ayrı bir uyarı rozeti hak ediyor. */}
                    {k.parolasiz && (
                      <span
                        title="Parola atanmamış — bu hesap giriş yapamaz"
                        className="ml-1.5 px-2 py-0.5 rounded text-xs bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300"
                      >Parola yok</span>
                    )}
                  </td>
                  <td className="px-3 py-2.5 whitespace-nowrap text-xs">
                    {k.iki_fa ? <span className="text-emerald-600 dark:text-emerald-400">Açık</span> : <span className="text-slate-400">Kapalı</span>}
                  </td>
                  <td className="px-3 py-2.5 whitespace-nowrap text-xs text-slate-500">
                    {k.son_giris || '—'}
                    {k.son_giris_ip && <span className="ml-1 opacity-60">({k.son_giris_ip})</span>}
                  </td>
                  <td className="px-3 py-2.5 text-right whitespace-nowrap">
                    {k.id === 1 ? (
                      <span className="text-xs text-slate-400">sistem hesabı</span>
                    ) : (
                      <>
                        <button onClick={() => { setParolaHedef(k); setYeniParola('') }} className="text-xs text-brand-600 dark:text-brand-400 hover:underline mr-3">
                          Parola
                        </button>
                        {/* Kota yalnız bayilerde anlamlı ve yalnız admin yönetir. */}
                        {adminMiyim && k.rol === 'reseller' && (
                          <button onClick={() => limitAc(k)} className="text-xs text-sky-600 dark:text-sky-400 hover:underline mr-3">
                            Limitler
                          </button>
                        )}
                        {!korumali(k) && (
                          <>
                            <button onClick={() => durumDegistir(k)} className="text-xs text-amber-600 dark:text-amber-400 hover:underline mr-3">
                              {k.durum === 'active' ? 'Askıya al' : 'Etkinleştir'}
                            </button>
                            <button onClick={() => setSilinecek(k)} className="text-xs text-red-600 dark:text-red-400 hover:underline">
                              Sil
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
      <Modal acik={yeni !== null} baslik={adminMiyim ? 'Yeni Hesap' : 'Yeni Müşteri Hesabı'} onKapat={() => setYeni(null)}>
        {yeni && (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Kullanıcı adı</label>
              <input
                value={yeni.kullanici_adi}
                onChange={(e) => setYeni({ ...yeni, kullanici_adi: e.target.value })}
                placeholder="ornek_bayi"
                className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
              <p className="mt-1 text-[11px] text-slate-400">3-32 karakter, küçük harfle başlar; harf, rakam, _ ve - içerebilir.</p>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Parola</label>
              <input
                type="text"
                value={yeni.parola}
                onChange={(e) => setYeni({ ...yeni, parola: e.target.value })}
                className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
              <p className="mt-1 text-[11px] text-slate-400">En az 8 karakter. Parola yalnız şimdi görünür — kullanıcıya iletin.</p>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Rol</label>
                <select
                  value={yeni.rol}
                  onChange={(e) => setYeni({ ...yeni, rol: e.target.value })}
                  disabled={!adminMiyim}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 disabled:opacity-60 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  {adminMiyim && <option value="admin">Yönetici</option>}
                  {adminMiyim && <option value="reseller">Bayi</option>}
                  <option value="user">Müşteri</option>
                </select>
                {!adminMiyim && <p className="mt-1 text-[11px] text-slate-400">Bayiler yalnız müşteri hesabı açabilir.</p>}
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">E-posta</label>
                <input
                  type="email"
                  value={yeni.eposta}
                  onChange={(e) => setYeni({ ...yeni, eposta: e.target.value })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Ad Soyad</label>
              <input
                value={yeni.ad_soyad}
                onChange={(e) => setYeni({ ...yeni, ad_soyad: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setYeni(null)} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">Vazgeç</button>
              <button onClick={olustur} disabled={kaydediliyor} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {kaydediliyor ? 'Oluşturuluyor…' : 'Oluştur'}
              </button>
            </div>
          </div>
        )}
      </Modal>

      {/* Parola sıfırlama */}
      <Modal acik={parolaHedef !== null} baslik="Parola Belirle" onKapat={() => setParolaHedef(null)}>
        {parolaHedef && (
          <div className="space-y-3">
            <p className="text-sm text-slate-600 dark:text-slate-400">
              <span className="font-mono">{parolaHedef.kullanici_adi}</span> için yeni parola.
              {parolaHedef.rol === 'user' && (
                <span className="block mt-1.5 text-xs text-slate-500">
                  Bu müşterinin panel hesabı parolası. Müşteri panele yalnızca bu kullanıcı adı ve
                  parolayla girebilir; parola atanmadan giriş yapamaz.
                </span>
              )}
            </p>
            <input
              type="text"
              value={yeniParola}
              onChange={(e) => setYeniParola(e.target.value)}
              placeholder="En az 8 karakter"
              className="w-full px-3 py-2 text-sm font-mono rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
            />
            <div className="flex justify-end gap-2 pt-2">
              <button onClick={() => setParolaHedef(null)} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">Vazgeç</button>
              <button onClick={parolaSifirla} disabled={kaydediliyor || yeniParola.length < 8} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {kaydediliyor ? 'Kaydediliyor…' : 'Parolayı Belirle'}
              </button>
            </div>
          </div>
        )}
      </Modal>

      {/* Bayi limitleri */}
      <Modal acik={limitHedef !== null} baslik="Bayi Limitleri" onKapat={() => { setLimitHedef(null); setLimit(null) }}>
        {limitYukleniyor ? (
          <div className="py-8 text-center text-sm text-slate-400">Yükleniyor…</div>
        ) : limit && limitHedef ? (
          <div className="space-y-4">
            <p className="text-sm text-slate-600 dark:text-slate-400">
              <span className="font-mono">{limitHedef.kullanici_adi}</span> için üst sınırlar.
              <span className="block mt-1 text-xs text-slate-500">
                <strong>0 = sınırsız.</strong> İkisi de 0 ise limit kaydı tamamen kaldırılır.
              </span>
            </p>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  En fazla müşteri
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.max_customer}
                  onChange={(e) => setLimit({ ...limit, max_customer: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  şu an {limit.mevcut_customer} kullanılıyor
                  {limit.max_customer > 0 && limit.mevcut_customer > limit.max_customer && (
                    <span className="text-amber-600 dark:text-amber-400"> — limit mevcut kullanımın altında</span>
                  )}
                </p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  En fazla domain
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.max_domain}
                  onChange={(e) => setLimit({ ...limit, max_domain: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  şu an {limit.mevcut_domain} kullanılıyor
                  {limit.max_domain > 0 && limit.mevcut_domain > limit.max_domain && (
                    <span className="text-amber-600 dark:text-amber-400"> — limit mevcut kullanımın altında</span>
                  )}
                </p>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  Disk kotası (MB)
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.disk_kota_mb}
                  onChange={(e) => setLimit({ ...limit, disk_kota_mb: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">şu an {limit.mevcut_disk_mb} MB kullanılıyor</p>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">
                  Trafik kotası (MB/ay)
                </label>
                <input
                  type="number"
                  min={0}
                  value={limit.trafik_kota_mb}
                  onChange={(e) => setLimit({ ...limit, trafik_kota_mb: Math.max(0, Number(e.target.value) || 0) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">şu an {limit.mevcut_trafik_mb} MB kullanılıyor</p>
              </div>
            </div>

            {!limit.tanimli && (
              <div className="px-3 py-2 rounded-lg bg-slate-50 dark:bg-slate-900 text-xs text-slate-500 dark:text-slate-400">
                Bu bayi için tanımlı limit yok — şu anda sınırsız.
              </div>
            )}

            <p className="text-[11px] text-slate-400">
              Limitin altına düşmek mevcut hesapları silmez ve siteleri kesmez; yalnız yeni
              müşteri/domain eklemeyi engeller. Disk ve trafik son ölçüme dayanır.
            </p>

            <div className="flex justify-end gap-2 pt-1">
              <button onClick={() => { setLimitHedef(null); setLimit(null) }} className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition">
                Vazgeç
              </button>
              <button onClick={limitKaydet} disabled={kaydediliyor} className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition">
                {kaydediliyor ? 'Kaydediliyor…' : 'Kaydet'}
              </button>
            </div>
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        acik={silinecek !== null}
        baslik="Hesabı sil"
        mesaj={`${silinecek?.kullanici_adi ?? ''} hesabı silinecek.${silinecek?.rol === 'reseller' ? ' Bu bayinin altındaki hesaplar silinmez, yöneticiye devredilir.' : ''}`}
        onayMetni="Sil"
        tehlikeli
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}
