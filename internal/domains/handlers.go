package domains

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/cliapi"
	"sanalcp/internal/dns"
	"sanalcp/internal/domainek"
	"sanalcp/internal/hesaplar"
	"sanalcp/internal/httpx"
	"sanalcp/internal/kaynaklimit"
	"sanalcp/internal/kota"
	"sanalcp/internal/mail"
	"sanalcp/internal/middleware"
	"sanalcp/internal/osfam"
	"sanalcp/internal/provisioner"
	"sanalcp/internal/redis"
	"sanalcp/internal/tenanthesap"

	"github.com/go-chi/chi/v5"
)

type Domain struct {
	ID       int64  `json:"id"`
	AlanAdi  string `json:"alan_adi"`
	PHPSurum string `json:"php_surum"`
	SSL      bool   `json:"ssl"`
	SSLBitis string `json:"ssl_bitis,omitempty"`
	// SSLKaynak: "letsencrypt" | "self-signed" | "" (bilinmiyor — sütun
	// eklenmeden önce kurulmuş eski kayıtlar). Arayüz self-signed'ı ayrı
	// gösterir: ziyaretçi tarayıcıda tam sayfa sertifika uyarısı görür,
	// yani site fiilen erişilemez durumdadır.
	SSLKaynak       string `json:"ssl_kaynak,omitempty"`
	Durum           string `json:"durum"`
	SistemKullanici string `json:"sistem_kullanici"`
	BoyutKB         int64  `json:"boyut_kb"`
	TrafikKB        int64  `json:"trafik_kb"`
	Olusturulma     string `json:"olusturulma"`
	IPv4            string `json:"ipv4"`
	FTPHost         string `json:"ftp_host"`
	FTPUser         string `json:"ftp_user"`
	DBHost          string `json:"db_host"`
	DBUser          string `json:"db_user"`
	DBAdi           string `json:"db_adi"`
	WebRoot         string `json:"web_root"`
	IsDemo          bool   `json:"is_demo"`
	Notlar          string `json:"notlar,omitempty"`
	PlanID          *int64 `json:"plan_id,omitempty"`
	SiteTipi        string `json:"site_tipi,omitempty"`
	PlanAd          string `json:"plan_ad,omitempty"`
	SshErisim       bool   `json:"ssh_erisim"`
	Askida          bool   `json:"askida"`
	// BayiAdi: domain'in bağlı olduğu müşterinin sahibi bayi kullanıcı adı —
	// boş = doğrudan admin'e ait (bkz. migrations/0048 "NULL = doğrudan admin").
	BayiAdi      string `json:"bayi_adi,omitempty"`
	BayiPaketAdi string `json:"bayi_paket_adi,omitempty"` // bayinin bağlı olduğu bayi paketi (varsa)
	// CustomerID/MusteriAd: domain'in bağlı olduğu müşteri kaydı. "Sahip
	// değiştir" (TopluSahip), bu müşterinin owner_user_id değerini güncelleyerek
	// domaini bayi kapsamına alır; customer_id ve sistem kullanıcısı değişmez.
	// sistem_kullanici bu işlemle DEĞİŞMEZ — dosyalar/FTP/DB eski tenant'ta kalır.
	CustomerID *int64 `json:"customer_id,omitempty"`
	MusteriAd  string `json:"musteri_ad,omitempty"`
}

type Handlers struct {
	DB   *sql.DB
	IPv4 string
}

const selectAll = `SELECT d.id, d.alan_adi, d.sistem_kullanici, d.php_surum, d.ssl_aktif,
  COALESCE(DATE_FORMAT(d.ssl_bitis,'%Y-%m-%d'),''), d.durum, d.ipv4, d.ftp_host, d.ftp_user,
  d.db_host, d.db_user, d.db_adi, d.web_root, d.boyut_kb, d.trafik_kb, d.is_demo,
  COALESCE(d.notlar,''), DATE_FORMAT(d.olusturulma,'%Y-%m-%d'),
  d.plan_id, COALESCE(p.ad,''), d.ssh_erisim, COALESCE(d.askida,0),
  COALESCE(bu.username,''), COALESCE(brp.ad,''), COALESCE(d.site_tipi,'php'),
  COALESCE(d.ssl_kaynak,''), d.customer_id, COALESCE(cu.ad,'')
  FROM domains d
  LEFT JOIN service_plans p ON p.id=d.plan_id
  LEFT JOIN customers cu ON cu.id = d.customer_id
  LEFT JOIN users bu ON bu.id = cu.owner_user_id
  LEFT JOIN reseller_limits brl ON brl.user_id = bu.id
  LEFT JOIN reseller_plans brp ON brp.id = brl.reseller_plan_id`

func scan(rs interface{ Scan(...any) error }) (Domain, error) {
	var d Domain
	var ssl, demo, sshE, askida int
	var planID, custID sql.NullInt64
	err := rs.Scan(&d.ID, &d.AlanAdi, &d.SistemKullanici, &d.PHPSurum, &ssl,
		&d.SSLBitis, &d.Durum, &d.IPv4, &d.FTPHost, &d.FTPUser,
		&d.DBHost, &d.DBUser, &d.DBAdi, &d.WebRoot, &d.BoyutKB, &d.TrafikKB, &demo,
		&d.Notlar, &d.Olusturulma,
		&planID, &d.PlanAd, &sshE, &askida,
		&d.BayiAdi, &d.BayiPaketAdi, &d.SiteTipi, &d.SSLKaynak,
		&custID, &d.MusteriAd)
	d.SSL = ssl == 1
	d.IsDemo = demo == 1
	d.SshErisim = sshE == 1
	d.Askida = askida == 1
	if planID.Valid {
		v := planID.Int64
		d.PlanID = &v
	}
	if custID.Valid {
		v := custID.Int64
		d.CustomerID = &v
	}
	return d, err
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	// Kapsam daraltması sorgunun İÇİNDE yapılır: admin tüm domainleri görür,
	// bayi yalnız kendi müşterilerininkini, müşteri yalnız kendi domainini.
	// Satır satır sahiplik kontrolü burada işe yaramaz — filtrelenmemiş bir
	// liste zaten tüm tenant adlarını sızdırırdı.
	kosul, arg := middleware.KapsamSQL(r, "d")
	rows, err := h.DB.QueryContext(r.Context(), selectAll+kosul+" ORDER BY d.id DESC", arg...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "veritabanı hatası: "+err.Error())
		return
	}
	defer rows.Close()
	out := make([]Domain, 0)
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
			return
		}
		out = append(out, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE d.id=?", id)
	d, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

type createReq struct {
	AlanAdi    string `json:"alan_adi"`
	PHPSurum   string `json:"php_surum"`
	CustomerID *int64 `json:"customer_id,omitempty"`
	PlanID     *int64 `json:"plan_id,omitempty"`
	// SiteTipi: php | wordpress | statik | reverse_proxy. Boş gelirse
	// "php" varsayılır — eski istemciler ve API çağrıları bozulmaz.
	SiteTipi       string `json:"site_tipi,omitempty"`
	ProxyScheme    string `json:"proxy_scheme,omitempty"`
	ProxyHost      string `json:"proxy_host,omitempty"`
	ProxyPort      int    `json:"proxy_port,omitempty"`
	ProxyWebSocket *bool  `json:"proxy_websocket,omitempty"`
	// BayiUserID: domainin bağlanacağı bayi (users.id, rol=reseller).
	// SADECE ADMİN geçebilir; nil = doğrudan admin'e ait.
	//
	// 🔴 Bayi rolündeki çağıranlarda BU ALAN YOK SAYILIR — aksi hâlde bir bayi
	// kendi domainini başka bir bayiye yazabilir ya da (daha kötüsü) başka bir
	// bayinin kotasını harcayabilirdi. Bayi için sahip daima kendisidir.
	BayiUserID *int64 `json:"bayi_user_id,omitempty"`
}

// gecerliSiteTipi: bilinmeyen bir değer sessizce "php"ye düşer. Tip yalnızca
// NE SAĞLANACAĞINI belirler; yanlış bir değer yüzünden domain oluşturmayı
// reddetmek, kullanıcıyı hiçbir şey kazandırmadan bloklardı.
func gecerliSiteTipi(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "statik":
		return "statik"
	case "wordpress":
		return "wordpress"
	case "reverse_proxy":
		return "reverse_proxy"
	default:
		return "php"
	}
}

