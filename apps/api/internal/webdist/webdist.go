// Package webdist embeds the built web bundle (apps/web/dist) into the server
// binary so a release is ONE file: no WEB_DIST_PATH, no directory to ship next
// to it. scripts/build.sh copies the Vite output into ./bundle before
// `go build`; a bare `go build` without that step still compiles (the directory
// holds a tracked placeholder) and simply has no bundle to serve.
package webdist

import (
	"embed"
	"io/fs"
)

//go:embed all:bundle
var bundle embed.FS

// FS returns the embedded bundle rooted at its index.html, and whether a real
// bundle is present. ok is false for a placeholder-only build (dev checkouts
// that never ran scripts/build.sh), in which case the server falls back to
// WEB_DIST_PATH or serves nothing.
func FS() (fsys fs.FS, ok bool) {
	sub, err := fs.Sub(bundle, "bundle")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return sub, false
	}
	return sub, true
}
