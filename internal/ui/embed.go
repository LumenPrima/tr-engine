// Package ui provides embedded static UI files for the tr-engine dashboard.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticFS embed.FS

// StaticFiles returns the embedded static filesystem rooted at "static/".
func StaticFiles() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}
