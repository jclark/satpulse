//go:build ignore

package main

import (
	"archive/zip"
	"bufio"
	"compress/flate"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s source-directory output-zip\n", filepath.Base(os.Args[0]))
		os.Exit(2)
	}
	if err := generate(os.Args[1], os.Args[2], os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(srcDir, zipPath string, r io.Reader) error {
	var names []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		name := strings.TrimSuffix(sc.Text(), "\r")
		if name != "" {
			names = append(names, name)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	sort.Strings(names)
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})
	for _, name := range names {
		if err := addFile(zw, srcDir, name); err != nil {
			zw.Close()
			out.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func addFile(zw *zip.Writer, srcDir, name string) error {
	in, err := os.Open(filepath.Join(srcDir, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	w, err := zw.CreateHeader(h)
	if err == nil {
		_, err = io.Copy(w, in)
	}
	if closeErr := in.Close(); err == nil {
		err = closeErr
	}
	return err
}
