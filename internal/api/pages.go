package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
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
