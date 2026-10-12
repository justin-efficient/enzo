// Package ui holds enzo's bubbletea models.
package ui

import "charm.land/lipgloss/v2"

// Styles controls how the picker renders. Tests use PlainStyles so golden
// output has no escape codes.
type Styles struct {
	Title    lipgloss.Style
	Cursor   lipgloss.Style
	Selected lipgloss.Style
	Normal   lipgloss.Style
	Number   lipgloss.Style
	// PriorityNumber styles an issue number in a GitHub option color such as
	// "RED". An unknown or empty color gives Number.
	PriorityNumber func(color string) lipgloss.Style
	Label          lipgloss.Style
	Help           lipgloss.Style
	Empty          lipgloss.Style
	CursorStr      string
}

// DefaultStyles is the colored style set used in a real terminal.
func DefaultStyles() Styles {
	st := Styles{
		Title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("170")),
		Cursor:    lipgloss.NewStyle().Foreground(lipgloss.Color("170")),
		Selected:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("170")),
		Normal:    lipgloss.NewStyle(),
		Number:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Label:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		Help:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Empty:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true),
		CursorStr: "›",
	}
	st.PriorityNumber = func(color string) lipgloss.Style {
		if c, ok := priorityColors[color]; ok {
			return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
		}
		return st.Number
	}
	return st
}

// priorityColors maps GitHub's single-select option colors to terminal ones.
var priorityColors = map[string]string{
	"RED":    "9",
	"ORANGE": "208",
	"YELLOW": "11",
	"GREEN":  "10",
	"BLUE":   "12",
	"PURPLE": "135",
	"PINK":   "205",
	"GRAY":   "245",
}

// PlainStyles renders without color, for tests and non-TTY output.
func PlainStyles() Styles {
	s := lipgloss.NewStyle()
	return Styles{
		Title: s, Cursor: s, Selected: s, Normal: s,
		Number: s, Label: s, Help: s, Empty: s,
		PriorityNumber: func(string) lipgloss.Style { return s },
		CursorStr:      ">",
	}
}
