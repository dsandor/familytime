// Package web embeds Family Time's built-in browser UI into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var static embed.FS

// Static returns the UI file tree with index.html at its root.
func Static() fs.FS {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
