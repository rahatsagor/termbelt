package tui

import (
	"errors"
	"os"
	"testing"
)

func TestTerminalInputReportsEOFAsError(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("q"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	input := &terminalInput{File: r}
	defer input.Close()
	buf := make([]byte, 8)
	if n, err := input.Read(buf); n != 1 || err != nil || buf[0] != 'q' {
		t.Fatalf("first read = %d, %v", n, err)
	}
	if _, err := input.Read(buf); !errors.Is(err, errTerminalClosed) {
		t.Fatalf("read after the writer closed = %v, want errTerminalClosed", err)
	}
}
