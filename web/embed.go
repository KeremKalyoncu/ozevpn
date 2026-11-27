package web

import "embed"

// FS embeds only the static web panel assets (excluding Go source files).
//
//go:embed *.html *.css *.js *.svg
var FS embed.FS
