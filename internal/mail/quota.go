package mail

// Posta kutusu kotası (kutu başına depolama sınırı).
//
// SÖZLEŞME
//   - mailboxes.quota_bytes: 0 = sınırsız. Dovecot userdb sorgusu bu değeri
//     doğrudan okur (bkz. assets/mail/dovecot), yani kota DB'de değiştiği anda
//     (auth önbelleği boşaltılınca) yeniden başlatmadan yürürlüğe girer.
//   - Yeni kutunun varsayılanı, domain'in KANONİK planının
//     service_plans.mailbox_quota_mb değeridir (0 = sınırsız).
//
// KİM PLANIN ÜSTÜNE ÇIKABİLİR (karar, kotaIzinli'de uygulanır)
//   - admin ve bayi (reseller): plan varsayılanından bağımsız, üst sınır
//     (kota.MaxMailboxQuotaMB) içinde herhangi bir değer verebilir.
//   - müşteri (ve tanınmayan her rol — fail-closed): plan mailbox_quota_mb > 0
//     ise 1..plan arasında olmalı; "sınırsız" (0) YASAK. Plan sınırsızsa (0)
//     üst sınır içinde serbesttir.
//
// KULLANIM OKUMA: Dovecot'un Maildir++ `maildirsize` dosyası (her teslimat/silme
// sonrası Dovecot'un kendi artışlı yazdığı özet). Dizin gezintisi YOKTUR; istek
// başına kutu başına tek openat2 + ≤16 KiB okuma. Dosya tenant'ın yazabildiği
// dizinde durduğundan root olarak açılırken symlink takip edilmez
// (jailpath.Ac → RESOLVE_NO_SYMLINKS|RESOLVE_BENEATH) ve yalnız düzenli dosya okunur.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"sanalcp/internal/auth"
	"sanalcp/internal/httpx"
	"sanalcp/internal/jailpath"
	"sanalcp/internal/kota"
	"sanalcp/internal/middleware"

	"github.com/go-chi/chi/v5"
)

const (
	bytesPerMB = int64(1) << 20
	// maildirsize Dovecot tarafından ~5 KiB'ta yeniden hesaplanıp küçültülür;
	// bundan büyük dosya bozuk/şüpheli sayılır.
	maildirsizeMaxBayt = 16 << 10
)

// kotaMBGecerli: bir MB değerinin sınırlar içinde olduğunu doğrular.
func kotaMBGecerli(mb int64) error {
	if mb < 0 || mb > kota.MaxMailboxQuotaMB {
		return fmt.Errorf("kota 0 (sınırsız) ile %d MB arasında olmalı", kota.MaxMailboxQuotaMB)
	}
	return nil
}

// kotaBayt: MB değerini bayta çevirir (sınır doğrulaması dahil; taşma yok).
func kotaBayt(mb int64) (int64, error) {
	if err := kotaMBGecerli(mb); err != nil {
		return 0, err
	}
	return mb * bytesPerMB, nil
}

// planiAsabilir: rol, müşteri plan sınırını aşan kota verebilir mi?
func planiAsabilir(rol string) bool {
	return rol == middleware.RolAdmin || rol == middleware.RolBayi
}

// kotaIzinli: istenen kota (MB) bu rol için, plan sınırı (MB, 0 = sınırsız)
// altında izinli mi? Bkz. dosya başlığındaki sözleşme.
func kotaIzinli(rol string, planMB, istenenMB int64) error {
	if err := kotaMBGecerli(istenenMB); err != nil {
		return err
	}
	if planiAsabilir(rol) || planMB <= 0 {
		return nil
	}
	if istenenMB == 0 {
		return fmt.Errorf("paketinizde kutu başına en fazla %d MB kota var; sınırsız seçilemez", planMB)
	}
	if istenenMB > planMB {
		return fmt.Errorf("paketinizde kutu başına en fazla %d MB kota var", planMB)
	}
	return nil
}

// rolOf: istekteki doğrulanmış rol ("" = claim yok).
func rolOf(r *http.Request) string {
	if c := auth.ClaimsFrom(r); c != nil {
		return c.Role
	}
	return ""
}

var reMaildirsizeBaslik = regexp.MustCompile(`^[0-9]+[SC](,[0-9]+[SC])?$`)

