// Bayi (reseller) paket kataloğu — /bayi-paketleri CRUD'un arayüzü.
//
// Bu paketler bayilere DOĞRUDAN atanmaz; admin, Kullanıcılar sayfasındaki
// "Bayi Limitleri" ekranında bir paket seçtiğinde limitler buradan ANLIK
// GÖRÜNTÜ olarak kopyalanır (bkz. internal/users LimitKaydet). Paketi sonradan
// değiştirmek zaten atanmış bayileri etkilemez.
import { useEffect, useState } from 'react'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ListToolbar from '@/components/ListToolbar'
import EmptyState from '@/components/EmptyState'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type Paket = {
  id: number
  ad: string
  aciklama: string
  max_customer: number
  max_domain: number
  disk_kota_mb: number
  trafik_kota_mb: number
  fiyat_kurus: number
  fazla_satis: boolean
  varsayilan: boolean
  bayi_sayisi: number
  olusturulma: string
}

export default function BayiPaketleriPage() {
  const [items, setItems] = useState<Paket[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [modal, setModal] = useState<Paket | null>(null)
  const [silinecek, setSilinecek] = useState<Paket | null>(null)

  function yukle() {
    setYuk(true); setHata(null)
    api.get<Paket[]>('/bayi-paketleri')
      .then(r => setItems(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }
  useEffect(yukle, [])

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/bayi-paketleri/${silinecek.id}`)
      setSilinecek(null); yukle()
    } catch (e) {
      alert(apiHata(e, 'Silme başarısız'))
    }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: 'Anasayfa', href: '/' }, { etiket: 'Bayi Paketleri' }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">Bayi Paketleri</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
        Bayilere hızlıca uygulayabileceğiniz hazır limit paketleri. Kullanıcılar sayfasında bir
        bayinin limitlerini düzenlerken buradan seçim yaparsanız değerler anlık olarak kopyalanır —
        paketi sonradan değiştirmek zaten atanmış bayileri etkilemez.
      </p>

      <ListToolbar
        birincil={{ etiket: 'Paket Ekle', onClick: () => setModal({} as Paket) }}
        butonlar={[]}
      />

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">Yükleniyor…</div>
      ) : items.length === 0 ? (
        <EmptyState
          baslik="Henüz bayi paketi yok"
          aciklama="İlk paketinizi tanımlayarak başlayın."
          buton={{ etiket: 'Paket Ekle', onClick: () => setModal({} as Paket) }}
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {items.map(p => (
            <div key={p.id} className={`bg-white dark:bg-slate-800 border rounded-2xl p-5 shadow-sm ${p.varsayilan ? 'border-brand-400 ring-2 ring-brand-100 dark:ring-brand-900/40' : 'border-slate-200 dark:border-slate-700'}`}>
              <div className="flex items-start justify-between mb-2">
                <div className="min-w-0">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-100 flex items-center gap-2">
                    {p.ad}
                    {p.varsayilan && <span className="text-[10px] uppercase tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-1.5 py-0.5 rounded font-semibold">Varsayılan</span>}
                  </h3>
                  {p.aciklama && <p className="text-sm text-slate-500 dark:text-slate-500 mt-0.5">{p.aciklama}</p>}
                </div>
                {p.fiyat_kurus > 0 && <span className="shrink-0 text-[11px] font-mono font-semibold bg-slate-100 dark:bg-slate-700/60 text-slate-600 dark:text-slate-300 px-2 py-0.5 rounded">{fmtFiyat(p.fiyat_kurus)}</span>}
              </div>

              <dl className="grid grid-cols-2 gap-y-1.5 text-sm mt-4">
                <Sat e="Müşteri" d={fmt(p.max_customer, 'adet')} />
                <Sat e="Domain" d={fmt(p.max_domain, 'adet')} />
                <Sat e="Disk" d={fmt(p.disk_kota_mb, 'MB')} />
                <Sat e="Trafik" d={fmt(p.trafik_kota_mb, 'MB/ay')} />
              </dl>

              <p className="mt-3 text-[11px] text-slate-400">
                {p.fazla_satis ? 'Fazla satış açık' : 'Fazla satış kapalı — taahhüt limiti aşamaz'}
              </p>
              <p className="text-[11px] text-slate-400">
                {p.bayi_sayisi > 0 ? `${p.bayi_sayisi} bayi bu paketten dolduruldu` : 'Henüz kullanılmadı'}
              </p>

              <div className="mt-4 flex gap-2">
                <button onClick={() => setModal(p)} className="flex-1 text-center text-sm px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md">
                  Düzenle
                </button>
                <button onClick={() => setSilinecek(p)} className="text-sm px-3 py-1.5 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 rounded-md">Sil</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {modal && (
        <PaketModal
          paket={modal}
          onKapat={() => setModal(null)}
          onKayit={() => { setModal(null); yukle() }}
        />
      )}

      <ConfirmDialog
        acik={!!silinecek}
        baslik="Paketi sil"
        mesaj={`"${silinecek?.ad}" bayi paketi silinsin mi?`}
        tehlikeli
        onayMetni="Evet, sil"
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}

function Sat({ e, d }: { e: string; d: string }) {
  return (
    <>
      <dt className="text-slate-500 dark:text-slate-500">{e}</dt>
      <dd className="text-slate-800 dark:text-slate-200 text-right font-mono">{d}</dd>
    </>
  )
}

function fmt(n: number, birim: string) {
  if (n <= 0) return 'sınırsız'
  if (birim.startsWith('MB') && n >= 1024) return `${(n / 1024).toFixed(1)} G${birim.slice(2)}`
  return `${n.toLocaleString('tr-TR')} ${birim}`
}

function fmtFiyat(kurus: number) {
  return (kurus / 100).toLocaleString('tr-TR', { style: 'currency', currency: 'TRY' })
}

function PaketModal({ paket, onKapat, onKayit }: { paket: Paket; onKapat: () => void; onKayit: () => void }) {
  const yeni = !paket.id
  const [form, setForm] = useState<Paket>({
    id: paket.id || 0,
    ad: paket.ad || '',
    aciklama: paket.aciklama || '',
    max_customer: paket.max_customer || 0,
    max_domain: paket.max_domain || 0,
    disk_kota_mb: paket.disk_kota_mb || 0,
    trafik_kota_mb: paket.trafik_kota_mb || 0,
    fiyat_kurus: paket.fiyat_kurus || 0,
    fazla_satis: paket.id ? paket.fazla_satis : true,
    varsayilan: paket.varsayilan || false,
    bayi_sayisi: 0,
    olusturulma: '',
  })
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  async function gonder(e: React.FormEvent) {
    e.preventDefault()
    setIsleniyor(true); setHata(null)
    try {
      if (yeni) await api.post('/bayi-paketleri', form)
      else await api.put(`/bayi-paketleri/${form.id}`, form)
      onKayit()
    } catch (e) {
      setHata(apiHata(e, 'Kayıt başarısız'))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={yeni ? 'Yeni Bayi Paketi' : 'Paketi Düzenle'} onKapat={onKapat} genislik="lg">
      <form onSubmit={gonder} className="space-y-4">
        <div className="grid grid-cols-2 gap-3">
          <Alan etiket="Paket adı" value={form.ad} setVal={v => setForm({ ...form, ad: v })} required />
          <Alan etiket="Açıklama" value={form.aciklama} setVal={v => setForm({ ...form, aciklama: v })} />
        </div>
        <div className="grid grid-cols-3 gap-3">
          <Sayi etiket="Max müşteri" value={form.max_customer} setVal={v => setForm({ ...form, max_customer: v })} />
          <Sayi etiket="Max domain" value={form.max_domain} setVal={v => setForm({ ...form, max_domain: v })} />
          <Sayi etiket="Disk (MB)" value={form.disk_kota_mb} setVal={v => setForm({ ...form, disk_kota_mb: v })} />
          <Sayi etiket="Trafik (MB/ay)" value={form.trafik_kota_mb} setVal={v => setForm({ ...form, trafik_kota_mb: v })} />
          <Sayi etiket="Fiyat (kuruş)" value={form.fiyat_kurus} setVal={v => setForm({ ...form, fiyat_kurus: v })} />
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.varsayilan} onChange={e => setForm({ ...form, varsayilan: e.target.checked })} className="rounded" />
          Bayi Limitleri ekranında varsayılan olarak öne çıkar
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.fazla_satis} onChange={e => setForm({ ...form, fazla_satis: e.target.checked })} className="rounded" />
          Fazla satışa izin ver (kapatılırsa bayi, müşterilerine atadığı disk/trafik taahhüdü toplamında bu paketin limitini aşamaz)
        </label>
        <p className="text-xs text-slate-500 dark:text-slate-500">
          0 = sınırsız. Fiyat yalnız bilgi amaçlıdır, panelin bir fatura/ödeme akışı yoktur.
        </p>

        {hata && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{hata}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onKapat} className="px-4 py-2 border border-slate-200 dark:border-slate-700 rounded-md text-sm">İptal</button>
          <button type="submit" disabled={isleniyor || !form.ad.trim()} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm rounded-md">{isleniyor ? 'Kaydediliyor…' : (yeni ? 'Ekle' : 'Güncelle')}</button>
        </div>
      </form>
    </Modal>
  )
}

function Alan({ etiket, value, setVal, required }: { etiket: string; value: string; setVal: (v: string) => void; required?: boolean }) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{etiket}</label>
      <input type="text" value={value} onChange={e => setVal(e.target.value)} required={required}
        className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
    </div>
  )
}
function Sayi({ etiket, value, setVal }: { etiket: string; value: number; setVal: (v: number) => void }) {
  return (
    <div>
      <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{etiket}</label>
      <input type="number" min={0} value={value} onChange={e => setVal(parseInt(e.target.value) || 0)}
        className="w-full px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded text-sm font-mono focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 outline-none" />
    </div>
  )
}
