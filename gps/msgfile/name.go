package msgfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
func EnvDirs() []string {
	if p := os.Getenv("SATPULSE_GPSMSG_PATH"); p != "" {
		dirs := []string{}
		for _, dir := range filepath.SplitList(p) {
			if dir != "" {
				dirs = append(dirs, dir)
			}
		}
		return dirs
	}
	return nil
}

// FindName returns the path of the message file named n: the first
// dir/Vendor/File.toml along dirs that is a regular file. It returns
// an error when either component of n is not a plain file name or no
// file is found.
func FindName(n Name, dirs []string) (string, error) {
	if !plainName(n.Vendor) || !plainName(n.File) {
		return "", fmt.Errorf("invalid message file name %s/%s", n.Vendor, n.File)
	}
	for _, dir := range dirs {
		p := filepath.Join(dir, n.Vendor, n.File+".toml")
		if isRegular(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("message file %s/%s not found", n.Vendor, n.File)
}

// ListNames returns the message files along dirs: each
// dir/vendor/file.toml, in the order given by dirs and lexically
// within each dir, keeping the first occurrence of a Name so the
// listing matches what FindName resolves. Missing or unreadable
// directories are skipped.
func ListNames(dirs []string) []Entry {
	var entries []Entry
	seen := make(map[Name]bool)
	for _, dir := range dirs {
		vendors, _ := os.ReadDir(dir) // sorted by name; nil on error
		for _, v := range vendors {
			if !plainName(v.Name()) {
				continue
			}
			vdir := filepath.Join(dir, v.Name())
			if !isDir(vdir) {
				continue
			}
			files, _ := os.ReadDir(vdir)
			for _, f := range files {
				file := strings.TrimSuffix(f.Name(), ".toml")
				if file == f.Name() || !plainName(file) || !isRegular(filepath.Join(vdir, f.Name())) {
					continue
				}
				n := Name{Vendor: v.Name(), File: file}
				if !seen[n] {
					seen[n] = true
					entries = append(entries, Entry{Name: n, Path: filepath.Join(vdir, f.Name())})
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

// isRegular and isDir use os.Stat so symlinked files and vendor
// directories count.
func isRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
