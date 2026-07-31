// Panel i18n — namespace başına bir dosya (src/i18n/locales/{tr,en}/<Bileşen>.json).
// Yeni bir namespace eklemek için sadece iki JSON dosyası oluşturmak yeterli;
// aşağıdaki import.meta.glob otomatik toplar, elle kayıt gerekmez.

import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

export type Lang = 'tr' | 'en'

const trModules = import.meta.glob('./locales/tr/*.json', { eager: true }) as Record<string, { default: Record<string, unknown> }>
const enModules = import.meta.glob('./locales/en/*.json', { eager: true }) as Record<string, { default: Record<string, unknown> }>

function buildResources(modules: Record<string, { default: Record<string, unknown> }>) {
  const out: Record<string, Record<string, unknown>> = {}
  for (const path in modules) {
    const match = path.match(/([^/]+)\.json$/)
    if (!match) continue
    out[match[1]] = modules[path].default
  }
  return out
}

const trResources = buildResources(trModules)
const enResources = buildResources(enModules)
const namespaces = Array.from(new Set([...Object.keys(trResources), ...Object.keys(enResources)]))

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

i18n.use(initReactI18next).init({
  resources: { tr: trResources, en: enResources },
  lng: getLang(),
  fallbackLng: 'tr',
  defaultNS: 'common',
  ns: namespaces,
  interpolation: { escapeValue: false },
  returnEmptyString: false,
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
