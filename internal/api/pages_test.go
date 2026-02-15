package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestPagesHandler(t *testing.T) {
	t.Run("returns_pages_with_card_title", func(t *testing.T) {
		fs := fstest.MapFS{
			"calls.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html>
<meta name="card-title" content="Calls">
<meta name="card-description" content="View calls">
<meta name="card-order" content="1">`)},
			"talkgroups.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html>
<meta name="card-title" content="Talkgroups">
<meta name="card-order" content="2">`)},
			"index.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html><title>Home</title>`)},
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/pages", nil)
		PagesHandler(fs)(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var pages []pageInfo
		if err := json.Unmarshal(rec.Body.Bytes(), &pages); err != nil {
			t.Fatalf("JSON decode: %v", err)
		}

		// index.html has no card-title, should be excluded
		if len(pages) != 2 {
			t.Fatalf("got %d pages, want 2", len(pages))
		}

		// Sorted by order
		if pages[0].Title != "Calls" {
			t.Errorf("first page title = %q, want Calls", pages[0].Title)
		}
		if pages[0].Description != "View calls" {
			t.Errorf("first page description = %q, want 'View calls'", pages[0].Description)
		}
		if pages[0].Path != "/calls.html" {
			t.Errorf("first page path = %q, want /calls.html", pages[0].Path)
		}
		if pages[0].Order != 1 {
			t.Errorf("first page order = %d, want 1", pages[0].Order)
		}
		if pages[1].Title != "Talkgroups" {
			t.Errorf("second page title = %q, want Talkgroups", pages[1].Title)
		}
	})

	t.Run("skips_directories", func(t *testing.T) {
		fs := fstest.MapFS{
			"subdir/page.html": &fstest.MapFile{Data: []byte(`<meta name="card-title" content="Nested">`)},
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/pages", nil)
		PagesHandler(fs)(rec, req)

		var pages []pageInfo
		json.Unmarshal(rec.Body.Bytes(), &pages)
		if len(pages) != 0 {
			t.Errorf("got %d pages, want 0 (nested files not scanned at root)", len(pages))
		}
	})

	t.Run("skips_non_html_files", func(t *testing.T) {
		fs := fstest.MapFS{
			"style.css": &fstest.MapFile{Data: []byte(`body { color: red; }`)},
			"app.js":    &fstest.MapFile{Data: []byte(`console.log("hi")`)},
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/pages", nil)
		PagesHandler(fs)(rec, req)

		var pages []pageInfo
		json.Unmarshal(rec.Body.Bytes(), &pages)
		if len(pages) != 0 {
			t.Errorf("got %d pages, want 0", len(pages))
		}
	})

	t.Run("empty_fs_returns_empty_array", func(t *testing.T) {
		fs := fstest.MapFS{}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/pages", nil)
		PagesHandler(fs)(rec, req)

		// json.Encoder writes "null\n" for nil slice
		body := rec.Body.String()
		if body != "null\n" && body != "[]\n" {
			t.Errorf("body = %q, want null or []", body)
		}
	})

	t.Run("order_defaults_to_zero", func(t *testing.T) {
		fs := fstest.MapFS{
			"page.html": &fstest.MapFile{Data: []byte(`<meta name="card-title" content="No Order">`)},
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/pages", nil)
		PagesHandler(fs)(rec, req)

		var pages []pageInfo
		json.Unmarshal(rec.Body.Bytes(), &pages)
		if len(pages) != 1 {
			t.Fatalf("got %d pages, want 1", len(pages))
		}
		if pages[0].Order != 0 {
			t.Errorf("order = %d, want 0 (default)", pages[0].Order)
		}
	})
}
