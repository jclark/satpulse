package web

import (
	"embed"
	"io/fs"
)

//go:generate npm --prefix ../../../webui run embed
//go:embed dist
var dist embed.FS

// Content returns the embedded web UI assets (index.html, app.js and
// style.css), built from the webui workspace by go generate.
func Content() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
