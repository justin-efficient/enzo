// Package ui holds enzo's bubbletea models.
package ui

import "charm.land/lipgloss/v2"

// Styles controls how the picker renders. Tests use PlainStyles so golden
// output has no escape codes.
type Styles struct {
	Title     lipgloss.Style
	Cursor    lipgloss.Style
	Selected  lipgloss.Style
	Normal    lipgloss.Style
	Number    lipgloss.Style
	Label     lipgloss.Style
	Help      lipgloss.Style
	Empty     lipgloss.Style
	CursorStr string
}

// DefaultStyles is the colored style set used in a real terminal.
func DefaultStyles() Styles {
	return Styles{
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
}

// PlainStyles renders without color, for tests and non-TTY output.
func PlainStyles() Styles {
	s := lipgloss.NewStyle()
	return Styles{
		Title: s, Cursor: s, Selected: s, Normal: s,
		Number: s, Label: s, Help: s, Empty: s,
		CursorStr: ">",
	}
}
