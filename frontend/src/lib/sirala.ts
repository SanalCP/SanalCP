// Liste sayfalarının ortak sıralama yardımcısı: tablolar ilk sütundaki veriye
// göre alfabetik gelir (domainler, DNS, e-posta, veritabanları, SSL,
// kullanıcılar, müşteriler).
//
// Neden Intl.Collator: Türkçe harf sırası ASCII'den farklıdır (ç, ğ, ı, ö, ş, ü)
// ve `a.localeCompare(b)` her karşılaştırmada yeni bir collator kurar — 100+
// satırda gereksiz maliyet. Collator bir kez kurulup yeniden kullanılır.
//
// numeric: true → "db2" < "db10" (sözlük sırasında tersi olurdu).
// sensitivity: 'base' → büyük/küçük harf ve aksan farkı sırayı bozmaz.
const karsilastirici = new Intl.Collator('tr', { sensitivity: 'base', numeric: true })

// Diziyi KOPYALAYARAK sıralar; çağıranın state dizisini yerinde değiştirmez
// (React'te aynı referansın mutasyonu yeniden render'ı kaçırır).
export function metneGoreSirala<T>(liste: T[], alan: (satir: T) => string): T[] {
  return [...liste].sort((a, b) => karsilastirici.compare(alan(a) ?? '', alan(b) ?? ''))
}
