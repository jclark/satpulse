package cmd

import "testing"

func TestShortenVersion(t *testing.T) {
	old := version
	defer func() { version = old }()
	tests := []struct {
		version string
		maxLen  int
		want    string
	}{
		{"1.3.4", 11, "1.3.4"},
		{"0.2-pre.20260524.abc1234", 11, "0.2-pre"},
		{"12.34.56-pre.20260524.abc1234", 11, "12.34.56"},
		{"abcdefghijklmno", 8, "abcdefgh"},
		{"1.2.3", 0, ""},
	}
	for _, tt := range tests {
		version = tt.version
		if got := ShortenVersion(tt.maxLen); got != tt.want {
			t.Errorf("ShortenVersion(%q, %d) = %q, want %q", tt.version, tt.maxLen, got, tt.want)
		}
	}
}
