package files

// files_ext.go — Yaz/Rename/Chmod + symlink-aware jail
// (jailJoin orijinali files.go'da; bu dosya ek handler'lar + sıkılastirma)

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sanalcp/internal/archivex"
	"sanalcp/internal/httpx"
	"sanalcp/internal/provisioner"

	"golang.org/x/sys/unix"
)

// NOT: jailJoinStrict KALDIRILDI. Yol'u EvalSymlinks ile ÇÖZÜP resolved bir string
// döndürüyordu; işlem sonradan o string üzerinde yapıldığı için kontrol ile işlem
// arasında ara-bileşen symlink'e takas edilebiliyordu (TOCTOU). Mutasyon yolları
// zaten openat2'ye taşınmıştı, okuma/exec yolları da taşındı — geriye kullanan
// kalmadı. Yeni bir yol işlemi için safeio.go'daki *Beneath yardımcılarını veya
// dış araçlar için dogrulanmisYol+tenantKomut ikilisini kullanın; bu fonksiyonu
// geri getirmeyin.

// ----- Yaz (editor save) -----

type yazReq struct {
	Yol    string `json:"yol"`
	Icerik string `json:"icerik"`
}

func (h *Handlers) Yaz(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req yazReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if len(req.Icerik) > 5*1024*1024 {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "5 MB üstü editor ile kaydedilemez")
		return
	}
	// TOCTOU symlink-güvenli yazma: hedefi openat2 ile aç (ara-bileşen/leaf symlink REDDEDİLİR),
	// fd'ye yaz, fd üzerinden tenant'a chown (bkz. safeio.go). Mevcut dosyanın izinleri korunur
	// (open create-dışında mode'a dokunmaz); yeni dosya 0644. Eski os.WriteFile(abs) resolved-
	// string üzerinde çalışıp ara-dizin symlink takasıyla jail-dışına yazmaya kandırılabilirdi.
	if err := writeBeneath(home, req.Yol, []byte(req.Icerik), 0644, sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "yazma: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"yol":   req.Yol,
		"boyut": len(req.Icerik),
	})
}

// ----- Rename / Move -----

type renameReq struct {
	Eski string `json:"eski"`
	Yeni string `json:"yeni"`
}

func (h *Handlers) Rename(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req renameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if p := relClean(req.Eski); p == "" || p == "." {
		httpx.WriteError(w, http.StatusBadRequest, "ana ev dizini taşınamaz")
		return
	}
	if p := relClean(req.Yeni); p == "" || p == "." {
		httpx.WriteError(w, http.StatusBadRequest, "ana ev dizini taşınamaz")
		return
	}
	// TOCTOU symlink-güvenli taşıma: kaynak+hedef PARENT'larını openat2 ile pinle, Renameat
	// ile taşı (rename final-bileşen symlink'ini takip etmez). Hedef ara-dizinler O_NOFOLLOW
	// mkdir-p ile oluşturulur (bkz. safeio.go). Eski os.Rename(eski, yeni) resolved-string'ler
	// üzerinde çalışıp ara-dizin symlink takasıyla jail-dışına taşımaya kandırılabilirdi.
	if err := renameBeneath(home, req.Eski, req.Yeni, sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rename: "+err.Error())
		return
	}
	_ = chownTreeBeneath(home, req.Yeni, sk) // taşınan öğeyi tenant'a chown (symlink-güvenli)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "eski": req.Eski, "yeni": req.Yeni})
}

// ----- Chmod -----

type chmodReq struct {
	Yol string `json:"yol"`
	Mod string `json:"mod"` // "0644" gibi octal string
}

