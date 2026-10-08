package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rahatsagor/termbelt/internal/core"
)

func TestAuditTickFromDifferentJobIsIgnored(t *testing.T) {
	m := testModel()
	m.screen = running
	m.jobID = 99
	m.ticks = 3
	updated, cmd := m.Update(tickMsg{id: 98})
	if cmd != nil {
		t.Fatal("stale tick scheduled another animation tick")
	}
	if updated.(*Model).ticks != 3 {
		t.Fatalf("stale tick changed spinner counter to %d", updated.(*Model).ticks)
	}
}

func TestReturningHomeReleasesLocalInputAndResult(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("json")
	m.openTool(tool)
	m.fields[0].area.SetValue(`{"secret":"private"}`)
	m.screen = resultScreen
	m.result = core.Result{Output: "private"}
	m.setResultContent()
	press(m, key(tea.KeyEscape))
	if m.screen != home || len(m.fields) != 0 || m.result.Output != "" || m.viewport.GetContent() != "" {
		t.Fatal("home retained local input or result")
	}
}

func TestVerySmallTerminalWarningFitsAndResizeRestoresState(t *testing.T) {
	m := testModel()
	m.search.SetValue("json")
	for _, size := range [][2]int{{1, 1}, {10, 3}, {38, 13}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%v: %d rows", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%v: overflow %q", size, line)
			}
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "JSON workbench") {
		t.Fatal("resize lost search state")
	}
}

func TestOversizedPasteCannotRunHashOfTruncatedInput(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("hash")
	m.openTool(tool)
	m.fields[0].area.CharLimit = 8
	m.fields[0].area.SetValue("before")
	m.Update(tea.PasteMsg{Content: "this paste is too long"})
	if m.fields[0].value() != "before" || !m.fields[0].rejectedInput {
		t.Fatal("paste was silently truncated")
	}
	m.start()
	if m.screen != form {
		t.Fatal("hash ran despite rejected input")
	}
	m.Update(tea.PasteMsg{Content: "!"})
	if m.fields[0].rejectedInput {
		t.Fatal("valid edit did not clear rejection")
	}
}
