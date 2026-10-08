package tui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/rahatsagor/termbelt/internal/core"
	"github.com/rahatsagor/termbelt/internal/render"
)

type screen int

const (
	home screen = iota
	form
	running
	resultScreen
)

type formField struct {
	spec          core.Field
	input         textinput.Model
	area          textarea.Model
	multiline     bool
	rejectedInput bool
}

func (f *formField) value() string {
	if f.multiline {
		return f.area.Value()
	}
	return f.input.Value()
}
func (f *formField) focus() tea.Cmd {
	if f.multiline {
		return f.area.Focus()
	}
	return f.input.Focus()
}
func (f *formField) blur() {
	if f.multiline {
		f.area.Blur()
	} else {
		f.input.Blur()
	}
}

type jobEvent struct {
	id       int
	progress *core.Progress
	result   *core.Result
	err      error
	done     bool
}
type tickMsg struct{ id int }
type clipboardMsg struct{ err error }
type clearToastMsg struct{ text string }
type Model struct {
	engine        *core.Engine
	version       string
	screen        screen
	width, height int
	category      int
	selected      int
	search        textinput.Model
	tool          core.Tool
	fields        []formField
	focus         int
	viewport      viewport.Model
	result        core.Result
	err           error
	jsonView      bool
	jobID         int
	events        chan jobEvent
	cancel        context.CancelFunc
	progress      core.Progress
	started       time.Time
	ticks         int
	toast         string
	recent        []string
	drafts        map[string]map[string]string
}

func New(engine *core.Engine, version string) *Model {
	search := textinput.New()
	search.Placeholder = "Search tools, commands, or a task…"
	search.Prompt = "⌕  "
	search.CharLimit = 100
	search.SetWidth(55)
	return &Model{engine: engine, version: version, search: search, width: 100, height: 32, viewport: viewport.New(), drafts: map[string]map[string]string{}}
}
func Run(engine *core.Engine, version string, plain bool) error {
	m := New(engine, version)
	defer func() {
		if m.cancel != nil {
			m.cancel()
		}
	}()
	var options []tea.ProgramOption
	if plain || os.Getenv("NO_COLOR") != "" {
		options = append(options, tea.WithColorProfile(colorprofile.Ascii))
	}
	_, err := tea.NewProgram(m, options...).Run()
	return err
}
func (m *Model) Init() tea.Cmd { return m.search.Focus() }

var categories = []string{"all tools", "network", "domains", "developer", "favorites", "recent"}

