package msgfile

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Dir is a directory in a message-file library search path.
type Dir interface {
	fs.FS
	// DisplayPath returns the user-visible path for name. For an OS
	// directory this is the actual filename; virtual directories return
	// a descriptive path.
	DisplayPath(name string) string
}

type osDir string

// OSDir returns a message-file library directory rooted at name.
func OSDir(name string) Dir {
	return osDir(name)
}

func (dir osDir) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.Open(dir.filePath(name))
}

func (dir osDir) DisplayPath(name string) string {
	return dir.filePath(name)
}

func (dir osDir) filePath(name string) string {
	return filepath.Join(string(dir), filepath.FromSlash(name))
}

// Name identifies a message file in a library search path: Vendor is
// the vendor directory name and File the file name without its .toml
// extension.
type Name struct {
	Vendor string `json:"vendor"`
	File   string `json:"file"`
}

// Entry is a ListNames result: a Name and the path FindName resolves
// it to.
type Entry struct {
	Name
	Path string `json:"path"`
}

// EnvDirs returns the library search path from the
// SATPULSE_GPSMSG_PATH environment variable, split like PATH on the
// OS-specific list separator with empty elements omitted, or nil when
// the variable is unset or empty.
func EnvDirs() []Dir {
	if p := os.Getenv("SATPULSE_GPSMSG_PATH"); p != "" {
		dirs := []Dir{}
		for _, name := range filepath.SplitList(p) {
			if name != "" {
				dirs = append(dirs, OSDir(name))
			}
		}
		return dirs
	}
	return nil
}

// FindName returns the directory and slash-separated path of the
// message file named n: the first Vendor/File.toml along dirs that is
// a regular file. It returns an error when either component of n is
// not a plain file name or no file is found.
func FindName(n Name, dirs []Dir) (Dir, string, error) {
	if !plainName(n.Vendor) || !plainName(n.File) {
		return nil, "", fmt.Errorf("invalid message file name %s/%s", n.Vendor, n.File)
	}
	name := path.Join(n.Vendor, n.File+".toml")
	for _, dir := range dirs {
		if isRegular(dir, name) {
			return dir, name, nil
		}
	}
	return nil, "", fmt.Errorf("message file %s/%s not found", n.Vendor, n.File)
}

// ListNames returns the message files along dirs: each
// dir/vendor/file.toml, in the order given by dirs and lexically
// within each dir, keeping the first occurrence of a Name so the
// listing matches what FindName resolves. Missing or unreadable
// directories are skipped.
func ListNames(dirs []Dir) []Entry {
	var entries []Entry
	seen := make(map[Name]bool)
	for _, dir := range dirs {
		vendors, _ := fs.ReadDir(dir, ".") // sorted by name; nil on error
		for _, v := range vendors {
			if !plainName(v.Name()) || !isDir(dir, v.Name()) {
				continue
			}
			files, _ := fs.ReadDir(dir, v.Name())
			for _, f := range files {
				file := strings.TrimSuffix(f.Name(), ".toml")
				name := path.Join(v.Name(), f.Name())
				if file == f.Name() || !plainName(file) || !isRegular(dir, name) {
					continue
				}
				n := Name{Vendor: v.Name(), File: file}
				if !seen[n] {
					seen[n] = true
					entries = append(entries, Entry{Name: n, Path: dir.DisplayPath(name)})
				}
			}
		}
	}
	return entries
}

// plainName reports whether s can be a Name component: a single
// relative path element, so that a Name can never resolve outside a
// search-path directory (Name values arrive from the network).
func plainName(s string) bool {
	return s != "" && s != "." && !strings.ContainsAny(s, `/\`) && filepath.IsLocal(s)
}

func isRegular(fsys fs.FS, name string) bool {
	fi, err := fs.Stat(fsys, name)
	return err == nil && fi.Mode().IsRegular()
}

func isDir(fsys fs.FS, name string) bool {
	fi, err := fs.Stat(fsys, name)
	return err == nil && fi.IsDir()
}