func proxyHedefDogrula(req *createReq) error {
	if req.SiteTipi != "reverse_proxy" {
		return nil
	}
	req.ProxyScheme = strings.ToLower(strings.TrimSpace(req.ProxyScheme))
	if req.ProxyScheme == "" {
		req.ProxyScheme = "http"
	}
	if req.ProxyScheme != "http" && req.ProxyScheme != "https" {
		return fmt.Errorf("proxy protokolü http veya https olmalı")
	}
	req.ProxyHost = strings.ToLower(strings.TrimSpace(req.ProxyHost))
	if req.ProxyHost == "localhost" {
		req.ProxyHost = "127.0.0.1"
	}
	if req.ProxyHost != "127.0.0.1" {
		return fmt.Errorf("ilk sürümde proxy hedefi yalnız 127.0.0.1 olabilir")
	}
	if req.ProxyPort < 1024 || req.ProxyPort > 65535 {
		return fmt.Errorf("proxy portu 1024–65535 arasında olmalı")
	}
	if req.ProxyPort == 8080 || req.ProxyPort == 8443 || req.ProxyPort == 10080 {
		return fmt.Errorf("bu port SanalCP tarafından ayrılmıştır")
	}
	return nil
}

// sahipBayiCoz: yeni domainin bağlanacağı sahip bayiyi belirler.
// Dönen hata metni boş değilse çağıran 400 yazıp durmalıdır.
//
// 🔴 YETKİ KURALI: istekteki bayi_user_id'ye SADECE ADMİN güvenilir. Bayi
// rolündeki çağıranda alan tamamen YOK SAYILIR ve sahip daima çağıranın
// kendisi olur — aksi hâlde bir bayi domaini başka bir bayiye yazabilir ya da
// onun kotasını harcayabilirdi.
//
// Admin'in verdiği id de doğrulanır: hedef gerçekten rol=reseller ve aktif
// olmalı. Rastgele bir users.id kabul edilseydi domain, bayi OLMAYAN bir
// hesaba bağlanır ve kapsam sorguları (middleware.BayiDomainiMi) onu bir daha
// bulamazdı — domain kimsenin göremediği bir yere düşerdi.
//
// nil sahip = doğrudan admin'e ait (customers.owner_user_id NULL).
func (h *Handlers) sahipBayiCoz(r *http.Request, istenen *int64) (*int64, string) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		return nil, ""
	}
	switch c.Role {
	case middleware.RolBayi:
		uid := c.UserID
		return &uid, ""
	case middleware.RolAdmin:
		if istenen == nil || *istenen <= 0 {
			return nil, "" // "Yönetici (bana ait)"
		}
		var rol, durum string
		if e := h.DB.QueryRowContext(r.Context(),
			`SELECT role, status FROM users WHERE id=?`, *istenen).Scan(&rol, &durum); e != nil {
			return nil, "seçilen bayi bulunamadı"
		}
		if rol != middleware.RolBayi {
			return nil, "seçilen hesap bayi değil"
		}
		if durum != "active" {
			return nil, "seçilen bayi hesabı askıya alınmış"
		}
		return istenen, ""
	}
	return nil, ""
}

type createResp struct {
	Domain
	OluşturulanParolalar struct {
		FTP string `json:"ftp"`
		DB  string `json:"db"`
	} `json:"olusturulan_parolalar"`
	// Nameserver: müşterinin kayıt şirketine gireceği çift. Oluşturma
	// yanıtında dönmesinin sebebi, bu bilginin tam da parolalarla birlikte
	// "bir kez göster, kaydettir" anında lazım olması — ayrıca çift, domaini
	// oluşturan BAYİYE göre değişebildiği için sonradan tahmin edilemez.
	// Nameserver tanımlı değilse alan boş kalır ve panel göstermez.
	Nameserver *nsCifti `json:"nameserver,omitempty"`
}

