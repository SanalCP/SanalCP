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

export default i18n
