// Package web embeds the built React frontend so the whole application ships
// as a single binary. The dist directory is produced by `vite build`
// (configured to output here) and is NOT committed — it is built in CI before
// `go build`, or locally via `make web`. `make` also drops a placeholder
// index.html (from internal/web/placeholder.html) so the Go package compiles
// even before the frontend has been built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the frontend file tree rooted at the dist directory.
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: failed to sub dist: " + err.Error())
	}
	return sub
}