type nsCifti struct {
	NS1 string `json:"ns1"`
	NS2 string `json:"ns2"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	req.AlanAdi = strings.ToLower(strings.TrimSpace(req.AlanAdi))
	// Plan seçilmediyse varsayılan planı ata — kaynak limitleri HER domaine uygulanır
	// (plan-driven default). Varsayılan yoksa plansız devam eder (limit uygulanmaz).
	if req.PlanID == nil {
		var defID int64
		if e := h.DB.QueryRowContext(r.Context(),
			`SELECT id FROM service_plans WHERE varsayilan=1 ORDER BY id LIMIT 1`).Scan(&defID); e == nil && defID > 0 {
			req.PlanID = &defID
		}
	}
	if req.PHPSurum == "" {
		req.PHPSurum = "8.3"
		// Plan seçildiyse PHP sürümünü plandan miras al
		if req.PlanID != nil {
			var pv string
			if e := h.DB.QueryRowContext(r.Context(), `SELECT php_surum FROM service_plans WHERE id=?`, *req.PlanID).Scan(&pv); e == nil && strings.TrimSpace(pv) != "" {
				req.PHPSurum = pv
			}
		}
	}
	if err := provisioner.ValidateDomain(req.AlanAdi); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var existing int64
	err := h.DB.QueryRowContext(r.Context(), `SELECT id FROM domains WHERE alan_adi=?`, req.AlanAdi).Scan(&existing)
	if err == nil {
		httpx.WriteError(w, http.StatusConflict, "bu alan adı zaten kayıtlı")
		return
	}

	// 1) Linux user + nginx + PHP pool
	if err := kota.CheckDomainEklenebilir(r.Context(), h.DB, nil); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	// Bayi kotası: bayinin TÜM müşterilerindeki domain toplamını sınırlar
	// (müşteri planındaki max_domain'den ayrı bir tavan).
	if c := middleware.ClaimsFrom(r); c != nil && c.Role == middleware.RolBayi {
		if err := kota.CheckBayiDomainEklenebilir(r.Context(), h.DB, c.UserID); err != nil {
			httpx.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		// Disk/trafik kotası: dolmuşsa yeni domain açılamaz. Var olan siteler
		// etkilenmez — bunlar "yeni kaynak ekleme" kapısıdır, kesme değil.
		if err := kota.CheckBayiDiskKotasi(r.Context(), h.DB, c.UserID); err != nil {
			httpx.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := kota.CheckBayiTrafikKotasi(r.Context(), h.DB, c.UserID); err != nil {
			httpx.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		// Bayi yalnız kendi müşterisine domain açabilir.
		if req.CustomerID == nil {
			httpx.WriteError(w, http.StatusBadRequest, "domain bir müşteriye bağlanmalı")
			return
		}
		if !middleware.BayiMusterisiMi(r, c.UserID, *req.CustomerID) {
			httpx.WriteError(w, http.StatusForbidden, "bu müşteriye erişim yok")
			return
		}
		// Bayi izinli_planlar ile belirli hizmet planlarına kısıtlanmış olabilir
		// (bkz. migrations/0056, internal/users LimitKaydet).
		if req.PlanID != nil {
			if err := kota.CheckBayiPlanIzinli(r.Context(), h.DB, c.UserID, *req.PlanID); err != nil {
				httpx.WriteError(w, http.StatusForbidden, err.Error())
				return
			}
			// Fazla satış kapalıysa (bkz. migrations/0057) taahhüt toplamı da
			// kendi disk/trafik limitini aşamaz — açıksa (varsayılan) bu adım
			// no-op'tur, yalnız yukarıdaki gerçek kullanım kontrolleri geçerlidir.
			if err := kota.CheckBayiTaahhutKotasi(r.Context(), h.DB, c.UserID, *req.PlanID); err != nil {
				httpx.WriteError(w, http.StatusForbidden, err.Error())
				return
			}
		}
	}
	siteTipi := gecerliSiteTipi(req.SiteTipi)
	req.SiteTipi = siteTipi
	if err := proxyHedefDogrula(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Sahip bayiyi SAĞLAMADAN ÖNCE çöz ve doğrula: aşağıdaki Provision Linux
	// kullanıcısı, nginx vhost'u ve FPM havuzu oluşturur. Doğrulamayı sonraya
	// bırakmak, geçersiz bir bayi id'sinde yarım sağlanmış bir domain bırakırdı.
	sahipBayi, sahipHata := h.sahipBayiCoz(r, req.BayiUserID)
	if sahipHata != "" {
		httpx.WriteError(w, http.StatusBadRequest, sahipHata)
		return
	}

	pr, err := provisioner.Provision(req.AlanAdi, req.PHPSurum)
	if err != nil {
		log.Printf("provision %q başarısız: %v", req.AlanAdi, err)
		httpx.WriteError(w, http.StatusInternalServerError, "sağlama başarısız: "+err.Error())
		return
	}

	// Statik sitede veritabanı SAĞLANMAZ; domains satırındaki db_* alanları da
	// boş kalır. Ad üretip veritabanını açmamak, panelde var olmayan bir
	// veritabanını varmış gibi gösterirdi.
	dbUser, dbName := "", ""
	if siteTipi != "statik" && siteTipi != "reverse_proxy" {
		dbUser = pr.SistemKullanici + "_db"
		dbName = pr.SistemKullanici + "_main"
	}

	// 2) domains satırı
	res, err := h.DB.ExecContext(r.Context(),
		`INSERT INTO domains(alan_adi, sistem_kullanici, php_surum, ssl_aktif, durum, ipv4,
		   ftp_host, ftp_user, db_host, db_user, db_adi, web_root, is_demo, site_tipi)
		 VALUES(?,?,?,0,'aktif',?,?,?, 'localhost',?,?,?, 0, ?)`,
		req.AlanAdi, pr.SistemKullanici, req.PHPSurum, h.IPv4,
		h.IPv4, pr.SistemKullanici, dbUser, dbName, pr.WebRoot, siteTipi)
	if err != nil {
		_ = provisioner.Deprovision(req.AlanAdi, pr.SistemKullanici)
		httpx.WriteError(w, http.StatusInternalServerError, "DB kayıt başarısız: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if siteTipi == "reverse_proxy" {
		ws := 1
		if req.ProxyWebSocket != nil && !*req.ProxyWebSocket {
			ws = 0
		}
		if _, err := h.DB.ExecContext(r.Context(),
			`UPDATE domains SET web_backend='reverse-proxy', proxy_scheme=?, proxy_host=?, proxy_port=?, proxy_websocket=? WHERE id=?`,
			req.ProxyScheme, req.ProxyHost, req.ProxyPort, ws, id); err != nil {
			_ = provisioner.Deprovision(req.AlanAdi, pr.SistemKullanici)
			_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domains WHERE id=?`, id)
			httpx.WriteError(w, http.StatusInternalServerError, "proxy kaydı başarısız: "+err.Error())
			return
		}
		if err := provisioner.RerenderVhost(h.DB, id); err != nil {
			_ = provisioner.Deprovision(req.AlanAdi, pr.SistemKullanici)
			_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domains WHERE id=?`, id)
			httpx.WriteError(w, http.StatusInternalServerError, "proxy vhost başarısız: "+err.Error())
			return
		}
	}

	if req.CustomerID != nil || req.PlanID != nil {
		_, _ = h.DB.ExecContext(r.Context(),
			`UPDATE domains SET customer_id=?, plan_id=? WHERE id=?`,
			req.CustomerID, req.PlanID, id)
	}

	// Panel hesabı + müşteri kaydını HEMEN üret. Eskiden bunu yalnız açılışta
	// çalışan gocis.MusteriHesapGocu yapıyordu: domain eklendikten sonra hesap
	// Kullanıcılar ekranında görünmüyor, ancak panel yeniden başlatılınca
	// beliriyordu. Çağıran açıkça bir müşteri belirttiyse ona dokunulmaz
	// (Hazirla yalnız customer_id IS NULL olan domainleri bağlar).
	//
	// 🔴 Sahiplik: domaini bayi oluşturduysa müşteri O BAYİYE bağlanmalı.
	// owner_user_id NULL kalsaydı müşteri doğrudan admin'e ait olur ve bayi
	// kendi eklediği domaini göremezdi (middleware.BayiDomainiMi).
	if req.CustomerID == nil {
		if _, err := tenanthesap.Hazirla(r.Context(), h.DB, pr.SistemKullanici, req.AlanAdi, sahipBayi); err != nil {
			// Domain sağlandı ve kaydedildi; hesap zinciri kurulamadıysa istek
			// başarısız SAYILMAZ — açılıştaki doldurma bunu yine yakalar.
			log.Printf("tenant hesabı hazırlanamadı (domain=%d, tenant=%s): %v", id, pr.SistemKullanici, err)
		}
	}
	// Plan seçildiyse nginx web-sunucusu varsayılanlarını domain'e tohumla + vhost yenile
	if req.PlanID != nil {
		h.applyPlanNginxDefaults(r.Context(), id, *req.PlanID, pr.SistemKullanici, req.PHPSurum)
		// Domain hazır yanıtı verilmeden kaynak limitleri ve per-tenant FPM
		// tamamlanmalıdır. Arka planda çalıştırmak aktarım sağlık kontrolüyle
		// yarışıyordu: 502 gören rollback domaini silerken goroutine silinmiş
		// vhost'u tekrar yazıp nginx -t'yi global olarak bozabiliyordu.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if err := kaynaklimit.UygulaHepsi(ctx, h.DB, id); err != nil {
			log.Printf("kaynaklimit apply (create) domain=%d: %v", id, err)
		}
		cancel()
		// UygulaHepsi bazı alt-sistem hatalarını best-effort olarak loglayıp nil
		// döndürür. API'nin "oluşturuldu" demesi için en azından seçilen FPM
		// socket'inin gerçekten oluşmuş olması gerekir; aksi halde ilk istek 502
		// verir ve uzak aktarım sağlık kontrolüyle yarışır.
		socket, socketErr := provisioner.PHPSocketFor(pr.SistemKullanici, req.PHPSurum)
		fi, statErr := os.Stat(socket)
		if socketErr != nil || statErr != nil || fi.Mode()&os.ModeSocket == 0 {
			_ = provisioner.Deprovision(req.AlanAdi, pr.SistemKullanici)
			_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domains WHERE id=?`, id)
			httpx.WriteError(w, http.StatusInternalServerError, "PHP-FPM hazır değil; domain geri alındı")
			return
		}
	}

	// 3) FTP hesap (random parola)
	ftpPass := hesaplar.RandomParola(20)
	uidN, gidN := uidGidOf(pr.SistemKullanici)
	if err := hesaplar.FTPCreate(h.DB, id, pr.SistemKullanici, ftpPass, uidN, gidN); err != nil {
		log.Printf("FTP create %q hata: %v", pr.SistemKullanici, err)
	}

	// 4) Default MySQL veritabanı + kullanıcı — STATİK sitede atlanır.
	// Statik HTML'in veritabanına ihtiyacı yok; koşulsuz açmak kullanılmayan
	// bir veritabanı, kullanılmayan bir DB kullanıcısı ve gereksiz bir saldırı
	// yüzeyi bırakırdı. Kullanıcı sonradan ihtiyaç duyarsa Veritabanları
	// sayfasından kendisi ekleyebilir.
	var dbPass string
	if siteTipi != "statik" && siteTipi != "reverse_proxy" {
		dbPass = hesaplar.RandomParola(24)
		if err := hesaplar.MySQLCreateDB(h.DB, id, dbName, dbUser, dbPass); err != nil {
			log.Printf("MySQL create %q hata: %v", dbName, err)
		}
	}

	// 4b) Site kullanıcısı CLI token'ı (db:export/import, cache:purge komutları için)
	if cliToken, err := cliapi.GenerateToken(h.DB, id); err != nil {
		log.Printf("CLI token oluştur %q hata: %v", pr.SistemKullanici, err)
	} else if err := cliapi.WriteTokenFile(pr.SistemKullanici, cliToken, uidN, gidN); err != nil {
		log.Printf("CLI token dosyası yaz %q hata: %v", pr.SistemKullanici, err)
	}

	// 5) DNS şablonu otomatik tohumla + BIND zone yaz + reload
	if _, err := dns.SeedDefaults(r.Context(), h.DB, id, req.AlanAdi, h.IPv4); err != nil {
		log.Printf("DNS SeedDefaults %q hata: %v", req.AlanAdi, err)
	}
	if err := dns.WriteZone(r.Context(), h.DB, id); err != nil {
		log.Printf("DNS WriteZone %q hata: %v", req.AlanAdi, err)
	}

	row := h.DB.QueryRowContext(r.Context(), selectAll+" WHERE d.id=?", id)
	d, _ := scan(row)

	resp := createResp{Domain: d}
	resp.OluşturulanParolalar.FTP = ftpPass
	resp.OluşturulanParolalar.DB = dbPass
	// Yalnız GERÇEK bir nameserver çifti tanımlıysa gösterilir; tanımlı
	// değilken dönen vanity değerler (ns1.<domain>) müşteriye VERİLEMEZ,
	// çünkü her domain için ayrı glue record gerektirir.
	if dns.NSAyarli(r.Context(), h.DB) {
		ns1, ns2 := dns.NameserverCifti(r.Context(), h.DB, d.ID, d.AlanAdi)
		resp.Nameserver = &nsCifti{NS1: ns1, NS2: ns2}
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alanAdi, sk string
	var isDemo int
	var anaDomainID sql.NullInt64
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, is_demo, ana_domain_id FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &isDemo, &anaDomainID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
		return
	}

	// Bu domain bir ek alan adıysa (addon/parked, sk'yi ana hesapla PAYLAŞIYOR),
	// aşağıdaki sk-genelindeki yıkıcı adımlara (Deprovision/SystemdSliceSil/
	// redis.KapatDomain) ASLA girmemeli — bunlar ana hesabın TÜM Linux kullanıcısını
	// silerdi. domainek.DeleteEkDomain kendi (nginx conf + docroot + DNS zone + DB) temizliğini
	// yapar ve döner.
	if anaDomainID.Valid {
		if err := domainek.DeleteEkDomain(r.Context(), h.DB, id, anaDomainID.Int64); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "silme hatası: "+err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"silinen": map[string]string{"alan_adi": alanAdi, "sistem_kullanici": sk},
		})
		return
	}

	// Ana domain siliniyor: altındaki ek alan adlarını (varsa) ÖNCE temizle — DB'de
	// FK CASCADE kasıtlı olarak yok (bkz. 0045 migration notu), aksi halde bunların
	// nginx conf/docroot/DNS zone dosyaları diskte öksüz kalırdı.
	if childRows, err := h.DB.QueryContext(r.Context(), `SELECT id FROM domains WHERE ana_domain_id=?`, id); err == nil {
		var childIDs []int64
		for childRows.Next() {
			var cid int64
			if childRows.Scan(&cid) == nil {
				childIDs = append(childIDs, cid)
			}
		}
		childRows.Close()
		for _, cid := range childIDs {
			if err := domainek.DeleteEkDomain(r.Context(), h.DB, cid, id); err != nil {
				log.Printf("ek alan adı silme uyarısı (domain_id=%d, ek=%d): %v", id, cid, err)
			}
		}
	}

	if isDemo == 0 {
		// MariaDB'deki gerçek DB'leri kaldır (CASCADE FK sadece panel DB metadata'sını siler)
		_ = hesaplar.MySQLDropAllForDomain(h.DB, id)
		// nginx vhost + PHP pool + Linux user + per-tenant FPM servisi (Deprovision içinde)
		if err := provisioner.Deprovision(alanAdi, sk); err != nil {
			log.Printf("deprovision warn (%s): %v", alanAdi, err)
		}
		// Kaynak-limit slice'ını (sanal-<sk>.slice) kaldır (Deprovision FPM'i söktü).
		_ = kaynaklimit.SystemdSliceSil(sk)
		// Redis tenant cache: Valkey ACL user + WP drop-in + cp_domain_redis satırı.
		// cp_domain_redis'te CASCADE FK olmadığı için domain silinince satır orphan kalıyordu.
		redisCtx, redisCancel := context.WithTimeout(r.Context(), 30*time.Second)
		redis.KapatDomain(redisCtx, h.DB, id, sk)
		redisCancel()
		// Mail: mail_domains/mailboxes/mail_aliases zaten domains(id) ON DELETE CASCADE FK'li,
		// DB satırları aşağıdaki DELETE FROM domains ile otomatik silinir. KapatDomain yine de
		// çağrılır (redis.KapatDomain ile aynı simetri) — ileride cascade-dışı bir yan etki eklenirse.
		mail.KapatDomain(h.DB, id, sk)
		// NOT: /var/backups/sanalcp/<sk>/ dizini KASITLI olarak korunur.
		// Müşteri domaini yanlışlıkla silmiş olabilir → yedekler kurtarma için saklanır.
		// (Manuel temizlik için backups.RemoveDomainBackups mevcut.)
	}

	// Orphan temizliği: bu tablolarda FK cascade yok (mevcut kurulumlar için),
	// domain silinince satırlar orphan kalmasın diye açıkça sil.
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domain_trafik WHERE domain_id=?`, id)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM domain_trafik_imlec WHERE domain_id=?`, id)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM wp_bakim WHERE domain_id=?`, id)
	_, _ = h.DB.ExecContext(r.Context(), `DELETE FROM cli_tokens WHERE domain_id=?`, id)

	if _, err := h.DB.ExecContext(r.Context(), `DELETE FROM domains WHERE id=?`, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "silme hatası: "+err.Error())
		return
	}

	// BIND zone temizliği DELETE'ten SONRA: updateZoneIncludes zones.conf'u domains
	// tablosundan yeniden üretir; domain hâlâ tabloda olsaydı (eski sıra) son silinen
	// domainin zone include'u geri yazılırdı (dangling → named reload hatası).
	if isDemo == 0 {
		if err := dns.DeleteZone(r.Context(), h.DB, alanAdi); err != nil {
			log.Printf("DNS DeleteZone warn (%s): %v", alanAdi, err)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"silinen": map[string]string{"alan_adi": alanAdi, "sistem_kullanici": sk},
	})
}

func uidGidOf(u string) (int, int) {
	uu, err := user.Lookup(u)
	if err != nil {
		return 0, 0
	}
	uid, _ := strconv.Atoi(uu.Uid)
	gid, _ := strconv.Atoi(uu.Gid)
	return uid, gid
}

type setPHPReq struct {
	PHPSurum string `json:"php_surum"`
}

func (h *Handlers) SetPHP(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setPHPReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if req.PHPSurum == "" {
		httpx.WriteError(w, http.StatusBadRequest, "php_surum zorunlu")
		return
	}
	var alanAdi, sk, backend, certPath, keyPath, sslKaynak string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, is_demo, COALESCE(web_backend,'php-fpm'), COALESCE(cert_path,''), COALESCE(key_path,''), COALESCE(ssl_kaynak,'') FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &isDemo, &backend, &certPath, &keyPath, &sslKaynak)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin PHP sürümü değiştirilemez")
		return
	}
	socket, err := provisioner.SetPHPVersion(alanAdi, sk, req.PHPSurum, certPath, keyPath, sslKaynak, backend)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "değişim başarısız: "+err.Error())
		return
	}
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET php_surum=? WHERE id=?`, req.PHPSurum, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "php_surum": req.PHPSurum, "socket": socket,
	})
}

