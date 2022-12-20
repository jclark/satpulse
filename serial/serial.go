package serial

import (
	"os"

	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
)

func Open(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func Flush(f *os.File) error {
	return termios.Tcflush(f.Fd(), unix.TCIFLUSH)
}

type State struct {
	attr unix.Termios
}

func GetState(f *os.File) (*State, error) {
	s := State{}
	err := termios.Tcgetattr(f.Fd(), &s.attr)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func SetState(f *os.File, s *State) error {
	return termios.Tcsetattr(f.Fd(), termios.TCSANOW, &s.attr)
}

func (s *State) SetRaw() {
	termios.Cfmakeraw(&s.attr)
	// interbyte time read - doesn't work
	// blocking read
	// s.attr.Cc[unix.VMIN] = 20
	// s.attr.Cc[unix.VTIME] = 1
}

func (s *State) Copy() *State {
	c := *s
	return &c
}