func (m *Model) filtered() []core.Tool {
	query := strings.ToLower(m.search.Value())
	out := []core.Tool{}
	for _, tool := range core.Tools {
		category := categories[m.category]
		if category == "favorites" && !m.engine.Config.IsFavorite(tool.ID) {
			continue
		}
		if category == "recent" {
			found := false
			for _, id := range m.recent {
				if id == tool.ID {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		if category != "all tools" && category != "favorites" && category != "recent" && strings.ToLower(tool.Category) != category {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(tool.ID + " " + tool.Name + " " + tool.Description + " " + tool.Hint)
			matched := true
			for _, word := range strings.Fields(query) {
				if !strings.Contains(haystack, word) {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, tool)
	}
	if categories[m.category] == "recent" {
		sort.SliceStable(out, func(i, j int) bool {
			a, b := 0, 0
			for index, id := range m.recent {
				if id == out[i].ID {
					a = index
				}
				if id == out[j].ID {
					b = index
				}
			}
			return a < b
		})
	}
	return out
}
func (m *Model) resize() {
	width := max(m.width-8, 20)
	m.search.SetWidth(max(10, width-7))
	for i := range m.fields {
		if m.fields[i].multiline {
			m.fields[i].area.SetWidth(max(10, width-8))
			m.fields[i].area.SetHeight(min(5, max(2, m.height-19)))
		} else {
			m.fields[i].input.SetWidth(max(10, width-8))
		}
	}
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(max(4, m.height-11))
	if m.screen == resultScreen {
		m.setResultContent()
	}
}
func (m *Model) openTool(tool core.Tool) tea.Cmd {
	m.tool = tool
	m.screen = form
	m.err = nil
	m.focus = 0
	m.fields = nil
	for _, spec := range tool.Fields {
		field := formField{spec: spec}
		multiline := spec.Key == "input" && tool.Local && (tool.ID == "json" || tool.ID == "jwt" || tool.ID == "regex" || tool.ID == "base64" || tool.ID == "hash")
		field.multiline = multiline
		value := spec.Default
		// Keep drafts only for non-sensitive network inputs. Local utility inputs are never retained.
		if draft := m.drafts[tool.ID]; draft != nil {
			if v, ok := draft[spec.Key]; ok {
				value = v
			}
		}
		if multiline {
			field.area = textarea.New()
			field.area.Placeholder = spec.Placeholder
			field.area.Prompt = "│ "
			field.area.ShowLineNumbers = false
			field.area.CharLimit = 1 << 20
			field.area.SetHeight(5)
			field.area.SetValue(value)
		} else {
			field.input = textinput.New()
			field.input.Prompt = "› "
			field.input.Placeholder = spec.Placeholder
			field.input.CharLimit = 4096
			field.input.SetValue(value)
		}
		m.fields = append(m.fields, field)
	}
	m.resize()
	if len(m.fields) > 0 {
		return m.fields[0].focus()
	}
	return nil
}
func waitJob(events <-chan jobEvent, id int) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return jobEvent{id: id, done: true, err: context.Canceled}
		}
		return event
	}
}
func tick(id int) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{id: id} })
}
func (m *Model) request() core.Request {
	req := core.Request{Tool: m.tool.ID, Options: map[string]string{}}
	for i := range m.fields {
		field := &m.fields[i]
		value := field.value()
		if field.spec.Key == "input" {
			req.Input = value
		} else {
			req.Options[field.spec.Key] = value
		}
	}
	return req
}
func (m *Model) start() tea.Cmd {
	for i := range m.fields {
		if m.fields[i].rejectedInput {
			return m.toastCmd("Input was rejected; edit it or use CLI stdin / --file before running")
		}
	}
	if m.cancel != nil {
		m.cancel()
	}
	req := m.request()
	if !m.tool.Local {
		draft := map[string]string{}
		for i := range m.fields {
			draft[m.fields[i].spec.Key] = m.fields[i].value()
		}
		m.drafts[m.tool.ID] = draft
	}
	for i := range m.fields {
		m.fields[i].blur()
	}
	m.screen = running
	m.jobID++
	id := m.jobID
	m.err = nil
	m.progress = core.Progress{Message: "Preparing " + m.tool.Name}
	m.started = time.Now()
	m.ticks = 0
	m.jsonView = false
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	events := make(chan jobEvent, 32)
	m.events = events
	engine := m.engine
	go func() {
		defer close(events)
		res, err := engine.Run(ctx, req, func(p core.Progress) {
			select {
			case events <- jobEvent{id: id, progress: &p}:
			default:
			}
		})
		select {
		case events <- jobEvent{id: id, result: &res, err: err, done: true}:
		case <-ctx.Done():
		}
	}()
	return tea.Batch(waitJob(events, id), tick(id))
}
func (m *Model) setResultContent() {
	if m.err != nil {
		m.viewport.SetContent(lipgloss.NewStyle().Foreground(render.Amber).Bold(true).Render("Couldn’t complete this request") + "\n\n" + ansi.Hardwrap(render.Safe(m.err.Error()), max(20, m.width-8), true) + "\n\n" + lipgloss.NewStyle().Foreground(render.Muted).Render("Press e to edit, r to retry, or esc to return to the launcher."))
		return
	}
	if m.jsonView {
		m.viewport.SetContent(ansi.Hardwrap(core.PrettyJSON(m.result), max(20, m.width-8), true))
	} else {
		m.viewport.SetContent(render.Result(m.result, max(20, m.width-8), os.Getenv("NO_COLOR") == ""))
	}
}
func copyText(text string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.CommandContext(ctx, "pbcopy")
		case "windows":
			cmd = exec.CommandContext(ctx, "clip.exe")
		default:
			if _, err := exec.LookPath("wl-copy"); err == nil {
				cmd = exec.CommandContext(ctx, "wl-copy")
			} else {
				cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
			}
		}
		cmd.Stdin = strings.NewReader(text)
		return clipboardMsg{cmd.Run()}
	}
}
func (m *Model) toastCmd(text string) tea.Cmd {
	m.toast = text
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearToastMsg{text} })
}

