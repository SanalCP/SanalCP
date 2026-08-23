// uretGucluParola: tarayıcı tarafı güçlü parola (harf+rakam karışık, min-güç geçer).
export function uretGucluParola(n = 20): string {
  const harf = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const buf = new Uint32Array(n)
  ;(window.crypto || (window as any).msCrypto).getRandomValues(buf)
  let s = ''
  for (let i = 0; i < n; i++) s += harf[buf[i] % harf.length]
  return s
}
