import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import Modal from '@/components/Modal'
import ConfirmDialog from '@/components/ConfirmDialog'
import DBParolaSifirlaModal from '@/components/DBParolaSifirlaModal'
import { uretGucluParola } from '@/lib/parola'

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
  const [kullaniciEkleAcik, setKullaniciEkleAcik] = useState(false)
  const [silinecekKullanici, setSilinecekKullanici] = useState<DBKullanici | null>(null)
  const [pwResetFor, setPwResetFor] = useState<DBKullanici | null>(null)

  async function kullaniciSil() {
    if (!silinecekKullanici || !id || !dbAdi) return
    try {
      await api.delete(`/domains/${id}/databases/${encodeURIComponent(dbAdi)}/kullanicilar/${silinecekKullanici.id}`)
      setSilinecekKullanici(null)
      yukle()
    } catch (e) {
      alert(apiHata(e, t('DomainDatabaseYonetPage:user_delete_failed')))
    }
  }

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

          <div className="ta-card p-5">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('DomainDatabaseYonetPage:users')}</h3>
              <button onClick={() => setKullaniciEkleAcik(true)} className="ta-secondary-button text-xs">{t('DomainDatabaseYonetPage:add_user')}</button>
            </div>
            <div className="space-y-2">
              {detay.kullanicilar.map(k => (
                <KullaniciSatiri
                  key={k.id}
                  kullanici={k}
                  sonKullanici={detay.kullanicilar.length <= 1}
                  onSifreDegistir={() => setPwResetFor(k)}
                  onSil={() => setSilinecekKullanici(k)}
                  t={t}
                />
              ))}
            </div>
          </div>

          <BakimKarti domainId={id!} dbAdi={dbAdi!} t={t} />
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

      {kullaniciEkleAcik && id && dbAdi && (
        <KullaniciEkleModal
          domainId={id}
          dbAdi={dbAdi}
          onKapat={() => setKullaniciEkleAcik(false)}
          onTamam={() => { setKullaniciEkleAcik(false); yukle() }}
        />
      )}

      {pwResetFor && dbAdi && (
        <DBParolaSifirlaModal
          db={{ id: pwResetFor.id, db_adi: dbAdi, db_kullanici: pwResetFor.db_kullanici }}
          onKapat={() => setPwResetFor(null)}
          onTamam={() => { setPwResetFor(null); yukle() }}
        />
      )}

      <ConfirmDialog
        acik={!!silinecekKullanici}
        baslik={t('DomainDatabaseYonetPage:user_delete_title')}
        mesaj={t('DomainDatabaseYonetPage:user_delete_msg', { ad: silinecekKullanici?.db_kullanici })}
        tehlikeli
        onayMetni={t('DomainDatabaseYonetPage:user_delete_confirm')}
        onOnay={kullaniciSil}
        onIptal={() => setSilinecekKullanici(null)}
      />
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

function KullaniciSatiri({ kullanici, sonKullanici, onSifreDegistir, onSil, t }: {
  kullanici: DBKullanici; sonKullanici: boolean
  onSifreDegistir: () => void; onSil: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}) {
  const [goster, setGoster] = useState(false)
  const [kopya, setKopya] = useState(false)
  return (
    <div className="flex items-center justify-between py-2 border-b border-slate-50 dark:border-slate-800 last:border-0">
      <div className="flex items-center gap-2">
        <span className="font-mono text-sm text-slate-800 dark:text-slate-200">{kullanici.db_kullanici}</span>
        <button onClick={() => setGoster(!goster)} className="font-mono text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 rounded">
          {goster ? kullanici.db_parola : '••••••••'}
        </button>
        {goster && (
          <button
            onClick={() => { navigator.clipboard.writeText(kullanici.db_parola); setKopya(true); setTimeout(() => setKopya(false), 1500) }}
            className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 hover:bg-brand-100 dark:hover:bg-brand-900/30 hover:text-brand-700 dark:text-brand-300 rounded"
          >
            {kopya ? '✓' : '⧉'}
          </button>
        )}
      </div>
      <div className="flex items-center gap-1">
        <button onClick={onSifreDegistir} className="text-xs text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 px-2 py-1 rounded">{t('DomainDatabaseYonetPage:change_password')}</button>
        <button
          onClick={onSil}
          disabled={sonKullanici}
          title={sonKullanici ? t('DomainDatabaseYonetPage:last_user_hint') : undefined}
          className="text-xs text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 px-2 py-1 rounded disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {t('common:delete')}
        </button>
      </div>
    </div>
  )
}

