import { create } from 'zustand'

export type Kullanici = {
  id: number
  adi: string
  rol: 'admin' | 'reseller' | 'user'
  ad_soyad?: string
}

// Oturum JWT'si ARTIK BURADA TUTULMUYOR.
//
// Eskiden token localStorage'da (`sanal.token`) duruyor ve her istekte
// Authorization başlığına yazılıyordu. Panelde çalışan herhangi bir XSS o
// değeri okuyup dışarı sızdırabilir, çalınan token da ömrü dolana kadar başka
// bir makineden geçerli olurdu. Artık sunucu oturumu HttpOnly çerezle
// gönderiyor (bkz. internal/auth/cookie.go): JavaScript onu okuyamaz, tarayıcı
// same-site isteklere kendiliğinden ekler.
//
// Burada kalanlar yalnız ARAYÜZ durumudur — kullanıcı adı/rolü ve bitiş anı.
// Hiçbiri sır değildir ve hiçbiri yetki kararında kullanılmaz; yetkiyi her
// istekte sunucu çerezden çözer. `oturumVar` da bir yetki bayrağı değil,
// yalnız "giriş ekranını mı yoksa paneli mi çizeyim" sorusunun yanıtıdır.
type AuthState = {
  oturumVar: boolean
  kullanici: Kullanici | null
  bitis: number | null
  giris: (kullanici: Kullanici, bitis: number) => void
  girisMusteri: (bitis: number, domainID: number, alanAdi: string, kullaniciAdi: string) => void
  guncelleAd: (adSoyad: string) => void
  cikis: () => void
  hidrate: () => void
}

const KEY_USER = 'sanal.user'
const KEY_EXP  = 'sanal.exp'

// Çerez öncesi sürümden kalan token anahtarı. Yükseltmeden sonra tarayıcıda
// öylece durmasın diye her açılışta siliniyor — artık hiçbir işe yaramıyor ama
// hâlâ geçerli bir JWT içerebilir.
const KEY_ESKI_TOKEN = 'sanal.token'

const KEY_MUSTERI      = 'sanalcp.musteri'
const KEY_MUSTERI_DOM  = 'sanalcp.musteri.domain_id'
const KEY_MUSTERI_ALAN = 'sanalcp.musteri.alan_adi'

function musteriBayrakSil() {
  localStorage.removeItem(KEY_MUSTERI)
  localStorage.removeItem(KEY_MUSTERI_DOM)
  localStorage.removeItem(KEY_MUSTERI_ALAN)
}

function yereliTemizle() {
  localStorage.removeItem(KEY_USER)
  localStorage.removeItem(KEY_EXP)
  musteriBayrakSil()
}

function ilkDurum() {
  if (typeof window === 'undefined') {
    return { oturumVar: false, kullanici: null as Kullanici | null, bitis: null as number | null }
  }
  try { localStorage.removeItem(KEY_ESKI_TOKEN) } catch { /* yoksay */ }

  const u = localStorage.getItem(KEY_USER)
  const e = localStorage.getItem(KEY_EXP)
  if (!u || !e) {
    yereliTemizle()
    return { oturumVar: false, kullanici: null, bitis: null }
  }
  const exp = Number(e)
  if (!Number.isFinite(exp) || exp * 1000 < Date.now()) {
    yereliTemizle()
    return { oturumVar: false, kullanici: null, bitis: null }
  }
  try {
    return { oturumVar: true, kullanici: JSON.parse(u) as Kullanici, bitis: exp }
  } catch {
    yereliTemizle()
    return { oturumVar: false, kullanici: null, bitis: null }
  }
}

// sunucudaCikis: oturum çerezini sunucuya sildirir.
//
// axios yerine düz fetch kullanılıyor: cikis() api.ts'teki 401 interceptor'ının
// içinden de çağrılıyor ve axios üzerinden gitmek o interceptor'ı yeniden
// tetikleyip döngü kurabilirdi. `credentials: 'include'` şart — silinecek
// çerezin isteğe eklenmesi gerekiyor.
function sunucudaCikis() {
  try {
    void fetch('/api/v1/auth/cikis', { method: 'POST', credentials: 'include' })
      .catch(() => { /* ağ hatası çıkışı engellemesin */ })
  } catch { /* yoksay */ }
}

export const useAuth = create<AuthState>((set) => ({
  ...ilkDurum(),
  giris: (kullanici, bitis) => {
    localStorage.setItem(KEY_USER, JSON.stringify(kullanici))
    localStorage.setItem(KEY_EXP, String(bitis))
    musteriBayrakSil()
    set({ oturumVar: true, kullanici, bitis })
  },
  girisMusteri: (bitis, domainID, alanAdi, kullaniciAdi) => {
    const synth: Kullanici = { id: 0, adi: kullaniciAdi, rol: 'user', ad_soyad: alanAdi }
    localStorage.setItem(KEY_USER, JSON.stringify(synth))
    localStorage.setItem(KEY_EXP, String(bitis))
    localStorage.setItem(KEY_MUSTERI, '1')
    localStorage.setItem(KEY_MUSTERI_DOM, String(domainID))
    localStorage.setItem(KEY_MUSTERI_ALAN, alanAdi)
    set({ oturumVar: true, kullanici: synth, bitis })
  },
  guncelleAd: (adSoyad) => set((s) => {
    if (!s.kullanici) return s
    const k = { ...s.kullanici, ad_soyad: adSoyad }
    try { localStorage.setItem(KEY_USER, JSON.stringify(k)) } catch { /* yoksay */ }
    return { kullanici: k }
  }),
  cikis: () => {
    // Önce yerel durum temizlenir: sunucu çağrısı başarısız olsa bile kullanıcı
    // arayüzde çıkmış olmalı.
    yereliTemizle()
    set({ oturumVar: false, kullanici: null, bitis: null })
    sunucudaCikis()
  },
  hidrate: () => {
    /* ilkDurum() ilk render'da yapıyor */
  },
}))
