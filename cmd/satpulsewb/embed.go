package main

import (
	"embed"
	"io/fs"
)

//go:generate npm --prefix ../../webui run embed-workbench

//go:embed dist
var dist embed.FS

// webContent returns the embedded SatPulse Workbench frontend assets
// (index.html, app.js and style.css), built from the webui workspace
// by go generate.
func webContent() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
