package ui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Busy shows a spinner and a message while work runs elsewhere. The program's
// owner sends a BusyDoneMsg when the work finishes.
type Busy struct {
	spin    spinner.Model
	styles  Styles
	message string

	err      error
	canceled bool
	done     bool
}

var _ tea.Model = Busy{}

// BusyDoneMsg tells a Busy its work has finished.
type BusyDoneMsg struct{ Err error }

// NewBusy builds a spinner with a message.
func NewBusy(message string, styles Styles) Busy {
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(styles.Cursor))
	return Busy{spin: s, styles: styles, message: message}
}

// Err returns the error the work finished with.
func (m Busy) Err() error { return m.err }

// Canceled reports whether the user pressed ctrl+c rather than waiting.
func (m Busy) Canceled() bool { return m.canceled }

// Init implements tea.Model.
func (m Busy) Init() tea.Cmd { return m.spin.Tick }

// Update implements tea.Model.
func (m Busy) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Only ctrl+c: esc must not abandon a half-finished write by accident.
		if msg.String() == "ctrl+c" {
			m.canceled, m.done = true, true
			return m, tea.Quit
		}
	case BusyDoneMsg:
		m.err, m.done = msg.Err, true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m Busy) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(fmt.Sprintf("%s %s", m.spin.View(), m.styles.Normal.Render(m.message)))
}
