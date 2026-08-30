// Package islemmerkezi panelde farklı tablolarda tutulan uzun süren işleri
// tek, salt-okunur bir akışta birleştirir. İşlerin sahibi yine kendi modülüdür;
// bu paket ikinci bir kuyruk veya durum kaynağı oluşturmaz.
package islemmerkezi

import (
	"database/sql"
	"net/http"

	"sanalcp/internal/httpx"
)

type Handlers struct {
	DB         *sql.DB
	Senkronize func()
}

type Islem struct {
	Anahtar   string `json:"anahtar"`
	Tur       string `json:"tur"`
	Baslik    string `json:"baslik"`
	Aciklama  string `json:"aciklama"`
	Durum     string `json:"durum"`
	Ilerleme  int    `json:"ilerleme"`
	Mesaj     string `json:"mesaj"`
	Yol       string `json:"yol"`
	Baslangic string `json:"baslangic"`
	Bitis     string `json:"bitis"`
}

const listeSQL = `
SELECT anahtar,tur,baslik,aciklama,durum,ilerleme,mesaj,yol,baslangic,bitis FROM (
  SELECT CONCAT('import:',j.id) COLLATE utf8mb4_unicode_ci anahtar,'ice_aktarim' COLLATE utf8mb4_unicode_ci tur,
    CONCAT(COALESCE(d.alan_adi,'Domain'),' içe aktarımı') COLLATE utf8mb4_unicode_ci baslik,
    IF(j.tur='files','Dosya içe aktarımı','Veritabanı içe aktarımı') COLLATE utf8mb4_unicode_ci aciklama,
    CASE j.durum WHEN 'queued' THEN 'bekliyor' WHEN 'running' THEN 'calisiyor'
      WHEN 'success' THEN 'basarili' WHEN 'rolled_back' THEN 'geri_alindi' ELSE 'basarisiz' END COLLATE utf8mb4_unicode_ci durum,
    j.ilerleme,COALESCE(j.mesaj,'') COLLATE utf8mb4_unicode_ci mesaj,CONCAT('/abonelikler/',j.domain_id,'/ice-aktarim') COLLATE utf8mb4_unicode_ci yol,
    DATE_FORMAT(j.created_at,'%Y-%m-%d %H:%i:%s') COLLATE utf8mb4_unicode_ci baslangic,
    COALESCE(DATE_FORMAT(j.finished_at,'%Y-%m-%d %H:%i:%s'),'') COLLATE utf8mb4_unicode_ci bitis,j.created_at sirala
  FROM import_jobs j LEFT JOIN domains d ON d.id=j.domain_id
  WHERE j.created_at>=NOW()-INTERVAL 7 DAY
  UNION ALL
  SELECT CONCAT('remote:',j.id) COLLATE utf8mb4_unicode_ci,'uzak_tasima' COLLATE utf8mb4_unicode_ci,CONCAT(j.source_domain,' taşıması') COLLATE utf8mb4_unicode_ci,
    CONCAT(UPPER(j.provider),' · ',j.source_account) COLLATE utf8mb4_unicode_ci,
    CASE WHEN j.status='queued' THEN 'bekliyor'
      WHEN j.status IN ('packaging','downloading','importing') THEN 'calisiyor'
      WHEN j.status='success' THEN 'basarili' ELSE 'basarisiz' END COLLATE utf8mb4_unicode_ci,
    j.progress,COALESCE(j.message,'') COLLATE utf8mb4_unicode_ci,'/hesap-aktarimi' COLLATE utf8mb4_unicode_ci,
    DATE_FORMAT(j.created_at,'%Y-%m-%d %H:%i:%s') COLLATE utf8mb4_unicode_ci,COALESCE(DATE_FORMAT(j.finished_at,'%Y-%m-%d %H:%i:%s'),'') COLLATE utf8mb4_unicode_ci,j.created_at
  FROM remote_transfer_jobs j WHERE j.created_at>=NOW()-INTERVAL 7 DAY
  UNION ALL
  SELECT CONCAT('laravel:',j.id) COLLATE utf8mb4_unicode_ci,'laravel' COLLATE utf8mb4_unicode_ci,CONCAT(COALESCE(d.alan_adi,'Domain'),' Laravel işi') COLLATE utf8mb4_unicode_ci,
    CONCAT(j.tur,' · ',j.komut) COLLATE utf8mb4_unicode_ci,
    CASE j.status WHEN 'queued' THEN 'bekliyor' WHEN 'running' THEN 'calisiyor'
      WHEN 'success' THEN 'basarili' WHEN 'rolled_back' THEN 'geri_alindi' ELSE 'basarisiz' END COLLATE utf8mb4_unicode_ci,
    j.progress,COALESCE(j.message,'') COLLATE utf8mb4_unicode_ci,CONCAT('/abonelikler/',j.domain_id,'/laravel') COLLATE utf8mb4_unicode_ci,
    DATE_FORMAT(j.created_at,'%Y-%m-%d %H:%i:%s') COLLATE utf8mb4_unicode_ci,COALESCE(DATE_FORMAT(j.finished_at,'%Y-%m-%d %H:%i:%s'),'') COLLATE utf8mb4_unicode_ci,j.created_at
  FROM laravel_deploy_jobs j LEFT JOIN domains d ON d.id=j.domain_id
  WHERE j.created_at>=NOW()-INTERVAL 7 DAY
  UNION ALL
  SELECT CONCAT('antivirus:',j.id) COLLATE utf8mb4_unicode_ci,'antivirus' COLLATE utf8mb4_unicode_ci,CONCAT(COALESCE(d.alan_adi,'Domain'),' zararlı taraması') COLLATE utf8mb4_unicode_ci,
    CONCAT(j.motor,' · ',j.taranan,' dosya') COLLATE utf8mb4_unicode_ci,
    CASE WHEN j.durum='calisiyor' THEN 'calisiyor' WHEN j.durum='bitti' THEN 'basarili' ELSE 'basarisiz' END COLLATE utf8mb4_unicode_ci,
    IF(j.durum='calisiyor',50,100),IF(j.enfekte>0,CONCAT(j.enfekte,' bulgu bulundu'),'') COLLATE utf8mb4_unicode_ci,
    CONCAT('/abonelikler/',j.domain_id,'/imunify') COLLATE utf8mb4_unicode_ci,DATE_FORMAT(j.baslangic,'%Y-%m-%d %H:%i:%s') COLLATE utf8mb4_unicode_ci,
    COALESCE(DATE_FORMAT(j.bitis,'%Y-%m-%d %H:%i:%s'),'') COLLATE utf8mb4_unicode_ci,j.baslangic
  FROM av_taramalar j LEFT JOIN domains d ON d.id=j.domain_id
  WHERE j.baslangic>=NOW()-INTERVAL 7 DAY
) x ORDER BY (durum IN ('bekliyor','calisiyor')) DESC,sirala DESC LIMIT 40`

