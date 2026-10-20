package tui

import (
	"errors"
	"io"
	"os"
)

var errTerminalClosed = errors.New("terminal input closed")

// terminalInput reports EOF as an error. Bubble Tea's input reader treats EOF
// as a clean stop and keeps the program running, which leaves it deaf to
// every key, including ctrl+c, because raw mode disables the terminal's own
// interrupt handling.
type terminalInput struct{ *os.File }

func (t *terminalInput) Read(p []byte) (int, error) {
	n, err := t.File.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errTerminalClosed
	}
	return n, err
}
