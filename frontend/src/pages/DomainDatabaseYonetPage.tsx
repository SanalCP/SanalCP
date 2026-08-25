import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'

type DBKullanici = { id: number; db_kullanici: string; db_parola: string; olusturulma: string }
export type DBGrupDetay = {
  db_adi: string; db_host: string; charset: string; collation: string
  boyut_mb: number; kullanicilar: DBKullanici[]
}
type Domain = { id: number; alan_adi: string; sistem_kullanici: string }

function fmtBoyut(mb: number): string {
  if (mb >= 1024) return (mb / 1024).toFixed(2) + ' GB'
  return mb.toFixed(1) + ' MB'
}

export default function DomainDatabaseYonetPage() {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const { id, dbAdi } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [detay, setDetay] = useState<DBGrupDetay | null>(null)
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [isimDegistirAcik, setIsimDegistirAcik] = useState(false)

  function yukle() {
    if (!id || !dbAdi) return
    setYuk(true); setHata(null)
    api.get<DBGrupDetay>(`/domains/${id}/databases/${encodeURIComponent(dbAdi)}`)
      .then(r => setDetay(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }

  useEffect(yukle, [id, dbAdi])
  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
  }, [id])

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('common:domain'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainDatabaseYonetPage:breadcrumb_databases'), href: `/abonelikler/${id}/veritabanlari` },
        { etiket: dbAdi || '...' },
      ]} />

      <div className="flex items-center justify-between mb-5">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 font-mono">{dbAdi}</h1>
        <Link
          to={`/abonelikler/${id}/veritabanlari`}
          className="text-sm text-slate-500 dark:text-slate-500 hover:text-slate-700 dark:hover:text-slate-300"
        >
          ← {t('DomainDatabaseYonetPage:back_to_list')}
        </Link>
      </div>

      {hata && <div className="mb-4 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      {yuk ? (
        <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div>
      ) : detay ? (
        <div className="space-y-4 max-w-3xl">
          <div className="ta-card p-5">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainDatabaseYonetPage:general_info')}</h3>
            <div className="grid grid-cols-2 gap-y-2 gap-x-4 text-sm">
              <BilgiSatiri e={t('DomainDatabaseYonetPage:size')} d={fmtBoyut(detay.boyut_mb)} />
              <BilgiSatiri e={t('DomainDatabaseYonetPage:server')} d={`${detay.db_host}:3306`} mono />
              <BilgiSatiri e={t('DomainDatabaseYonetPage:charset')} d={detay.charset || '—'} mono />
              <BilgiSatiri e={t('DomainDatabaseYonetPage:collation')} d={detay.collation || '—'} mono />
            </div>
            <div className="mt-4 pt-3 border-t border-slate-100 dark:border-slate-800">
              <button onClick={() => setIsimDegistirAcik(true)} className="ta-secondary-button">{t('DomainDatabaseYonetPage:rename_button')}</button>
            </div>
          </div>
        </div>
      ) : null}

      {isimDegistirAcik && dbAdi && (
        <IsimDegistirModal
          domainId={id!}
          eskiAd={dbAdi}
          onKapat={() => setIsimDegistirAcik(false)}
          onTamam={(yeniAd) => { setIsimDegistirAcik(false); window.location.href = `/abonelikler/${id}/veritabanlari/${encodeURIComponent(yeniAd)}` }}
        />
      )}
    </div>
  )
}

function BilgiSatiri({ e, d, mono }: { e: string; d: string; mono?: boolean }) {
  return (
    <>
      <div className="text-slate-500 dark:text-slate-500">{e}</div>
      <div className={`text-slate-800 dark:text-slate-200 ${mono ? 'font-mono' : ''}`}>{d}</div>
    </>
  )
}

function IsimDegistirModal({ domainId, eskiAd, onKapat, onTamam }: {
  domainId: string; eskiAd: string; onKapat: () => void; onTamam: (yeniAd: string) => void
}) {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const [sonek, setSonek] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [onaySoruluyor, setOnaySoruluyor] = useState(false)

  async function uygula() {
    setIsleniyor(true); setHata(null)
    try {
      const { data } = await api.put(`/domains/${domainId}/databases/${encodeURIComponent(eskiAd)}/isim`, { yeni_sonek: sonek })
      onTamam(data.yeni_ad)
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:rename_failed')))
      setOnaySoruluyor(false)
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <>
      <Modal acik={!onaySoruluyor} baslik={t('DomainDatabaseYonetPage:rename_modal_title')} onKapat={onKapat} genislik="md">
        <div className="space-y-4">
          <div className="ta-form-error !bg-amber-50 dark:!bg-amber-900/20 !border-amber-200 dark:!border-amber-800 !text-amber-800 dark:!text-amber-200">
            {t('DomainDatabaseYonetPage:rename_warning')}
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:new_suffix_label')}</label>
            <input value={sonek} onChange={e => setSonek(e.target.value.toLowerCase())} placeholder="yeniisim" className="ta-input ta-input-sm w-full font-mono" />
          </div>
          {hata && <div className="ta-form-error">{hata}</div>}
          <div className="ta-form-actions">
            <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
            <button onClick={() => setOnaySoruluyor(true)} disabled={isleniyor || !sonek} className="ta-primary-button">{t('DomainDatabaseYonetPage:rename_button')}</button>
          </div>
        </div>
      </Modal>
      <ConfirmDialog
        acik={onaySoruluyor}
        baslik={t('DomainDatabaseYonetPage:rename_confirm_title')}
        mesaj={t('DomainDatabaseYonetPage:rename_confirm_msg')}
        tehlikeli
        onayMetni={t('DomainDatabaseYonetPage:rename_confirm_button')}
        onOnay={uygula}
        onIptal={() => setOnaySoruluyor(false)}
      />
    </>
  )
}
