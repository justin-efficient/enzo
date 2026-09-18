package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// TokenPrompt asks for a GitHub token, masking what is typed.
type TokenPrompt struct {
	input    textinput.Model
	styles   Styles
	repo     string
	value    string
	canceled bool
	done     bool
}

var _ tea.Model = TokenPrompt{}

// NewTokenPrompt builds the prompt shown by `enzo setup`.
func NewTokenPrompt(repo string, styles Styles) TokenPrompt {
	ti := textinput.New()
	ti.Prompt = "token: "
	ti.Placeholder = "ghp_…"
	ti.EchoMode = textinput.EchoPassword
	ti.CharLimit = 255
	ti.Focus()
	return TokenPrompt{input: ti, styles: styles, repo: repo}
}

// Value returns the token that was entered.
func (m TokenPrompt) Value() string { return strings.TrimSpace(m.value) }

// Canceled reports whether the user aborted the prompt.
func (m TokenPrompt) Canceled() bool { return m.canceled }

// Init implements tea.Model.
func (m TokenPrompt) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m TokenPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc", "ctrl+c":
			m.canceled, m.done = true, true
			return m, tea.Quit
		case "enter":
			m.value = m.input.Value()
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m TokenPrompt) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("enzo setup — "+m.repo) + "\n\n")
	b.WriteString(m.styles.Help.Render("paste a GitHub token with `repo` scope") + "\n")
	b.WriteString(m.input.View() + "\n\n")
	b.WriteString(m.styles.Help.Render("enter save · esc cancel"))
	return tea.NewView(b.String())
}