func (h *Handlers) Chmod(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req chmodReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	mod := strings.TrimPrefix(req.Mod, "0")
	n, err := strconv.ParseUint(mod, 8, 32)
	if err != nil || n > 0o777 {
		httpx.WriteError(w, http.StatusBadRequest, "mod oktal olmalı (0000-0777)")
		return
	}
	// TOCTOU symlink-güvenli chmod: hedefi openat2 ile aç (ara-bileşen/leaf symlink REDDEDİLİR),
	// Fchmod (bkz. safeio.go). Eski os.Chmod(abs) resolved-string üzerinde çalışıp ara-dizin
	// symlink takasıyla jail-dışı (ör. /etc) dosyaya chmod'a kandırılabilirdi (LPE).
	if err := chmodBeneath(home, req.Yol, uint32(n)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "chmod: "+err.Error())
		return
	}
	_ = chownTreeBeneath(home, req.Yol, sk) // sahiplik domain user'ında kalsın (symlink-güvenli)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "yol": req.Yol, "mod": req.Mod})
}

// IzinSifirla: POST /domains/{id}/files/izin-sifirla — CloudPanel'in
// "clpctl system:permissions:reset" karşılığı. public_html'i (kök klasör + tüm alt
// klasör/dosyalar) sağlama sırasında kullanılan CANONICAL izinlere döndürür: dizinler
// 0750, dosyalar 0644, sahiplik sitenin kendi kullanıcısı (bkz. provisioner.Provision,
// aynı değerler). Symlink'ler chmod/chown'lanmaz (bkz. safeio.go chmodTreeBeneath —
// jail-dışı bir hedefe symlink varsa dokunulmadan atlanır).
//
// Yalnız public_html hedeflenir: home'un kardeşleri (logs/tmp/ssl/.cron) panelin kendi
// yönettiği dizinlerdir, kullanıcı dosya bozulmasından etkilenmez ve burada sıfırlanmaz.
func (h *Handlers) IzinSifirla(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	if err := chownTreeBeneath(home, "public_html", sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "sahiplik sıfırlanamadı: "+err.Error())
		return
	}
	if err := chmodTreeBeneath(home, "public_html", 0750, 0644); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "izinler sıfırlanamadı: "+err.Error())
		return
	}
	// nginx erişim ACL'i (u:nginx:rX + default-ACL) chmod'un ardından yeniden uygulanır —
	// olası bir yedek geri-yükleme/rsync --acls'siz işlem ACL'leri silmiş olabilir.
	provisioner.HardenHomePermsRecursive(filepath.Join(home, "public_html"))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

var _ = errors.New // keep import

// ----- Extract (ZIP / TAR / TAR.GZ aç) -----

type extractReq struct {
	Yol   string `json:"yol"`   // arşivin yolu
	Hedef string `json:"hedef"` // çıkarılacak dizin (opsiyonel; boşsa arşivin dizini)
}

