// Müşteri kayıtları (customers tablosu) — /customers CRUD uçları panelin
// başından beri vardı ama arayüzü hiç yazılmamıştı; müşteri eklemek yalnız
// API'den mümkündü. Domainler bu kayıtlara domains.customer_id ile bağlanır.
//
// NOT: Bunlar panel giriş hesabı DEĞİLDİR — fatura/iletişim kaydıdır. Panel
// girişi tek admindir (root); müşteriler kendi domainlerine FTP kimliğiyle
// /cp adresinden girer.
import { useEffect, useMemo, useState } from 'react'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import ListToolbar from '@/components/ListToolbar'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type Musteri = {
  id: number
  ad: string
  eposta: string
  plan_id: number | null
  durum: string
  notlar: string
  olusturma: string
}

type Plan = { id: number; ad: string }

const BOS: Musteri = { id: 0, ad: '', eposta: '', plan_id: null, durum: 'aktif', notlar: '', olusturma: '' }

export default function MusterilerPage() {
  const [liste, setListe] = useState<Musteri[]>([])
  const [planlar, setPlanlar] = useState<Plan[]>([])
  const [yukleniyor, setYukleniyor] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [aranan, setAranan] = useState('')

  const [duzenlenen, setDuzenlenen] = useState<Musteri | null>(null)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [silinecek, setSilinecek] = useState<Musteri | null>(null)

  async function getir() {
    setYukleniyor(true)
    try {
      const r = await api.get<Musteri[]>('/customers')
      setListe(Array.isArray(r.data) ? r.data : [])
      setHata(null)
    } catch (e) {
      setHata(apiHata(e, 'Müşteriler alınamadı'))
    } finally {
      setYukleniyor(false)
    }
  }

  useEffect(() => {
    getir()
    api.get<Plan[]>('/plans')
      .then((r) => setPlanlar(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [])

  const suzulmus = useMemo(() => {
    const t = aranan.trim().toLowerCase()
    if (!t) return liste
    return liste.filter((m) => `${m.ad} ${m.eposta} ${m.notlar}`.toLowerCase().includes(t))
  }, [liste, aranan])

  async function kaydet() {
    if (!duzenlenen) return
    const ad = duzenlenen.ad.trim()
    const eposta = duzenlenen.eposta.trim()
    if (!ad || !eposta) {
      setHata('Ad ve e-posta zorunlu')
      return
    }
    setKaydediliyor(true)
    setHata(null)
    try {
      const govde = {
        ad,
        eposta,
        plan_id: duzenlenen.plan_id,
        durum: duzenlenen.durum,
        notlar: duzenlenen.notlar,
      }
      if (duzenlenen.id === 0) {
        await api.post('/customers', govde)
        setBasari(`${ad} eklendi.`)
      } else {
        await api.put(`/customers/${duzenlenen.id}`, govde)
        setBasari(`${ad} güncellendi.`)
      }
      setDuzenlenen(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, 'Kaydedilemedi'))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/customers/${silinecek.id}`)
      setBasari(`${silinecek.ad} silindi.`)
      setSilinecek(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, 'Silinemedi'))
      setSilinecek(null)
    }
  }

  const planAdi = (id: number | null) =>
    id === null ? '—' : (planlar.find((p) => p.id === id)?.ad ?? `#${id}`)

  return (
    <div className="max-w-5xl mx-auto px-4 py-6">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Müşteriler' }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Müşteriler</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          Fatura ve iletişim kayıtları. Domainler bu kayıtlara bağlanır — panel giriş hesabı değildir.
        </p>
      </div>

      <ListToolbar
        birincil={{ etiket: 'Yeni Müşteri', onClick: () => setDuzenlenen({ ...BOS }) }}
        aranan={aranan}
        arananSetter={setAranan}
      />

      {hata && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">{hata}</div>
      )}
      {basari && (
        <div className="mb-4 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-sm">{basari}</div>
      )}

      {yukleniyor ? (
        <div className="py-16 text-center text-sm text-slate-400">Yükleniyor…</div>
      ) : liste.length === 0 ? (
        <EmptyState
          baslik="Henüz müşteri kaydı yok"
          aciklama="Domainleri bir müşteriye bağlamak için önce müşteri kaydı oluşturun."
          buton={{ etiket: 'Yeni Müşteri', onClick: () => setDuzenlenen({ ...BOS }) }}
        />
      ) : suzulmus.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">Aramayla eşleşen müşteri yok.</div>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900/60">
              <tr>
                {['Ad', 'E-posta', 'Plan', 'Durum', 'Kayıt', ''].map((b, i) => (
                  <th key={i} className="px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 whitespace-nowrap">
                    {b}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800 bg-white dark:bg-slate-950">
              {suzulmus.map((m) => (
                <tr key={m.id} className="hover:bg-slate-50 dark:hover:bg-slate-900/60 transition">
                  <td className="px-3 py-2.5 font-medium text-slate-900 dark:text-slate-100 whitespace-nowrap">{m.ad}</td>
                  <td className="px-3 py-2.5 text-slate-600 dark:text-slate-400 whitespace-nowrap">{m.eposta}</td>
                  <td className="px-3 py-2.5 text-slate-600 dark:text-slate-400 whitespace-nowrap">{planAdi(m.plan_id)}</td>
                  <td className="px-3 py-2.5 whitespace-nowrap">
                    {m.durum === 'aktif'
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">Aktif</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">Pasif</span>}
                  </td>
                  <td className="px-3 py-2.5 text-xs text-slate-500 whitespace-nowrap">{m.olusturma}</td>
                  <td className="px-3 py-2.5 text-right whitespace-nowrap">
                    <button onClick={() => setDuzenlenen({ ...m })} className="text-xs text-brand-600 dark:text-brand-400 hover:underline mr-3">
                      Düzenle
                    </button>
                    <button onClick={() => setSilinecek(m)} className="text-xs text-red-600 dark:text-red-400 hover:underline">
                      Sil
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Modal
        acik={duzenlenen !== null}
        baslik={duzenlenen?.id ? 'Müşteriyi Düzenle' : 'Yeni Müşteri'}
        onKapat={() => setDuzenlenen(null)}
      >
        {duzenlenen && (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Ad</label>
              <input
                value={duzenlenen.ad}
                onChange={(e) => setDuzenlenen({ ...duzenlenen, ad: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">E-posta</label>
              <input
                type="email"
                value={duzenlenen.eposta}
                onChange={(e) => setDuzenlenen({ ...duzenlenen, eposta: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Plan</label>
                <select
                  value={duzenlenen.plan_id ?? ''}
                  onChange={(e) => setDuzenlenen({ ...duzenlenen, plan_id: e.target.value === '' ? null : Number(e.target.value) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  <option value="">Plansız</option>
                  {planlar.map((p) => <option key={p.id} value={p.id}>{p.ad}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Durum</label>
                <select
                  value={duzenlenen.durum}
                  onChange={(e) => setDuzenlenen({ ...duzenlenen, durum: e.target.value })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  <option value="aktif">Aktif</option>
                  <option value="pasif">Pasif</option>
                </select>
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">Notlar</label>
              <input
                value={duzenlenen.notlar}
                onChange={(e) => setDuzenlenen({ ...duzenlenen, notlar: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={() => setDuzenlenen(null)}
                className="px-3.5 py-2 text-sm rounded-full text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition"
              >
                Vazgeç
              </button>
              <button
                onClick={kaydet}
                disabled={kaydediliyor}
                className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition"
              >
                {kaydediliyor ? 'Kaydediliyor…' : 'Kaydet'}
              </button>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        acik={silinecek !== null}
        baslik="Müşteriyi sil"
        mesaj={`${silinecek?.ad ?? ''} kaydı silinecek. Bu müşteriye bağlı domainler silinmez, yalnız bağlantıları kopar.`}
        onayMetni="Sil"
        tehlikeli
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}
