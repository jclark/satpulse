package logobs

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/phcsync"
	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestClockLogObserver(t *testing.T) {
	// Create temp dir for log file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "clock.log")

	lg := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ls := ptime.LeapSecond2016() // Use standard 2016 leap second

	// Create observer
	obs, err := NewClockLogObserver(lg, logPath, ls)
	if err != nil {
		t.Fatalf("NewClockLogObserver failed: %v", err)
	}
	defer obs.Release()

	// Test that header was written
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), "# date time offset freq outlier era sync") {
		t.Error("Log header not written correctly")
	}

	// Test normal sample
	// Use SysToTime to convert from time.Time to ptime.Time
	sysTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ref, _ := ls.SysToTime(sysTime)
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOK,
		Ref:    ref,
		Offset: 42 * time.Nanosecond,
		Freq:   1234.5,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeTracking,
	})

	// Test outlier sample
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOutlier,
		Ref:    ref.Add(time.Second),
		Offset: -100 * time.Nanosecond,
		Freq:   1234.5,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeReset,
	})

	// Test that missing samples are not logged
	lenBefore := getFileSize(t, logPath)
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleMissing,
		Ref:    ref.Add(2 * time.Second),
		Offset: 0,
		Freq:   0,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeReset,
	})
	lenAfter := getFileSize(t, logPath)
	if lenAfter != lenBefore {
		t.Error("Missing sample should not be logged")
	}

	// Read final content
	content, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 lines (header + 2 samples), got %d", len(lines))
	}

	// Check first data line (normal sample)
	// Expected format: 2024-01-01 12:00:00  42 1234 0 1 2
	if !strings.Contains(lines[1], "2024-01-01 12:00:00") {
		t.Errorf("First sample date/time incorrect: %s", lines[1])
	}
	if !strings.Contains(lines[1], " 42 ") {
		t.Errorf("First sample offset incorrect: %s", lines[1])
	}
	if !strings.Contains(lines[1], " 1234 ") {
		t.Errorf("First sample freq incorrect: %s", lines[1])
	}
	if !strings.Contains(lines[1], " 0 1 1") {
		t.Errorf("First sample outlier/era/sync incorrect: %s", lines[1])
	}

	// Check second data line (outlier sample)
	// Expected format: 2024-01-01 12:00:01 -100 1234 1 1 1
	if !strings.Contains(lines[2], "2024-01-01 12:00:01") {
		t.Errorf("Second sample date/time incorrect: %s", lines[2])
	}
	if !strings.Contains(lines[2], "-100 ") {
		t.Errorf("Second sample offset incorrect: %s", lines[2])
	}
	if !strings.Contains(lines[2], " 1 1 0") {
		t.Errorf("Second sample outlier/era/sync incorrect: %s", lines[2])
	}
}

func TestClockLogObserverReopenLog(t *testing.T) {
	// Log reopen renames the open log file (the chrony/logrotate idiom),
	// which Windows forbids while a handle is open. The reopen path is
	// driven by SIGHUP, never delivered on Windows, so it is dormant there.
	if runtime.GOOS == "windows" {
		t.Skip("log reopen renames an open file, unsupported on Windows")
	}
	// Create temp dir for log file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "clock.log")

	lg := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ls := ptime.LeapSecond2016()

	// Create observer
	obs, err := NewClockLogObserver(lg, logPath, ls)
	if err != nil {
		t.Fatalf("NewClockLogObserver failed: %v", err)
	}
	defer obs.Release()

	// Write a sample
	sysTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ref, _ := ls.SysToTime(sysTime)
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOK,
		Ref:    ref,
		Offset: 10 * time.Nanosecond,
		Freq:   100,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeTracking,
	})

	// Simulate log rotation by renaming the file
	rotatedPath := logPath + ".1"
	err = os.Rename(logPath, rotatedPath)
	if err != nil {
		t.Fatalf("Failed to rename log file: %v", err)
	}

	// Reopen log (should create new file)
	obs.ReopenLog()

	// Write another sample
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOK,
		Ref:    ref.Add(time.Second),
		Offset: 20 * time.Nanosecond,
		Freq:   200,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeTracking,
	})

	// Check that new file exists and has header + sample
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed for new log: %v", err)
	}

	if !strings.Contains(string(content), "# date time offset freq outlier era sync") {
		t.Error("New log should have header")
	}

	if !strings.Contains(string(content), "2024-01-01 12:00:01") {
		t.Error("New log should have the second sample")
	}

	// Check that old file still exists and has the first sample
	oldContent, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("ReadFile failed for rotated log: %v", err)
	}

	if !strings.Contains(string(oldContent), "2024-01-01 12:00:00") {
		t.Error("Rotated log should still have the first sample")
	}
}

