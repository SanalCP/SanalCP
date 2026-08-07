// Bayi (reseller) paket kataloğu — /bayi-paketleri CRUD'un arayüzü.
//
// Bu paketler bayilere DOĞRUDAN atanmaz; admin, Kullanıcılar sayfasındaki
// "Bayi Limitleri" ekranında bir paket seçtiğinde limitler buradan ANLIK
// GÖRÜNTÜ olarak kopyalanır (bkz. internal/users LimitKaydet). Paketi sonradan
// değiştirmek zaten atanmış bayileri etkilemez.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from '@/i18n'
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
  const { t } = useTranslation(['BayiPaketleriPage', 'common'])
  const locale = i18n.language === 'en' ? 'en-US' : 'tr-TR'
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
      alert(apiHata(e, t('BayiPaketleriPage:errors.delete_failed')))
    }
  }

  return (
    <div className="px-6 py-5">
      <Breadcrumb items={[{ etiket: t('BayiPaketleriPage:breadcrumb.home'), href: '/' }, { etiket: t('BayiPaketleriPage:breadcrumb.title') }]} />
      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-2">{t('BayiPaketleriPage:title')}</h1>
      <p className="text-sm text-slate-500 dark:text-slate-500 mb-6">
        {t('BayiPaketleriPage:subtitle')}
      </p>

      <ListToolbar
        birincil={{ etiket: t('BayiPaketleriPage:buttons.add_package'), onClick: () => setModal({} as Paket) }}
        butonlar={[]}
      />

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('BayiPaketleriPage:loading')}</div>
      ) : items.length === 0 ? (
        <EmptyState
          baslik={t('BayiPaketleriPage:empty.title')}
          aciklama={t('BayiPaketleriPage:empty.description')}
          buton={{ etiket: t('BayiPaketleriPage:buttons.add_package'), onClick: () => setModal({} as Paket) }}
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {items.map(p => (
            <div key={p.id} className={`bg-white dark:bg-slate-800 border rounded-2xl p-5 shadow-sm ${p.varsayilan ? 'border-brand-400 ring-2 ring-brand-100 dark:ring-brand-900/40' : 'border-slate-200 dark:border-slate-700'}`}>
              <div className="flex items-start justify-between mb-2">
                <div className="min-w-0">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-100 flex items-center gap-2">
                    {p.ad}
                    {p.varsayilan && <span className="text-[10px] uppercase tracking-wider bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 px-1.5 py-0.5 rounded font-semibold">{t('BayiPaketleriPage:card.default_badge')}</span>}
                  </h3>
                  {p.aciklama && <p className="text-sm text-slate-500 dark:text-slate-500 mt-0.5">{p.aciklama}</p>}
                </div>
                {p.fiyat_kurus > 0 && <span className="shrink-0 text-[11px] font-mono font-semibold bg-slate-100 dark:bg-slate-700/60 text-slate-600 dark:text-slate-300 px-2 py-0.5 rounded">{fmtFiyat(p.fiyat_kurus, locale)}</span>}
              </div>

              <dl className="grid grid-cols-2 gap-y-1.5 text-sm mt-4">
                <Sat e={t('BayiPaketleriPage:card.labels.customer')} d={fmt(p.max_customer, 'count', locale, t)} />
                <Sat e={t('BayiPaketleriPage:card.labels.domain')} d={fmt(p.max_domain, 'count', locale, t)} />
                <Sat e={t('BayiPaketleriPage:card.labels.disk')} d={fmt(p.disk_kota_mb, 'mb', locale, t)} />
                <Sat e={t('BayiPaketleriPage:card.labels.traffic')} d={fmt(p.trafik_kota_mb, 'mb_per_month', locale, t)} />
              </dl>

              <p className="mt-3 text-[11px] text-slate-400">
                {p.fazla_satis ? t('BayiPaketleriPage:card.oversell_enabled') : t('BayiPaketleriPage:card.oversell_disabled')}
              </p>
              <p className="text-[11px] text-slate-400">
                {p.bayi_sayisi > 0 ? t('BayiPaketleriPage:card.used_count', { count: p.bayi_sayisi }) : t('BayiPaketleriPage:card.unused')}
              </p>

              <div className="mt-4 flex gap-2">
                <button onClick={() => setModal(p)} className="flex-1 text-center text-sm px-3 py-1.5 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 rounded-md">
                  {t('BayiPaketleriPage:card.actions.edit')}
                </button>
                <button onClick={() => setSilinecek(p)} className="text-sm px-3 py-1.5 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 rounded-md">{t('BayiPaketleriPage:card.actions.delete')}</button>
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
        baslik={t('BayiPaketleriPage:confirm.title')}
        mesaj={t('BayiPaketleriPage:confirm.message', { ad: silinecek?.ad || '' })}
        tehlikeli
        onayMetni={t('BayiPaketleriPage:confirm.yes')}
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

type UnitKey = 'count' | 'mb' | 'mb_per_month'

function fmt(n: number, birim: UnitKey, locale: string, t: (key: string) => string) {
  if (n <= 0) return t('BayiPaketleriPage:units.unlimited')
  if ((birim === 'mb' || birim === 'mb_per_month') && n >= 1024) {
    const perMonth = birim === 'mb_per_month' ? '/' + t('BayiPaketleriPage:units.mb_per_month').split('/')[1] : ''
    return `${(n / 1024).toFixed(1)} GB${perMonth}`
  }
  return `${n.toLocaleString(locale)} ${t(`BayiPaketleriPage:units.${birim}`)}`
}

function fmtFiyat(kurus: number, locale: string) {
  return (kurus / 100).toLocaleString(locale, { style: 'currency', currency: 'TRY' })
}

function PaketModal({ paket, onKapat, onKayit }: { paket: Paket; onKapat: () => void; onKayit: () => void }) {
  const { t } = useTranslation(['BayiPaketleriPage', 'common'])
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
      setHata(apiHata(e, t('BayiPaketleriPage:errors.save_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={yeni ? t('BayiPaketleriPage:modal.title_new') : t('BayiPaketleriPage:modal.title_edit')} onKapat={onKapat} genislik="lg">
      <form onSubmit={gonder} className="space-y-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Alan etiket={t('BayiPaketleriPage:modal.fields.package_name')} value={form.ad} setVal={v => setForm({ ...form, ad: v })} required />
          <Alan etiket={t('BayiPaketleriPage:modal.fields.description')} value={form.aciklama} setVal={v => setForm({ ...form, aciklama: v })} />
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Sayi etiket={t('BayiPaketleriPage:modal.fields.max_customer')} value={form.max_customer} setVal={v => setForm({ ...form, max_customer: v })} />
          <Sayi etiket={t('BayiPaketleriPage:modal.fields.max_domain')} value={form.max_domain} setVal={v => setForm({ ...form, max_domain: v })} />
          <Sayi etiket={t('BayiPaketleriPage:modal.fields.disk_mb')} value={form.disk_kota_mb} setVal={v => setForm({ ...form, disk_kota_mb: v })} />
          <Sayi etiket={t('BayiPaketleriPage:modal.fields.traffic_mb_per_month')} value={form.trafik_kota_mb} setVal={v => setForm({ ...form, trafik_kota_mb: v })} />
          <Sayi etiket={t('BayiPaketleriPage:modal.fields.price_kurus')} value={form.fiyat_kurus} setVal={v => setForm({ ...form, fiyat_kurus: v })} />
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.varsayilan} onChange={e => setForm({ ...form, varsayilan: e.target.checked })} className="rounded" />
          {t('BayiPaketleriPage:modal.checkboxes.default_promoted')}
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
          <input type="checkbox" checked={form.fazla_satis} onChange={e => setForm({ ...form, fazla_satis: e.target.checked })} className="rounded" />
          {t('BayiPaketleriPage:modal.checkboxes.allow_oversell')}
        </label>
        <p className="ta-hint">
          {t('BayiPaketleriPage:modal.info_note')}
        </p>

        {hata && <div className="ta-form-error" role="alert">{hata}</div>}

        <div className="ta-form-actions">
          <button type="button" onClick={onKapat} className="ta-secondary-button">{t('BayiPaketleriPage:modal.buttons.cancel')}</button>
          <button type="submit" disabled={isleniyor || !form.ad.trim()} className="ta-primary-button">{isleniyor ? t('BayiPaketleriPage:modal.buttons.saving') : (yeni ? t('BayiPaketleriPage:modal.buttons.add') : t('BayiPaketleriPage:modal.buttons.update'))}</button>
        </div>
      </form>
    </Modal>
  )
}

function Alan({ etiket, value, setVal, required }: { etiket: string; value: string; setVal: (v: string) => void; required?: boolean }) {
  return (
    <div>
      <label className="ta-label-sm">{etiket}</label>
      <input type="text" value={value} onChange={e => setVal(e.target.value)} required={required}
        className="ta-input ta-input-sm w-full" />
    </div>
  )
}
function Sayi({ etiket, value, setVal }: { etiket: string; value: number; setVal: (v: number) => void }) {
  return (
    <div>
      <label className="ta-label-sm">{etiket}</label>
      <input type="number" min={0} value={value} onChange={e => setVal(parseInt(e.target.value) || 0)}
        className="ta-input ta-input-sm w-full font-mono" />
    </div>
  )
}
