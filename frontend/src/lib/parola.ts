// uretGucluParola: tarayıcı tarafı güçlü parola (harf+rakam karışık, min-güç geçer).
export function uretGucluParola(n = 20): string {
  const harf = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const buf = new Uint32Array(n)
  ;(window.crypto || (window as any).msCrypto).getRandomValues(buf)
  let s = ''
  for (let i = 0; i < n; i++) s += harf[buf[i] % harf.length]
  return s
}

// parolaGucluMu: backend hesaplar.ParolaGucluMu ile AYNI politika.
// >=12 karakter + en az bir harf ve bir rakam + tek satır.
// true = geçerli; false = geçersiz.
export function parolaGucluMu(pw: string): boolean {
  // eslint-disable-next-line no-control-regex -- backend ParolaGecerli ile aynı NUL/CR/LF reddi
  if (/[\r\n\x00]/.test(pw)) return false
  if (pw.length < 12) return false
  return /[A-Za-z]/.test(pw) && /[0-9]/.test(pw)
}