function KullaniciEkleModal({ domainId, dbAdi, onKapat, onTamam }: {
  domainId: string; dbAdi: string; onKapat: () => void; onTamam: () => void
}) {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const [tip, setTip] = useState<'yeni' | 'mevcut'>('yeni')
  const [sonek, setSonek] = useState('')
  const [mevcutKullanici, setMevcutKullanici] = useState('')
  const [parola, setParola] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  async function ekle() {
    setIsleniyor(true); setHata(null)
    try {
      const body = tip === 'yeni'
        ? { kullanici_tipi: 'yeni', kullanici_sonek: sonek, parola }
        : { kullanici_tipi: 'mevcut', mevcut_kullanici: mevcutKullanici }
      await api.post(`/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/kullanicilar`, body)
      onTamam()
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:add_user_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <Modal acik={true} baslik={t('DomainDatabaseYonetPage:add_user_modal_title')} onKapat={onKapat} genislik="md">
      <div className="space-y-4">
        <div className="flex gap-4">
          <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
            <input type="radio" checked={tip === 'yeni'} onChange={() => setTip('yeni')} className="accent-brand-600" />
            {t('DomainDatabaseYonetPage:new_user_radio')}
          </label>
          <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
            <input type="radio" checked={tip === 'mevcut'} onChange={() => setTip('mevcut')} className="accent-brand-600" />
            {t('DomainDatabaseYonetPage:existing_user_radio')}
          </label>
        </div>

        {tip === 'yeni' ? (
          <>
            <div>
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:user_suffix_label')}</label>
              <input value={sonek} onChange={e => setSonek(e.target.value.toLowerCase())} placeholder="ikincikullanici" className="ta-input ta-input-sm w-full font-mono" />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:password_label')}</label>
              <div className="flex gap-2">
                <input type="text" value={parola} onChange={e => setParola(e.target.value)} placeholder={t('DomainDatabaseYonetPage:password_placeholder')} className="ta-input ta-input-sm w-full font-mono" />
                <button type="button" onClick={() => setParola(uretGucluParola())} className="ta-secondary-button whitespace-nowrap text-xs">{t('DomainDatabaseYonetPage:generate')}</button>
              </div>
            </div>
          </>
        ) : (
          <div>
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabaseYonetPage:existing_user_label')}</label>
            <input value={mevcutKullanici} onChange={e => setMevcutKullanici(e.target.value)} placeholder="sk_baska_kullanici" className="ta-input ta-input-sm w-full font-mono" />
          </div>
        )}

        {hata && <div className="ta-form-error">{hata}</div>}
        <div className="ta-form-actions">
          <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
          <button onClick={ekle} disabled={isleniyor || (tip === 'yeni' ? !sonek : !mevcutKullanici)} className="ta-primary-button">
            {isleniyor ? t('DomainDatabaseYonetPage:adding') : t('DomainDatabaseYonetPage:add_user')}
          </button>
        </div>
      </div>
    </Modal>
  )
}

function BakimKarti({ domainId, dbAdi, t }: { domainId: string; dbAdi: string; t: (k: string, opts?: Record<string, unknown>) => string }) {
  const [isleniyor, setIsleniyor] = useState<string | null>(null)
  const [sonucMetni, setSonucMetni] = useState<string | null>(null)
  const [hata, setHata] = useState<string | null>(null)
  const [geriYukleAcik, setGeriYukleAcik] = useState(false)

  function yedekle() {
    setIsleniyor('yedek'); setHata(null)
    fetch(`/api/v1/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/yedek`, { credentials: 'include' })
      .then(async r => {
        if (!r.ok) throw new Error(await r.text())
        return r.blob()
      })
      .then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = `${dbAdi}.sql.gz`
        a.click()
      })
      .catch(() => setHata(t('DomainDatabaseYonetPage:backup_failed')))
      .finally(() => setIsleniyor(null))
  }

  async function mysqlcheckCalistir(uc: 'optimize' | 'onar') {
    setIsleniyor(uc); setHata(null); setSonucMetni(null)
    try {
      const { data } = await api.post(`/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/${uc}`)
      setSonucMetni(data.sonuc)
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:maintenance_failed')))
    } finally {
      setIsleniyor(null)
    }
  }

  return (
    <div className="ta-card p-5">
      <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100 mb-3">{t('DomainDatabaseYonetPage:maintenance')}</h3>
      <div className="flex flex-wrap gap-2">
        <button onClick={yedekle} disabled={!!isleniyor} className="ta-secondary-button">
          {isleniyor === 'yedek' ? t('DomainDatabaseYonetPage:backing_up') : t('DomainDatabaseYonetPage:backup_button')}
        </button>
        <button onClick={() => setGeriYukleAcik(true)} disabled={!!isleniyor} className="ta-secondary-button">
          {t('DomainDatabaseYonetPage:restore_button')}
        </button>
        <button onClick={() => mysqlcheckCalistir('optimize')} disabled={!!isleniyor} className="ta-secondary-button">
          {isleniyor === 'optimize' ? t('DomainDatabaseYonetPage:optimizing') : t('DomainDatabaseYonetPage:optimize_button')}
        </button>
        <button onClick={() => mysqlcheckCalistir('onar')} disabled={!!isleniyor} className="ta-secondary-button">
          {isleniyor === 'onar' ? t('DomainDatabaseYonetPage:repairing') : t('DomainDatabaseYonetPage:repair_button')}
        </button>
      </div>

      {hata && <div className="mt-3 ta-form-error">{hata}</div>}
      {sonucMetni && (
        <pre className="mt-3 p-3 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded text-xs font-mono text-slate-700 dark:text-slate-300 overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap">
          {sonucMetni}
        </pre>
      )}

      {geriYukleAcik && (
        <GeriYukleModal
          domainId={domainId}
          dbAdi={dbAdi}
          onKapat={() => setGeriYukleAcik(false)}
          onTamam={(msg) => { setGeriYukleAcik(false); setSonucMetni(msg); setHata(null) }}
        />
      )}
    </div>
  )
}

