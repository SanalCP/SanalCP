import { modalOnay } from '@/lib/dialog'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, apiHata } from '@/lib/api'

// Ortak nameserver çifti — müşterilerin domainlerini yönlendireceği adresler.
// Gerçek hosting sağlayıcılarının "domaininizi ns1./ns2.saglayici.com adreslerine
// yönlendirin" yönergesinin panel karşılığı (bkz. internal/dns/nameserver.go).
//
// Sunucu geneli bir ayar olduğu için Araçlar & Ayarlar sayfasında durur;
// bayilerin kendi white-label çifti Profil sayfasındadır.

type Durum = {
  ns1: string
  ns2: string
  kaynak?: string
  uyari?: string
  oneri1?: string
  oneri2?: string
}

export default function NameserverAyari() {
  const { t } = useTranslation(['NameserverAyari'])
  const [durum, setDurum] = useState<Durum | null>(null)
  const [ns1, setNS1] = useState('')
  const [ns2, setNS2] = useState('')
  const [kaydediliyor, setKaydediliyor] = useState(false)
  const [tasiniyor, setTasiniyor] = useState(false)
  const [hata, setHata] = useState<string | null>(null)
  const [basari, setBasari] = useState<string | null>(null)

  const yukle = useCallback(async () => {
    try {
      const r = await api.get<Durum>('/nameserver')
      setDurum(r.data)
      // Ayarlı değilse öneri ALANA yazılır ama KAYDEDİLMEZ — yanlış nameserver
      // yayınlamak müşteri domainlerini sessizce çözülemez hâle getirir.
      setNS1(r.data.ns1 || r.data.oneri1 || '')
      setNS2(r.data.ns2 || r.data.oneri2 || '')
    } catch { /* yetkisiz/geçici — bileşen sessiz kalır */ }
  }, [])

  useEffect(() => { yukle() }, [yukle])

  async function kaydet() {
    setKaydediliyor(true); setHata(null); setBasari(null)
    try {
      await api.put('/nameserver', { ns1: ns1.trim(), ns2: ns2.trim() })
      setBasari(t('NameserverAyari:saved'))
      yukle()
    } catch (e) { setHata(apiHata(e)) } finally { setKaydediliyor(false) }
  }

  async function tasi() {
    if (!(await modalOnay(t('NameserverAyari:migrate_confirm')))) return
    setTasiniyor(true); setHata(null); setBasari(null)
    try {
      const { data } = await api.post('/dns/nameserver-tasi', {})
      setBasari(t('NameserverAyari:migrated', { g: data.guncellenen, n: data.toplam }))
      if (data.hatalar?.length) setHata(data.hatalar.join(' | '))
    } catch (e) { setHata(apiHata(e)) } finally { setTasiniyor(false) }
  }

  const girdi = 'flex-1 max-w-xs px-3 py-1.5 border border-slate-300 dark:border-slate-600 rounded-lg text-xs font-mono bg-white dark:bg-slate-900 text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-emerald-500'

  return (
    <div className="h-full p-4 border rounded-2xl bg-emerald-50 dark:bg-emerald-900/15 border-emerald-200 dark:border-emerald-800/50">
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center text-xl flex-shrink-0 bg-emerald-100 dark:bg-emerald-900/40">🌐</div>
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-2 flex-wrap">
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('NameserverAyari:title')}</span>
            {durum?.kaynak === 'yok' && (
              <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300">
                {t('NameserverAyari:badge_unset')}
              </span>
            )}
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-500 mt-0.5">{t('NameserverAyari:desc')}</div>

          {durum?.uyari && (
            <div className="mt-2 px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 text-xs">{durum.uyari}</div>
          )}
          {durum?.kaynak === 'yok' && durum?.oneri1 && (
            <div className="mt-2 text-xs text-slate-500 dark:text-slate-400">{t('NameserverAyari:suggested')}</div>
          )}
          {hata && <div className="mt-2 px-3 py-2 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-xs">{hata}</div>}
          {basari && <div className="mt-2 px-3 py-2 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-xs">{basari}</div>}

          <div className="mt-3 flex items-center gap-2 flex-wrap">
            <input value={ns1} onChange={e => setNS1(e.target.value)} placeholder="ns1.ornek.com"
              autoComplete="off" spellCheck={false} className={girdi} />
            <input value={ns2} onChange={e => setNS2(e.target.value)} placeholder="ns2.ornek.com"
              autoComplete="off" spellCheck={false} className={girdi} />
            <button onClick={kaydet} disabled={kaydediliyor || !ns1.trim() || !ns2.trim()}
              className="text-xs px-3 py-1.5 rounded-lg bg-emerald-600 text-white hover:bg-emerald-700 transition font-medium disabled:opacity-40 disabled:cursor-not-allowed">
              {kaydediliyor ? t('NameserverAyari:saving') : t('NameserverAyari:save_button')}
            </button>
            <button onClick={tasi} disabled={tasiniyor || durum?.kaynak === 'yok'}
              title={durum?.kaynak === 'yok' ? t('NameserverAyari:migrate_needs_ns') : ''}
              className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition disabled:opacity-40">
              {tasiniyor ? t('NameserverAyari:migrating') : t('NameserverAyari:migrate_button')}
            </button>
          </div>

          <div className="mt-2 text-[11px] text-slate-500 dark:text-slate-500">{t('NameserverAyari:hint_a_record')}</div>
        </div>
      </div>
    </div>
  )
}
