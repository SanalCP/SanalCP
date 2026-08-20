import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'

// /araclar-ayarlar başlık satırındaki kırmızı kapatma butonu — gerçek bir
// `systemctl poweroff` tetikler (bkz. internal/system/reboot.go → Kapat).
// Reboot'tan FARKI: sunucu kendi kendine geri gelmez, ancak sağlayıcı panelinden
// veya fiziksel olarak açılabilir. Bu yüzden yanındaki turuncu reboot butonundan
// daha ağır bir onay kullanır: "Emin misiniz?" yerine KAPAT yazma zorunluluğu.
export default function SunucuKapat() {
  const { t } = useTranslation(['SunucuKapat', 'common'])
  const [onay, setOnay] = useState(false)
  const [metin, setMetin] = useState('')
  const [kapatiliyor, setKapatiliyor] = useState(false)
  const [kapatildi, setKapatildi] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  const anahtar = t('SunucuKapat:confirm_word')
  const eslesti = metin.trim().toUpperCase() === anahtar.toUpperCase()

  async function kapat() {
    if (!eslesti) return
    setHata(null); setKapatiliyor(true)
    try {
      await api.post('/system/kapat')
      setKapatildi(true); setOnay(false)
    } catch (e: any) {
      setHata(e?.response?.data?.hata || e?.message || t('SunucuKapat:start_failed'))
    } finally { setKapatiliyor(false) }
  }

  function vazgec() { setOnay(false); setMetin('') }

  if (kapatildi) {
    return (
      <div className="rounded-lg bg-red-50 px-3 py-1.5 text-xs font-medium text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {t('SunucuKapat:shutting_down')}
      </div>
    )
  }

  if (onay) {
    return (
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-slate-600 dark:text-slate-300">
          {t('SunucuKapat:confirm_hint', { word: anahtar })}
        </span>
        <input
          value={metin}
          onChange={e => setMetin(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') kapat() }}
          placeholder={anahtar}
          aria-label={t('SunucuKapat:confirm_hint', { word: anahtar })}
          autoFocus
          className="w-24 rounded-lg border border-slate-300 bg-white px-2 py-1.5 font-mono text-xs uppercase text-slate-800 focus:outline-none focus:ring-2 focus:ring-red-500 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
        />
        <button onClick={kapat} disabled={kapatiliyor || !eslesti}
          className="rounded-lg bg-red-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-red-700 disabled:opacity-40">
          {kapatiliyor ? t('SunucuKapat:starting') : t('SunucuKapat:yes_shutdown')}
        </button>
        <button onClick={vazgec} disabled={kapatiliyor}
          className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs text-slate-600 transition hover:bg-slate-100 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800">
          {t('common:giveUp')}
        </button>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      {hata && <span className="text-xs text-red-600 dark:text-red-400">{hata}</span>}
      <button onClick={() => setOnay(true)}
        className="flex items-center gap-1.5 rounded-lg bg-red-600 px-3 py-1.5 text-xs font-medium text-white transition hover:bg-red-700">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3.5 w-3.5">
          <path strokeLinecap="round" strokeLinejoin="round" d="M18.36 6.64a9 9 0 1 1-12.73 0M12 2v10" />
        </svg>
        {t('SunucuKapat:shutdown_button')}
      </button>
    </div>
  )
}
