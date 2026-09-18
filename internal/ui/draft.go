package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
)

// Draft is what the issue form collected.
type Draft struct {
	Title string
	Body  string
}

// DraftForm asks for an issue title and body.
type DraftForm struct {
	title  textinput.Model
	body   textarea.Model
	styles Styles
	repo   string
	// parent, when set, is the issue the new one will hang under.
	parent   *ghclient.Issue
	onBody   bool
	draft    Draft
	canceled bool
	done     bool
}

var _ tea.Model = DraftForm{}

// NewDraftForm builds the form shown by `enzo new`. parent may be nil.
func NewDraftForm(repo string, parent *ghclient.Issue, styles Styles) DraftForm {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "a short, specific title"
	ti.CharLimit = 255
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "optional details…"
	ta.SetHeight(6)
	ta.ShowLineNumbers = false
	ta.Blur()

	return DraftForm{title: ti, body: ta, styles: styles, repo: repo, parent: parent}
}

// Draft returns what was entered. Title is trimmed; an empty one means the
// form was not completed.
func (m DraftForm) Draft() Draft {
	return Draft{Title: strings.TrimSpace(m.draft.Title), Body: strings.TrimSpace(m.draft.Body)}
}

// Canceled reports whether the user aborted the form.
func (m DraftForm) Canceled() bool { return m.canceled }

// Init implements tea.Model.
func (m DraftForm) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m DraftForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if w := msg.Width - 4; w > 0 {
			m.title.SetWidth(w)
			m.body.SetWidth(w)
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.canceled, m.done = true, true
			return m, tea.Quit

		case "tab", "shift+tab":
			return m.toggleFocus(), nil

		case "enter":
			// Enter advances from the title; inside the body it is a newline.
			if !m.onBody {
				return m.toggleFocus(), nil
			}

		case "ctrl+n":
			// Submitting with a blank title would create a nameless issue.
			if strings.TrimSpace(m.title.Value()) == "" {
				return m, nil
			}
			m.draft = Draft{Title: m.title.Value(), Body: m.body.Value()}
			m.done = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	if m.onBody {
		m.body, cmd = m.body.Update(msg)
	} else {
		m.title, cmd = m.title.Update(msg)
	}
	return m, cmd
}

// toggleFocus moves between the title and body fields.
func (m DraftForm) toggleFocus() DraftForm {
	m.onBody = !m.onBody
	if m.onBody {
		m.title.Blur()
		m.body.Focus()
	} else {
		m.body.Blur()
		m.title.Focus()
	}
	return m
}

// View implements tea.Model.
func (m DraftForm) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	s := m.styles

	heading := "new issue in " + m.repo
	if m.parent != nil {
		heading = fmt.Sprintf("new sub-issue of #%d in %s", m.parent.Number, m.repo)
	}
	b.WriteString(s.Title.Render(heading) + "\n")
	if m.parent != nil {
		b.WriteString(s.Help.Render("parent: "+m.parent.Title) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(m.fieldLabel("title", !m.onBody) + "\n")
	b.WriteString(m.title.View() + "\n\n")
	b.WriteString(m.fieldLabel("body", m.onBody) + "\n")
	b.WriteString(m.body.View() + "\n\n")

	help := "tab switch · ctrl+n create · esc cancel"
	if strings.TrimSpace(m.title.Value()) == "" {
		help = "tab switch · a title is required · esc cancel"
	}
	b.WriteString(s.Help.Render(help))
	return tea.NewView(b.String())
}

// fieldLabel marks which field has focus.
func (m DraftForm) fieldLabel(name string, focused bool) string {
	if focused {
		return m.styles.Cursor.Render(m.styles.CursorStr) + " " + m.styles.Selected.Render(name)
	}
	return strings.Repeat(" ", len([]rune(m.styles.CursorStr))) + " " + m.styles.Normal.Render(name)
}
