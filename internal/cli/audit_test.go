package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditRejectsConflictingOutputAndDomainFlags(t *testing.T) {
	for _, args := range [][]string{
		{"uuid", "--json", "--raw"},
		{"domains", "example", "--all", "--tlds", "com"},
	} {
		if _, err := execute(t, args, ""); err == nil {
			t.Errorf("accepted conflicting arguments %v", args)
		}
	}
}

func TestBlockedStdinReadIsInterruptible(t *testing.T) {
	source, writer := io.Pipe()
	defer source.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, cleanup := interruptibleInput(ctx, source)
	defer cleanup()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(reader); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked input error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not unblock stdin consumer")
	}
}

func TestAuditNoInputCommandsRejectExtraArguments(t *testing.T) {
	for _, args := range [][]string{
		{"ports", "unexpected"},
		{"speed", "unexpected"},
		{"uuid", "unexpected"},
		{"password", "unexpected"},
	} {
		if _, err := execute(t, args, ""); err == nil {
			t.Errorf("accepted extra argument for no-input command %v", args)
		}
	}
}

func TestAuditHashRejectsTextAndFileSourcesTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, []string{"hash", "text", "--file", path}, ""); err == nil {
		t.Fatal("accepted both text and --file as hash sources")
	}
}
