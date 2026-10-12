// uretGucluParola: tarayıcı tarafı güçlü parola. n >= 2 için en az bir harf ve
// bir rakam garanti edilir (alfabede 8/56 rakam var; garanti olmadan 20
// karakterlik parolaların ~%4,7'si rakamsız çıkıp parolaGucluMu'ya takılıyordu).
export function uretGucluParola(n = 20): string {
  const harf = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const buf = new Uint32Array(n)
  for (;;) {
    window.crypto.getRandomValues(buf)
    let s = ''
    for (let i = 0; i < n; i++) s += harf[buf[i] % harf.length]
    if (n < 2 || (/[A-Za-z]/.test(s) && /[0-9]/.test(s))) return s
  }
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
