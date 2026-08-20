package panelayarlari

// Panelin root/shadow giriş yolu anahtarı (bkz. migrations/0069).
//
// Bu bayrak YALNIZ :8443 panel girişini etkiler. Sunucunun SSH root erişimi,
// root parolası ve sshd yapılandırması bundan HİÇ etkilenmez.

import (
	"encoding/json"
	"net/http"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
	"sanalcp/internal/middleware"
)

// Root/shadow yolunun bağlı olduğu satır: users.id=1 ('root'). Oturum ve API
// token iptali bu id üzerinden yapılır.
const rootUserID = 1

// audit_log eylem adları — mevcut sözlükle aynı biçim (auth.2fa.enable /
// auth.2fa.disable gibi yön eylem adında taşınır).
const (
	auditRootGirisiAc    = "auth.rootgirisi.ac"
	auditRootGirisiKapat = "auth.rootgirisi.kapat"
)

// RootGirisi — GET /api/v1/system/root-girisi (AdminOnly).
//
// Burada panelbayrak.RootGirisiAcik KULLANILMAZ: o okuyucu fail-closed'dır
// (okunamazsa "kapalı" der) ve dört güvenlik kararı için doğrusu budur. Ama bu
// uç bir güvenlik kararı vermiyor, ekrana DURUM yazdırıyor: geçici bir DB
// hatasında bayrağı aslında 1 olan bir sunucu için "Kapalı" göstermek,
// sıkılaştırmasını doğrulayan operatöre yanlış bir "güvendesin" demek olurdu.
// Bu yüzden üç durum ayrıştırılır: 1 => açık, 0 => kapalı, okunamadı => hata.
func (h *Handlers) RootGirisi(w http.ResponseWriter, r *http.Request) {
	var acik int
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT root_girisi_acik FROM panel_ayarlari WHERE id=1`).Scan(&acik); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "root girişi durumu okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"acik": acik == 1})
}

type rootGirisiKaydetReq struct {
	Acik bool `json:"acik"`
}

// RootGirisiKaydet — PUT /api/v1/system/root-girisi (AdminOnly).
//
// KİLİTLENME KORUMASI: kapatma yönünde, sistemde aktif ve root OLMAYAN bir
// admin hesabı yoksa istek reddedilir. Aksi halde tek istekle kendini dışarı
// kilitlemek mümkün olurdu — root kapanır, girebilecek başka hesap yoktur ve
// geri açacak oturum da açılamaz. Açma yönünde böyle bir risk yok, sayım
// yapılmaz.
//
// Bu kontrol internal/users.sonAdminMi'den BAĞIMSIZDIR: o, hesap silmeyi/
// pasifleştirmeyi korur; bu, bayrağın kendisini kapatmayı korur. İkisi de
// gerekli — biri olmadan diğeri kilitlenmeyi tek adımda mümkün bırakır.
//
// KAPATMA = MEVCUT ERİŞİMİ DE İPTAL ETMEK. Bayrağı çevirmek tek başına yetmez;
// bayrak yalnız YENİ girişleri (auth.Login) durdurur. Kapatmadan önce açılmış
// root erişimi iki yoldan hayatta kalır:
//   - Canlı JWT'ler: middleware.RequireAuth bir oturumu ancak users.auth_version
//     değişince geçersiz sayar; bayrak o sütuna dokunmuyordu.
//   - API token'ları: auth.APITokenSahibi yalnız users tablosuna bakar, bayrağı
//     hiç okumaz — root oturumu süresiz (gun_sonra=0) bir token üretmişse
//     kapatmadan sonra da sonsuza dek çalışırdı.
//
// Bu yüzden kapatma yönünde auth_version artırılır (canlı oturumlar düşer) ve
// root'un API token'ları aktif=0 yapılır (0 = iptal edilmiş, bkz. 0068).
//
// ÜÇ YAZMA TEK TRANSACTION'DA: yarım uygulanmış bir kapatma — bayrak kapalı ama
// token'lar canlı — tam olarak bu düzeltmenin engellemek için var olduğu durum
// olurdu. Ya hepsi ya hiçbiri.
func (h *Handlers) RootGirisiKaydet(w http.ResponseWriter, r *http.Request) {
	var req rootGirisiKaydetReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}

	ctx := r.Context()
	if !req.Acik {
		var n int
		if err := h.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE role='admin' AND status='active' AND id<>1`).
			Scan(&n); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "admin sayısı okunamadı")
			return
		}
		if n == 0 {
			httpx.WriteError(w, http.StatusBadRequest,
				"root girişi kapatılamaz: önce root dışında aktif bir yönetici hesabı oluşturun")
			return
		}
	}

	deger := 0
	if req.Acik {
		deger = 1
	}

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	// Commit başarılıysa no-op; her hata yolunda geri alır.
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE panel_ayarlari SET root_girisi_acik=? WHERE id=1`, deger); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	if !req.Acik {
		// Canlı root oturumlarını düşür (RequireAuth auth_version'ı karşılaştırır).
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET auth_version=auth_version+1, updated_at=NOW() WHERE id=?`,
			rootUserID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
			return
		}
		// Root'un ürettiği API token'larını iptal et (aktif=0).
		if _, err := tx.ExecContext(ctx,
			`UPDATE api_tokenlari SET aktif=0 WHERE user_id=?`, rootUserID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}

	// Audit kaydı — HER İKİ yön de loglanır. Root/shadow yolunu tekrar AÇMAK
	// paneldeki en yüksek etkili tek anahtardır; izsiz kalmamalı. Commit'ten
	// SONRA yazılır: auth.WriteAudit *sql.DB alır (tx'e katılamaz) ve
	// best-effort'tur — audit yazımı bayrak değişikliğini geri almamalı.
	eylem := auditRootGirisiKapat
	if req.Acik {
		eylem = auditRootGirisiAc
	}
	var uid int64
	kadi := ""
	if c := middleware.ClaimsFrom(r); c != nil {
		uid, kadi = c.UserID, c.Username
	}
	auth.WriteAudit(h.DB, uid, kadi, httpx.ClientIP(r), eylem, "panel", true)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "acik": req.Acik})
}