// Web backend seçici — "php-fpm" | "apache" | "static"
type setBackendReq struct {
	Backend string `json:"backend"`
}

var gecerliBackendler = map[string]bool{"php-fpm": true, "apache": true, "static": true}

// backendKullanilabilir: bu SUNUCUDA seçilebilir mi?
//
// 🔴 Apache backend'i Debian ailesinde v1'de KAPALI (bkz. osfam.ApacheBackendDestekli).
// Kapı daha önce yalnız yorumlarda anılıyordu, hiçbir yerde UYGULANMIYORDU; Ubuntu
// 24.04'te canlı denendiğinde olan şu oldu:
//
//  1. DB satırı 'apache' olarak GÜNCELLENDİ,
//  2. nginx vhost'una 127.0.0.1:10080 proxy bloğu yazıldı ve reload edildi,
//  3. Apache vhost yazımı /etc/httpd/conf.d yok diye patladı (RHEL yolu),
//  4. istek HTTP 500 + ham iç hata döndü, SİTE 502 OLDU.
//
// Yani sunucuda hiç Apache yokken bile seçenek sunuluyor ve seçilince site
// düşüyordu. Kontrol artık DB'ye DOKUNMADAN ÖNCE yapılır.
func backendKullanilabilir(b string) bool {
	if b == "apache" {
		return osfam.ApacheBackendDestekli()
	}
	return gecerliBackendler[b]
}