func (h *Handlers) Extract(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req extractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	// Arşivin kendisi: symlink-güvenli doğrula (openat2) — eski os.Stat(jailJoinStrict(...))
	// resolved-string üzerindeydi.
	abs, err := dogrulanmisYol(home, req.Yol)
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "dosya bulunamadı")
		return
	}
	if info, serr := statBeneath(home, req.Yol); serr != nil || info.IsDir() {
		httpx.WriteError(w, http.StatusBadRequest, "dosya bulunamadı veya klasör")
		return
	}

	hedef := req.Hedef
	if hedef == "" {
		hedef = filepath.Dir(req.Yol)
	}
	hedefRel := relClean(hedef)
	// Hedef dizini symlink-güvenli oluştur (her bileşen Mkdirat+O_NOFOLLOW) — eski
	// os.MkdirAll(hedefAbs) bir ara-bileşen symlink'ini izleyip jail DIŞINDA dizin açardı.
	if err := mkdirAllBeneath(home, hedefRel, sk); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "mkdir hedef: "+err.Error())
		return
	}
	hedefAbs, err := dogrulanmisYol(home, hedefRel)
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "hedef güvenli değil (symlink?)")
		return
	}
	// GÜVENLİK: hedef dizini çıkarmadan ÖNCE tenant kullanıcısına devret ki
	// çıkarma root DEĞİL, tenant olarak (DAC altında) çalışabilsin. chownTreeBeneath
	// fd-özyinelemelidir (symlink takip etmez); eski `chown -R` yol üzerinde çalışıyordu.
	_ = chownTreeBeneath(home, hedefRel, sk)

	low := strings.ToLower(abs)
	if strings.HasSuffix(low, ".gz") && archivex.TuruBelirle(low) == archivex.TurBilinmeyen {
		// Tek dosyalık .gz: üye yolu yoktur; tek risk çıktı dosyasının symlink
		// üzerinden dışarı yazması.
		rel := filepath.Join(hedefRel, strings.TrimSuffix(filepath.Base(abs), ".gz"))
		// openat2: yalnız leaf değil, YOLUN TÜM bileşenleri symlink'e karşı korunur.
		// Eski os.OpenFile(...O_NOFOLLOW) yalnız son bileşeni koruyordu — ara bir
		// dizin symlink'e takas edilirse çıktı jail dışına yazılabilirdi.
		gzOut, gzErr := openAt2Beneath(home, rel, unix.O_CREAT|unix.O_WRONLY|unix.O_TRUNC, 0644)
		if gzErr != nil {
			httpx.WriteError(w, statusFromFsErr(gzErr), "gz hedef güvenli değil")
			return
		}
		defer gzOut.Close()
		fchownRestoreFd(home, gzOut, sk)
		var eb bytes.Buffer
		// gunzip de tenant kimliğinde koşar: doğrulama-exec arasındaki yarışta bile
		// jail dışı bir kaynak dosya açılamaz (arşivin kendisi root'a okutulmaz).
		gzCtx, gzCancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer gzCancel()
		gzc, cerr := tenantKomut(gzCtx, sk, "gunzip", "-k", "-c", "--", abs)
		if cerr != nil {
			httpx.WriteError(w, http.StatusBadRequest, cerr.Error())
			return
		}
		gzc.Stdout = gzOut
		gzc.Stderr = &eb
		if runErr := gzc.Run(); runErr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "extract: "+strings.TrimSpace(eb.String()))
			return
		}
	} else {
		// zip / tar ailesi: ORTAK güvenli-extract helper (çift savunma:
		// tenant-user DAC + üye-yolu doğrulama, symlink/hardlink reddi).
		tur := archivex.TuruBelirle(low)
		if tur == archivex.TurBilinmeyen {
			httpx.WriteError(w, http.StatusBadRequest, "desteklenmeyen format (zip, tar, tar.gz/tgz, tar.bz2, tar.xz, gz, rar)")
			return
		}
		if out, exErr := archivex.GuvenliCikar(abs, hedefAbs, sk); exErr != nil {
			msg := exErr.Error()
			if strings.TrimSpace(out) != "" {
				msg += ": " + strings.TrimSpace(out)
			}
			httpx.WriteError(w, http.StatusBadRequest, "extract: "+msg)
			return
		}
	}

	// İzole ortam: çıkartılan tüm dosyaları domain user'ına chown (+ SELinux context).
	// chown artık fd-özyinelemeli ve symlink-güvenli (bkz. safeio.go) — eski
	// `chown -R <yol>`, hedef bir symlink'e takas edilirse jail dışında çalışabilirdi.
	_ = chownTreeBeneath(home, hedefRel, sk)
	// restorecon/setfacl'ın fd karşılığı yok, yol üzerinden çalışmak zorundalar.
	// restorecon ağacı lstat ile gezer (bağı takip etmez); setfacl ise varsayılanda
	// takip edebildiği için -P (physical) AÇIKÇA verilir — aksi hâlde ağaçtaki bir
	// bağ üzerinden jail dışı bir dosyaya ACL yazılabilirdi.
	_, _ = exec.Command("restorecon", "-R", hedefAbs).CombinedOutput()
	// Per-user izin modeli (FIX 1): çıkarılan içeriğe nginx okuma-ACL'ini teyit et. docroot'un
	// default-ACL'i genelde bunu zaten miras verir; hedef docroot-dışıysa/ACL yoksa garanti.
	// setfacl yoksa (acl paketi eksik) sessiz atlanır — dosyalar tenant'ta, site diğer yolla servis edilir.
	if _, err := exec.LookPath("setfacl"); err == nil {
		_, _ = exec.Command("setfacl", "-P", "-R", "-m", "u:nginx:rX", hedefAbs).CombinedOutput()
		_, _ = exec.Command("setfacl", "-P", "-R", "-d", "-m", "u:nginx:rX", hedefAbs).CombinedOutput()
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"yol":   req.Yol,
		"hedef": hedef,
	})
}

