package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	yescrypt "github.com/openwall/yescrypt-go"

	"sanalcp/internal/httpx"
	"sanalcp/internal/panelbayrak"
	"sanalcp/internal/system"
)

type Handlers struct {
	DB          *sql.DB
	Secret      []byte
	LifetimeSec int
}

type loginReq struct {
	Kullanici string `json:"kullanici"`
	Parola    string `json:"parola"`
	Kod       string `json:"kod"`
}

type loginResp struct {
	Token     string `json:"token"`
	Bitis     int64  `json:"bitis"`
	Kullanici struct {
		ID      int64  `json:"id"`
		Adi     string `json:"adi"`
		Rol     string `json:"rol"`
		AdSoyad string `json:"ad_soyad"`
	} `json:"kullanici"`
}

// rootShadowHash — /etc/shadow'dan root parola hash'ini okur ("" = bulunamadı).
func rootShadowHash() string {
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "root:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				return parts[1]
			}
			return ""
		}
	}
	return ""
}

// rootParolaDogrula — /etc/shadow'daki root hash'iyle parolayı doğrular.
//
// yescrypt ($y$) — AlmaLinux 10 varsayılanı — NATİF Go ile hesaplanır
// (github.com/openwall/yescrypt-go: yescrypt yazarlarının kendi uygulaması).
// Eski formatlar ($6$/$5$/$1$) için `openssl passwd` çağrılır — ÖNCEDEN
// python3'ün `crypt` modülüne shell-out ediyordu, o modül Python 3.13'te
// kaldırıldığı için sunucu Python'u güncellediğinde panel girişi kırılırdı.
// openssl her AlmaLinux/RHEL kurulumunda temel bağımlılık olduğu için daha
// sağlam bir seçim; ayrıca denenen (github.com/GehirnInc/crypt) pure-Go
// kütüphanesi rounds= içeren sha256/sha512 hash'lerinde salt'ı yanlış
// ayrıştırdığı (bkz. legacycrypt_test.go'daki "rounds=" vakası) için elendi —
// openssl glibc ile aynı referans uygulamayı kullanır.
//
// Karşılaştırma iki yolda da sabit zamanlıdır (timing side-channel'a karşı):
// yescrypt yolunda subtle.ConstantTimeCompare, legacyCryptDogrula içinde de
// aynısı.
func rootParolaDogrula(parola string) bool {
	hash := rootShadowHash()
	// Kilitli ("!", "!!", "*") veya parolasız hesap → asla kabul etme.
	if len(hash) < 3 || !strings.HasPrefix(hash, "$") {
		return false
	}
	if strings.HasPrefix(hash, "$y$") { // yescrypt → natif Go
		hesap, err := yescrypt.Hash([]byte(parola), []byte(hash))
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare(hesap, []byte(hash)) == 1
	}
	return legacyCryptDogrula(parola, hash)
}

// rootParolaDogrulaFn — rootParolaDogrula'nın test edilebilir sarmalayıcısı.
// Gerçek doğrulama /etc/shadow'u okur; testler bu değişkeni geçici olarak
// değiştirerek Login'in bayrak kapısını dosya sistemine bağımlı olmadan
// doğrular. Üretimde ASLA değiştirilmez.
var rootParolaDogrulaFn = rootParolaDogrula

// legacyCryptSaltAyikla — "$id$[rounds=N$]salt$digest" hash'inden openssl'in
// -salt argümanına verilecek "[rounds=N$]salt" bölümünü çıkarır. Salt ve
// digest crypt(3) alfabesinde ($ hariç ./0-9A-Za-z) olduğundan "$" ile
// bölmek güvenlidir. openssl'in KENDİSİ tam hash'i -salt olarak verilince
// (glibc crypt(3)'ün aksine) digest'in fazladan karakterlerini salt'a
// karıştırıyor — bu yüzden burada elle en dar segment çıkarılır.
func legacyCryptSaltAyikla(hash string) (id, saltArg string, tamam bool) {
	parcalar := strings.Split(hash, "$")
	// "$6$salt$digest" → ["", "6", "salt", "digest"] (4)
	// "$6$rounds=N$salt$digest" → ["", "6", "rounds=N", "salt", "digest"] (5)
	if len(parcalar) == 4 {
		return parcalar[1], parcalar[2], true
	}
	if len(parcalar) == 5 && strings.HasPrefix(parcalar[2], "rounds=") {
		return parcalar[1], parcalar[2] + "$" + parcalar[3], true
	}
	return "", "", false
}