// maildirsizeParse: Maildir++ maildirsize içeriğinden toplam kullanımı (bayt)
// çıkarır. Biçim: ilk satır limit başlığı ("<bayt>S[,<adet>C]"), sonraki her
// satır "<bayt> <adet>" (negatif olabilir, toplanır). Başlık eksik, satır
// bozuk ya da dosya kırpılmışsa ok=false — Dovecot da böyle dosyayı yeniden
// hesaplatır, biz "bilinmiyor" deriz (sahte 0 göstermeyiz).
func maildirsizeParse(r io.Reader) (used int64, ok bool) {
	ham, err := io.ReadAll(io.LimitReader(r, maildirsizeMaxBayt+1))
	if err != nil || len(ham) > maildirsizeMaxBayt {
		return 0, false
	}
	satirlar := strings.Split(string(ham), "\n")
	if !reMaildirsizeBaslik.MatchString(strings.TrimSpace(satirlar[0])) {
		return 0, false
	}
	var toplam int64
	for _, ln := range satirlar[1:] {
		f := strings.Fields(ln)
		if len(f) == 0 {
			continue
		}
		if len(f) != 2 {
			return 0, false
		}
		b, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			return 0, false
		}
		if _, err := strconv.ParseInt(f[1], 10, 64); err != nil {
			return 0, false
		}
		toplam += b
	}
	if toplam < 0 {
		toplam = 0
	}
	return toplam, true
}

// kutuKullanimi: kutunun diskteki kullanımı (bayt). Hata/yokluk → ok=false;
// çağıran listelemeyi ASLA bu yüzden düşürmez.
func kutuKullanimi(sk, lp string) (int64, bool) {
	if !reLocalPart.MatchString(lp) {
		return 0, false
	}
	home, err := jailpath.TenantHome(sk)
	if err != nil {
		return 0, false
	}
	// Dovecot userdb home'u: /home/<sk>/mail/<local_part> (bkz. dovecot şablonları).
	f, err := jailpath.Ac(home, "mail/"+lp+"/maildirsize", unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return 0, false
	}
	return maildirsizeParse(f)
}

// authOnbellekBosalt: Dovecot auth önbelleği userdb sonuçlarını (kota dahil)
// auth_cache_ttl (1 saat) boyunca tutar. Kota değişince ilgili kullanıcı
// önbellekten atılır; aksi hâlde yeni limit bir saate kadar yürürlüğe girmez.
// Paket değişkeni — testler Dovecot'suz çalışsın diye değiştirir.
var authOnbellekBosalt = func(ctx context.Context, email string) error {
	ctx, iptal := context.WithTimeout(ctx, 5*time.Second)
	defer iptal()
	out, err := exec.CommandContext(ctx, "doveadm", "auth", "cache", "flush", email).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PUT /domains/{id}/mail/{mid}/quota  {quota_mb}   (0 = sınırsız)
func (h *Handlers) KotaGuncelle(w http.ResponseWriter, r *http.Request) {
	id, _, demo, ok := h.domain(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if demo {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğinde kullanılamaz")
		return
	}
	mid, _ := strconv.ParseInt(chi.URLParam(r, "mid"), 10, 64)
	var req struct {
		QuotaMB *int64 `json:"quota_mb"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.QuotaMB == nil {
		httpx.WriteError(w, http.StatusBadRequest, "quota_mb (tam sayı, MB; 0 = sınırsız) zorunlu")
		return
	}
	bayt, err := kotaBayt(*req.QuotaMB)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var email string
	if err := h.DB.QueryRowContext(r.Context(),
		`SELECT email FROM mailboxes WHERE id=? AND domain_id=?`, mid, id).Scan(&email); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "kutu bulunamadı")
		return
	}
	planMB, err := kota.PlanMailboxQuotaMB(r.Context(), h.DB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "paket kota bilgisi doğrulanamadı")
		return
	}
	if err := kotaIzinli(rolOf(r), planMB, *req.QuotaMB); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE mailboxes SET quota_bytes=? WHERE id=? AND domain_id=?`, bayt, mid, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncellenemedi")
		return
	}
	if err := authOnbellekBosalt(r.Context(), email); err != nil {
		// Kota DB'de güncel; yalnız Dovecot önbelleği TTL dolana kadar eski değeri tutabilir.
		log.Printf("mail kota: auth önbelleği boşaltılamadı (%s): %v", email, err)
	}
	h.audit(r, "mail.quota", email, true)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "quota_bytes": bayt})
}

// yeniKutuKotasi: yeni kutunun kotası (bayt) — istekte verilmişse doğrulanmış
// değer, verilmemişse domain'in kanonik planının varsayılanı.
func (h *Handlers) yeniKutuKotasi(ctx context.Context, rol string, domainID int64, istenen *int64) (int64, error) {
	planMB, err := kota.PlanMailboxQuotaMB(ctx, h.DB, domainID)
	if err != nil {
		return 0, errPlanOkunamadi
	}
	if istenen == nil {
		return min(planMB, kota.MaxMailboxQuotaMB) * bytesPerMB, nil
	}
	if err := kotaIzinli(rol, planMB, *istenen); err != nil {
		return 0, err
	}
	return *istenen * bytesPerMB, nil
}

var errPlanOkunamadi = errors.New("paket kota bilgisi doğrulanamadı")
