//go:build linux

package daemon

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNTPConfigNewSHMWriterPropagatesAttachError(t *testing.T) {
	const unit = 253
	key := int(0x4e545030 + uint32(unit))
	id, err := unix.SysvShmGet(key, 1, unix.IPC_CREAT|unix.IPC_EXCL|0o600)
	if errors.Is(err, unix.EEXIST) {
		t.Skipf("NTP SHM test unit %d already exists", unit)
	}
	if err != nil {
		t.Fatalf("creating incompatible SHM segment: %v", err)
	}
	t.Cleanup(func() {
		if _, err := unix.SysvShmCtl(id, unix.IPC_RMID, nil); err != nil {
			t.Errorf("removing incompatible SHM segment: %v", err)
		}
	})
	u := uint8(unit)
	cfg := NTPConfig{SHM: &NTPSHMConfig{Unit: &u}}
	w, err := cfg.NewSHMWriter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatalf("NewSHMWriter returned nil error for incompatible segment")
	}
	if w != nil {
		t.Fatalf("NewSHMWriter returned writer %v with error %v", w, err)
	}
	if !errors.Is(err, unix.EINVAL) {
		t.Fatalf("NewSHMWriter error = %v, want EINVAL", err)
	}
}
