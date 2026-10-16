import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

// Node 25+ kendi deneysel localStorage'ını tanımlıyor; --localstorage-file
// verilmezse undefined döner ve jsdom'unkini gölgeler. Store modül yüklenirken
// localStorage okuduğu için bellek içi bir taklit, import'tan ÖNCE kurulmalı.
const depo = new Map<string, string>()
vi.stubGlobal('localStorage', {
  getItem: (k: string) => depo.get(k) ?? null,
  setItem: (k: string, v: string) => { depo.set(k, String(v)) },
  removeItem: (k: string) => { depo.delete(k) },
  clear: () => { depo.clear() },
})
vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(null))))

let auth: typeof import('./auth')

beforeAll(async () => {
  auth = await import('./auth')
})

describe('girisYolu', () => {
  beforeEach(() => {
    depo.clear()
  })

  it('hiç giriş yapılmamışsa yönetici giriş ekranı', () => {
    expect(auth.girisYolu()).toBe('/giris')
  })

  it('müşteri çıkış yapınca /cp ekranına döner', () => {
    const s = auth.useAuth.getState()
    s.girisMusteri(Date.now() / 1000 + 3600, 7, 'ornek.com', 'c_ornek_com')
    s.cikis()
    expect(auth.useAuth.getState().oturumVar).toBe(false)
    expect(auth.girisYolu()).toBe('/cp')
  })

  it('aynı tarayıcıda yönetici girişi bayrağı temizler', () => {
    const s = auth.useAuth.getState()
    s.girisMusteri(Date.now() / 1000 + 3600, 7, 'ornek.com', 'c_ornek_com')
    s.cikis()
    s.giris({ id: 1, adi: 'root', rol: 'admin' }, Date.now() / 1000 + 3600)
    s.cikis()
    expect(auth.girisYolu()).toBe('/giris')
  })
})