func (m *Model) returnHome() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.screen = home
	m.fields = nil
	m.result = core.Result{}
	m.err = nil
	m.events = nil
	m.progress = core.Progress{}
	m.viewport.SetContent("")
	return m.search.Focus()
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Widgets otherwise truncate oversized paste silently, which can turn a
	// hash or encoding into a successful result for incomplete input.
	if m.screen == form && len(m.fields) > 0 {
		text := ""
		switch input := msg.(type) {
		case tea.PasteMsg:
			text = input.Content
		case tea.KeyPressMsg:
			text = input.Text
		}
		if text != "" {
			field := &m.fields[m.focus]
			limit := field.input.CharLimit
			if field.multiline {
				limit = field.area.CharLimit
			}
			if limit > 0 && utf8.RuneCountInString(field.value())+utf8.RuneCountInString(text) > limit {
				field.rejectedInput = true
				return m, m.toastCmd(fmt.Sprintf("Input exceeds %d characters; use CLI stdin or hash --file", limit))
			}
			field.rejectedInput = false
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case clearToastMsg:
		if m.toast == msg.text {
			m.toast = ""
		}
		return m, nil
	case clipboardMsg:
		if msg.err != nil {
			return m, m.toastCmd("Clipboard unavailable: " + msg.err.Error())
		}
		return m, m.toastCmd("Copied to clipboard")
	case tickMsg:
		if m.screen == running && msg.id == m.jobID {
			m.ticks++
			return m, tick(m.jobID)
		}
		return m, nil
	case jobEvent:
		if msg.id != m.jobID || m.screen != running {
			return m, nil
		}
		if msg.done {
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.screen = resultScreen
			m.err = msg.err
			if msg.result != nil {
				m.result = *msg.result
			}
			m.setResultContent()
			m.viewport.GotoTop()
			if msg.err == nil {
				recent := []string{m.tool.ID}
				for _, id := range m.recent {
					if id != m.tool.ID {
						recent = append(recent, id)
					}
				}
				m.recent = recent
			}
			return m, nil
		}
		if msg.progress != nil {
			m.progress = *msg.progress
		}
		return m, waitJob(m.events, m.jobID)
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" || key == "ctrl+q" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if m.screen == home {
			items := m.filtered()
			switch key {
			case "esc":
				if m.search.Value() != "" {
					m.search.SetValue("")
					m.selected = 0
					return m, nil
				}
				return m, tea.Quit
			case "tab", "shift+tab":
				delta := 1
				if key == "shift+tab" {
					delta = -1
				}
				m.category = (m.category + delta + len(categories)) % len(categories)
				m.selected = 0
				return m, nil
			case "down", "ctrl+n":
				if len(items) > 0 {
					m.selected = (m.selected + 1) % len(items)
				}
				return m, nil
			case "up", "ctrl+p":
				if len(items) > 0 {
					m.selected = (m.selected - 1 + len(items)) % len(items)
				}
				return m, nil
			case "enter":
				if len(items) > 0 {
					return m, m.openTool(items[min(m.selected, len(items)-1)])
				}
				return m, nil
			case "ctrl+f":
				if len(items) > 0 {
					id := items[min(m.selected, len(items)-1)].ID
					m.engine.Config.ToggleFavorite(id)
					if err := core.SaveFavorites(m.engine.Config.Favorites); err != nil {
						m.engine.Config.ToggleFavorite(id)
						return m, m.toastCmd("Could not save favorite: " + err.Error())
					}
					return m, m.toastCmd("Favorites updated")
				}
				return m, nil
			}
			previous := m.search.Value()
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			if previous != m.search.Value() {
				m.selected = 0
			}
			return m, cmd
		}
		if m.screen == form {
			switch key {
			case "esc":
				return m, m.returnHome()
			case "ctrl+r", "ctrl+enter":
				return m, m.start()
			case "tab", "shift+tab":
				if len(m.fields) > 0 {
					m.fields[m.focus].blur()
					delta := 1
					if key == "shift+tab" {
						delta = -1
					}
					m.focus = (m.focus + delta + len(m.fields)) % len(m.fields)
					return m, m.fields[m.focus].focus()
				}
				return m, nil
			case "enter":
				if len(m.fields) == 0 || !m.fields[m.focus].multiline {
					return m, m.start()
				}
			}
		}
		if m.screen == running && key == "esc" {
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.jobID++
			m.screen = form
			if len(m.fields) > 0 {
				return m, m.fields[m.focus].focus()
			}
			return m, nil
		}
		if m.screen == resultScreen {
			switch key {
			case "esc", "q":
				return m, m.returnHome()
			case "e":
				m.screen = form
				if len(m.fields) > 0 {
					return m, m.fields[m.focus].focus()
				}
				return m, nil
			case "r":
				return m, m.start()
			case "j":
				m.jsonView = !m.jsonView
				m.setResultContent()
				m.viewport.GotoTop()
				return m, nil
			case "c":
				if m.err == nil {
					value := render.Result(m.result, 120, false)
					if m.result.Output != "" {
						value = m.result.Output
					}
					if m.jsonView {
						value = core.PrettyJSON(m.result)
					}
					return m, copyText(value)
				}
			}
		}
	}
	if m.screen == form && len(m.fields) > 0 {
		var cmd tea.Cmd
		field := &m.fields[m.focus]
		var previous string
		if field.rejectedInput {
			previous = field.value()
		}
		if field.multiline {
			field.area, cmd = field.area.Update(msg)
		} else {
			field.input, cmd = field.input.Update(msg)
		}
		if field.rejectedInput && previous != field.value() {
			field.rejectedInput = false
		}
		return m, cmd
	}
	if m.screen == resultScreen {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	if m.screen == home {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}
func style(color color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(color) }
func fit(s string, width, height int) string {
	width = max(width, 1)
	height = max(height, 0)
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
func (m *Model) header(width int) string {
	brand := style(render.Accent).Bold(true).Render("◈  T E R M B E L T")
	subtitle := "YOUR DAILY WORKBENCH"
	if m.screen != home {
		subtitle = m.tool.Category + " / " + strings.ToUpper(m.tool.ID)
	}
	right := style(render.Muted).Render("v" + m.version + "  ·  " + fmt.Sprint(len(core.Tools)) + " tools")
	space := max(1, width-ansi.StringWidth(brand)-ansi.StringWidth(right))
	return brand + strings.Repeat(" ", space) + right + "\n" + style(render.Faint).Render(subtitle) + "\n" + style(render.Border).Render(strings.Repeat("─", width))
}
func (m *Model) homeView(width, height int) string {
	items := m.filtered()
	selected := min(m.selected, max(0, len(items)-1))
	tabs := []string{}
	for i, category := range categories {
		label := category
		if i == m.category {
			label = style(render.Accent).Bold(true).Render("[ " + category + " ]")
		} else {
			label = style(render.Muted).Render(category)
		}
		tabs = append(tabs, label)
	}
	tabBar := strings.Join(tabs, "  ")
	if width < 90 {
		tabBar = style(render.Accent).Bold(true).Render("[ "+categories[m.category]+" ]") + style(render.Faint).Render("  tab / shift+tab to browse")
	}
	top := m.search.View() + "\n\n" + tabBar + "\n\n"
	remaining := max(3, height-5)
	listWidth := width
	sideWidth := 0
	if width >= 100 {
		listWidth = width * 54 / 100
		sideWidth = width - listWidth - 4
	}
	visible := max(1, (remaining-2)/3)
	offset := max(0, selected-visible+1)
	if offset+visible > len(items) {
		offset = max(0, len(items)-visible)
	}
	list := []string{}
	for i := offset; i < len(items) && i < offset+visible; i++ {
		tool := items[i]
		marker := "  "
		star := " "
		if m.engine.Config.IsFavorite(tool.ID) {
			star = "★"
		}
		name := tool.Name
		desc := tool.Description
		color := render.Text
		if i == selected {
			marker = "› "
			color = render.Accent
		}
		title := marker + star + " " + name
		title = ansi.Truncate(title, listWidth-2, "…")
		description := "    " + ansi.Truncate(desc, listWidth-6, "…")
		if i == selected {
			row := lipgloss.NewStyle().Foreground(color).Bold(true).Background(render.Panel).Width(listWidth).Render(title) + "\n" + lipgloss.NewStyle().Foreground(render.Muted).Background(render.Panel).Width(listWidth).Render(description)
			list = append(list, row, "")
		} else {
			list = append(list, style(color).Render(title)+"\n"+style(render.Faint).Render(description), "")
		}
	}
	if len(items) == 0 {
		list = append(list, style(render.Muted).Render("  No matching tools."), "", style(render.Faint).Render("  Try a different search or category."))
	}
	list = append(list, style(render.Faint).Render(fmt.Sprintf("  %d results · type to search", len(items))))
	left := fit(strings.Join(list, "\n"), listWidth, remaining)
	if sideWidth == 0 {
		return top + left
	}
	preview := ""
	if len(items) > 0 {
		tool := items[selected]
		badge := tool.Category + "  /  NETWORK ACCESS"
		if tool.Local {
			badge = tool.Category + "  /  RUNS LOCALLY"
		}
		preview = style(render.Teal).Render(badge) + "\n\n" + style(render.Text).Bold(true).Render(tool.Name) + "\n\n" + ansi.Hardwrap(style(render.Muted).Render(tool.Hint), sideWidth-4, true) + "\n\n" + style(render.Faint).Render("QUICK COMMAND") + "\n" + ansi.Hardwrap(style(render.Accent).Render(tool.Example), sideWidth-4, true) + "\n\n" + style(render.Muted).Render("enter  open workbench\nctrl+f toggle favorite") + "\n\n" + style(render.Faint).Render("One terminal. Fewer tabs.")
	}
	right := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(render.Border).Padding(1, 1).Width(sideWidth - 4).Render(fit(preview, sideWidth-4, max(1, remaining-4)))
	return top + lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", right)
}
func (m *Model) formView(width, height int) string {
	var b strings.Builder
	focusBottom := 0
	b.WriteString(style(render.Text).Bold(true).Render(m.tool.Name) + "\n")
	b.WriteString(ansi.Hardwrap(style(render.Muted).Render(m.tool.Hint), width-4, true) + "\n\n")
	for i := range m.fields {
		field := &m.fields[i]
		label := field.spec.Label
		color := render.Muted
		if i == m.focus {
			color = render.Accent
			label = "● " + label
		} else {
			label = "  " + label
		}
		b.WriteString(style(color).Bold(i == m.focus).Render(label) + "\n")
		view := field.input.View()
		if field.multiline {
			view = field.area.View()
		}
		b.WriteString(view + "\n")
		if field.spec.Help != "" {
			b.WriteString(style(render.Faint).Render("  "+field.spec.Help) + "\n")
		}
		b.WriteString("\n")
		if i == m.focus {
			focusBottom = len(strings.Split(b.String(), "\n")) - 1
		}
	}
	if m.err != nil {
		b.WriteString(style(render.Amber).Render(render.Safe(m.err.Error())) + "\n")
	}
	b.WriteString(style(render.Teal).Bold(true).Render("  ▶  Run utility") + style(render.Muted).Render("   ctrl+r"))
	b.WriteString("\n\n" + style(render.Faint).Render("CLI: "+m.tool.Example))
	lines := strings.Split(b.String(), "\n")
	offset := max(0, focusBottom-max(3, height-2))
	if offset >= len(lines) {
		offset = 0
	}
	return fit(strings.Join(lines[offset:], "\n"), width, height)
}
func (m *Model) runningView(width, height int) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := frames[m.ticks%len(frames)]
	barWidth := min(48, width-8)
	filled := int(float64(barWidth) * min(max(m.progress.Fraction, 0), 1))
	bar := style(render.Teal).Render(strings.Repeat("━", filled)) + style(render.Border).Render(strings.Repeat("━", barWidth-filled))
	content := style(render.Accent).Bold(true).Render(spinner+"  "+m.tool.Name) + "\n\n" + ansi.Hardwrap(style(render.Muted).Render(render.Safe(m.progress.Message)), width-8, true) + "\n\n" + bar + "\n\n"
	for _, metric := range m.progress.Metrics {
		content += style(render.Teal).Bold(true).Render(metric.Value+" "+metric.Unit) + "  " + style(render.Muted).Render(metric.Label) + "\n"
	}
	content += "\n" + style(render.Faint).Render(time.Since(m.started).Round(time.Second).String()+" elapsed  ·  esc cancels")
	return lipgloss.NewStyle().PaddingTop(min(4, height/5)).PaddingLeft(2).Render(content)
}
func (m *Model) View() tea.View {
	if m.width < 42 || m.height < 16 {
		content := "TERMBELT · resize to 42×16\n\nMake the terminal at least 42 columns × 16 rows.\nCtrl+C quits."
		if m.width < 24 {
			content = "Resize to 42×16\nCtrl+C quits."
		}
		v := tea.NewView(fit(content, m.width, min(4, m.height)))
		v.AltScreen = true
		return v
	}
	width := m.width - 4
	bodyHeight := m.height - 9
	content := ""
	footer := ""
	switch m.screen {
	case home:
		content = m.homeView(width, bodyHeight)
		footer = "↑↓ select   enter open   tab category   ctrl+f favorite   esc quit"
	case form:
		content = m.formView(width, bodyHeight)
		footer = "tab next field   ctrl+r run   enter run / new line   esc back"
	case running:
		content = m.runningView(width, bodyHeight)
		footer = "esc cancel   ctrl+c quit"
	case resultScreen:
		content = m.viewport.View()
		footer = fmt.Sprintf("↑↓ scroll   c copy   j JSON   e edit   r retry   esc home   %.0f%%", m.viewport.ScrollPercent()*100)
	}
	if m.toast != "" {
		footer = m.toast
	}
	frame := m.header(width) + "\n\n" + fit(content, width, bodyHeight) + "\n" + style(render.Border).Render(strings.Repeat("─", width)) + "\n" + style(render.Muted).Render(ansi.Truncate(render.Safe(footer), width, "…"))
	v := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(frame))
	v.AltScreen = true
	v.WindowTitle = "Termbelt · daily developer utilities"
	return v
}
