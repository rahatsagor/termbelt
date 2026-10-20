package tui

import "errors"

func openTerminalInput() (*terminalInput, error) {
	return nil, errors.New("the console input handle is used directly on Windows")
}
