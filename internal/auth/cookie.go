package auth

import (
	"net/http"
	"strings"
)

// OturumCerezAdi: panel oturum JWT'sini taşıyan HttpOnly çerezin adı.
//
// Oturum eskiden localStorage'da tutulup her istekte Authorization başlığına
// yazılıyordu. Panelde çalışan HERHANGİ bir XSS o değeri okuyup dışarı
// sızdırabilirdi ve çalınan token, ömrü dolana kadar başka bir makineden de
// geçerliydi. HttpOnly çerez JavaScript'e kapalıdır; XSS artık oturumu
// okuyamaz, yalnızca kurbanın tarayıcısı üzerinden istek atabilir.
const OturumCerezAdi = "sanalcp_oturum"

// istekHTTPS: istek tarayıcıya HTTPS olarak mı göründü?
//
// Panel nginx arkasında çalışır ve TLS'i nginx sonlandırır, yani r.TLS burada
// daima nil'dir; şema bilgisi X-Forwarded-Proto ile gelir (bkz.
// internal/nginxconf/_panel.conf, proxy_set_header X-Forwarded-Proto $scheme).
//
// Başlık sahtelenirse ne olur: sonuç yalnızca çerezin DAHA kısıtlı olmasıdır
// (Secure işaretlenir, düz HTTP üzerinde gönderilmez). Tehlikeli yön olan
// "Secure'u düşürme" mümkün değil, çünkü varsayılan zaten Secure değil.
func istekHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// oturumCerezi: ortak çerez şablonu. Yazma ve silme aynı Path/SameSite/Secure
// üçlüsünü kullanmalıdır — tarayıcı çerezi bu üçlüye göre eşler, biri
// tutmazsa silme isteği ESKİ çerezi yerinde bırakıp yanına ikinci bir çerez
// yazar ve kullanıcı çıkış yapamaz.
func oturumCerezi(r *http.Request, deger string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:   OturumCerezAdi,
		Value:  deger,
		Path:   "/",
		MaxAge: maxAge,
		// JavaScript okuyamaz — bu değişikliğin bütün amacı.
		HttpOnly: true,
		// Taze kurulumda panel SSL kurulana kadar düz HTTP üzerinden açılır;
		// koşulsuz Secure o aşamada girişi tamamen kırardı.
		Secure: istekHTTPS(r),
		// Strict: çerez yalnız panelin kendi kaynağından doğan isteklerde
		// gider. Panel bir SPA olduğu için bu bir şeyi bozmaz — HTML ve JS
		// zaten kimlik istemeden yüklenir, ardından SPA'nın attığı XHR'ler
		// same-site sayılır ve çerezi taşır.
		SameSite: http.SameSiteStrictMode,
	}
}

// OturumCerezYaz: giriş başarılı olduğunda oturum çerezini set eder.
func OturumCerezYaz(w http.ResponseWriter, r *http.Request, token string, omurSaniye int) {
	http.SetCookie(w, oturumCerezi(r, token, omurSaniye))
}

// OturumCerezSil: çıkışta oturum çerezini geçersiz kılar.
func OturumCerezSil(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, oturumCerezi(r, "", -1))
}

// OturumCerezDegeri: istekteki oturum çerezinin ham değeri ("" = çerez yok).
func OturumCerezDegeri(r *http.Request) string {
	c, err := r.Cookie(OturumCerezAdi)
	if err != nil {
		return ""
	}
	return c.Value
}
