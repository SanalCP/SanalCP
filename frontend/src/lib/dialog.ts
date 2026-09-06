export type DialogIstek = {
  id: number
  tip: 'uyari' | 'onay' | 'girdi' | 'kopyala'
  mesaj: string
  deger: string
  gizli: boolean
}

type Sonuc = string | boolean | null
type Bekleyen = DialogIstek & { tamamla: (sonuc: Sonuc) => void }
let sira = 0
let kuyruk: Bekleyen[] = []
const dinleyiciler = new Set<() => void>()

function bildir() {
  dinleyiciler.forEach(dinle => dinle())
}

export function dialogDinle(dinle: () => void) {
  dinleyiciler.add(dinle)
  return () => { dinleyiciler.delete(dinle) }
}

export function aktifDialog(): DialogIstek | null {
  return kuyruk[0] ?? null
}

function ekle(tip: DialogIstek['tip'], mesaj: string, deger = '', gizli = false) {
  return new Promise<Sonuc>(tamamla => {
    kuyruk.push({ id: ++sira, tip, mesaj, deger, gizli, tamamla })
    bildir()
  })
}

export function dialogBitir(id: number, sonuc: Sonuc) {
  // Çift tıklama veya eski bir pencerenin olayı sıradaki isteği onaylamasın.
  if (kuyruk[0]?.id !== id) return
  const istek = kuyruk.shift()!
  bildir()
  istek.tamamla(sonuc)
}

export function dialoglariIptalEt() {
  const bekleyenler = kuyruk
  kuyruk = []
  bildir()
  bekleyenler.forEach(istek => istek.tamamla(null))
}

export async function modalUyari(mesaj: string): Promise<void> {
  await ekle('uyari', mesaj)
}

export async function modalOnay(mesaj: string): Promise<boolean> {
  return (await ekle('onay', mesaj)) === true
}

export async function modalGirdi(mesaj: string, deger = '', secenekler: { gizli?: boolean } = {}): Promise<string | null> {
  const sonuc = await ekle('girdi', mesaj, deger, secenekler.gizli)
  return typeof sonuc === 'string' ? sonuc : null
}

export async function modalKopyala(mesaj: string, metin: string): Promise<void> {
  await ekle('kopyala', mesaj, metin)
}
