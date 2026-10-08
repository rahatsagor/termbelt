package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rahatsagor/termbelt/internal/core"
)

func testModel() *Model {
	return New(&core.Engine{}, "test")
}

func press(m *Model, key tea.KeyPressMsg) {
	m.Update(key)
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func typeText(m *Model, text string) {
	for _, r := range text {
		press(m, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func TestSearchArrowAndEnterOpenSelectedTool(t *testing.T) {
	m := testModel()
	m.search.Focus()
	typeText(m, "json")
	items := m.filtered()
	if len(items) != 1 || items[0].ID != "json" {
		t.Fatalf("search results = %#v, want only json", items)
	}
	press(m, key(tea.KeyDown))
	press(m, key(tea.KeyEnter))
	if m.screen != form || m.tool.ID != "json" {
		t.Fatalf("screen=%v tool=%q, want JSON form", m.screen, m.tool.ID)
	}
}

func TestCategoryTabAndShiftTabCycle(t *testing.T) {
	m := testModel()
	press(m, key(tea.KeyTab))
	if m.category != 1 {
		t.Fatalf("Tab selected category %d, want 1", m.category)
	}
	press(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	if m.category != 0 {
		t.Fatalf("Shift+Tab selected category %d, want 0", m.category)
	}
	press(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	if m.category != len(categories)-1 {
		t.Fatalf("Shift+Tab did not wrap: category=%d", m.category)
	}
}

func TestEscapeReturnsFromFormToHome(t *testing.T) {
	m := testModel()
	tool, ok := core.FindTool("ports")
	if !ok {
		t.Fatal("json tool missing")
	}
	m.openTool(tool)
	press(m, key(tea.KeyEscape))
	if m.screen != home {
		t.Fatalf("screen after Escape = %v, want home", m.screen)
	}
}

func TestRenderedViewFitsTerminalSizes(t *testing.T) {
	m := testModel()
	for _, size := range [][2]int{{42, 16}, {80, 24}, {120, 40}, {160, 45}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		content := m.View().Content
		lines := strings.Split(content, "\n")
		if len(lines) > size[1] {
			t.Errorf("%dx%d view has %d rows", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if width := ansi.StringWidth(line); width > size[0] {
				t.Errorf("%dx%d line %d is %d columns: %q", size[0], size[1], i+1, width, ansi.Strip(line))
			}
		}
	}
}

func TestFocusedFormFieldsStayVisibleAtSmallSizes(t *testing.T) {
	for _, size := range [][2]int{{42, 16}, {80, 24}} {
		for _, tool := range core.Tools {
			m := testModel()
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.openTool(tool)
			for i, field := range tool.Fields {
				m.focus = i
				view := ansi.Strip(m.View().Content)
				if !strings.Contains(view, "● "+field.Label) {
					t.Errorf("%dx%d %s field %d invisible: %s", size[0], size[1], tool.ID, i, field.Label)
				}
				for _, line := range strings.Split(view, "\n") {
					if ansi.StringWidth(line) > size[0] {
						t.Errorf("%s form overflows", tool.ID)
					}
				}
			}
		}
	}
}

func TestResultJSONToggleAndEditBackNavigation(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("json")
	m.tool = tool
	m.result = core.Result{Title: "JSON result", Output: `{"ok":true}`, Data: map[string]any{"ok": true}}
	m.screen = resultScreen
	m.resize()
	m.setResultContent()
	if m.jsonView {
		t.Fatal("result should start in the normal view")
	}
	press(m, key('j'))
	if !m.jsonView || !strings.Contains(m.viewport.View(), `"ok": true`) {
		t.Fatalf("j did not show formatted JSON: view=%q", m.viewport.View())
	}
	press(m, key('j'))
	if m.jsonView {
		t.Fatal("second j did not restore the result view")
	}
	press(m, key('e'))
	if m.screen != form {
		t.Fatalf("e selected screen %v, want form", m.screen)
	}
	press(m, key(tea.KeyEscape))
	if m.screen != home {
		t.Fatalf("Escape from edit form selected screen %v, want home", m.screen)
	}
}

func TestStaleJobEventAfterCancellationIsIgnored(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("ports")
	m.tool = tool
	_, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.screen = running
	m.jobID = 12
	press(m, key(tea.KeyEscape))
	if m.screen != form || m.jobID != 13 {
		t.Fatalf("after cancellation: screen=%v jobID=%d", m.screen, m.jobID)
	}
	stale := core.Result{Title: "stale"}
	m.Update(jobEvent{id: 12, result: &stale, done: true})
	if m.screen != form || m.result.Title == "stale" {
		t.Fatalf("stale event changed model: screen=%v result=%q", m.screen, m.result.Title)
	}
}

func TestLocalInputsAreNotSavedAsDrafts(t *testing.T) {
	m := testModel()
	tool, _ := core.FindTool("json")
	m.tool = tool
	input := textinput.New()
	input.SetValue(`{"secret":"local-only"}`)
	m.fields = []formField{{spec: tool.Fields[0], input: input}}
	if _, ok := m.drafts[tool.ID]; ok {
		t.Fatalf("local draft existed before execution: %#v", m.drafts[tool.ID])
	}
	m.start()
	if _, ok := m.drafts[tool.ID]; ok {
		t.Fatalf("local input was saved in drafts: %#v", m.drafts[tool.ID])
	}
	if m.cancel != nil {
		m.cancel()
	}
}