// legacyCryptDogrula — ESKİ YOL: yalnız yescrypt-dışı formatlar (sha512/sha256/
// md5crypt) için yedek.
func legacyCryptDogrula(parola, hash string) bool {
	id, saltArg, tamam := legacyCryptSaltAyikla(hash)
	if !tamam {
		return false
	}
	var bayrak string
	switch id {
	case "6":
		bayrak = "-6"
	case "5":
		bayrak = "-5"
	case "1":
		bayrak = "-1"
	default:
		return false // DES-crypt gibi desteklenmeyen eski formatlar
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openssl", "passwd", bayrak, "-salt", saltArg, "-stdin")
	cmd.Stdin = strings.NewReader(parola)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(out))), []byte(hash)) == 1
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 64KB üstü login gövdesi = kötüye kullanım (DoS)
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	req.Kullanici = strings.TrimSpace(req.Kullanici)
	if req.Kullanici == "" || req.Parola == "" {
		httpx.WriteError(w, http.StatusBadRequest, "kullanıcı adı ve parola zorunlu")
		return
	}

	// GÜVENLİK: kaba-kuvvet koruması artık router seviyesinde middleware.GirisLimiti
	// ile yapılıyor (bkz. cmd/server/main.go) — IP başına kayan pencere + kademeli
	// gecikme + kilit, bu handler her 401 dönüşünde otomatik sayılır. Eskiden burada
	// ayrı ayrı audit_log sorgulayan iki kontrol (auth.login/auth.2fa) vardı; tek
	// katmanda birleştirildi (daha güçlü: progresif gecikme + panel restart'tan
	// bağımsız DB yükü yok).
	ip := httpx.ClientIP(r)

	// Kimlik çözümleme — iki ayrı parola dünyası (bkz. parola.go):
	//
	//   root  → /etc/shadow (yescrypt). Bu yol çok kullanıcılı desteğe
	//           geçerken bilinçli olarak DEĞİŞTİRİLMEDİ; panelden kilitlenme
	//           riskini sıfırda tutmanın tek yolu buydu.
	//   diğer → users.password_hash (bcrypt), yalnız status='active' hesaplar.
	//
	// İki dalın da başarısızlık yanıtı aynıdır ("kullanıcı adı veya parola
	// hatalı") — hangi kullanıcı adlarının var olduğunu sızdırmamak için.
	var (
		uid     int64
		kadi    string
		rol     string
		adSoyad string
		surum   uint64
	)

	if KullaniciRootMu(req.Kullanici) {
		// Root/shadow yolu artık BAYRAKLI (bkz. migrations/0069). Kapalıysa
		// parola DOĞRU olsa bile reddedilir — panel girişi sunucunun root
		// parolasıyla eşleşmeyi bıraksın diye. Kısa devre bilinçli: bayrak
		// kapalıyken /etc/shadow hiç okunmaz.
		//
		// Yanıt hatalı-parola dalıyla BİREBİR aynı: root yolunun kapalı
		// olduğu sızarsa, saldırgan hangi sunucularda bu yolun hâlâ açık
		// olduğunu tarayarak ayıklayabilirdi. 401 döndüğü için deneme
		// middleware.GirisLimiti sayacına da girer.
		if !panelbayrak.RootGirisiAcik(r.Context(), h.DB) || !rootParolaDogrulaFn(req.Parola) {
			WriteAudit(h.DB, 0, req.Kullanici, ip, "auth.login", req.Kullanici, false)
			httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı adı veya parola hatalı")
			return
		}
		uid, kadi, rol = 1, "root", "admin"
		_ = h.DB.QueryRow(`SELECT full_name FROM users WHERE id=1`).Scan(&adSoyad)
	} else {
		var hash, durum string
		err := h.DB.QueryRow(
			`SELECT id, username, password_hash, role, status, full_name FROM users WHERE username=?`,
			req.Kullanici).Scan(&uid, &kadi, &hash, &rol, &durum, &adSoyad)
		if err != nil || !ParolaEslesiyorMu(hash, req.Parola) {
			WriteAudit(h.DB, 0, req.Kullanici, ip, "auth.login", req.Kullanici, false)
			httpx.WriteError(w, http.StatusUnauthorized, "kullanıcı adı veya parola hatalı")
			return
		}
		if durum != "active" {
			WriteAudit(h.DB, uid, kadi, ip, "auth.login", kadi, false)
			httpx.WriteError(w, http.StatusForbidden, "hesap askıya alınmış")
			return
		}
		// Müşteri rolü panel oturumu açamaz; müşteriler /musteri/login ile
		// kendi domain panellerine girer.
		if rol != "admin" && rol != "reseller" {
			WriteAudit(h.DB, uid, kadi, ip, "auth.login", kadi, false)
			httpx.WriteError(w, http.StatusForbidden, "bu hesap yönetim paneline giriş yapamaz")
			return
		}
	}

	// 2FA — parola doğru; 2FA açıksa TOTP kodu da gerekir. Artık giriş yapan
	// kullanıcının kendi kaydından okunuyor (eskiden id=1'e sabitti).
	// FAIL-CLOSED: 2FA durumu okunamıyorsa (DB hatası) giriş REDDEDİLİR (eskiden
	// hata yutulup 2FA sessizce atlanıyordu = fail-open).
	{
		var en int
		var sec string
		var sonAdim int64
		if err := h.DB.QueryRow(`SELECT totp_enabled, totp_secret, totp_last_step, auth_version FROM users WHERE id=?`, uid).Scan(&en, &sec, &sonAdim, &surum); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "2FA durumu doğrulanamadı")
			return
		}
		if en == 1 {
			if strings.TrimSpace(sec) == "" {
				httpx.WriteError(w, http.StatusInternalServerError, "2FA yapılandırması hatalı")
				return
			}
			if strings.TrimSpace(req.Kod) == "" {
				httpx.WriteJSON(w, http.StatusOK, map[string]any{"iki_fa_gerekli": true})
				return
			}
			adim, ok := TOTPVerifyAdim(sec, req.Kod, sonAdim)
			if !ok {
				WriteAudit(h.DB, uid, kadi, ip, "auth.2fa", kadi, false)
				httpx.WriteError(w, http.StatusUnauthorized, "2FA kodu hatalı veya tekrar kullanıldı")
				return
			}
			_, _ = h.DB.Exec(`UPDATE users SET totp_last_step=? WHERE id=?`, adim, uid) // replay koruması
		}
	}

	tok, err := Issue(h.Secret, h.LifetimeSec, uid, kadi, rol, surum)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "token üretilemedi")
		return
	}
	WriteAudit(h.DB, uid, kadi, ip, "auth.login", kadi, true)
	_, _ = h.DB.Exec(`UPDATE users SET last_login_at=NOW(), last_login_ip=? WHERE id=?`, ip, uid)
	// Bu handler'a yalnız admin/bayi ulaşabilir (müşteri rolü satır 205'te
	// reddedilir) — arka plandaki ~24 saatlik periyodu beklemeden giriş anında
	// güncel bir sürüm kontrolü tetikle (cooldown'lu, bkz. system.SurumKontrolTetikle).
	system.SurumKontrolTetikle()

	resp := loginResp{Token: tok, Bitis: time.Now().Add(time.Duration(h.LifetimeSec) * time.Second).Unix()}
	resp.Kullanici.ID = uid
	resp.Kullanici.Adi = kadi
	resp.Kullanici.Rol = rol
	resp.Kullanici.AdSoyad = adSoyad
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// WriteAudit: audit_log'a bir girişim kaydeder (login/2FA/parola vb.). Diğer
// paketler (ör. musteri) de kendi login denemelerini burada loglar — kalıcı
// izleme/analiz için tek ortak tablo.
func WriteAudit(db *sql.DB, uid int64, username, ip, action, target string, ok bool) {
	var uidVal any
	if uid > 0 {
		uidVal = uid
	}
	okv := 0
	if ok {
		okv = 1
	}
	_, _ = db.Exec(
		`INSERT INTO audit_log(actor_user_id, actor_username, ip, action, target, ok)
		 VALUES(?,?,?,?,?,?)`,
		uidVal, username, ip, action, target, okv)
}

