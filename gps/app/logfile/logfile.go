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

func (lf *LogFile) Open(path string) error {
	lf.path = path
	if path == "" {
		return nil
	}
	dir := filepath.Dir(lf.path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}
	return lf.doOpen()
}

func (lf *LogFile) Reopen(lg *slog.Logger) {
	if lf.path == "" {
		return
	}
	lf.Close(lg)
	err := lf.doOpen()
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

func (lf *LogFile) doOpen() error {
	f, err := os.OpenFile(lf.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
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
