package skill

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const importSkillBody = `---
key: imported-skill
name: Imported
description: a skill from afar
order: 99
---
Instructions here.
`

const secondSkillBody = `---
key: second-skill
name: Second
description: another one
---
Body two.
`

func tarGzBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportFromSingleMarkdownURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(importSkillBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	installed, err := ImportSkills(dir, srv.URL+"/SKILL.md", false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(installed) != 1 || installed[0] != "imported-skill" {
		t.Errorf("installed = %v", installed)
	}
	b, err := os.ReadFile(filepath.Join(dir, "imported-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Instructions here") {
		t.Errorf("content missing instructions: %q", string(b)[:80])
	}
	if !strings.Contains(string(b), "sp CLI") {
		t.Errorf("imported skill missing CLI usage appendix")
	}
}

func TestImportFromTarGzArchive(t *testing.T) {
	archive := tarGzBytes(t, map[string]string{
		"skills/imported-skill/SKILL.md": importSkillBody,
		"skills/second-skill/SKILL.md":   secondSkillBody,
		"skills/imported-skill/extra.md": "ignored",
		"README.md":                      "no skill here",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	installed, err := ImportSkills(dir, srv.URL+"/skills.tar.gz", false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("installed = %v, want 2 skills", installed)
	}
	for _, key := range installed {
		if _, err := os.Stat(filepath.Join(dir, key, "SKILL.md")); err != nil {
			t.Errorf("%s not installed: %v", key, err)
		}
	}
}

func TestImportFromZipArchive(t *testing.T) {
	archive := zipBytes(t, map[string]string{
		"imported-skill/SKILL.md": importSkillBody,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	installed, err := ImportSkills(dir, srv.URL+"/skills.zip", false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(installed) != 1 || installed[0] != "imported-skill" {
		t.Errorf("installed = %v", installed)
	}
}

func TestImportFromLocalFile(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "pack.tar.gz")
	if err := os.WriteFile(archivePath, tarGzBytes(t, map[string]string{
		"second-skill/SKILL.md": secondSkillBody,
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	installed, err := ImportSkills(target, archivePath, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(installed) != 1 || installed[0] != "second-skill" {
		t.Errorf("installed = %v", installed)
	}
}

func TestImportForceOverwrites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(importSkillBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	if _, err := ImportSkills(dir, srv.URL, false); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if _, err := ImportSkills(dir, srv.URL, false); err == nil {
		t.Error("second import without force succeeded, want ErrSkillExists")
	}
	installed, err := ImportSkills(dir, srv.URL, true)
	if err != nil {
		t.Fatalf("force import: %v", err)
	}
	if len(installed) != 1 {
		t.Errorf("installed = %v", installed)
	}
}

func TestImportRejectsEscapingArchivePaths(t *testing.T) {
	malicious := tarGzBytes(t, map[string]string{
		"../evil/SKILL.md": importSkillBody,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(malicious)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if _, err := ImportSkills(dir, srv.URL, false); err == nil {
		t.Error("escaping archive path accepted")
	}
}

func TestImportUnsupportedContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain text, not a skill"))
	}))
	defer srv.Close()

	if _, err := ImportSkills(t.TempDir(), srv.URL, false); err == nil {
		t.Error("unsupported content accepted")
	}
}

func TestImportFetchErrorAndStatus(t *testing.T) {
	if _, err := ImportSkills(t.TempDir(), "http://127.0.0.1:1/nope", false); err == nil {
		t.Error("connection error not surfaced")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := ImportSkills(t.TempDir(), srv.URL, false); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404 not surfaced: %v", err)
	}
}
