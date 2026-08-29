package laravel

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sanalcp/internal/httpx"
)

func (h *Handlers) Discover(w http.ResponseWriter, r *http.Request) {
	d, e := h.domain(r)
	if e != nil {
		httpx.WriteError(w, 404, "domain bulunamadı")
		return
	}
	roots := discoverRoots(d.Home)
	httpx.WriteJSON(w, 200, map[string]any{"kurulumlar": roots})
}

func discoverRoots(home string) []string {
	base := filepath.Join(home, "public_html")
	out := []string{}
	_ = filepath.WalkDir(base, func(p string, de fs.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if de.Type()&os.ModeSymlink != 0 {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !de.IsDir() {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = len(strings.Split(filepath.ToSlash(rel), "/"))
		}
		if depth > 3 {
			return filepath.SkipDir
		}
		if dosyaVar(filepath.Join(p, "artisan")) && dosyaVar(filepath.Join(p, "composer.json")) && dosyaVar(filepath.Join(p, "bootstrap", "app.php")) {
			r := "public_html"
			if rel != "." {
				r += "/" + filepath.ToSlash(rel)
			}
			out = append(out, r)
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(out)
	return out
}
