// Package webui embeds the static frontend assets so the service ships as a
// single self-contained binary.
package webui

import "embed"

//go:embed index.html app.js style.css
var FS embed.FS