// ----- Copy / Move (toplu) -----

type bulkMoveCopyReq struct {
	Kaynaklar []string `json:"kaynaklar"`
	Hedef     string   `json:"hedef"` // hedef KLASÖR (içine konulacak)
}

func (h *Handlers) Copy(w http.ResponseWriter, r *http.Request) {
	h.bulkMoveCopy(w, r, false)
}

func (h *Handlers) Move(w http.ResponseWriter, r *http.Request) {
	h.bulkMoveCopy(w, r, true)
}

func (h *Handlers) bulkMoveCopy(w http.ResponseWriter, r *http.Request, move bool) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req bulkMoveCopyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	// TOCTOU symlink-güvenli: hedef klasörü openat2 ile doğrula (ara-bileşen symlink REDDEDİLİR).
	hedefRel := relClean(req.Hedef)
	if ok, err := isDirBeneath(home, hedefRel); err != nil || !ok {
		httpx.WriteError(w, http.StatusBadRequest, "hedef klasör değil")
		return
	}

	basarili := 0
	hatalar := []string{}
	for _, k := range req.Kaynaklar {
		kRel := relClean(k)
		if kRel == "" || kRel == "." {
			hatalar = append(hatalar, k+": geçersiz kaynak")
			continue
		}
		dstRel := filepath.Join(hedefRel, filepath.Base(kRel))
		if dstRel == kRel {
			hatalar = append(hatalar, k+": kaynak ve hedef aynı")
			continue
		}
		// Symlink-güvenli taşı/kopyala: parent'lar openat2 ile pinlenir, hiçbir symlink takip
		// edilmez; kopyada jail-dışı symlink İÇERİĞİ okunmaz (bilgi sızması yok) (bkz. safeio.go).
		var op error
		if move {
			op = renameBeneath(home, kRel, dstRel, sk)
		} else {
			op = copyTreeBeneath(home, kRel, dstRel, sk)
		}
		if op != nil {
			hatalar = append(hatalar, k+": "+op.Error())
			continue
		}
		_ = chownTreeBeneath(home, dstRel, sk)
		basarili++
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": len(hatalar) == 0, "basarili": basarili, "hatalar": hatalar,
	})
}

// copyAny/copyFile/copyDir KALDIRILDI: path-tabanlı eski kopya (os.Open/os.OpenFile string
// yol üzerinde) TOCTOU symlink-yarışına açıktı. Kopya artık safeio.go'daki symlink-güvenli
// copyTreeBeneath (openat2 + O_NOFOLLOW, jail-dışı symlink içeriğini sızdırmaz) ile yapılır.

// ----- Arşivle (seçili dosyaları zip yap) -----

type archiveReq struct {
	Kaynaklar []string `json:"kaynaklar"`
	CiktiYol  string   `json:"cikti_yol"` // örn /public_html/yedek.zip
	Format    string   `json:"format"`    // zip | tar.gz
}

