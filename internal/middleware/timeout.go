package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// IslemZamanAsimi gives streaming upload/download endpoints enough time while
// keeping the short default deadline for the rest of the API. Extending only
// the socket deadline in a handler is insufficient when its parent context has
// already been capped at five minutes.
func IslemZamanAsimi(varsayilan, uzun time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sure := varsayilan
			if uzunIslemRotasi(r.Method, r.URL.Path) {
				sure = uzun
			}
			ctx, cancel := context.WithTimeout(r.Context(), sure)
			defer func() {
				cancel()
				if ctx.Err() == context.DeadlineExceeded {
					w.WriteHeader(http.StatusGatewayTimeout)
				}
			}()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func uzunIslemRotasi(method, path string) bool {
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	switch {
	case method == http.MethodGet && strings.HasSuffix(path, "/files/indir"):
		return true
	case method == http.MethodPost && strings.HasSuffix(path, "/files/upload"):
		return true
	case method == http.MethodGet && strings.Contains(path, "/backups/") && strings.HasSuffix(path, "/indir"):
		return true
	case method == http.MethodPost && strings.Contains(path, "/backups/") &&
		(strings.HasSuffix(path, "/geriyukle") || strings.HasSuffix(path, "/dogrula")):
		return true
	case method == http.MethodPost && (strings.HasSuffix(path, "/ice-aktarim/arsiv") || strings.HasSuffix(path, "/ice-aktarim/sql")):
		return true
	case method == http.MethodGet && strings.Contains(path, "/databases/") && strings.HasSuffix(path, "/yedek"):
		return true
	case method == http.MethodPost && strings.Contains(path, "/databases/") && strings.HasSuffix(path, "/geri-yukle"):
		return true
	case method == http.MethodPost && (path == "/api/v1/admin/transfers/analyze" || path == "/api/v1/admin/transfers/import"):
		return true
	default:
		return false
	}
}