func (h *Handlers) Liste(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), listeSQL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "işlemler yüklenemedi")
		return
	}
	defer rows.Close()
	out := []Islem{}
	for rows.Next() {
		var x Islem
		if err := rows.Scan(&x.Anahtar, &x.Tur, &x.Baslik, &x.Aciklama, &x.Durum, &x.Ilerleme, &x.Mesaj, &x.Yol, &x.Baslangic, &x.Bitis); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "işlemler okunamadı")
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "işlemler okunamadı")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) Ozet(w http.ResponseWriter, r *http.Request) {
	if h.Senkronize != nil {
		h.Senkronize()
	}
	var aktif, basarisiz, bildirim, kritik int
	err := h.DB.QueryRowContext(r.Context(), `SELECT
    (SELECT COUNT(*) FROM import_jobs WHERE durum IN ('queued','running'))+
    (SELECT COUNT(*) FROM remote_transfer_jobs WHERE status IN ('queued','packaging','downloading','importing'))+
    (SELECT COUNT(*) FROM laravel_deploy_jobs WHERE status IN ('queued','running'))+
    (SELECT COUNT(*) FROM av_taramalar WHERE durum='calisiyor') aktif,
    (SELECT COUNT(*) FROM import_jobs WHERE durum='failed' AND created_at>=NOW()-INTERVAL 1 DAY)+
    (SELECT COUNT(*) FROM remote_transfer_jobs WHERE status='failed' AND created_at>=NOW()-INTERVAL 1 DAY)+
    (SELECT COUNT(*) FROM laravel_deploy_jobs WHERE status IN ('failed','rolled_back') AND created_at>=NOW()-INTERVAL 1 DAY) basarisiz,
    (SELECT COUNT(*) FROM guvenlik_bildirimleri WHERE durum='acik') bildirim,
    (SELECT COUNT(*) FROM guvenlik_bildirimleri WHERE durum='acik' AND seviye='kritik') kritik`).Scan(&aktif, &basarisiz, &bildirim, &kritik)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "işlem özeti yüklenemedi")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"aktif": aktif, "basarisiz": basarisiz, "bildirim": bildirim, "kritik": kritik})
}
