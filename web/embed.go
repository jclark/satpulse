package web

import (
	"embed"
	"io/fs"
)

const RootFile = "index.html"

//go:embed *.js *.html
var content embed.FS

func Content() fs.FS {
	return content
}