func (h *Handlers) Archive(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req archiveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	if len(req.Kaynaklar) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "kaynak yok")
		return
	}
	if req.Format == "" {
		req.Format = "zip"
	}
	// 🔴 GÜVENLİK (iki ayrı kusur, birlikte düzeltildi):
	//
	//  1) `zip` VARSAYILAN OLARAK sembolik bağı TAKİP EDER ve hedefin İÇERİĞİNİ
	//     arşive koyar (`-y` bayrağı tam olarak bunu kapatmak için vardır). Arşivleme
	//     root olarak koştuğu için tenant, home'una `ln -s /etc/shadow link` koyup o
	//     dizini arşivleyerek indirdiğinde /etc/shadow'u okuyabiliyordu — YARIŞ BİLE
	//     GEREKMEDEN, güvenilir biçimde. (tar bağı bağ olarak saklar, o yol temizdi.)
	//  2) Araçlar root koşuyordu; internal/archivex çıkarma tarafında kurduğu
	//     "tenant kimliğinde çalıştır" (DAC) katmanı burada uygulanmamıştı.
	//
	// Düzeltme: `-y` + `--` (seçenek/dosya ayracı) + runuser ile tenant kimliği +
	// üst-sınırlı süre. Artık bir bağ hedefi arşive giremez; girse bile tenant'ın
	// zaten okuyabildiği bir şey olurdu.
	ciktiAbs, err := ciktiHazirla(home, req.CiktiYol, sk)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "cikti: "+err.Error())
		return
	}

	kaynakAbs := make([]string, 0, len(req.Kaynaklar))
	for _, k := range req.Kaynaklar {
		kAbs, err := dogrulanmisYol(home, k)
		if err != nil {
			continue // jail dışı / symlink bileşenli kaynak sessizce atlanır
		}
		kaynakAbs = append(kaynakAbs, kAbs)
	}
	if len(kaynakAbs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "geçerli kaynak yok")
		return
	}

	// Arşivleme büyük ağaçlarda uzun sürer; istek işleyen goroutine'i ve alt süreci
	// süresiz açık bırakmamak için üst sınır.
	arCtx, arCancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer arCancel()

	// Yolların hepsi ciktiHazirla/dogrulanmisYol'dan gelir, yani daima "/home/c_..."
	// ile başlar — hiçbiri "-" ile başlayamayacağı için argüman-olarak-seçenek
	// (option injection) mümkün değil. Info-ZIP zip'in "--" ayracını desteklediği
	// kesin olmadığından orada kullanılmaz; GNU tar'da destekli, bırakıldı.
	var argv []string
	if req.Format == "zip" {
		// -y: sembolik bağı BAĞ olarak sakla (hedefi dereference ETME) — bu bayrak
		// olmadan zip, bağın gösterdiği dosyanın İÇERİĞİNİ arşive koyar.
		argv = append([]string{"zip", "-r", "-q", "-y", ciktiAbs}, kaynakAbs...)
	} else { // tar.gz
		argv = append([]string{"tar", "-czf", ciktiAbs, "--"}, kaynakAbs...)
	}
	cmd, err := tenantKomut(arCtx, sk, argv...)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError,
			req.Format+": "+strings.TrimSpace(string(out)))
		return
	}
	// Arşiv zaten tenant kimliğinde üretildi; yalnız SELinux etiketini düzelt.
	_, _ = exec.Command("restorecon", ciktiAbs).CombinedOutput()

	var boyut int64
	if info, serr := statBeneath(home, req.CiktiYol); serr == nil {
		boyut = info.Size()
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok": true, "cikti_yol": req.CiktiYol, "boyut": boyut,
	})
}

// ----- Yeni boş dosya -----

type yeniDosyaReq struct {
	Yol string `json:"yol"`
}

func (h *Handlers) YeniDosya(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	var req yeniDosyaReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "geçersiz gövde")
		return
	}
	// TOCTOU symlink-güvenli yeni-dosya: openat2 + O_EXCL (ara-bileşen/leaf symlink REDDEDİLİR),
	// fd üzerinden tenant'a chown (bkz. safeio.go). Eski os.Stat+os.OpenFile(abs) resolved-string
	// üzerinde çalışıp ara-dizin symlink takasına açıktı.
	if err := createExclBeneath(home, req.Yol, sk); err != nil {
		if errors.Is(err, os.ErrExist) {
			httpx.WriteError(w, http.StatusConflict, "dosya zaten var")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "yazma: "+err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"ok": true, "yol": req.Yol})
}

// ----- Boyut hesapla (du -sb) -----

