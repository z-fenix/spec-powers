package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSPAHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := SPAHandler(dir)

	t.Run("root serves index", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusOK || w.Body.String() != "<html>spa</html>" {
			t.Fatalf("root: %d %q", w.Code, w.Body.String())
		}
	})

	t.Run("existing asset served", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
		if w.Code != http.StatusOK || w.Body.String() != "console.log(1)" {
			t.Fatalf("asset: %d %q", w.Code, w.Body.String())
		}
	})

	t.Run("client-side route falls back to index", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/issues/abc", nil))
		if w.Code != http.StatusOK || w.Body.String() != "<html>spa</html>" {
			t.Fatalf("fallback: %d %q", w.Code, w.Body.String())
		}
	})

	t.Run("missing asset is not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/nope.js", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("missing asset: %d", w.Code)
		}
	})
}
