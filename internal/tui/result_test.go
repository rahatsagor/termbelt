package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rahatsagor/termbelt/internal/core"
)

func TestExactInputToolsRejectPasteTheTextareaWouldAlter(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("hash")
	m.openTool(tool)
	m.Update(tea.PasteMsg{Content: "a\tb"})
	if !m.fields[0].rejectedInput || m.fields[0].value() != "" {
		t.Fatalf("hash accepted a tab: rejected=%v value=%q", m.fields[0].rejectedInput, m.fields[0].value())
	}
	m.Update(tea.PasteMsg{Content: "line one\nline two"})
	if m.fields[0].rejectedInput || m.fields[0].value() != "line one\nline two" {
		t.Fatalf("hash rejected plain multiline text: value=%q", m.fields[0].value())
	}

	tool, _ = core.FindTool("json")
	m.openTool(tool)
	m.Update(tea.PasteMsg{Content: "{\t\"a\": 1}"})
	if m.fields[0].rejectedInput {
		t.Fatal("JSON input with a tab was rejected, but whitespace does not change the result")
	}
}

func TestErrorResultOffersOnlyWorkingKeys(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("cidr")
	m.openTool(tool)
	m.screen = resultScreen
	m.err = errors.New("invalid CIDR prefix")
	m.setResultContent()
	press(m, tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if m.jsonView {
		t.Fatal("j switched to the JSON view of an error")
	}
	content := m.View().Content
	if strings.Contains(content, "c copy") || strings.Contains(content, "j JSON") || !strings.Contains(content, "e edit") {
		t.Fatalf("error footer advertises the wrong keys:\n%s", content)
	}
}

func TestJSONViewEscapesBidiControls(t *testing.T) {
	m := testModel()
	m.screen = resultScreen
	m.result = core.Result{Tool: "jwt", Output: "\u202eabc"}
	m.jsonView = true
	m.setResultContent()
	if view := m.viewport.View(); strings.Contains(view, "\u202e") {
		t.Fatal("JSON view printed a raw right-to-left override")
	}
}