// kullanilabilirBackendler: UI'ın göstereceği liste — desteklenmeyen seçenek
// hiç listelenmez (kullanıcıya çalışmayacak bir seçim sunmayalım).
func kullanilabilirBackendler() []string {
	out := []string{"php-fpm"}
	if osfam.ApacheBackendDestekli() {
		out = append(out, "apache")
	}
	return append(out, "static")
}

func (h *Handlers) GetWebBackend(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var backend, siteTipi string
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(web_backend,'php-fpm'), COALESCE(site_tipi,'php') FROM domains WHERE id=?`, id).Scan(&backend, &siteTipi)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
		return
	}
	mevcutlar := kullanilabilirBackendler()
	if siteTipi == "reverse_proxy" {
		mevcutlar = []string{"reverse-proxy"}
	}
	yanit := map[string]any{
		"backend":   backend,
		"mevcutlar": mevcutlar,
	}
	// Neden eksik olduğunu SÖYLE: seçeneğin sessizce yok olması, kullanıcıya
	// panelin bozuk olduğunu düşündürür.
	if !osfam.ApacheBackendDestekli() {
		yanit["apache_notu"] = "Apache backend bu işletim sisteminde henüz desteklenmiyor (yalnızca AlmaLinux/RHEL ailesi)."
	}
	httpx.WriteJSON(w, http.StatusOK, yanit)
}

func (h *Handlers) SetWebBackend(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setBackendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if !gecerliBackendler[req.Backend] {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz backend (php-fpm|apache|static)")
		return
	}
	// 🔴 DB'ye dokunmadan ÖNCE: desteklenmeyen backend seçilirse hiçbir şey değişmesin.
	if !backendKullanilabilir(req.Backend) {
		httpx.WriteError(w, http.StatusNotImplemented,
			"Apache backend bu işletim sisteminde henüz desteklenmiyor (yalnızca AlmaLinux/RHEL ailesi)")
		return
	}
	var alanAdi, sk, phpSurum, siteTipi string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT alan_adi, sistem_kullanici, php_surum, is_demo, COALESCE(site_tipi,'php') FROM domains WHERE id=?`, id).
		Scan(&alanAdi, &sk, &phpSurum, &isDemo, &siteTipi)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma hatası: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin backend'i değiştirilemez")
		return
	}
	if siteTipi == "reverse_proxy" {
		httpx.WriteError(w, http.StatusConflict, "reverse proxy hesabının backend türü değiştirilemez")
		return
	}
	_ = alanAdi
	// 1) DB güncelle
	if _, err := h.DB.ExecContext(r.Context(),
		`UPDATE domains SET web_backend=? WHERE id=?`, req.Backend, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB güncelleme: "+err.Error())
		return
	}
	// 2) Vhost'u yeniden uygula (nginx + apache yöneticisi web_backend'i DB'den okur)
	socket, _ := provisioner.PHPSocketFor(sk, phpSurum)
	if err := provisioner.ApplyVhostForDomain(h.DB, id, socket, phpSurum); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "vhost render: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "backend": req.Backend,
	})
}