// AuditKayit — güvenlik günlüğü satırı (yalnız okuma).
type AuditKayit struct {
	ID        int64  `json:"id"`
	Zaman     string `json:"zaman"`
	Kullanici string `json:"kullanici"`
	IP        string `json:"ip"`
	Eylem     string `json:"eylem"`
	Hedef     string `json:"hedef"`
	Basarili  bool   `json:"basarili"`
}

// AuditListe: audit_log'u tersten (en yeni önce) döndürür.
//
// Tablo yıllardır yazılıyordu ama okunacak bir uç yoktu — başarısız giriş
// denemelerini görmek için sunucuya SSH ile girip MySQL sorgulamak gerekiyordu.
//
// Filtreler: ?limit=N (varsayılan 200, tavan 1000), ?eylem=auth.login,
// ?sadece_hata=1. Tarih aralığı yerine limit tercih edildi — bu ekranın işi
// "son ne oldu", arşiv analizi değil.
func (h *Handlers) AuditListe(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	q := `SELECT id, DATE_FORMAT(ts, '%Y-%m-%d %H:%i:%s'), actor_username, ip, action, target, ok
	      FROM audit_log`
	kosul := make([]string, 0, 2)
	arg := make([]any, 0, 3)
	if e := strings.TrimSpace(r.URL.Query().Get("eylem")); e != "" {
		kosul = append(kosul, "action = ?")
		arg = append(arg, e)
	}
	if r.URL.Query().Get("sadece_hata") == "1" {
		kosul = append(kosul, "ok = 0")
	}
	if len(kosul) > 0 {
		q += " WHERE " + strings.Join(kosul, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	arg = append(arg, limit)

	rows, err := h.DB.QueryContext(r.Context(), q, arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := make([]AuditKayit, 0)
	for rows.Next() {
		var k AuditKayit
		var okv int
		if err := rows.Scan(&k.ID, &k.Zaman, &k.Kullanici, &k.IP, &k.Eylem, &k.Hedef, &okv); err != nil {
			continue
		}
		k.Basarili = okv == 1
		out = append(out, k)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// AuditEylemler: filtre açılır listesini doldurmak için tablodaki farklı
// eylem adları (sabit liste tutmak yerine — yeni eylem eklendikçe kendiliğinden
// görünür).
func (h *Handlers) AuditEylemler(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT DISTINCT action FROM audit_log ORDER BY action`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err == nil {
			out = append(out, a)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
