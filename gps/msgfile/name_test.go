package msgfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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

func TestListNames(t *testing.T) {
	dir1 := writeLibrary(t,
		"u-blox/gen9.toml",
		"u-blox/README.md",
		"u-blox/sub/nested.toml",
		"unicore/um980.toml",
		"loose.toml",
	)
	dir2 := writeLibrary(t,
		"u-blox/gen9.toml",
		"u-blox/x20.toml",
		"zhongke/casic.toml",
	)
	got := ListNames([]string{dir1, dir2, filepath.Join(dir1, "no-such-dir")})
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
	dir1 := writeLibrary(t, "u-blox/gen9.toml")
	dir2 := writeLibrary(t, "u-blox/gen9.toml", "unicore/um980.toml")
	dirs := []string{dir1, dir2}
	tests := []struct {
		name      string
		fileName  Name
		expect    string
		expectErr bool
	}{
		{
			name:     "first dir wins",
			fileName: Name{"u-blox", "gen9"},
			expect:   filepath.Join(dir1, "u-blox", "gen9.toml"),
		},
		{
			name:     "found in second dir",
			fileName: Name{"unicore", "um980"},
			expect:   filepath.Join(dir2, "unicore", "um980.toml"),
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
			name:      "empty vendor",
			fileName:  Name{"", "gen9"},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindName(tc.fileName, dirs)
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
		})
	}
}

func TestEnvDirs(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expect []string
	}{
		{name: "unset", value: "", expect: nil},
		{name: "two dirs", value: strings.Join([]string{"/a", "/b"}, string(os.PathListSeparator)), expect: []string{"/a", "/b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SATPULSE_GPSMSG_PATH", tc.value)
			if got := EnvDirs(); !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}
