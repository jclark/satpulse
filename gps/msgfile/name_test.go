package msgfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

type testDir struct {
	fs.FS
	prefix string
}

func (dir testDir) DisplayPath(name string) string {
	return dir.prefix + name
}

// writeLibrary creates a library directory whose vendor/file structure
// is given by slash-separated relative paths.
func writeLibrary(t *testing.T, paths ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, p := range paths {
		p = filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestOSDirDisplayPath(t *testing.T) {
	root := filepath.Join(string(os.PathSeparator), "foo")
	got := OSDir(root).DisplayPath("bar/baz.toml")
	want := filepath.Join(root, "bar", "baz.toml")
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestListNames(t *testing.T) {
	paths := []string{
		"u-blox/gen9.toml",
		"u-blox/README.md",
		"u-blox/sub/nested.toml",
		"unicore/um980.toml",
		"loose.toml",
	}
	if os.PathSeparator != '\\' {
		paths = append(paths, `bad\vendor/file.toml`, `u-blox/bad\file.toml`)
	}
	dir1 := writeLibrary(t, paths...)
	dir2 := writeLibrary(t,
		"u-blox/gen9.toml",
		"u-blox/x20.toml",
		"zhongke/casic.toml",
	)
	got := ListNames([]Dir{OSDir(dir1), OSDir(dir2), OSDir(filepath.Join(dir1, "no-such-dir"))})
	expect := []Entry{
		{Name{"u-blox", "gen9"}, filepath.Join(dir1, "u-blox", "gen9.toml")},
		{Name{"unicore", "um980"}, filepath.Join(dir1, "unicore", "um980.toml")},
		{Name{"u-blox", "x20"}, filepath.Join(dir2, "u-blox", "x20.toml")},
		{Name{"zhongke", "casic"}, filepath.Join(dir2, "zhongke", "casic.toml")},
	}
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
}

func TestFindName(t *testing.T) {
	dir1 := writeLibrary(t, "u-blox/gen9.toml", "loose.toml")
	dir2 := writeLibrary(t, "u-blox/gen9.toml", "unicore/um980.toml")
	dirs := []Dir{OSDir(dir1), OSDir(dir2)}
	tests := []struct {
		name      string
		fileName  Name
		expectDir string
		expect    string
		expectErr bool
	}{
		{
			name:      "first dir wins",
			fileName:  Name{"u-blox", "gen9"},
			expectDir: dir1,
			expect:    "u-blox/gen9.toml",
		},
		{
			name:      "found in second dir",
			fileName:  Name{"unicore", "um980"},
			expectDir: dir2,
			expect:    "unicore/um980.toml",
		},
		{
			name:      "not found",
			fileName:  Name{"u-blox", "gen8"},
			expectErr: true,
		},
		{
			name:      "vendor with separator",
			fileName:  Name{"u-blox/sub", "gen9"},
			expectErr: true,
		},
		{
			name:      "file with dotdot",
			fileName:  Name{"u-blox", ".."},
			expectErr: true,
		},
		{
			name:      "dot vendor",
			fileName:  Name{".", "loose"},
			expectErr: true,
		},
		{
			name:      "empty vendor",
			fileName:  Name{"", "gen9"},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, got, err := FindName(tc.fileName, dirs)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expect {
				t.Errorf("got  %q\nwant %q", got, tc.expect)
			}
			if dir.DisplayPath(".") != tc.expectDir {
				t.Errorf("got dir %q want %q", dir.DisplayPath("."), tc.expectDir)
			}
		})
	}
}

func TestFSDirs(t *testing.T) {
	dir1 := testDir{FS: fstest.MapFS{
		"u-blox/gen9.toml": &fstest.MapFile{},
	}, prefix: "first:"}
	dir2 := testDir{FS: fstest.MapFS{
		"u-blox/gen9.toml":   &fstest.MapFile{},
		"unicore/um980.toml": &fstest.MapFile{},
	}, prefix: "second:"}
	dirs := []Dir{dir1, dir2}
	expect := []Entry{
		{Name: Name{"u-blox", "gen9"}, Path: "first:u-blox/gen9.toml"},
		{Name: Name{"unicore", "um980"}, Path: "second:unicore/um980.toml"},
	}
	if got := ListNames(dirs); !reflect.DeepEqual(got, expect) {
		t.Errorf("got  %+v\nwant %+v", got, expect)
	}
	dir, name, err := FindName(Name{"u-blox", "gen9"}, dirs)
	if err != nil {
		t.Fatal(err)
	}
	if dir.DisplayPath(name) != "first:u-blox/gen9.toml" || name != "u-blox/gen9.toml" {
		t.Errorf("got %q %q", dir.DisplayPath(name), name)
	}
	if _, err := fs.Stat(dir, name); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltin(t *testing.T) {
	dir, name, err := FindName(Name{"u-blox", "gen9"}, []Dir{Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	if dir.DisplayPath(name) != "built-in:u-blox/gen9.toml" {
		t.Errorf("unexpected display path %q", dir.DisplayPath(name))
	}
	if _, err := LoadFS(dir, name); err != nil {
		t.Fatal(err)
	}
}

func TestEnvDirs(t *testing.T) {
	root := string(os.PathSeparator)
	dirA := filepath.Join(root, "a")
	dirB := filepath.Join(root, "b")
	tests := []struct {
		name   string
		value  string
		expect []string
	}{
		{name: "unset", value: "", expect: nil},
		{name: "two dirs", value: strings.Join([]string{dirA, dirB}, string(os.PathListSeparator)), expect: []string{dirA, dirB}},
		{name: "empty elements", value: strings.Join([]string{"", dirA, ""}, string(os.PathListSeparator)), expect: []string{dirA}},
		{name: "only empty elements", value: string(os.PathListSeparator), expect: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SATPULSE_GPSMSG_PATH", tc.value)
			gotDirs := EnvDirs()
			var got []string
			if gotDirs != nil {
				got = make([]string, len(gotDirs))
			}
			for i, dir := range gotDirs {
				got[i] = dir.DisplayPath(".")
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