function GeriYukleModal({ domainId, dbAdi, onKapat, onTamam }: {
  domainId: string; dbAdi: string; onKapat: () => void; onTamam: (sonuc: string) => void
}) {
  const { t } = useTranslation(['DomainDatabaseYonetPage', 'common'])
  const [dosya, setDosya] = useState<File | null>(null)
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [onaySoruluyor, setOnaySoruluyor] = useState(false)

  async function yukle() {
    if (!dosya) return
    setIsleniyor(true); setHata(null)
    try {
      const form = new FormData()
      form.append('dosya', dosya)
      const { data } = await api.post(`/domains/${domainId}/databases/${encodeURIComponent(dbAdi)}/geri-yukle`, form, {
        timeout: 0, // buyuk geri yukleme: client tarafinda iptal etme (backend 15dk sinir) — DomainFilesPage.tsx upload deseniyle ayni
      })
      onTamam(data.sonuc)
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabaseYonetPage:restore_failed')))
      setOnaySoruluyor(false)
    } finally {
      setIsleniyor(false)
    }
  }

  return (
    <>
      <Modal acik={!onaySoruluyor} baslik={t('DomainDatabaseYonetPage:restore_modal_title')} onKapat={onKapat} genislik="md">
        <div className="space-y-4">
          <div className="ta-form-error !bg-amber-50 dark:!bg-amber-900/20 !border-amber-200 dark:!border-amber-800 !text-amber-800 dark:!text-amber-200">
            {t('DomainDatabaseYonetPage:restore_warning')}
          </div>
          <input type="file" accept=".sql,.gz" onChange={e => setDosya(e.target.files?.[0] ?? null)} className="ta-input ta-input-sm w-full" />
          {hata && <div className="ta-form-error">{hata}</div>}
          <div className="ta-form-actions">
            <button onClick={onKapat} disabled={isleniyor} className="ta-secondary-button">{t('common:cancel')}</button>
            <button onClick={() => setOnaySoruluyor(true)} disabled={isleniyor || !dosya} className="ta-primary-button">{t('DomainDatabaseYonetPage:restore_button')}</button>
          </div>
        </div>
      </Modal>
      <ConfirmDialog
        acik={onaySoruluyor}
        baslik={t('DomainDatabaseYonetPage:restore_confirm_title')}
        mesaj={t('DomainDatabaseYonetPage:restore_confirm_msg')}
        tehlikeli
        onayMetni={t('DomainDatabaseYonetPage:restore_confirm_button')}
        onOnay={yukle}
        onIptal={() => setOnaySoruluyor(false)}
      />
    </>
  )
}
