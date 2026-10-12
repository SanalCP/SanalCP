import { describe, expect, it } from 'vitest'
import { parolaGucluMu, uretGucluParola } from './parola'

describe('uretGucluParola', () => {
  // Eski üreteç rakam garantisi vermiyordu: ~%4,7 olasılıkla kendi
  // ürettiği parola parolaGucluMu'ya (dolayısıyla backend'e) takılıyordu.
  it('ürettiği parola her zaman politikaya uyar', () => {
    for (const n of [12, 20]) {
      for (let i = 0; i < 2000; i++) {
        const p = uretGucluParola(n)
        expect(p).toHaveLength(n)
        expect(parolaGucluMu(p), p).toBe(true)
      }
    }
  })
})
