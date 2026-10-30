package middleware

import (
	"net/http"
	"strconv"

	"sanalcp/internal/httpx"
	"sanalcp/internal/planfeatures"

	"github.com/go-chi/chi/v5"
)

// FeatureScope applies existing domain ownership/suspension policy before the
// customer management grant. Admin and reseller authority is not plan-limited.
func FeatureScope(module planfeatures.Module) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return MusteriScope(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
			if !RequireFeature(w, r, id, module) {
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// RequireFeature is for derived resources after their domain ownership check.
// It deliberately reads the current plan instead of trusting session claims.
func RequireFeature(w http.ResponseWriter, r *http.Request, domainID int64, module planfeatures.Module) bool {
	c := ClaimsFrom(r)
	if c == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "yetkilendirme gerekli")
		return false
	}
	if c.Role == RolAdmin || c.Role == RolBayi {
		return true
	}
	if c.Role != RolMusteri {
		httpx.WriteError(w, http.StatusForbidden, "yetkiniz yok")
		return false
	}
	features, err := planfeatures.Resolve(r.Context(), scopeDB, domainID)
	if err != nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"hata": "plan modül izinleri doğrulanamadı", "code": "feature_unavailable", "feature": module,
		})
		return false
	}
	if !features.Allows(module) {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]any{
			"hata": "bu modül hizmet paketinizde devre dışı", "code": "feature_disabled", "feature": module,
		})
		return false
	}
	return true
}

// DomainFeatures is registered behind MusteriScope and reports actual grants
// even when the caller's management role exempts them from plan enforcement.
func DomainFeatures(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	features, err := planfeatures.Resolve(r.Context(), scopeDB, id)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "plan modül izinleri doğrulanamadı")
		return
	}
	c := ClaimsFrom(r)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"features": features, "enforced": c != nil && c.Role == RolMusteri})
}
