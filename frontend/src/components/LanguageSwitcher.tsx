import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getLang, setLang, type Lang } from '@/i18n'
import { useAuth } from '@/store/auth'
import { api } from '@/lib/api'

const LABELS: Record<Lang, string> = { tr: 'TR', en: 'EN' }
const NEXT: Record<Lang, Lang> = { tr: 'en', en: 'tr' }

// Sağ üst çubuktaki dil değiştirici. Anasayfa/Profil dışında da (ör. giriş
// ekranı) kullanılabilsin diye kimlik doğrulaması olmadan da çalışır —
// oturum varsa tercihi sunucuya da (best-effort) yazar.
export default function LanguageSwitcher({ className }: { className?: string }) {
  const { i18n } = useTranslation()
  const token = useAuth((s) => s.token)
  const [lang, setLangState] = useState<Lang>(getLang())

  useEffect(() => {
    const h = (e: Event) => setLangState((e as CustomEvent<Lang>).detail)
    window.addEventListener('sanalcp:lang-change', h)
    return () => window.removeEventListener('sanalcp:lang-change', h)
  }, [])

  function degistir() {
    const yeni = NEXT[lang]
    setLang(yeni)
    if (token) {
      // PUT /me tüm alanları birlikte yazıyor (kısmi güncelleme desteklemiyor) —
      // önce mevcut profili çekip ad_soyad/eposta/tema'yı sıfırlamadan gönderiyoruz.
      api.get<{ ad_soyad: string; eposta: string; tercih_tema: string }>('/me')
        .then((r) => api.put('/me', {
          ad_soyad: r.data.ad_soyad, eposta: r.data.eposta,
          tercih_tema: r.data.tercih_tema, tercih_dil: yeni,
        }))
        .catch(() => { /* best-effort */ })
    }
  }

  return (
    <button
      onClick={degistir}
      title={i18n.t('common:select') + ': ' + (lang === 'tr' ? 'English' : 'Türkçe')}
      className={className ?? 'p-2 min-w-[2.25rem] text-xs font-semibold text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md transition'}
    >
      {LABELS[lang]}
    </button>
  )
}
