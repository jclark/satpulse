package logobs

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestGPSLogObserver(t *testing.T) {
	tests := []struct {
		name   string
		levels []gpsprot.FixLevel
		expect []string
	}{
		{
			name:   "fix level never known",
			levels: []gpsprot.FixLevel{0, 0, 0},
		},
		{
			name:   "initial then change",
			levels: []gpsprot.FixLevel{0, gpsprot.FixLevelNone, gpsprot.FixLevelNone, gpsprot.FixLevelCode},
			expect: []string{
				`level=WARN msg="initial GPS fix status" fixLevel=none`,
				`level=INFO msg="new GPS fix status" fixLevel=code`,
			},
		},
		{
			name:   "zero between known levels",
			levels: []gpsprot.FixLevel{gpsprot.FixLevelCode, 0, gpsprot.FixLevelCode, 0},
			expect: []string{
				`level=INFO msg="initial GPS fix status" fixLevel=code`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			lg := slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey && len(groups) == 0 {
						return slog.Attr{}
					}
					return a
				},
			}))
			o := NewGPSLogObserver(lg)
			for _, lev := range tc.levels {
				o.NavEpoch(&gpsprot.NavEpochMsg{FixLevel: lev}, time.Time{})
			}
			var got []string
			if s := strings.TrimSuffix(b.String(), "\n"); s != "" {
				got = strings.Split(s, "\n")
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %q\nwant %q", got, tc.expect)
			}
		})
	}
}