// FTP parola değiştir
type setFTPPwReq struct {
	Parola string `json:"parola"`
}

func (h *Handlers) SetFTPPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req setFTPPwReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	if req.Parola == "" {
		req.Parola = hesaplar.RandomParola(20)
	}
	if !hesaplar.ParolaGecerli(req.Parola) {
		httpx.WriteError(w, http.StatusBadRequest, "parola geçersiz karakter (satır sonu) içeriyor")
		return
	}
	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin FTP parolası değiştirilemez")
		return
	}
	if err := hesaplar.FTPUpdatePassword(h.DB, sk, req.Parola); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "FTP parola güncelleme: "+err.Error())
		return
	}
	// SSH açıksa sistem (SSH) parolasını da FTP ile senkronla
	var sshOn int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT ssh_erisim FROM domains WHERE id=?`, id).Scan(&sshOn)
	if sshOn == 1 {
		_ = hesaplar.SyncSSHPassword(h.DB, sk)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "username": sk, "parola": req.Parola,
	})
}

// Veritabanı listele (domain'e ait)
type DBAccount struct {
	ID          int64  `json:"id"`
	DomainID    int64  `json:"domain_id"`
	DBAdi       string `json:"db_adi"`
	DBKullanici string `json:"db_kullanici"`
	DBHost      string `json:"db_host"`
	DBParola    string `json:"db_parola"`
	Olusturulma string `json:"olusturulma"`
}

func (h *Handlers) ListDatabases(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.DB.QueryContext(r.Context(),
		`SELECT id, domain_id, db_name, db_user, db_host, db_pass_plain, DATE_FORMAT(created_at,'%Y-%m-%d %H:%i')
		 FROM db_accounts WHERE domain_id=? ORDER BY id`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB sorgu: "+err.Error())
		return
	}
	defer rows.Close()
	out := make([]DBAccount, 0)
	for rows.Next() {
		var d DBAccount
		if err := rows.Scan(&d.ID, &d.DomainID, &d.DBAdi, &d.DBKullanici, &d.DBHost, &d.DBParola, &d.Olusturulma); err != nil {
			continue
		}
		if dec, err := hesaplar.DecryptDBPassword(d.DBParola); err == nil {
			d.DBParola = dec
		}
		out = append(out, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// createDBReq: "Yeni Veritabanı" istegi.
//
// Otomatik=true (veya hicbir alan verilmezse) → DB adi/kullanici/parola OTOMATIK uretilir
// (eski davranis, geriye uyumlu). Aksi halde musteri OZELLESTIRIR:
//   - DBSonek: DB adi soneki → panel `<sk>_` onekini ZORUNLU ekler (cakisma-guvenli).
//   - KullaniciTipi "yeni": KullaniciSonek gir (onek eklenir); "mevcut": MevcutKullanici sec.
//   - Parola: musteri girer (guclu olmali) VEYA bos → panel guclu rastgele uretir.
type createDBReq struct {
	Otomatik        bool   `json:"otomatik"`
	DBSonek         string `json:"db_sonek"`
	KullaniciTipi   string `json:"kullanici_tipi"` // "yeni" | "mevcut"
	KullaniciSonek  string `json:"kullanici_sonek"`
	MevcutKullanici string `json:"mevcut_kullanici"`
	Parola          string `json:"parola"`
}

func (h *Handlers) CreateDatabase(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req createDBReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz istek gövdesi")
		return
	}
	var sk string
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT sistem_kullanici, is_demo FROM domains WHERE id=?`, id).
		Scan(&sk, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "domain sorgu: "+err.Error())
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğe veritabanı eklenemez")
		return
	}
	if err := kota.CheckDBEklenebilir(r.Context(), h.DB, id); err != nil {
		httpx.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	// Geriye uyumlu: gövde boş / Otomatik=true → hepsini otomatik üret (eski davranış).
	otomatik := req.Otomatik ||
		(req.DBSonek == "" && req.KullaniciSonek == "" && req.MevcutKullanici == "" && req.Parola == "")

	var dbAdi, dbKullanici, parola string
	mevcutKullaniciModu := false

	if otomatik {
		dbAdi = sk + "_ek" + strconv.FormatInt(id, 10)
		dbKullanici = dbAdi
		parola = hesaplar.RandomParola(24)
	} else {
		// --- DB adı: müşteri SONEK verir, panel `<sk>_` önekini ZORUNLU ekler ---
		if req.DBSonek == "" {
			httpx.WriteError(w, http.StatusBadRequest, "veritabanı adı soneki gerekli")
			return
		}
		if !hesaplar.GecerliDBSonek(req.DBSonek) {
			httpx.WriteError(w, http.StatusBadRequest, "geçersiz veritabanı soneki (yalnız küçük harf/rakam/alt-çizgi, 1-32 karakter)")
			return
		}
		dbAdi = sk + "_" + req.DBSonek
		if !hesaplar.GecerliDBKimlik(dbAdi) {
			httpx.WriteError(w, http.StatusBadRequest, "veritabanı adı çok uzun (önek + sonek ≤64 karakter olmalı)")
			return
		}

		// --- Kullanıcı: yeni (sonek) VEYA mevcut (bu domaine ait) ---
		switch req.KullaniciTipi {
		case "mevcut":
			if req.MevcutKullanici == "" || !hesaplar.GecerliDBKimlik(req.MevcutKullanici) {
				httpx.WriteError(w, http.StatusBadRequest, "geçersiz mevcut kullanıcı")
				return
			}
			// Sahiplik: seçilen kullanıcı GERÇEKTEN bu domaine ait olmalı (önek garantisi).
			var n int
			_ = h.DB.QueryRowContext(r.Context(),
				`SELECT COUNT(*) FROM db_accounts WHERE domain_id=? AND db_user=?`, id, req.MevcutKullanici).Scan(&n)
			if n == 0 {
				httpx.WriteError(w, http.StatusBadRequest, "seçilen kullanıcı bu domaine ait değil")
				return
			}
			dbKullanici = req.MevcutKullanici
			mevcutKullaniciModu = true
		default: // "yeni"
			if req.KullaniciSonek == "" {
				httpx.WriteError(w, http.StatusBadRequest, "kullanıcı adı soneki gerekli")
				return
			}
			if !hesaplar.GecerliDBSonek(req.KullaniciSonek) {
				httpx.WriteError(w, http.StatusBadRequest, "geçersiz kullanıcı soneki (yalnız küçük harf/rakam/alt-çizgi, 1-32 karakter)")
				return
			}
			dbKullanici = sk + "_" + req.KullaniciSonek
			if !hesaplar.GecerliDBKimlik(dbKullanici) {
				httpx.WriteError(w, http.StatusBadRequest, "kullanıcı adı çok uzun (önek + sonek ≤64 karakter olmalı)")
				return
			}
			// Yeni kullanıcı için parola: müşteri girer (güçlü) VEYA boş → panel üretir.
			if req.Parola == "" {
				parola = hesaplar.RandomParola(24)
			} else {
				if ok, neden := hesaplar.ParolaGucluMu(req.Parola); !ok {
					httpx.WriteError(w, http.StatusBadRequest, neden)
					return
				}
				parola = req.Parola
			}
		}
	}

	// İsim çakışması → net 409 (duplicate-key 500 yerine).
	var cakisma int
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM db_accounts WHERE db_name=?`, dbAdi).Scan(&cakisma)
	if cakisma > 0 {
		httpx.WriteError(w, http.StatusConflict, "bu isimde bir veritabanı zaten var: "+dbAdi)
		return
	}

	if mevcutKullaniciModu {
		if err := hesaplar.MySQLCreateDBForUser(h.DB, id, dbAdi, dbKullanici); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "DB oluşturma: "+err.Error())
			return
		}
		// Mevcut kullanıcının parolasını yanıtta göster (müşteri zaten sahibi).
		var encParola string
		_ = h.DB.QueryRowContext(r.Context(),
			`SELECT db_pass_plain FROM db_accounts WHERE db_user=? LIMIT 1`, dbKullanici).Scan(&encParola)
		if dec, err := hesaplar.DecryptDBPassword(encParola); err == nil {
			parola = dec
		}
	} else {
		if err := hesaplar.MySQLCreateDB(h.DB, id, dbAdi, dbKullanici, parola); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "DB oluşturma: "+err.Error())
			return
		}
	}

	// Governor/limit: yeni DB-kullanıcısına plan limitlerini uygula (arka planda, best-effort).
	go func(did int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := kaynaklimit.UygulaHepsi(ctx, h.DB, did); err != nil {
			log.Printf("kaynaklimit apply (db-create) domain=%d: %v", did, err)
		}
	}(id)

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"ok": true, "domain_id": id, "db_adi": dbAdi, "db_kullanici": dbKullanici, "db_parola": parola,
	})
}

func (h *Handlers) DeleteDatabase(w http.ResponseWriter, r *http.Request) {
	dbid, _ := strconv.ParseInt(chi.URLParam(r, "dbid"), 10, 64)
	var dbName string
	var domainID int64
	var isDemo int
	err := h.DB.QueryRowContext(r.Context(),
		`SELECT db.db_name, db.domain_id, d.is_demo
		 FROM db_accounts db JOIN domains d ON d.id=db.domain_id
		 WHERE db.id=?`, dbid).Scan(&dbName, &domainID, &isDemo)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "DB kaydı bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "okuma: "+err.Error())
		return
	}
	// IDOR korumasi: bu route'ta {id} domain param'i yok, MusteriScope
	// uygulanamaz — pma.TokenIste'deki gibi manuel kontrol (bkz. Task 11).
	if !middleware.DomainSahibiMi(r, domainID) {
		httpx.WriteError(w, http.StatusNotFound, "DB kaydı bulunamadı")
		return
	}
	if isDemo == 1 {
		httpx.WriteError(w, http.StatusForbidden, "demo aboneliğin DB'si silinemez")
		return
	}

	// Coklu-kullanicili DB: HER kullanicinin erisimini ayri ayri temizle (baska
	// DB'de kullaniliyorsa yalniz revoke, degilse DROP USER) — tek bir dbid'nin
	// kullanicisina odaklanip digerlerini MariaDB'de "hayalet grant" olarak
	// birakmamak icin (bkz. spec: coklu-kullanici destegiyle bu artik sik durum).
	kullanicilar, err := dbKullanicilariGetir(r.Context(), h.DB, domainID, dbName)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı sorgu: "+err.Error())
		return
	}
	for _, u := range kullanicilar {
		var baskaYerde int
		// Hata yutulursa baskaYerde 0 kalır ve dropUser=true olur — başka bir
		// DB'de hâlâ kullanılan MariaDB hesabı DROP edilebilirdi. Kapalı düş.
		if err := h.DB.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM db_accounts WHERE db_user=? AND db_name<>?`, u, dbName).Scan(&baskaYerde); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı kullanım sorgu: "+err.Error())
			return
		}
		if err := hesaplar.MySQLRevokeUser(h.DB, dbName, u, baskaYerde == 0); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "kullanıcı temizliği: "+err.Error())
			return
		}
	}
	if err := hesaplar.MySQLDropDBKeepUser(h.DB, dbName); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "DB silme: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "silinen": dbName})
}

