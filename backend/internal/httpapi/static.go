package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SPAHandler serves static files from dir with a single-page-app fallback:
// requests that do not match an existing file (client-side routes) get
// index.html, while missing files under a real prefix (assets) 404 so bad
// asset references are visible.
func SPAHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean("/" + r.URL.Path)
		if _, err := os.Stat(filepath.Join(dir, clean)); err == nil {
			fs.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
