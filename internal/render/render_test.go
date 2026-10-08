package render

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/rahatsagor/termbelt/internal/core"
	"strings"
	"testing"
)

func TestRemoteContentCannotInjectTerminalCommands(t *testing.T) {
	r := core.Result{Title: "fixture", Output: "remote\x1b]52;c;Y2xpcA==\x07\rtext", Sections: []core.Section{{Title: "HEADERS", Rows: []core.Row{{Label: "Server", Value: "x\x1b[2J"}}}}}
	out := Result(r, 80, false)
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\x07") || strings.Contains(out, "\r") {
		t.Fatalf("unsafe terminal controls: %q", out)
	}
}
func TestTablesWrapWithoutLosingLongValues(t *testing.T) {
	value := strings.Repeat("abcdefghij", 25)
	out := Table(core.Table{Headers: []string{"DOMAIN", "STATUS", "DETAIL"}, Rows: [][]string{{"fixture.test", "unknown", value}}}, 60, false)
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Fatalf("overflow: %q", line)
		}
	}
	if !strings.Contains(out, "abcdefghij") {
		t.Fatal("lost row values")
	}
}
