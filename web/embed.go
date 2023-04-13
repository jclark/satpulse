package web

import (
	"embed"
	"io/fs"
)

//go:generate npm run build
//go:embed *.js *.html
var content embed.FS

func Content() fs.FS {
	return content
}
