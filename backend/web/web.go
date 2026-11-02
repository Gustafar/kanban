package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend (frontend/ builds into web/dist).
func FS() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}
