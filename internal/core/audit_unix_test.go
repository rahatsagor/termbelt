//go:build !windows

package core

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestHashFileRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := hashTool(Request{Options: map[string]string{"file": path}})
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("FIFO error = %v", err)
	}
}
