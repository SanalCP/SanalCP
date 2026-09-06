// sanal-dark-swept
// sanal-dark-swept-v2
import KopyalaButton from '@/components/KopyalaButton'
import { modalUyari } from '@/lib/dialog'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'
import Breadcrumb from '@/components/Breadcrumb'
import ConfirmDialog from '@/components/ConfirmDialog'
import Modal from '@/components/Modal'
import { T } from '@/lib/tablo'
import { uretGucluParola } from '@/lib/parola'

type Domain = { id: number; alan_adi: string; sistem_kullanici: string }
type DB = {
  id: number; domain_id: number; db_adi: string; db_kullanici: string;
  db_host: string; db_parola: string; olusturulma: string
}
type DBGrup = { db_adi: string; ilkId: number; kullaniciSayisi: number; olusturulma: string }

export default function DomainDatabasesPage() {
  const { t } = useTranslation(['DomainDatabasesPage', 'common'])
  const { id } = useParams()
  const [domain, setDomain] = useState<Domain | null>(null)
  const [dbler, setDbler] = useState<DB[]>([])
  const [yuk, setYuk] = useState(true)
  const [hata, setHata] = useState<string | null>(null)
  const [silinecek, setSilinecek] = useState<DBGrup | null>(null)
  const [ekleAcik, setEkleAcik] = useState(false)

  const yukle = useCallback(() => {
    if (!id) return
    setYuk(true)
    api.get<DB[]>(`/domains/${id}/databases`)
      .then(r => setDbler(r.data))
      .catch(e => setHata(apiHata(e)))
      .finally(() => setYuk(false))
  }, [id])

  useEffect(() => {
    if (id) api.get<Domain>(`/domains/${id}`).then(r => setDomain(r.data)).catch(() => {})
    yukle()
  }, [id, yukle])

  // db_adi'na gore grupla: ayni DB'ye birden fazla kullanici baglanabilir
  // (bkz. Yonet sayfasi "kullanici ekle"), listede tek satir kalir.
  const gruplar = useMemo<DBGrup[]>(() => {
    const map = new Map<string, DBGrup>()
    for (const d of dbler) {
      const mevcut = map.get(d.db_adi)
      if (mevcut) {
        mevcut.kullaniciSayisi++
      } else {
        map.set(d.db_adi, { db_adi: d.db_adi, ilkId: d.id, kullaniciSayisi: 1, olusturulma: d.olusturulma })
      }
    }
    return Array.from(map.values())
  }, [dbler])

  const mevcutKullanicilar = useMemo(
    () => Array.from(new Set(dbler.map(d => d.db_kullanici))),
    [dbler],
  )

  async function sil() {
    if (!silinecek) return
    try { await api.delete(`/databases/${silinecek.ilkId}`); setSilinecek(null); yukle() }
    catch (e) { await modalUyari(apiHata(e, t('DomainDatabasesPage:delete_failed'))) }
  }

  return (
    <div className="w-full px-6 py-5">
      <Breadcrumb items={[
        { etiket: t('common:home'), href: '/' }, { etiket: t('common:domain'), href: '/domainler' },
        { etiket: domain?.alan_adi || '...', href: `/abonelikler/${id}` },
        { etiket: t('DomainDatabasesPage:breadcrumb_title') },
      ]} />

      <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100 mb-1">{t('DomainDatabasesPage:title')}</h1>
      {domain && <p className="text-sm text-slate-500 dark:text-slate-500 mb-5"><Link to={`/abonelikler/${id}`} className="text-brand-600 dark:text-brand-400 hover:text-brand-700 dark:hover:text-brand-300 font-medium">{domain.alan_adi}</Link></p>}

      <div className="flex items-center gap-2 mb-4">
        <button onClick={() => setEkleAcik(true)} className="px-3.5 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm font-medium rounded-md">{t('DomainDatabasesPage:new_database')}</button>
        <button onClick={yukle} className="px-3 py-2 bg-white hover:bg-slate-50 dark:bg-slate-900 dark:hover:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 text-sm rounded-md">↻ {t('DomainDatabasesPage:refresh')}</button>
        <span className="ml-auto text-sm text-slate-500 dark:text-slate-500">{gruplar.length} {t('DomainDatabasesPage:count_suffix')}</span>
      </div>

      {hata && <div className="mb-3 px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md text-sm text-red-700 dark:text-red-300">{hata}</div>}

      <div className="bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-2xl overflow-hidden">
        {yuk ? <div className="py-12 text-center text-sm text-slate-400 dark:text-slate-500">{t('common:loading')}</div> :
         gruplar.length === 0 ? <div className="py-12 text-center text-sm text-slate-500 dark:text-slate-500">{t('DomainDatabasesPage:no_databases')}</div> :
        <table className={T.tablo}>
          <thead className={`${T.baslikGrubu} bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700`}>
            <tr>
              <th className={T.baslik}>{t('DomainDatabasesPage:col_database')}</th>
              <th className={T.baslik}>{t('DomainDatabasesPage:col_user_count')}</th>
              <th className={T.baslik}>{t('DomainDatabasesPage:col_created')}</th>
              <th className={`${T.baslik} text-right`}>{t('DomainDatabasesPage:col_actions')}</th>
            </tr>
          </thead>
          <tbody className={`${T.govde} lg:divide-y lg:divide-slate-100 dark:lg:divide-slate-800`}>
            {gruplar.map(g => (
              <tr key={g.db_adi} className={`${T.satir} lg:hover:bg-slate-50 dark:lg:hover:bg-slate-800`}>
                <td className={T.hucreBaslik}><span className="font-mono lg:text-sm text-base">{g.db_adi}</span></td>
                <td className={T.hucre} data-etiket={t('DomainDatabasesPage:col_user_count')}>
                  <span className="text-sm text-slate-600 dark:text-slate-400">{g.kullaniciSayisi}</span>
                </td>
                <td className={T.hucre} data-etiket={t('DomainDatabasesPage:col_created')}><span className="text-sm text-slate-600 dark:text-slate-400">{g.olusturulma}</span></td>
                <td className={T.hucreAksiyon}>
                  <Link to={`/abonelikler/${id}/veritabanlari/${encodeURIComponent(g.db_adi)}`} className="text-sm text-brand-600 dark:text-brand-400 hover:bg-brand-50 dark:hover:bg-brand-900/30 dark:bg-brand-900/20 px-2 py-1 rounded">{t('DomainDatabasesPage:manage')}</Link>
                  <button onClick={() => setSilinecek(g)} className="text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30 dark:bg-red-900/20 px-2 py-1 rounded">{t('common:delete')}</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>}
      </div>

      {ekleAcik && domain && (
        <YeniDBModal
          domainId={Number(id)}
          sk={domain.sistem_kullanici}
          mevcutKullanicilar={mevcutKullanicilar}
          onKapat={() => setEkleAcik(false)}
          onTamam={() => { setEkleAcik(false); yukle() }}
          t={t}
        />
      )}

      <ConfirmDialog
        acik={!!silinecek}
        baslik={t('DomainDatabasesPage:delete_dialog_title')}
        mesaj={t('DomainDatabasesPage:delete_dialog_msg', { ad: silinecek?.db_adi })}
        tehlikeli
        onayMetni={t('DomainDatabasesPage:delete_confirm')}
        onOnay={sil}
        onIptal={() => setSilinecek(null)}
      />
    </div>
  )
}

type YeniDBModalProps = {
  domainId: number
  sk: string
  mevcutKullanicilar: string[]
  onKapat: () => void
  onTamam: () => void
  t: (k: string, opts?: Record<string, unknown>) => string
}

const SONEK_RE = /^[a-z0-9_]{1,32}$/

function YeniDBModal({ domainId, sk, mevcutKullanicilar, onKapat, onTamam, t }: YeniDBModalProps) {
  const onek = sk + '_'
  const [otomatik, setOtomatik] = useState(true)
  const [dbSonek, setDbSonek] = useState('')
  const [kullaniciTipi, setKullaniciTipi] = useState<'yeni' | 'mevcut'>(
    mevcutKullanicilar.length ? 'yeni' : 'yeni',
  )
  const [kullaniciSonek, setKullaniciSonek] = useState('')
  const [mevcutKullanici, setMevcutKullanici] = useState(mevcutKullanicilar[0] || '')
  const [parola, setParola] = useState('')
  const [isleniyor, setIsleniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [sonuc, setSonuc] = useState<{ db_adi: string; db_kullanici: string; db_parola: string } | null>(null)

  const dbAdiOnizleme = onek + (dbSonek || '…')
  const kullaniciOnizleme = onek + (kullaniciSonek || '…')
  const parolaGucSorunu =
    parola !== '' && (parola.length < 12 || !/[A-Za-z]/.test(parola) || !/[0-9]/.test(parola))

  function yerelDogrula(): string | null {
    if (otomatik) return null
    if (!SONEK_RE.test(dbSonek)) return t('DomainDatabasesPage:validation_db_suffix')
    if ((onek + dbSonek).length > 64) return t('DomainDatabasesPage:validation_db_long')
    if (kullaniciTipi === 'yeni') {
      if (!SONEK_RE.test(kullaniciSonek)) return t('DomainDatabasesPage:validation_user_suffix')
      if ((onek + kullaniciSonek).length > 64) return t('DomainDatabasesPage:validation_user_long')
      if (parola !== '' && parolaGucSorunu) return t('DomainDatabasesPage:validation_password')
    } else {
      if (!mevcutKullanici) return t('DomainDatabasesPage:validation_select_user')
    }
    return null
  }

  async function olustur() {
    const y = yerelDogrula()
    if (y) { setHata(y); return }
    setIsleniyor(true); setHata(null)
    try {
      const body: Record<string, unknown> = otomatik
        ? { otomatik: true }
        : {
            db_sonek: dbSonek,
            kullanici_tipi: kullaniciTipi,
            ...(kullaniciTipi === 'yeni'
              ? { kullanici_sonek: kullaniciSonek, parola }
              : { mevcut_kullanici: mevcutKullanici }),
          }
      const { data } = await api.post(`/domains/${domainId}/databases`, body)
      setSonuc({ db_adi: data.db_adi, db_kullanici: data.db_kullanici, db_parola: data.db_parola })
    } catch (e) {
      setHata(apiHata(e, t('DomainDatabasesPage:create_failed')))
    } finally {
      setIsleniyor(false)
    }
  }

  const inputCls = 'ta-input ta-input-sm w-full font-mono'

  return (
    <Modal acik={true} baslik={t('DomainDatabasesPage:new_db_modal_title')} onKapat={sonuc ? onTamam : onKapat} genislik="lg">
      {sonuc ? (
        <div className="space-y-4">
          <div className="bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-800 rounded-md p-4 space-y-3">
            <p className="text-sm text-emerald-800 dark:text-emerald-200 font-medium">{t('DomainDatabasesPage:created_title')}</p>
            <p className="text-xs text-emerald-700 dark:text-emerald-300">{t('DomainDatabasesPage:save_info')}</p>
            <SonucSatir e={t('DomainDatabasesPage:suffix_db')} v={sonuc.db_adi} t={t} />
            <SonucSatir e={t('DomainDatabasesPage:suffix_user')} v={sonuc.db_kullanici} t={t} />
            <SonucSatir e={t('DomainDatabasesPage:suffix_password')} v={sonuc.db_parola} t={t} />
          </div>
          <div className="flex justify-end">
            <button onClick={onTamam} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 text-sm rounded-md">{t('DomainDatabasesPage:ok')}</button>
          </div>
        </div>
      ) : (
        <div className="space-y-5">
          <label className="flex items-center gap-3 cursor-pointer select-none">
            <input type="checkbox" checked={otomatik} onChange={e => setOtomatik(e.target.checked)} className="h-4 w-4 accent-brand-600" />
            <span className="text-sm text-slate-700 dark:text-slate-300">
              <strong className="font-medium">{t('DomainDatabasesPage:auto_label')}</strong> {t('DomainDatabasesPage:auto_desc')}
            </span>
          </label>

          {!otomatik && (
            <div className="space-y-5 pt-1">
              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabasesPage:db_name_label')}</label>
                <div className="flex items-stretch">
                  <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 dark:border-slate-600 bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 text-sm font-mono select-none">{onek}</span>
                  <input value={dbSonek} onChange={e => setDbSonek(e.target.value.toLowerCase())} placeholder="blog" className={inputCls + ' rounded-l-none'} />
                </div>
                <p className="mt-1 text-xs text-slate-400 dark:text-slate-500 font-mono">→ {dbAdiOnizleme}</p>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1.5">{t('DomainDatabasesPage:db_user_label')}</label>
                <div className="flex gap-4 mb-2">
                  <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300 cursor-pointer">
                    <input type="radio" name="kullaniciTipi" checked={kullaniciTipi === 'yeni'} onChange={() => setKullaniciTipi('yeni')} className="accent-brand-600" />
                    {t('DomainDatabasesPage:new_user_radio')}
                  </label>
                  <label className={'flex items-center gap-1.5 text-sm cursor-pointer ' + (mevcutKullanicilar.length ? 'text-slate-700 dark:text-slate-300' : 'text-slate-400 dark:text-slate-600 cursor-not-allowed')}>
                    <input type="radio" name="kullaniciTipi" disabled={!mevcutKullanicilar.length} checked={kullaniciTipi === 'mevcut'} onChange={() => setKullaniciTipi('mevcut')} className="accent-brand-600" />
                    {t('DomainDatabasesPage:existing_user_radio')}
                  </label>
                </div>

                {kullaniciTipi === 'yeni' ? (
                  <>
                    <div className="flex items-stretch">
                      <span className="inline-flex items-center px-3 rounded-l-md border border-r-0 border-slate-300 dark:border-slate-600 bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 text-sm font-mono select-none">{onek}</span>
                      <input value={kullaniciSonek} onChange={e => setKullaniciSonek(e.target.value.toLowerCase())} placeholder="bloguser" className={inputCls + ' rounded-l-none'} />
                    </div>
                    <p className="mt-1 text-xs text-slate-400 dark:text-slate-500 font-mono">→ {kullaniciOnizleme}</p>
                  </>
                ) : (
                  <select value={mevcutKullanici} onChange={e => setMevcutKullanici(e.target.value)} className={inputCls}>
                    {mevcutKullanicilar.map(u => <option key={u} value={u}>{u}</option>)}
                  </select>
                )}
              </div>

              {kullaniciTipi === 'yeni' && (
                <div>
                  <label className="block text-xs font-medium text-slate-600 dark:text-slate-400 mb-1">{t('DomainDatabasesPage:password_label')} <span className="text-slate-400 dark:text-slate-500">{t('DomainDatabasesPage:password_optional')}</span></label>
                  <div className="flex gap-2">
                    <input type="text" value={parola} onChange={e => setParola(e.target.value)} placeholder={t('DomainDatabasesPage:password_placeholder')} className={inputCls} />
                    <button type="button" onClick={() => setParola(uretGucluParola())} className="whitespace-nowrap px-3 py-2 bg-white dark:bg-slate-800 border border-brand-600 text-brand-700 dark:text-brand-300 hover:bg-brand-50 dark:hover:bg-brand-900/30 text-sm rounded-md">{t('DomainDatabasesPage:generate')}</button>
                  </div>
                  {parolaGucSorunu && <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">{t('DomainDatabasesPage:password_warning')}</p>}
                </div>
              )}
            </div>
          )}

          {hata && <div className="px-3 py-2 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-700 dark:text-red-300">{hata}</div>}

          <div className="flex justify-end gap-2 pt-1">
            <button onClick={onKapat} disabled={isleniyor} className="px-4 py-2 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 rounded-md text-sm">{t('DomainDatabasesPage:cancel')}</button>
            <button onClick={olustur} disabled={isleniyor} className="px-4 py-2 bg-slate-900 hover:bg-slate-800 dark:bg-white dark:hover:bg-slate-100 text-white dark:text-slate-900 disabled:opacity-60 text-sm font-medium rounded-md">{isleniyor ? t('DomainDatabasesPage:creating') : t('DomainDatabasesPage:create')}</button>
          </div>
        </div>
      )}
    </Modal>
  )
}

function SonucSatir({ e, v, t }: { e: string; v: string; t: (k: string, opts?: Record<string, unknown>) => string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="w-24 shrink-0 text-xs text-emerald-700 dark:text-emerald-300">{e}</span>
      <code className="flex-1 bg-white dark:bg-slate-800 px-3 py-1.5 font-mono text-sm text-slate-900 dark:text-slate-100 rounded border border-emerald-200 dark:border-emerald-800 break-all">{v}</code>
      <KopyalaButton metin={v} className="px-2.5 py-1.5 bg-emerald-100 dark:bg-emerald-900/30 hover:bg-emerald-200 text-emerald-800 dark:text-emerald-200 text-xs rounded" />
    </div>
  )
}
