package msgfile

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io/fs"
	"sync"
)

//go:generate sh -c "git -C ../../configs/gpsmsg ls-files -- '*/*.toml' | go run gen.go ../../configs/gpsmsg gpsmsg.zip"

//go:embed gpsmsg.zip
var builtinZip []byte

var builtinOnce sync.Once
var builtinFS fs.FS

type builtinDir struct {
	fs.FS
}

// Builtin returns the message-file library compiled into the package.
func Builtin() Dir {
	builtinOnce.Do(func() {
		zr, err := zip.NewReader(bytes.NewReader(builtinZip), int64(len(builtinZip)))
		if err != nil {
			panic(err)
		}
		builtinFS = zr
	})
	return builtinDir{builtinFS}
}

func (dir builtinDir) DisplayPath(name string) string {
	return "built-in:" + name
}