func TestClockLogObserverNoPath(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ls := ptime.LeapSecond2016()

	// Create observer with empty path
	obs, err := NewClockLogObserver(lg, "", ls)
	if err != nil {
		t.Fatalf("NewClockLogObserver with empty path should succeed: %v", err)
	}
	defer obs.Release()

	// Should not panic when sampling with no file
	sysTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ref, _ := ls.SysToTime(sysTime)
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOK,
		Ref:    ref,
		Offset: 42 * time.Nanosecond,
		Freq:   1234.5,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeTracking,
	})

	// Should not panic when reopening with no file
	obs.ReopenLog()
}

func TestClockLogObserverLeapSecond(t *testing.T) {
	// Create temp dir for log file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "clock.log")

	lg := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Start with 2016 leap second (37 second offset)
	initialLS := ptime.LeapSecond2016()

	// Create observer
	obs, err := NewClockLogObserver(lg, logPath, initialLS)
	if err != nil {
		t.Fatalf("NewClockLogObserver failed: %v", err)
	}
	defer obs.Release()

	// Write a sample with the initial leap second
	// Time before a potential leap second (June 30, 2024)
	sysTime1 := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
	ref1, _ := initialLS.SysToTime(sysTime1)
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOK,
		Ref:    ref1,
		Offset: 10 * time.Nanosecond,
		Freq:   100,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeTracking,
	})

	// Simulate a leap second announcement from GPS
	// This would announce a leap second at the end of June 30, 2024
	futureLeapTime := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	futureLeapPTime, _ := initialLS.SysToTime(futureLeapTime)

	leapMsg := &gpsprot.LeapSecondMsg{
		LeapSecond: ptime.LeapSecond{
			OffChangeTime: futureLeapPTime,
			UTCOffBefore:  37, // Current offset
			UTCOffAfter:   38, // New offset after leap second
		},
		GNSS: gpsprot.GPS,
	}

	// This should update the observer's cached leap second
	obs.LeapSecond(leapMsg, time.Now())

	// Write a sample after the leap second update
	// Time after the leap second (July 1, 2024)
	sysTime2 := time.Date(2024, 7, 1, 0, 0, 1, 0, time.UTC)
	// Use the updated leap second in the observer to calculate ref2
	ref2, _ := obs.ls.SysToTime(sysTime2)
	obs.Sample(phcsync.Sample{
		Kind:   phcsync.SampleOK,
		Ref:    ref2,
		Offset: 20 * time.Nanosecond,
		Freq:   200,
		Era:    phctime.Era(1),
		Mode:   phcsync.ModeTracking,
	})

	// Read and verify log content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 lines (header + 2 samples), got %d", len(lines))
	}

	// Check that timestamps are formatted correctly
	// The first sample should be formatted with the initial leap second
	if !strings.Contains(lines[1], "2024-06-30 23:59:59") {
		t.Errorf("First sample date/time incorrect: %s", lines[1])
	}

	// The second sample should be formatted with the updated leap second
	if !strings.Contains(lines[2], "2024-07-01 00:00:01") {
		t.Errorf("Second sample date/time incorrect after leap second update: %s", lines[2])
	}

	// Verify that the observer's leap second was actually updated
	if obs.ls.UTCOffAfter != 38 {
		t.Errorf("Observer's leap second was not updated. Expected UTCOffAfter=38, got %d", obs.ls.UTCOffAfter)
	}
}

func getFileSize(t *testing.T, path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	return info.Size()
}
