import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'

// /araclar-ayarlar sayfasının başlık satırındaki kırmızı reboot butonu — gerçek bir
// `systemctl reboot` tetikler (bkz. internal/system/reboot.go). Domain silmedeki gibi
// yazarak-onay değil, SunucuOptimize.tsx'teki gibi hafif "Emin misiniz?" iki adımlı onay:
// bu geri dönüşsüz veri kaybı değil, ~1 dakikalık güvenli bir kesinti.

export default function SunucuYenidenBaslat() {
  const { t } = useTranslation(['SunucuYenidenBaslat', 'common'])
  const [onay, setOnay] = useState(false)
  const [baslatiliyor, setBaslatiliyor] = useState(false)
  const [baslatildi, setBaslatildi] = useState(false)
  const [hata, setHata] = useState<string | null>(null)

  async function baslat() {
    setHata(null); setBaslatiliyor(true)
    try {
      await api.post('/system/reboot')
      setBaslatildi(true); setOnay(false)
    } catch (e: any) {
      setHata(e?.response?.data?.hata || e?.message || t('SunucuYenidenBaslat:start_failed'))
    } finally { setBaslatiliyor(false) }
  }

  if (baslatildi) {
    return (
      <div className="text-xs px-3 py-1.5 rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 font-medium">
        {t('SunucuYenidenBaslat:rebooting')}
      </div>
    )
  }

  if (onay) {
    return (
      <div className="flex items-center gap-2">
        <span className="text-xs text-slate-600 dark:text-slate-300">{t('SunucuYenidenBaslat:confirm')}</span>
        <button onClick={baslat} disabled={baslatiliyor}
          className="text-xs px-3 py-1.5 rounded-lg bg-red-600 text-white hover:bg-red-700 transition font-medium disabled:opacity-40">
          {baslatiliyor ? t('SunucuYenidenBaslat:starting') : t('SunucuYenidenBaslat:yes_reboot')}
        </button>
        <button onClick={() => setOnay(false)} disabled={baslatiliyor}
          className="text-xs px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition">
          {t('common:giveUp')}
        </button>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      {hata && <span className="text-xs text-red-600 dark:text-red-400">{hata}</span>}
      <button onClick={() => setOnay(true)}
        className="text-xs px-3 py-1.5 rounded-lg bg-red-600 text-white hover:bg-red-700 transition font-medium flex items-center gap-1.5">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3.5 w-3.5">
          <path strokeLinecap="round" strokeLinejoin="round" d="M5.636 5.636a9 9 0 1 0 12.728 0M12 3v8" />
        </svg>
        {t('SunucuYenidenBaslat:reboot_button')}
      </button>
    </div>
  )
}
