package render

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rahatsagor/termbelt/internal/core"
)

var (
	Accent = lipgloss.Color("#B8A1FF")
	Teal   = lipgloss.Color("#67E8C9")
	Text   = lipgloss.Color("#E4E6EE")
	Muted  = lipgloss.Color("#9699B0")
	Faint  = lipgloss.Color("#555A73")
	Panel  = lipgloss.Color("#191C2B")
	Border = lipgloss.Color("#363B53")
	Amber  = lipgloss.Color("#F6C177")
)

// Terminal data is untrusted, even when it comes from a user's clipboard.
func Safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
		case unicode.IsControl(r):
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
func colored(s string, color color.Color, bold, enabled bool) string {
	if !enabled {
		return s
	}
	return lipgloss.NewStyle().Foreground(color).Bold(bold).Render(s)
}
func Result(r core.Result, width int, color bool) string {
	width = max(width, 20)
	var b strings.Builder
	b.WriteString(colored(Safe(r.Title), Accent, true, color))
	b.WriteString("\n")
	if r.Summary != "" {
		b.WriteString(ansi.Hardwrap(colored(Safe(r.Summary), Muted, false, color), width, true))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if len(r.Metrics) > 0 {
		cards := []string{}
		for _, m := range r.Metrics {
			value := Safe(m.Value)
			if m.Unit != "" {
				value += " " + Safe(m.Unit)
			}
			cards = append(cards, colored(value, Teal, true, color)+"  "+colored(Safe(m.Label), Muted, false, color))
		}
		b.WriteString(ansi.Hardwrap(strings.Join(cards, "    │    "), width, true))
		b.WriteString("\n\n")
	}
	if r.Output != "" {
		b.WriteString(ansi.Hardwrap(Safe(r.Output), width, true))
		b.WriteString("\n\n")
	}
	for _, section := range r.Sections {
		if len(section.Rows) == 0 && section.Text == "" {
			continue
		}
		b.WriteString(colored(Safe(section.Title), Accent, true, color))
		b.WriteString("\n")
		labelWidth := 0
		for _, row := range section.Rows {
			labelWidth = max(labelWidth, ansi.StringWidth(Safe(row.Label)))
		}
		labelWidth = min(labelWidth, min(26, width/3))
		for _, row := range section.Rows {
			label := ansi.Truncate(Safe(row.Label), labelWidth, "…")
			label = label + strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(label)))
			value := Safe(row.Value)
			if value == "" {
				value = "—"
			}
			lines := strings.Split(ansi.Hardwrap(value, max(8, width-labelWidth-3), true), "\n")
			for i, line := range lines {
				if i == 0 {
					b.WriteString(colored(label, Muted, false, color) + "   " + line)
				} else {
					b.WriteString(strings.Repeat(" ", labelWidth+3) + line)
				}
				b.WriteString("\n")
			}
		}
		if section.Text != "" {
			b.WriteString(ansi.Hardwrap(Safe(section.Text), width, true))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if r.Table != nil {
		b.WriteString(Table(*r.Table, width, color))
		b.WriteString("\n")
	}
	for _, note := range r.Notes {
		wrapped := ansi.Hardwrap(Safe(note), width-2, true)
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			prefix := "  "
			if i == 0 {
				prefix = "· "
			}
			b.WriteString(colored(prefix+line, Muted, false, color))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
func Table(table core.Table, width int, color bool) string {
	if len(table.Headers) == 0 {
		return ""
	}
	n := len(table.Headers)
	available := max(n*5, width-3*(n-1))
	sizes := make([]int, n)
	for i, h := range table.Headers {
		sizes[i] = max(4, ansi.StringWidth(Safe(h)))
		for _, row := range table.Rows {
			if i < len(row) {
				for _, line := range strings.Split(Safe(row[i]), "\n") {
					sizes[i] = max(sizes[i], min(60, ansi.StringWidth(line)))
				}
			}
		}
	}
	for {
		sum := 0
		largest := 0
		for i, v := range sizes {
			sum += v
			if v > sizes[largest] {
				largest = i
			}
		}
		if sum <= available {
			break
		}
		sizes[largest]--
		if sizes[largest] <= 4 {
			break
		}
	}
	var b strings.Builder
	headers := []string{}
	for i, h := range table.Headers {
		h = ansi.Truncate(Safe(h), sizes[i], "…")
		headers = append(headers, h+strings.Repeat(" ", max(0, sizes[i]-ansi.StringWidth(h))))
	}
	b.WriteString(colored(strings.Join(headers, "   "), Muted, true, color))
	b.WriteString("\n")
	b.WriteString(colored(strings.Repeat("─", min(width, available+3*(n-1))), Faint, false, color))
	b.WriteString("\n")
	for _, row := range table.Rows {
		lines := make([][]string, n)
		height := 1
		for i := 0; i < n; i++ {
			value := ""
			if i < len(row) {
				value = Safe(row[i])
			}
			lines[i] = strings.Split(ansi.Hardwrap(value, sizes[i], true), "\n")
			height = max(height, len(lines[i]))
		}
		for line := 0; line < height; line++ {
			cells := []string{}
			for i := 0; i < n; i++ {
				value := ""
				if line < len(lines[i]) {
					value = lines[i][line]
				}
				padding := strings.Repeat(" ", max(0, sizes[i]-ansi.StringWidth(value)))
				if color && line == 0 {
					switch value {
					case "unregistered", "valid", "connected":
						value = colored(value, Teal, true, color)
					case "registered":
						value = colored(value, Muted, false, color)
					case "unknown", "failed":
						value = colored(value, Amber, false, color)
					}
				}
				cells = append(cells, value+padding)
			}
			b.WriteString(strings.TrimRight(strings.Join(cells, "   "), " "))
			b.WriteString("\n")
		}
	}
	if len(table.Rows) == 0 {
		b.WriteString(colored("No matching rows.", Muted, false, color) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
