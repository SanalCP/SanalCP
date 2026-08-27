// Müşteri kayıtları (customers tablosu) — /customers CRUD uçları panelin
// başından beri vardı ama arayüzü hiç yazılmamıştı; müşteri eklemek yalnız
// API'den mümkündü. Domainler bu kayıtlara domains.customer_id ile bağlanır.
//
// NOT: Bunlar panel giriş hesabı DEĞİLDİR — fatura/iletişim kaydıdır. Giriş
// hesabı users tablosundadır (rol='user') ve customers.user_id ile buraya
// bağlanır; müşteri o hesapla /cp adresinden girer.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import EmptyState from '@/components/EmptyState'
import ListToolbar from '@/components/ListToolbar'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'
import { T } from '@/lib/tablo'
import { metneGoreSirala } from '@/lib/sirala'

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
  const { t } = useTranslation(['MusterilerPage', 'common'])
  const [aramaParam] = useSearchParams()
  const [liste, setListe] = useState<Musteri[]>([])
  const [planlar, setPlanlar] = useState<Plan[]>([])
  const [yukleniyor, setYukleniyor] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)
  const [aranan, setAranan] = useState(() => aramaParam.get('arama') || '')

  useEffect(() => { setAranan(aramaParam.get('arama') || '') }, [aramaParam])

  const [duzenlenen, setDuzenlenen] = useState<Musteri | null>(null)
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [silinecek, setSilinecek] = useState<Musteri | null>(null)

  const getir = useCallback(async () => {
    setYukleniyor(true)
    try {
      const r = await api.get<Musteri[]>('/customers')
      setListe(Array.isArray(r.data) ? r.data : [])
      setHata(null)
    } catch (e) {
      setHata(apiHata(e, t('MusterilerPage:error.load_failed')))
    } finally {
      setYukleniyor(false)
    }
  }, [t])

  useEffect(() => {
    getir()
    api.get<Plan[]>('/plans')
      .then((r) => setPlanlar(Array.isArray(r.data) ? r.data : []))
      .catch(() => {})
  }, [getir])

  const suzulmus = useMemo(() => {
    const t = aranan.trim().toLowerCase()
    const temel = t
      ? liste.filter((m) => `${m.ad} ${m.eposta} ${m.notlar}`.toLowerCase().includes(t))
      : liste
    // İlk sütun müşteri adı.
    return metneGoreSirala(temel, (m) => m.ad)
  }, [liste, aranan])

  async function kaydet() {
    if (!duzenlenen) return
    const ad = duzenlenen.ad.trim()
    const eposta = duzenlenen.eposta.trim()
    if (!ad || !eposta) {
      setHata(t('MusterilerPage:error.name_email_required'))
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
        setBasari(t('MusterilerPage:success.added', { name: ad }))
      } else {
        await api.put(`/customers/${duzenlenen.id}`, govde)
        setBasari(t('MusterilerPage:success.updated', { name: ad }))
      }
      setDuzenlenen(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, t('MusterilerPage:error.save_failed')))
    } finally {
      setKaydediliyor(false)
    }
  }

  async function sil() {
    if (!silinecek) return
    try {
      await api.delete(`/customers/${silinecek.id}`)
      setBasari(t('MusterilerPage:success.deleted', { name: silinecek.ad }))
      setSilinecek(null)
      await getir()
    } catch (e) {
      setHata(apiHata(e, t('MusterilerPage:error.delete_failed')))
      setSilinecek(null)
    }
  }

  const planAdi = (id: number | null) =>
    id === null ? '—' : (planlar.find((p) => p.id === id)?.ad ?? `#${id}`)

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[{ etiket: t('common:home'), href: '/' }, { etiket: t('MusterilerPage:breadcrumb_title') }]} />

      <div className="mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">{t('MusterilerPage:title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {t('MusterilerPage:subtitle')}
        </p>
      </div>

      <ListToolbar
        birincil={{ etiket: t('MusterilerPage:toolbar_new'), onClick: () => setDuzenlenen({ ...BOS }) }}
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
        <div className="py-16 text-center text-sm text-slate-400">{t('common:loading')}</div>
      ) : liste.length === 0 ? (
        <EmptyState
          baslik={t('MusterilerPage:empty.title')}
          aciklama={t('MusterilerPage:empty.description')}
          buton={{ etiket: t('MusterilerPage:toolbar_new'), onClick: () => setDuzenlenen({ ...BOS }) }}
        />
      ) : suzulmus.length === 0 ? (
        <div className="py-12 text-center text-sm text-slate-400">{t('MusterilerPage:search_empty')}</div>
      ) : (
        <div className="lg:overflow-x-auto lg:rounded-xl lg:border lg:border-slate-200 dark:lg:border-slate-800">
          <table className={T.tablo}>
            <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900/60`}>
              <tr>
                {[t('MusterilerPage:table.name'), t('MusterilerPage:table.email'), t('MusterilerPage:table.plan'), t('MusterilerPage:table.status'), t('MusterilerPage:table.created'), ''].map((b, i) => (
                  <th key={i} className={`${T.baslik} whitespace-nowrap`}>
                    {b}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800 lg:bg-white dark:lg:bg-slate-950`}>
              {suzulmus.map((m) => (
                <tr key={m.id} className={T.satir}>
                  <td className={T.hucreBaslik}>{m.ad}</td>
                  <td className={T.hucre} data-etiket={t('MusterilerPage:table.email')}><span className="break-all text-slate-600 dark:text-slate-400">{m.eposta}</span></td>
                  <td className={T.hucre} data-etiket={t('MusterilerPage:table.plan')}><span className="text-slate-600 dark:text-slate-400">{planAdi(m.plan_id)}</span></td>
                  <td className={T.hucre} data-etiket={t('MusterilerPage:table.status')}>
                    {m.durum === 'aktif'
                      ? <span className="px-2 py-0.5 rounded text-xs bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">{t('MusterilerPage:status.active')}</span>
                      : <span className="px-2 py-0.5 rounded text-xs bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">{t('MusterilerPage:status.inactive')}</span>}
                  </td>
                  <td className={`${T.hucre} text-xs text-slate-500`} data-etiket={t('MusterilerPage:table.created')}>{m.olusturma}</td>
                  <td className={T.hucreAksiyon}>
                    <button onClick={() => setDuzenlenen({ ...m })} className="text-xs text-brand-600 dark:text-brand-400 hover:underline mr-3">
                      {t('MusterilerPage:actions.edit')}
                    </button>
                    <button onClick={() => setSilinecek(m)} className="text-xs text-red-600 dark:text-red-400 hover:underline">
                      {t('MusterilerPage:actions.delete')}
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
        baslik={duzenlenen?.id ? t('MusterilerPage:modal.title_edit') : t('MusterilerPage:modal.title_new')}
        onKapat={() => setDuzenlenen(null)}
      >
        {duzenlenen && (
          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('MusterilerPage:fields.name')}</label>
              <input
                value={duzenlenen.ad}
                onChange={(e) => setDuzenlenen({ ...duzenlenen, ad: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('MusterilerPage:fields.email')}</label>
              <input
                type="email"
                value={duzenlenen.eposta}
                onChange={(e) => setDuzenlenen({ ...duzenlenen, eposta: e.target.value })}
                className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
              />
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('MusterilerPage:fields.plan')}</label>
                <select
                  value={duzenlenen.plan_id ?? ''}
                  onChange={(e) => setDuzenlenen({ ...duzenlenen, plan_id: e.target.value === '' ? null : Number(e.target.value) })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  <option value="">{t('MusterilerPage:fields.plan_none')}</option>
                  {planlar.map((p) => <option key={p.id} value={p.id}>{p.ad}</option>)}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('MusterilerPage:fields.status')}</label>
                <select
                  value={duzenlenen.durum}
                  onChange={(e) => setDuzenlenen({ ...duzenlenen, durum: e.target.value })}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-1 focus:ring-brand-500"
                >
                  <option value="aktif">{t('MusterilerPage:status.active')}</option>
                  <option value="pasif">{t('MusterilerPage:status.inactive')}</option>
                </select>
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-500 dark:text-slate-400 mb-1">{t('MusterilerPage:fields.notes')}</label>
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
                {t('MusterilerPage:buttons.cancel')}
              </button>
              <button
                onClick={kaydet}
                disabled={kaydediliyor}
                className="px-3.5 py-2 text-sm font-medium rounded-full bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 transition"
              >
                {kaydediliyor ? t('MusterilerPage:buttons.saving') : t('MusterilerPage:buttons.save')}
              </button>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        acik={silinecek !== null}
        baslik={t('MusterilerPage:confirm.title')}
        mesaj={t('MusterilerPage:confirm.message', { name: silinecek?.ad ?? '' })}
        onayMetni={t('MusterilerPage:confirm.yes')}
        tehlikeli
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}
