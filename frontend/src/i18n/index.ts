// Panel i18n — namespace başına bir dosya (src/i18n/locales/{tr,en}/<Bileşen>.json).
// Yeni bir namespace eklemek için sadece iki JSON dosyası oluşturmak yeterli;
// aşağıdaki import.meta.glob otomatik toplar, elle kayıt gerekmez.

import i18n, { type BackendModule } from 'i18next'
import { initReactI18next } from 'react-i18next'
import { chunkYenilemeDene, chunkYuklemeHatasiMi } from '@/lib/chunk'

export type Lang = 'tr' | 'en'

type LocaleModule = { default: Record<string, unknown> }

// Her JSON dosyası ayrı bir dinamik parça olur. Önceki eager glob iki dildeki
// 168 namespace'i ana JavaScript paketine gömüyordu; artık yalnız aktif dil ve
// ekranda kullanılan namespace indirilir.
const localeModules = import.meta.glob<LocaleModule>('./locales/*/*.json')

const localeBackend: BackendModule = {
  type: 'backend',
  init() { /* ek ayar gerekmiyor */ },
  read(language, namespace, callback) {
    const lang = language.split('-')[0]
    const load = localeModules[`./locales/${lang}/${namespace}.json`]
    if (!load) {
      callback(new Error(`Çeviri bulunamadı: ${lang}/${namespace}`), false)
      return
    }
    load()
      .then(module => callback(null, module.default))
      .catch(error => {
        // Çeviri dosyaları da ayrı birer dinamik parça. Yeni sürüm yayınlandıktan
        // sonra açık kalan sekmede bunlar da 404 alır; sayfayı yenilemek çözer.
        if (chunkYuklemeHatasiMi(error) && chunkYenilemeDene()) return
        callback(error instanceof Error ? error : new Error(String(error)), false)
      })
  },
}

const KEY = 'sanalcp.lang'

export function getLang(): Lang {
  if (typeof window === 'undefined') return 'tr'
  return localStorage.getItem(KEY) === 'en' ? 'en' : 'tr'
}

function hasStoredLang(): boolean {
  if (typeof window === 'undefined') return true
  try { return localStorage.getItem(KEY) !== null } catch { return true }
}

export function setLang(lang: Lang) {
  try { localStorage.setItem(KEY, lang) } catch { /* yoksay */ }
  i18n.changeLanguage(lang)
  document.documentElement.lang = lang
  window.dispatchEvent(new CustomEvent('sanalcp:lang-change', { detail: lang }))
}

export const i18nReady = i18n.use(localeBackend).use(initReactI18next).init({
  lng: getLang(),
  fallbackLng: 'tr',
  supportedLngs: ['tr', 'en'],
  load: 'languageOnly',
  defaultNS: 'common',
  ns: ['common'],
  interpolation: { escapeValue: false },
  returnEmptyString: false,
  react: { useSuspense: true },
})

// Boot-time: main.tsx bootTheme ile aynı noktada çağırır (FOUC engelleme dahil değil, sadece <html lang>).
export function bootLang() {
  document.documentElement.lang = getLang()
}

// Kullanıcı henüz HİÇ dil seçmemişse (localStorage boş — ilk ziyaret / login ekranı)
// kurulumda seçilen sunucu-varsayılan dilini bir kerelik uygular. Daha önce dil
// değiştirmiş ya da giriş yapmış kullanıcıyı asla geçersiz kılmaz (fetch, lib/api'deki
// axios instance'ı değil ham fetch kullanır — @/lib/api bu dosyayı import ediyor,
// tersi döngüsel import olurdu).
export async function applyServerDefaultLang() {
  if (hasStoredLang()) return
  try {
    const base = (import.meta.env.VITE_API_BASE as string) || '/api/v1'
    const r = await fetch(`${base}/public/dil`)
    if (!r.ok) return
    const data = await r.json()
    if (data?.dil === 'en' || data?.dil === 'tr') setLang(data.dil)
  } catch { /* ağ hatası — TR varsayılanında kal */ }
}

export default i18n
