package siteratelimit

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"sanalcp/internal/httpx"
	"sanalcp/internal/provisioner"
)

type Handlers struct{ DB *sql.DB }
type Ayar struct {
	Profil         string   `json:"profil"`
	IstekDakika    int      `json:"istek_dakika"`
	Burst          int      `json:"burst"`
	BotEngelle     bool     `json:"bot_engelle"`
	IPIstisnalari  []string `json:"ip_istisnalari"`
	YolIstisnalari []string `json:"yol_istisnalari"`
}

func (h *Handlers) Goster(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var alan string
	a := Ayar{Profil: "kapali", IstekDakika: 120, Burst: 30}
	var bot int
	var ips, yollar string
	err := h.DB.QueryRowContext(r.Context(), `SELECT d.alan_adi,COALESCE(x.profil,'kapali'),COALESCE(x.istek_dakika,120),COALESCE(x.burst,30),COALESCE(x.bot_engelle,0),COALESCE(x.ip_istisnalari,''),COALESCE(x.yol_istisnalari,'') FROM domains d LEFT JOIN domain_rate_limits x ON x.domain_id=d.id WHERE d.id=?`, id).Scan(&alan, &a.Profil, &a.IstekDakika, &a.Burst, &bot, &ips, &yollar)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, err.Error())
		return
	}
	a.BotEngelle = bot == 1
	a.IPIstisnalari = satirlar(ips)
	a.YolIstisnalari = satirlar(yollar)
	httpx.WriteJSON(w, 200, map[string]any{"alan_adi": alan, "ayar": a, "olaylar": olaylar(alan, 50)})
}

func (h *Handlers) Kaydet(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		Ayar Ayar `json:"ayar"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		httpx.WriteError(w, 400, "geçersiz gövde")
		return
	}
	a := req.Ayar
	if a.Profil != "kapali" && a.Profil != "dengeli" && a.Profil != "siki" && a.Profil != "ozel" {
		httpx.WriteError(w, 400, "geçersiz profil")
		return
	}
	if a.Profil == "dengeli" {
		a.IstekDakika = 120
		a.Burst = 30
	}
	if a.Profil == "siki" {
		a.IstekDakika = 30
		a.Burst = 10
	}
	if a.IstekDakika < 1 || a.IstekDakika > 60000 || a.Burst < 0 || a.Burst > 10000 {
		httpx.WriteError(w, 400, "limit 1-60000, burst 0-10000 arası olmalı")
		return
	}
	ips, err := ipler(a.IPIstisnalari)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	yollar, err := yollarDogrula(a.YolIstisnalari)
	if err != nil {
		httpx.WriteError(w, 400, err.Error())
		return
	}
	var eski Ayar
	var eskiBot int
	var eskiIP, eskiYol string
	_ = h.DB.QueryRow(`SELECT profil,istek_dakika,burst,bot_engelle,COALESCE(ip_istisnalari,''),COALESCE(yol_istisnalari,'') FROM domain_rate_limits WHERE domain_id=?`, id).Scan(&eski.Profil, &eski.IstekDakika, &eski.Burst, &eskiBot, &eskiIP, &eskiYol)
	bot := 0
	if a.BotEngelle {
		bot = 1
	}
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO domain_rate_limits(domain_id,profil,istek_dakika,burst,bot_engelle,ip_istisnalari,yol_istisnalari) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE profil=VALUES(profil),istek_dakika=VALUES(istek_dakika),burst=VALUES(burst),bot_engelle=VALUES(bot_engelle),ip_istisnalari=VALUES(ip_istisnalari),yol_istisnalari=VALUES(yol_istisnalari)`, id, a.Profil, a.IstekDakika, a.Burst, bot, strings.Join(ips, "\n"), strings.Join(yollar, "\n"))
	if err != nil {
		httpx.WriteError(w, 500, "DB: "+err.Error())
		return
	}
	if err = provisioner.RateLimitGlobalYaz(h.DB); err == nil {
		err = provisioner.RerenderVhost(h.DB, id)
	}
	if err != nil {
		if eski.Profil == "" {
			_, _ = h.DB.Exec(`DELETE FROM domain_rate_limits WHERE domain_id=?`, id)
		} else {
			_, _ = h.DB.Exec(`UPDATE domain_rate_limits SET profil=?,istek_dakika=?,burst=?,bot_engelle=?,ip_istisnalari=?,yol_istisnalari=? WHERE domain_id=?`, eski.Profil, eski.IstekDakika, eski.Burst, eskiBot, eskiIP, eskiYol, id)
		}
		_ = provisioner.RateLimitGlobalYaz(h.DB)
		_ = provisioner.RerenderVhost(h.DB, id)
		httpx.WriteError(w, 500, "nginx doğrulama başarısız; önceki ayar geri yüklendi: "+err.Error())
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true, "ayar": a})
}

func satirlar(s string) []string {
	var o []string
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		if x = strings.TrimSpace(x); x != "" {
			o = append(o, x)
		}
	}
	return o
}
func ipler(in []string) ([]string, error) {
	var o []string
	for _, x := range in {
		for _, s := range satirlar(x) {
			if ip := net.ParseIP(s); ip != nil {
				o = append(o, ip.String())
				continue
			}
			if _, n, e := net.ParseCIDR(s); e == nil {
				o = append(o, n.String())
				continue
			}
			return nil, errors.New("geçersiz IP/CIDR: " + s)
		}
	}
	return o, nil
}
func yollarDogrula(in []string) ([]string, error) {
	var o []string
	for _, x := range in {
		for _, s := range satirlar(x) {
			if !strings.HasPrefix(s, "/") || strings.ContainsAny(s, " \t\"'{};") {
				return nil, errors.New("geçersiz yol istisnası: " + s)
			}
			o = append(o, s)
		}
	}
	return o, nil
}

type olay struct {
	Zaman string `json:"zaman"`
	IP    string `json:"ip"`
	Yol   string `json:"yol"`
	Durum int    `json:"durum"`
}

func olaylar(domain string, n int) []olay {
	f, e := os.Open("/var/log/nginx/" + domain + ".access.log")
	if e != nil {
		return []olay{}
	}
	defer f.Close()
	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		x := s.Text()
		if strings.Contains(x, " 429 ") || strings.Contains(x, " 403 ") {
			lines = append(lines, x)
			if len(lines) > n {
				lines = lines[1:]
			}
		}
	}
	var out []olay
	for _, x := range lines {
		p := strings.Fields(x)
		if len(p) > 8 {
			dur, _ := strconv.Atoi(p[8])
			out = append(out, olay{Zaman: strings.Trim(p[3], "["), IP: p[0], Yol: p[6], Durum: dur})
		}
	}
	return out
}
