package auth

import (
	"context"
	"net/http"
)

// Doğrulanmış kimliğin TEK taşıyıcısı: istek context'i.
//
// Anahtar neden middleware'de değil burada: bu paketteki handler'ların da
// (profile.go, dashboard.go) kimliğe ihtiyacı var, ama middleware zaten auth'u
// import ettiği için ters yönde import edilemez. Anahtar middleware'de
// durduğu sürece o handler'lar kimliği kendileri türetmek zorunda kalıyordu ve
// bunu Authorization başlığını yeniden ayrıştırarak yapıyorlardı — oturum
// HttpOnly çereze taşınınca başlık ortadan kalktı ve yedi uç birden
// "oturum yok" diyerek 401 dönmeye başladı. Anahtarı buraya almak o
// çoğaltmayı tamamen ortadan kaldırıyor.
type ctxKey int

const claimsKey ctxKey = 1

// ClaimsContext: doğrulanmış claim'leri context'e yerleştirir.
//
// Üretimde YALNIZCA middleware.RequireAuth çağırır; oraya konan değer, imza
// doğrulaması VE veritabanı kontrollerinden (hesap durumu, rol, auth_version,
// boşta kalma) geçmiş demektir. Handler'lar bu değeri yeniden türetmemeli.
func ClaimsContext(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFrom: istekteki doğrulanmış claim'ler (yoksa nil).
func ClaimsFrom(r *http.Request) *Claims {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*Claims)
	return c
}
