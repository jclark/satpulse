package serial

import (
	"os"

	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
)

type Port struct {
	fd   int
	path string
	attr unix.Termios
}

func Raw(path string) (*Port, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, &os.PathError{
			Op:   "open",
			Path: path,
			Err:  err,
		}
	}
	p := &Port{path: path, fd: fd}
	err = termios.Tcgetattr(p.ufd(), &p.attr)
	if err != nil {
		unix.Close(fd)
		return nil, p.wrapErr(err, "tcgetattr")
	}
	rawAttr := p.attr
	termios.Cfmakeraw(&rawAttr)
	// This means read with timeout of 11 tenths of a second
	rawAttr.Cc[unix.VTIME] = 11
	rawAttr.Cc[unix.VMIN] = 0
	err = p.setAttr(&rawAttr)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return p, nil
}

func (p *Port) ufd() uintptr {
	return uintptr(p.fd)
}

func (p *Port) Read(buf []byte) (n int, err error) {
	for {
		n, err = unix.Read(p.fd, buf)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		err = p.wrapErr(err, "read")
	}
	return
}

// Close resets the port's attributes and then closes it
func (p *Port) Close() (err error) {
	err = p.setAttr(&p.attr)
	e2 := unix.Close(int(p.fd))
	if e2 != nil && err == nil {
		err = p.wrapErr(e2, "close")
	}
	return
}

func (p *Port) Flush() error {
	return p.wrapErr(termios.Tcflush(p.ufd(), unix.TCIFLUSH), "tcflush")
}

func (p *Port) setAttr(attr *unix.Termios) error {
	return p.wrapErr(termios.Tcsetattr(p.ufd(), termios.TCSANOW, attr), "tcsetattr")
}

func (p *Port) wrapErr(err error, op string) error {
	if err == nil {
		return err
	}
	return &os.PathError{
		Op:   op,
		Path: p.path,
		Err:  err,
	}
}
