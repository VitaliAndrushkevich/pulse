// Package frontend provides the embedded frontend static assets.
// The dist/ directory is populated at build time by copying the SvelteKit
// static build output. For local development without embedding, HasAssets()
// returns false and the SPA routes are not registered.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var FS embed.FS

// HasAssets reports whether the embedded filesystem contains actual frontend
// build output (i.e., more than just the .gitkeep placeholder).
func HasAssets() bool {
	entries, err := fs.ReadDir(FS, "dist")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}