// TopluSahip: birden çok domain'in bağlı olduğu müşteriyi hedef bayiye devret.
// Görünürlük domains.customer_id -> customers.owner_user_id zincirinden
// çözüldüğü için zincirin gerçek bayi sahipliği alanı güncellenir.
type topluSahipReq struct {
	IDs        []int64 `json:"ids"`
	BayiUserID *int64  `json:"bayi_user_id"`
}

func (h *Handlers) TopluSahip(w http.ResponseWriter, r *http.Request) {
	var req topluSahipReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if len(req.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "boş ids")
		return
	}
	hedefBayi, sahipHata := h.sahipBayiCoz(r, req.BayiUserID)
	if sahipHata != "" {
		httpx.WriteError(w, http.StatusBadRequest, sahipHata)
		return
	}
	// IN clause icin placeholder
	placeholders := make([]string, len(req.IDs))
	var sahip any
	if hedefBayi != nil {
		sahip = *hedefBayi
	}
	args := []any{sahip}
	for i, id := range req.IDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	sql := `UPDATE customers c
		JOIN domains d ON d.customer_id = c.id
		SET c.owner_user_id=?
		WHERE d.id IN (` + strings.Join(placeholders, ",") + `)`
	res, err := h.DB.ExecContext(r.Context(), sql, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncelleme: "+err.Error())
		return
	}
	// RowsAffected DEĞİŞEN satırı sayar: zaten o müşteride olan domainlerde 0
	// döner. Arayüz bunu "hiçbir şey olmadı" diye gösterirse yanıltıcı olur,
	// tersine hiç yazılmamış olsa da 0 döner. Bu yüzden asıl ölçüt istenen
	// sahipliğe SAHİP OLAN satır sayısı — tekrar okuyup onu döndürüyoruz.
	n, _ := res.RowsAffected()
	sonKosul := `c.owner_user_id IS NULL`
	sonArgs := []any{}
	if hedefBayi != nil {
		sonKosul = `c.owner_user_id = ?`
		sonArgs = append(sonArgs, *hedefBayi)
	}
	sonArgs = append(sonArgs, args[1:]...)
	var dogrulanan int64
	_ = h.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM domains d JOIN customers c ON c.id=d.customer_id WHERE `+sonKosul+
			` AND d.id IN (`+strings.Join(placeholders, ",")+`)`, sonArgs...).Scan(&dogrulanan)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "guncellenen": n, "dogrulanan": dogrulanan, "istenen": len(req.IDs)})
}

// TopluDurum: aktif/pasif toggle
type topluDurumReq struct {
	IDs   []int64 `json:"ids"`
	Durum string  `json:"durum"` // "aktif" | "pasif"
}

func (h *Handlers) TopluDurum(w http.ResponseWriter, r *http.Request) {
	var req topluDurumReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if len(req.IDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "boş ids")
		return
	}
	if req.Durum != "aktif" && req.Durum != "pasif" {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz durum")
		return
	}
	placeholders := make([]string, len(req.IDs))
	args := []any{req.Durum}
	for i, id := range req.IDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	sql := `UPDATE domains SET durum=? WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	res, err := h.DB.ExecContext(r.Context(), sql, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "güncelleme: "+err.Error())
		return
	}
	n, _ := res.RowsAffected()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "guncellenen": n})
}

