# Panel Lisans Numarası + Kurulum Telemetrisi — Tasarım

**Tarih:** 2026-08-24
**Durum:** Onaylandı (brainstorming), uygulama planı bekleniyor

## Amaç

SanalCP paneli ücretsiz dağıtılıyor. Her kurulumun bir "lisans numarası" olması ve bu numaranın `/araclar-ayarlar` sayfasında görünmesi isteniyor. Ayrıca proje sahibinin, tüm kurulumların envanterini (hangi IP'ye ne zaman hangi sürüm kurulmuş, şu an hangi sürümde) merkezi bir yerden (Firebase/Firestore) görebilmesi isteniyor.

**Kapsam dışı (bilinçli):** Lisans zorlaması / kilitleme yok — panel lisans numarası olmadan da tam çalışır, bu numara sadece envanter ve kimlik amaçlıdır. Firebase Cloud Functions kullanılmıyor (bkz. Alınan Kararlar). Firestore Rules'un otomatik testi (emulator) kapsam dışı.

## Mevcut Durum

`internal/system/surumkontrol.go` zaten bir kurulum kimliği üretiyor (`KurulumKimligi()`, 16 byte rastgele hex, `/etc/sanalcp/kurulum-kimlik`, 0600) ve günde bir kez (+giriş tetikli, ±jitter'lı) `surum.json` manifestini çekiyor. Dosyanın başındaki yorum bilinçli olarak kimliğin **gönderilmediğini** belirtiyor: *"Şu anki uç statik bir dosya; kimliği sayan kimse yok, dolayısıyla göndermek karşılıksız bir kimlik sızıntısı olurdu."* Bu iş, tam olarak o "sayan" tarafı kurar.

`PANEL_SURUM_KONTROL=0` bugün de tüm dış istekleri tamamen kapatıyor (goroutine hiç başlamıyor).

## Alınan Kararlar

Brainstorming sırasında netleşen kararlar:

| Konu | Karar | Elenen alternatif |
|---|---|---|
| Lisans numarası kaynağı | Mevcut `KurulumKimligi()` aynen kullanılır | Ayrı, insan-okunur biçimli yeni bir numara üretmek |
| Açma/kapama bayrağı | Mevcut `PANEL_SURUM_KONTROL` genişletilir | Ayrı `PANEL_TELEMETRI` bayrağı |
| Firebase yazım yolu | Panel sunucusu doğrudan Firestore REST API'ye yazar | Ara katman Cloud Function |
| Telemetri alanları | kurulum_kimlik + sürüm + IP + ilk/son görülme + osfam + panel dili | Yalnız asgari küme (osfam/dil olmadan) |
| IP tespiti | Panelin kendi beyanı (harici "IP nedir" ucuna sorup sonucu yazar) | Cloud Function'ın bağlantıdan gördüğü IP (sahteciliğe kapalı ama daha ağır kurulum) |

IP alanının kendi-beyanlı olmasının bilinçli sonucu: teorik olarak sahteciliğe açık (panel sahibi isterse farklı bir IP bildirebilir). Bu, kendi müşterilerinin kurulumları için kabul edilebilir bir risk olarak değerlendirildi — kötüye kullanımın maliyeti/faydası yok, tek zarar kendi envanterinin yanlış olması.

## Mimari

### 1. Veri modeli (Firestore)

Proje: proje sahibinin kendi Firebase projesi (tüm kurulumlar aynı projeye yazar).
Koleksiyon: `kurulumlar`. Doküman ID = `kurulum_kimlik` (mevcut kimlikle birebir aynı — bu aynı zamanda panelde gösterilecek "lisans numarası").

Alanlar:

```
kurulum_kimlik      string   — doc ID ile birebir aynı, tutarlılık kontrolü için ayrıca alan olarak da tutulur
mevcut_surum        string   — örn "0.9.7" (surumMevcut ile aynı kaynak)
kaynak_ip           string   — kendi beyanı, harici IP sorgusundan
osfam               string   — AlmaLinux/Ubuntu vb. (internal/osfam'da zaten tespit ediliyor)
panel_dili           string   — tr/en (internal/panelayarlari/dil.go)
ilk_kurulum_zamani  timestamp — yalnız doküman CREATE edilirken serverTimestamp ile yazılır
son_gorulme_zamani  timestamp — her yazımda serverTimestamp ile güncellenir
```

`ilk_kurulum_zamani` create sonrası hiçbir update isteğinde değiştirilemez (bkz. Rules).

### 2. Firestore Security Rules

```
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    match /kurulumlar/{kurulumId} {
      allow read: if false;  // yalnız proje sahibi Firebase Console / Admin SDK üzerinden okur
      allow create: if request.resource.data.keys().hasOnly([
                        'kurulum_kimlik','mevcut_surum','kaynak_ip',
                        'osfam','panel_dili','ilk_kurulum_zamani','son_gorulme_zamani'
                      ])
                      && request.resource.data.kurulum_kimlik == kurulumId
                      && request.resource.data.kurulum_kimlik is string
                      && request.resource.data.kurulum_kimlik.size() >= 16;
      allow update: if request.resource.data.keys().hasOnly([
                        'kurulum_kimlik','mevcut_surum','kaynak_ip',
                        'osfam','panel_dili','ilk_kurulum_zamani','son_gorulme_zamani'
                      ])
                      && request.resource.data.kurulum_kimlik == resource.data.kurulum_kimlik
                      && request.resource.data.ilk_kurulum_zamani == resource.data.ilk_kurulum_zamani;
      allow delete: if false;
    }
  }
}
```

Bu dosya repoya `firestore.rules` olarak eklenir (belgesel amaçlı — proje sahibi Firebase Console'a elle de girer, ama repo kaynak-doğrusu olur).

### 3. Backend (Go)

Yeni dosya: `internal/system/telemetri.go`

- `telemetriGonder()` — `SurumBaslat` içindeki mevcut goroutine döngüsünde `surumGetir()`'in hemen yanında çağrılır. Aynı açık/kapalı bayrağı (`surumKontrolAcikMi()`), aynı periyot ve jitter'ı, aynı giriş-tetikli refresh'i (`SurumKontrolTetikle`) paylaşır — ayrı bir zamanlayıcı kurulmaz.
- IP tespiti: `PANEL_IP_UC` ortam değişkeniyle override edilebilen varsayılan bir "IP nedir" ucuna (`https://api.ipify.org`) düz GET. Hata = alan boş bırakılır, panel etkilenmez (mevcut "ağ hatası = sessiz" felsefesiyle birebir aynı).
- Firestore yazımı: `PATCH https://firestore.googleapis.com/v1/{proje}/databases/(default)/documents/kurulumlar/{kurulum_kimlik}?updateMask.fieldPaths=...&key={api_anahtari}` — sadece değişen alanlar `updateMask` ile gönderilir, `ilk_kurulum_zamani` yalnız ilk yazımda (doküman yoksa) gönderilir.
- Yeni ortam değişkenleri (ikisi de gömülü varsayılanla, `surumUC()` deseniyle birebir aynı): `PANEL_FIREBASE_PROJE`, `PANEL_FIREBASE_API_ANAHTARI`. Firestore API key'i zaten Firebase'in genel tasarımı gereği "gizli" değildir — güvenlik Rules ile sağlanır, bu yüzden binary'ye gömülmesi sorun teşkil etmez.
- `PANEL_SURUM_KONTROL=0` → telemetri de hiç çalışmaz (aynı goroutine, ek kontrol gerekmez).

`internal/system/surumkontrol.go` içindeki `SurumBilgi` handler'ı `kurulum_kimlik` alanıyla genişletilir — bu uç zaten tüm oturum açmış kullanıcılara (rol kısıtı yok, footer'da sürüm göstermek için) açık.

### 4. Frontend

Yeni bileşen: `frontend/src/components/LisansBilgisi.tsx`, `AraclarAyarlarPage.tsx`'in "Sunucu bakımı" bölümüne (`PanelGuncelleme`, `HostnameAyari` vb. ile aynı grid) eklenir. `/system/surum-bilgi` ucundan `kurulum_kimlik`'i okur, salt-okunur gösterir + kopyala butonu. i18n dosyaları (`tr`/`en`) `PanelGuncelleme.json` deseniyle aynı klasöre eklenir.

### 5. Hata davranışı ve gizlilik

- Ağ hatası (IP ucu veya Firestore ulaşılamaz) tamamen sessiz — panel hiçbir yerde hata göstermez, mevcut `surumHataYaz` deseninin dışında ayrı bir hata alanı bile tutulmaz (telemetri en iyi çaba, kritik değil).
- `PANEL_SURUM_KONTROL=0` ile kapatan kurulumlar hiç istek atmaz — ne surum.json'a ne Firestore'a.
- Firestore'da okuma tamamen kapalı: yalnızca proje sahibi (Firebase Console veya Admin SDK ile) görebilir, üçüncü bir taraf/başka bir kurulum başka bir kurulumun verisini okuyamaz.

## Test Planı

- Mevcut `surumTetikleGerekliMi` tarzı saf karar fonksiyonları eklenir: whitelist alan kontrolü ve "ilk_kurulum_zamani sadece create'te gönderilir" mantığı Go tarafında da (Firestore'a gitmeden önce) test edilir.
- `telemetriGonder()` için mock HTTP sunucularla: IP ucu düşükken, Firestore düşükken davranışın sessiz kaldığı ve panelin etkilenmediği doğrulanır (mevcut `surumGetir` testleriyle aynı desen, bkz. `internal/system/surumkontrol_test.go` — dosya varsa oraya eklenir, yoksa `telemetri_test.go` olarak açılır).
- Firestore Rules'un kendisi bu işin kapsamında test edilmez (emulator gerektirir); kurallar elle Firebase Console'dan doğrulanır.

## Değişecek/Eklenecek Dosyalar

- `internal/system/telemetri.go` (yeni)
- `internal/system/telemetri_test.go` (yeni)
- `internal/system/surumkontrol.go` (SurumBaslat içine telemetriGonder çağrısı, SurumBilgi'ye kurulum_kimlik eklenir)
- `firestore.rules` (yeni, repo kökü)
- `frontend/src/components/LisansBilgisi.tsx` (yeni)
- `frontend/src/pages/AraclarAyarlarPage.tsx` (kart eklenir)
- `frontend/src/i18n/locales/{tr,en}/LisansBilgisi.json` (yeni)
