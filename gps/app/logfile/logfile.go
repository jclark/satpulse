package logfile

import (
	"log/slog"
	"os"
	"path/filepath"
)

type LogFile struct {
	path string
	File *os.File
}

// Open opens the log file at path. If append is true, the file is opened
// in append mode; otherwise, an existing file is truncated.
func (lf *LogFile) Open(path string, append bool) error {
	lf.path = path
	if path == "" {
		return nil
	}
	dir := filepath.Dir(lf.path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}
	return lf.doOpen(append)
}

// Reopen reopens the log file in append mode (e.g. after log rotation).
func (lf *LogFile) Reopen(lg *slog.Logger) {
	if lf.path == "" {
		return
	}
	lf.Close(lg)
	err := lf.doOpen(true)
	if err != nil {
		lg.Error("error reopening log file", "err", err, "path", lf.path)
	}
}

func (lf *LogFile) Close(lg *slog.Logger) {
	if lf.File == nil {
		return
	}
	err := lf.File.Close()
	if err != nil {
		lg.Error("error closing log file", "err", err, "path", lf.path)
	}
	lf.File = nil
}

func (lf *LogFile) doOpen(append bool) error {
	flags := os.O_CREATE | os.O_WRONLY
	if append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(lf.path, flags, 0644)
	if err != nil {
		return err
	}
	lf.File = f
	return nil
}

func (lf *LogFile) HandleWriteError(err error, lg *slog.Logger) {
	if err == nil {
		return
	}
	lg.Error("error writing to log file", "err", err, "path", lf.path)
	lf.File.Close()
	lf.File = nil
}