// applyPlanNginxDefaults, yeni domain bir plana bağlandığında planın nginx
// varsayılanlarını (FastCGI cache + client_max_body + ek direktifler) domain'in
// nginx_settings satırına yazar ve vhost'u bu ayarlarla yeniden render eder.
// Best-effort: hata olursa domain yine de oluşturulmuş kalır (yalnızca loglanır).
func (h *Handlers) applyPlanNginxDefaults(ctx context.Context, domainID, planID int64, sk, php string) {
	var fc, cmb int
	var ekPlan string
	if err := h.DB.QueryRowContext(ctx,
		`SELECT fastcgi_cache, client_max_body_mb, COALESCE(nginx_ek_direktifler,'')
		   FROM service_plans WHERE id=?`, planID).Scan(&fc, &cmb, &ekPlan); err != nil {
		log.Printf("plan nginx defaults oku (plan=%d): %v", planID, err)
		return
	}
	ek := ""
	if cmb > 0 {
		ek = "client_max_body_size " + strconv.Itoa(cmb) + "m;\n"
	}
	if strings.TrimSpace(ekPlan) != "" {
		ek += ekPlan
	}
	if _, err := h.DB.ExecContext(ctx,
		`INSERT INTO nginx_settings(domain_id, fastcgi_cache, ek_direktifler)
		 VALUES(?,?,?)
		 ON DUPLICATE KEY UPDATE fastcgi_cache=VALUES(fastcgi_cache), ek_direktifler=VALUES(ek_direktifler)`,
		domainID, fc, ek); err != nil {
		log.Printf("nginx_settings tohumla (domain=%d): %v", domainID, err)
		return
	}
	socket, err := provisioner.PHPSocketFor(sk, php)
	if err != nil {
		log.Printf("php socket (domain=%d): %v", domainID, err)
		return
	}
	if err := provisioner.ApplyVhostForDomain(h.DB, domainID, socket, php); err != nil {
		log.Printf("plan vhost yeniden render (domain=%d): %v", domainID, err)
	}
}