func (h *Handlers) BoyutHesapla(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	rel := r.URL.Query().Get("yol")
	abs, err := dogrulanmisYol(home, rel)
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "geçersiz yol")
		return
	}
	// du tüm alt ağacı gezer; çok dosyalı bir dizinde istek işleyen goroutine'i
	// süresiz bloklamasın diye ÜST SINIRLI çalıştırılır. Ayrıca tenant kimliğinde
	// koşar: doğrulama ile exec arasındaki yarışta bile jail dışı bir ağaç
	// ölçülemez (bkz. safeio.go'daki çift-savunma notu).
	duCtx, duCancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer duCancel()
	cmd, err := tenantKomut(duCtx, sk, "du", "-sb", "--", abs)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// du, tenant kimliğinde koştuğu için okunamayan bir alt dizinde sıfır-olmayan
	// kod döndürür ama TOPLAMI yine de basar. Çıkış kodunu değil, ayrıştırılabilir
	// bir toplam olup olmadığını ölçüt al — aksi hâlde tek bir izin pürüzü bütün
	// "boyut hesapla" işlemini 500'e düşürürdü.
	out, err := cmd.Output()
	parts := strings.Fields(string(out))
	if len(parts) < 1 {
		mesaj := "du çıktı parse edilemedi"
		if err != nil {
			mesaj = "du: " + err.Error()
		}
		httpx.WriteError(w, http.StatusInternalServerError, mesaj)
		return
	}
	var b int64
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			break
		}
		b = b*10 + int64(c-'0')
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"yol":     rel,
		"boyut_b": b,
	})
}

// ----- Arama (recursive find by name pattern) -----

func (h *Handlers) Ara(w http.ResponseWriter, r *http.Request) {
	home, sk, err := h.home(r)
	if err != nil {
		httpx.WriteError(w, statusFromErr(err), err.Error())
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"icerik": []any{}, "toplam": 0})
		return
	}
	rel := r.URL.Query().Get("yol")
	if rel == "" {
		rel = "/"
	}
	baseAbs, err := dogrulanmisYol(home, rel)
	if err != nil {
		httpx.WriteError(w, statusFromFsErr(err), "geçersiz yol")
		return
	}

	// Güvenlik: q sadece dosya adı pattern, shell injection olmaması için iname kullan
	q = strings.ReplaceAll(q, "*", "")
	q = strings.ReplaceAll(q, "?", "")
	pattern := "*" + q + "*"

	// 🔴 find ÜST SINIRSIZ çalışıyordu: milyonlarca dosyalı bir ağaçta istek işleyen
	// goroutine'i ve alt süreci süresiz meşgul ederdi — kimliği doğrulanmış bir
	// tenant, ucu tekrar tekrar çağırarak sunucuyu CPU/proses tükenmesine
	// sürükleyebilirdi. Artık süre sınırlı ve tenant kimliğinde koşar.
	// (-maxdepth yok: arama derinliği kullanıcı beklentisi; sınır süre + 500 sonuç.)
	araCtx, araCancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer araCancel()
	cmd, err := tenantKomut(araCtx, sk, "find", baseAbs, "-iname", pattern,
		"-printf", "%p\t%s\t%y\t%T@\n")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, _ := cmd.Output() // zaman aşımı/kısmi çıktı: elde olan sonuçlar döndürülür
	results := []Entry{}
	for _, ln := range strings.Split(string(out), "\n") {
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		absp := parts[0]
		size := int64(0)
		for _, c := range parts[1] {
			if c < '0' || c > '9' {
				break
			}
			size = size*10 + int64(c-'0')
		}
		tip := "dosya"
		if parts[2] == "d" {
			tip = "klasor"
		} else if parts[2] == "l" {
			tip = "sembolik"
		}
		// rel yol home altina goreceli
		yolRel := strings.TrimPrefix(absp, home)
		if yolRel == "" {
			yolRel = "/"
		}
		// find'in bastigi mutlak yol yerine home'a-goreli yolu kullan: statBeneath
		// openat2 ile acar, yani jail disina isaret eden bir girdi stat'lanamaz.
		info, _ := statBeneath(home, yolRel)
		mod := "0644"
		var degisme string
		if info != nil {
			mod = "0" + strconv.FormatInt(int64(info.Mode().Perm()), 8)
			degisme = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
		}
		results = append(results, Entry{
			Adi: filepath.Base(absp), Yol: filepath.ToSlash(yolRel),
			Tip: tip, BoyutB: size, Mod: mod, Degisme: degisme,
		})
		if len(results) >= 500 {
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"icerik": results, "toplam": len(results), "q": q,
	})
}
