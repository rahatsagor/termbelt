//go:build !windows

package tui

import (
	"errors"
	"os"

	"golang.org/x/term"
)

// Bubble Tea polls os.Stdin with kqueue on macOS, and kqueue can stop
// reporting a terminal as readable (for example after the Mac sleeps). The
// reader then waits forever while keys pile up unread. The same reader uses
// select(2) for files named /dev/tty, which does not have this problem.
func openTerminalInput() (*terminalInput, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if !term.IsTerminal(int(f.Fd())) {
		_ = f.Close()
		return nil, errors.New("/dev/tty is not a terminal")
	}
	return &terminalInput{File: f}, nil
}
