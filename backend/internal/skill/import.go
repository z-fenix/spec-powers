package skill

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// maxImportSize bounds a downloaded or extracted skill payload (a skill
// is markdown; anything larger is a mistake or an attack).
const maxImportSize = 16 << 20

// importClient fetches remote skills. A package-level client with a
// timeout so a hung source cannot stall the CLI forever.
var importClient = &http.Client{Timeout: 30 * time.Second}

// ImportSkills fetches a skill from source and installs it into dir.
// Source is a URL or a local path pointing at a SKILL.md file, a
// tar.gz archive or a zip archive; archives may contain one or more
// skill directories, each holding a SKILL.md at its root. An already
// installed skill of the same key is refused unless force is set.
func ImportSkills(dir, source string, force bool) ([]string, error) {
	data, err := readSource(source)
	if err != nil {
		return nil, err
	}
	files, err := extractSkillFiles(source, data)
	if err != nil {
		return nil, err
	}
	var skills []*Skill
	for name, content := range files {
		s, err := Parse(content)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		skills = append(skills, s)
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("%s contains no skills (no SKILL.md found)", source)
	}
	return InstallSkills(dir, skills, force)
}

// readSource loads the raw bytes of a URL or local file.
func readSource(source string) ([]byte, error) {
	if isURL(source) {
		resp, err := importClient.Get(source)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", source, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: status %s", source, resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxImportSize+1))
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", source, err)
		}
		if len(data) > maxImportSize {
			return nil, fmt.Errorf("source %s exceeds %d bytes", source, maxImportSize)
		}
		return data, nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	if len(data) > maxImportSize {
		return nil, fmt.Errorf("source %s exceeds %d bytes", source, maxImportSize)
	}
	return data, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// extractSkillFiles maps each discovered SKILL.md (named by its
// containing directory, or "SKILL.md" for a bare file) to its content.
func extractSkillFiles(source string, data []byte) (map[string]string, error) {
	switch {
	case bytes.HasPrefix(data, []byte("---\n")) || bytes.HasPrefix(data, []byte("---\r\n")):
		return map[string]string{"SKILL.md": string(data)}, nil
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return extractZip(data)
	case bytes.HasPrefix(data, []byte("\x1f\x8b")):
		return extractTarGz(data)
	default:
		return nil, fmt.Errorf("unsupported source %s (want SKILL.md, .tar.gz or .zip)", source)
	}
}

func extractZip(data []byte) (map[string]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		clean, err := safeArchivePath(f.Name)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(clean, "/SKILL.md") || clean == "SKILL.md" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", f.Name, err)
			}
			content, err := readLimited(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			dir := path.Dir(clean)
			files[dirName(dir)] = string(content)
		}
	}
	return files, nil
}

func extractTarGz(data []byte) (map[string]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		clean, err := safeArchivePath(hdr.Name)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(clean, "/SKILL.md") || clean == "SKILL.md" {
			content, err := readLimited(tr)
			if err != nil {
				return nil, err
			}
			files[dirName(path.Dir(clean))] = string(content)
		}
	}
	return files, nil
}

// safeArchivePath rejects absolute paths, ".." segments and backslash
// separators, so a malicious archive cannot escape the skills directory.
func safeArchivePath(name string) (string, error) {
	name = filepath.ToSlash(name)
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("archive contains absolute path %q", name)
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive contains escaping path %q", name)
	}
	return clean, nil
}

// dirName maps a SKILL.md's containing directory to the install key:
// "skills/brainstorm/SKILL.md" -> "brainstorm", "SKILL.md" -> "SKILL.md"
// (a bare file's key comes from its frontmatter anyway).
func dirName(dir string) string {
	if dir == "." {
		return "SKILL.md"
	}
	return dir
}

func readLimited(r io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(r, maxImportSize+1))
	if err != nil {
		return nil, fmt.Errorf("read archive entry: %w", err)
	}
	if len(content) > maxImportSize {
		return nil, fmt.Errorf("archive entry exceeds %d bytes", maxImportSize)
	}
	return content, nil
}
