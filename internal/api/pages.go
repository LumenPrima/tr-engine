package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type pageInfo struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

var (
	cardTitleRe = regexp.MustCompile(`<meta\s+name="card-title"\s+content="([^"]*)"`)
	cardDescRe  = regexp.MustCompile(`<meta\s+name="card-description"\s+content="([^"]*)"`)
	cardOrderRe = regexp.MustCompile(`<meta\s+name="card-order"\s+content="(\d+)"`)
)

// PagesHandler returns a handler that scans the web FS for HTML pages with
// card-title meta tags and returns them as JSON. Pages without card-title
// are excluded (e.g. index.html). Rescans on every request so new files
// are picked up in dev mode without restart.
func PagesHandler(webFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pages []pageInfo

		entries, err := fs.ReadDir(webFS, ".")
		if err != nil {
			http.Error(w, "failed to read web directory", http.StatusInternalServerError)
			return
		}

		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".html") {
				continue
			}

			// Read first 2KB — meta tags are always near the top
			f, err := webFS.Open(name)
			if err != nil {
				continue
			}
			buf := make([]byte, 2048)
			n, _ := f.Read(buf)
			f.Close()
			head := string(buf[:n])

			// Only include pages that opt in with card-title
			m := cardTitleRe.FindStringSubmatch(head)
			if m == nil {
				continue
			}

			p := pageInfo{
				Path:  "/" + name,
				Title: m[1],
			}
			if m := cardDescRe.FindStringSubmatch(head); m != nil {
				p.Description = m[1]
			}
			if m := cardOrderRe.FindStringSubmatch(head); m != nil {
				p.Order, _ = strconv.Atoi(m[1])
			}
			pages = append(pages, p)
		}

		if pages == nil {
			pages = []pageInfo{}
		}

		sort.Slice(pages, func(i, j int) bool {
			return pages[i].Order < pages[j].Order
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pages)
	}
}

// protectedFiles are core files that must never be overwritten by SavePageHandler.
var protectedFiles = map[string]bool{
	"index.html":      true,
	"auth.js":         true,
	"theme-config.js": true,
	"theme-engine.js": true,
	"playground.html": true,
}

// sanitizeFilename strips directory components and non-safe characters.
// Returns empty string if nothing valid remains.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			b.WriteRune(c)
		}
	}
	name = b.String()
	// Strip leading dots (no hidden files)
	name = strings.TrimLeft(name, ".")
	return name
}

// savePageRequest is the JSON body for POST /pages.
type savePageRequest struct {
	Filename  string `json:"filename"`
	HTML      string `json:"html"`
	Overwrite bool   `json:"overwrite"`
}

// SavePageHandler writes an HTML page to the web directory on disk.
// Returns 503 if the server is using embedded web files (no disk web/ directory).
func SavePageHandler(webDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if webDir == "" {
			WriteErrorDetail(w, http.StatusServiceUnavailable,
				"embedded web files",
				"Server is using embedded web files; create a web/ directory and restart to enable saving pages.")
			return
		}

		var req savePageRequest
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		// Sanitize and validate filename
		filename := sanitizeFilename(req.Filename)
		if filename == "" {
			WriteError(w, http.StatusBadRequest, "filename is empty after sanitization")
			return
		}
		if !strings.HasSuffix(filename, ".html") {
			filename += ".html"
		}
		if len(filename) > 100 {
			WriteError(w, http.StatusBadRequest, "filename too long (max 100 characters)")
			return
		}

		// Reject protected files
		if protectedFiles[filename] {
			WriteErrorDetail(w, http.StatusForbidden, "protected file",
				fmt.Sprintf("%s is a core file and cannot be overwritten", filename))
			return
		}

		// Validate HTML size (1 MB max)
		if len(req.HTML) > 1<<20 {
			WriteError(w, http.StatusBadRequest, "HTML content too large (max 1 MB)")
			return
		}

		// Require card-title meta tag
		if !cardTitleRe.MatchString(req.HTML) {
			WriteErrorDetail(w, http.StatusBadRequest, "missing card-title",
				`HTML must contain <meta name="card-title" content="..."> for page discovery`)
			return
		}

		destPath := filepath.Join(webDir, filename)

		// Check for existing file when overwrite is false
		if !req.Overwrite {
			if _, err := os.Stat(destPath); err == nil {
				WriteErrorDetail(w, http.StatusConflict, "file already exists",
					fmt.Sprintf("%s already exists; set overwrite=true to replace it", filename))
				return
			}
		}

		// Atomic write: write to .tmp then rename
		tmpPath := destPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(req.HTML), 0644); err != nil {
			WriteErrorDetail(w, http.StatusInternalServerError, "write failed", err.Error())
			return
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			os.Remove(tmpPath)
			WriteErrorDetail(w, http.StatusInternalServerError, "rename failed", err.Error())
			return
		}

		// Extract title for response
		title := ""
		if m := cardTitleRe.FindStringSubmatch(req.HTML); m != nil {
			title = m[1]
		}

		WriteJSON(w, http.StatusCreated, map[string]string{
			"path":    "/" + filename,
			"title":   title,
			"message": "Page saved",
		})
	}
}
