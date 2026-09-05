package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cliImportSkill = `---
key: remote-skill
name: Remote
description: arrived over http
---
Do the thing.
`

func tarGzSingle(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSkillInstallFromURL(t *testing.T) {
	chdirTemp(t)
	srv := serve(t, tarGzSingle(t, "remote-skill/SKILL.md", cliImportSkill))
	dir := t.TempDir()

	code, out, errOut := runCLI(t, "skill", "install", "--from", srv.URL+"/pack.tar.gz", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "remote-skill") {
		t.Errorf("output missing installed key: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "remote-skill", "SKILL.md")); err != nil {
		t.Fatalf("imported skill missing: %v", err)
	}
}

func TestSkillInstallFromRejectsKeys(t *testing.T) {
	chdirTemp(t)
	srv := serve(t, []byte("---\nkey: x\n---\nbody"))
	code, _, _ := runCLI(t, "skill", "install", "--from", srv.URL, "brainstorm", "--dir", t.TempDir())
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestSkillInstallFromFetchFailure(t *testing.T) {
	chdirTemp(t)
	code, _, errOut := runCLI(t, "skill", "install", "--from", "http://127.0.0.1:1/nope", "--dir", t.TempDir())
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if errOut == "" {
		t.Error("expected an error message on stderr")
	}
}

func TestSkillInstallFromBadPointer(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "install", "--from", "ftp://example.com/skill.tar.gz", "--dir", t.TempDir())
	if code != 1 {
		t.Errorf("exit = %d, want 1 (unsupported source)", code)
	}
}
