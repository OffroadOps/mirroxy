// Package web shares the download window assets with the desktop client.
package web

import "embed"

//go:embed *.html *.js *.css *.png *.ico *.json
var Files embed.FS
