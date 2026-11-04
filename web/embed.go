// Package web embeds the built single-page application.
//
// The frontend lives in web/ (SvelteKit, static adapter) and its production
// build is written to web/dist. That directory is embedded here so the
// ironman binary serves the UI from "/" without any files on disk. Before the
// frontend is built, web/dist holds only a placeholder index.html so that
// `go build` works.
package web

import "embed"

// Dist holds the static build output (web/dist). Use fs.Sub(Dist, "dist")
// to get a filesystem rooted at the site.
//
//go:embed all:dist
var Dist embed.FS
