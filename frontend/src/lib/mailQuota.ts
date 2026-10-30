// Posta kutusu kotası yardımcıları (DomainMailPage).
//
// Sözleşme backend ile aynı (bkz. internal/mail/quota.go): quota_bytes = 0
// sınırsız; used_bytes = null "bilinmiyor" (Dovecot maildirsize henüz yok).

export const KOTA_MAKS_MB = 1048576 // 1 TiB — backend kota.MaxMailboxQuotaMB ile aynı

const MB = 1024 * 1024

/** Bayt sayısını kısa okunur metne çevirir (1024 tabanlı). */
export function formatBayt(bayt: number): string {
  if (bayt < 1024) return `${bayt} B`
  const birimler = ['KB', 'MB', 'GB', 'TB']
  let v = bayt / 1024
  let i = 0
  while (v >= 1024 && i < birimler.length - 1) { v /= 1024; i++ }
  const metin = v >= 100 || Number.isInteger(v) ? String(Math.round(v)) : v.toFixed(1)
  return `${metin} ${birimler[i]}`
}

export type KotaDurumu = 'sinirsiz' | 'bilinmiyor' | 'normal' | 'yaklasti' | 'asildi'

/**
 * Doluluk yüzdesi (0..100, çubuk genişliği için üst sınırlı) ve durum.
 * Limit varken kullanım ≥ limit ise 'asildi'; ≥ %90 ise 'yaklasti'.
 */
export function kotaDurumu(kullanilan: number | null, limit: number): { durum: KotaDurumu; yuzde: number } {
  if (limit <= 0) return { durum: 'sinirsiz', yuzde: 0 }
  if (kullanilan === null) return { durum: 'bilinmiyor', yuzde: 0 }
  const oran = (kullanilan / limit) * 100
  const yuzde = Math.min(100, Math.round(oran))
  if (kullanilan >= limit) return { durum: 'asildi', yuzde: 100 }
  return { durum: oran >= 90 ? 'yaklasti' : 'normal', yuzde }
}

export type KotaGirdiSonucu =
  | { ok: true; mb: number }
  | { ok: false; neden: 'gecersiz' | 'plan_ustu'; azami?: number }

/**
 * Kullanıcının yazdığı MB değerini doğrular (backend yine de zorlar).
 * planMB > 0 ve rol planı aşamıyorsa: 1..planMB aralığı zorunlu, 0 (sınırsız) yasak.
 */
export function kotaGirdisiniDogrula(girdi: string, planMB: number, planiAsabilir: boolean): KotaGirdiSonucu {
  const s = girdi.trim()
  if (!/^\d+$/.test(s)) return { ok: false, neden: 'gecersiz' }
  const mb = Number(s)
  if (!Number.isSafeInteger(mb) || mb > KOTA_MAKS_MB) return { ok: false, neden: 'gecersiz' }
  if (!planiAsabilir && planMB > 0 && (mb === 0 || mb > planMB)) {
    return { ok: false, neden: 'plan_ustu', azami: planMB }
  }
  return { ok: true, mb }
}

/** Bayt → MB (kota düzenleme penceresinin başlangıç değeri). */
export function baytToMB(bayt: number): number {
  return Math.round(bayt / MB)
}
