package domains

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	"sanalpanel/internal/httpx"
	"sanalpanel/internal/provisioner"

	"github.com/go-chi/chi/v5"
)

// ErrDemoAskiya: demo abonelikler hiçbir yoldan (tekil veya bayi zinciri ile) askıya alınamaz.
var ErrDemoAskiya = errors.New("demo abonelik askıya alınamaz")

// AskiyaAl: POST /domains/{id}/askiya-al — hesabı askıya al.
// domains.askida=1 + durum=pasif işaretlenir, vhost 503 "askıya alındı" olarak yeniden render edilir.
// (Askıda durumu DB'de kalıcıdır; SetPHP/SSL gibi her yeniden render'da tekrar uygulanır.)
func (h *Handlers) AskiyaAl(w http.ResponseWriter, r *http.Request) {
	h.askiToggle(w, r, true)
}

// AskidanAl: POST /domains/{id}/askidan-al — askıyı kaldır, siteyi geri getir.
func (h *Handlers) AskidanAl(w http.ResponseWriter, r *http.Request) {
	h.askiToggle(w, r, false)
}

func (h *Handlers) askiToggle(w http.ResponseWriter, r *http.Request, askida bool) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	alanAdi, err := AskiUygula(r.Context(), h.DB, id, askida)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	case errors.Is(err, ErrDemoAskiya):
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "alan_adi": alanAdi, "askida": askida,
	})
}

// AskiUygula: tek bir domain'in askı durumunu değiştirir (vhost + FTP + mail + runtime).
// HTTP'den bağımsızdır — tekil uç (askiToggle) ve toplu bayi zinciri (BayiAskisiUygula)
// aynı fonksiyonu paylaşır ki iki yolun davranışı asla birbirinden sapmasın.
func AskiUygula(ctx context.Context, db *sql.DB, id int64, askida bool) (alanAdi string, err error) {
	var sk string
	var isDemo int
	if err := db.QueryRowContext(ctx,
		`SELECT alan_adi, sistem_kullanici, is_demo FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &isDemo); err != nil {
		return "", err
	}
	if isDemo == 1 {
		return alanAdi, ErrDemoAskiya
	}

	ak := 0
	durum := "aktif"
	if askida {
		ak = 1
		durum = "pasif"
	}
	if _, err := db.ExecContext(ctx, `UPDATE domains SET askida=?, durum=? WHERE id=?`, ak, durum, id); err != nil {
		return alanAdi, err
	}

	// vhost'u yeniden render et (askida durumu DB'den okunur)
	if err := provisioner.RerenderVhost(db, id); err != nil {
		// DB güncellendi ama vhost render başarısız → geri al ki tutarlı kalsın
		geriDurum := map[bool]string{true: "aktif", false: "pasif"}[askida]
		_, _ = db.ExecContext(ctx, `UPDATE domains SET askida=?, durum=? WHERE id=?`, 1-ak, geriDurum, id)
		return alanAdi, err
	}

	// FTP + panel-login kilidi: askıda => ftp_accounts.status='suspended'. Hem Pure-FTPd
	// auth sorgusu hem musteri.Login "status='active'" şartı arar → askıdayken her ikisi
	// de reddedilir. Askıdan alınca 'active'e döner.
	ftpStatus := "active"
	if askida {
		ftpStatus = "suspended"
	}
	if _, e := db.ExecContext(ctx, `UPDATE ftp_accounts SET status=? WHERE domain_id=?`, ftpStatus, id); e != nil {
		log.Printf("askiUygula: ftp_accounts status güncelleme (domain %d): %v", id, e)
	}

	// Mail: Postfix/Dovecot SQL sorguları durum/status='active' filtreler, bu yüzden bu iki
	// UPDATE servis restart'sız anında hem gelen postayı reddeder hem SMTP AUTH'u keser.
	// Kutular SİLİNMEZ — askıdan alınca aynı UPDATE ile 'active'e döner.
	mailStatus := "active"
	if askida {
		mailStatus = "suspended"
	}
	if _, e := db.ExecContext(ctx, `UPDATE mail_domains SET durum=? WHERE domain_id=?`, mailStatus, id); e != nil {
		log.Printf("askiUygula: mail_domains durum güncelleme (domain %d): %v", id, e)
	}
	if _, e := db.ExecContext(ctx, `UPDATE mailboxes SET status=? WHERE domain_id=?`, mailStatus, id); e != nil {
		log.Printf("askiUygula: mailboxes status güncelleme (domain %d): %v", id, e)
	}

	// Çalışan tenant süreçlerini (php-fpm worker) durdur + crontab'ı devre dışı bırak /
	// geri getir. Best-effort (birincil askı durumu DB + 503 vhost ile zaten uygulandı).
	if sk != "" {
		provisioner.SuspendUserRuntime(sk, askida)
	}

	return alanAdi, nil
}

// BayiAskisiUygula: bir bayinin TÜM müşteri domainlerine askı durumunu uygular.
// Çağıran taraf (internal/users.DurumDegistir) kilit.Bayi(bayiID) ile sarmalamalıdır —
// aksi hâlde askı zinciri sürerken tam o anda oluşturulan bir domain listenin
// dışında kalıp askıdan muaf canlı kalabilir.
func BayiAskisiUygula(ctx context.Context, db *sql.DB, bayiID int64, askida bool) (etkilenen, basarisiz int, err error) {
	rows, err := db.QueryContext(ctx,
		`SELECT d.id FROM domains d JOIN customers c ON c.id = d.customer_id WHERE c.owner_user_id = ?`, bayiID)
	if err != nil {
		return 0, 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, id := range ids {
		if _, err := AskiUygula(ctx, db, id, askida); err != nil {
			if errors.Is(err, ErrDemoAskiya) {
				continue // demo abonelik hiçbir zaman askıya alınmaz, bilinçli atlanır
			}
			basarisiz++
			log.Printf("bayi %d askı zinciri: domain %d: %v", bayiID, id, err)
			continue
		}
		etkilenen++
	}
	return etkilenen, basarisiz, nil
}
