import { describe, expect, it } from 'vitest'
import { formatBayt, kotaDurumu, kotaGirdisiniDogrula, KOTA_MAKS_MB } from './mailQuota'

describe('kotaGirdisiniDogrula', () => {
  it('müşteri planın üstüne çıkamaz ve limitli planda sınırsız (0) seçemez', () => {
    expect(kotaGirdisiniDogrula('512', 1024, false)).toEqual({ ok: true, mb: 512 })
    expect(kotaGirdisiniDogrula('1024', 1024, false)).toEqual({ ok: true, mb: 1024 })
    expect(kotaGirdisiniDogrula('1025', 1024, false)).toEqual({ ok: false, neden: 'plan_ustu', azami: 1024 })
    expect(kotaGirdisiniDogrula('0', 1024, false)).toEqual({ ok: false, neden: 'plan_ustu', azami: 1024 })
  })
  it('sınırsız planda müşteri serbest, yönetici/bayi her planda serbest', () => {
    expect(kotaGirdisiniDogrula('0', 0, false)).toEqual({ ok: true, mb: 0 })
    expect(kotaGirdisiniDogrula('99999', 0, false)).toEqual({ ok: true, mb: 99999 })
    expect(kotaGirdisiniDogrula('5000', 1024, true)).toEqual({ ok: true, mb: 5000 })
    expect(kotaGirdisiniDogrula('0', 1024, true)).toEqual({ ok: true, mb: 0 })
  })
  it('geçersiz girdileri ve mutlak sınırı reddeder', () => {
    for (const k of ['', ' ', '-1', '1.5', '1e3', 'abc', String(KOTA_MAKS_MB + 1), '99999999999999999999']) {
      expect(kotaGirdisiniDogrula(k, 0, true), k).toEqual({ ok: false, neden: 'gecersiz' })
    }
    expect(kotaGirdisiniDogrula(String(KOTA_MAKS_MB), 0, true)).toEqual({ ok: true, mb: KOTA_MAKS_MB })
  })
})

describe('kotaDurumu', () => {
  const MB = 1024 * 1024
  it('sınırsız, bilinmiyor, normal, yaklaştı ve aşıldı durumlarını ayırır', () => {
    expect(kotaDurumu(5 * MB, 0)).toEqual({ durum: 'sinirsiz', yuzde: 0 })
    expect(kotaDurumu(null, 100 * MB)).toEqual({ durum: 'bilinmiyor', yuzde: 0 })
    expect(kotaDurumu(50 * MB, 100 * MB)).toEqual({ durum: 'normal', yuzde: 50 })
    expect(kotaDurumu(95 * MB, 100 * MB)).toEqual({ durum: 'yaklasti', yuzde: 95 })
    expect(kotaDurumu(100 * MB, 100 * MB)).toEqual({ durum: 'asildi', yuzde: 100 })
    // Limit aşıldığında çubuk taşmaz.
    expect(kotaDurumu(250 * MB, 100 * MB)).toEqual({ durum: 'asildi', yuzde: 100 })
  })
})

describe('formatBayt', () => {
  it('okunur birimler üretir', () => {
    expect(formatBayt(0)).toBe('0 B')
    expect(formatBayt(512)).toBe('512 B')
    expect(formatBayt(1024)).toBe('1 KB')
    expect(formatBayt(1536 * 1024)).toBe('1.5 MB')
    expect(formatBayt(1024 ** 3)).toBe('1 GB')
  })
})
