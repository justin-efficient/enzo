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
	// PriorityID is the chosen priority option, or empty when the form had no
	// priority to choose.
	PriorityID string
}

// Which field has focus.
const (
	focusTitle = iota
	focusBody
	focusPriority
)

// DraftForm asks for an issue title and body, and when editing an issue that is
// on a board, its priority.
type DraftForm struct {
	title  textinput.Model
	body   textarea.Model
	styles Styles
	repo   string
	// parent, when set, is the issue the new one will hang under.
	parent *ghclient.Issue
	// edit, when set, is the issue being changed rather than created.
	edit *ghclient.Issue
	// prio, when set, adds a priority selector; choice indexes prio.Options,
	// or is -1 for none chosen.
	prio     *ghclient.PriorityField
	choice   int
	focus    int
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

// NewEditForm builds the form `enzo list` shows for an existing issue, filled
// in with its current title and body. prio may be nil, for an issue that is on
// no board with a Priority field; the form then has no selector.
//
// The selector starts on the issue's current priority, or on P2 when it has
// none.
func NewEditForm(repo string, issue ghclient.Issue, prio *ghclient.PriorityField, styles Styles) DraftForm {
	m := NewDraftForm(repo, nil, styles)
	m.edit = &issue
	m.title.SetValue(issue.Title)
	// GitHub keeps whatever line endings the web form sent, often CRLF.
	m.body.SetValue(strings.ReplaceAll(issue.Body, "\r\n", "\n"))
	m.body.SetHeight(10)

	return m.WithPriority(prio)
}

// WithPriority adds the priority selector. prio may be nil, which leaves the
// form without one. The selector starts on the issue's current priority, or on
// P2 when it has none.
func (m DraftForm) WithPriority(prio *ghclient.PriorityField) DraftForm {
	m.choice = -1
	if prio == nil {
		return m
	}
	m.prio = prio
	for i, o := range prio.Options {
		if prio.Current != "" && o.ID == prio.Current {
			m.choice = i
			break
		}
		if prio.Current == "" && o.Name == defaultPriority {
			m.choice = i
		}
	}
	return m
}

// defaultPriority is what the selector starts on for an issue with none.
const defaultPriority = "P2"

// submitKey is the key that accepts the form.
const submitKey = "ctrl+s"

// Draft returns what was entered. Title is trimmed; an empty one means the
// form was not completed.
func (m DraftForm) Draft() Draft {
	return Draft{
		Title:      strings.TrimSpace(m.draft.Title),
		Body:       strings.TrimSpace(m.draft.Body),
		PriorityID: m.draft.PriorityID,
	}
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

		case "tab":
			return m.moveFocus(1), nil

		case "shift+tab":
			return m.moveFocus(-1), nil

		case "enter":
			// Enter advances from the title; inside the body it is a newline.
			switch m.focus {
			case focusTitle:
				return m.moveFocus(1), nil
			case focusPriority:
				return m, nil
			}

		case submitKey, "ctrl+n":
			return m.submit()

		case "left", "h", "up", "k":
			if m.focus == focusPriority {
				return m.moveChoice(-1), nil
			}

		case "right", "l", "down", "j":
			if m.focus == focusPriority {
				return m.moveChoice(1), nil
			}
		}

		// The selector has no text input to type into.
		if m.focus == focusPriority {
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focus == focusBody {
		m.body, cmd = m.body.Update(msg)
	} else {
		m.title, cmd = m.title.Update(msg)
	}
	return m, cmd
}

// submit accepts the form unless the title is blank.
func (m DraftForm) submit() (tea.Model, tea.Cmd) {
	// Submitting with a blank title would leave a nameless issue.
	if strings.TrimSpace(m.title.Value()) == "" {
		return m, nil
	}
	m.draft = Draft{Title: m.title.Value(), Body: m.body.Value()}
	if m.prio != nil && m.choice >= 0 {
		m.draft.PriorityID = m.prio.Options[m.choice].ID
	}
	m.done = true
	return m, tea.Quit
}

// moveChoice steps the priority selection, wrapping at the ends. From "none"
// it lands on the first option going right and the last going left.
func (m DraftForm) moveChoice(d int) DraftForm {
	n := len(m.prio.Options)
	switch {
	case m.choice < 0 && d > 0:
		m.choice = 0
	case m.choice < 0:
		m.choice = n - 1
	default:
		m.choice = (m.choice + d + n) % n
	}
	return m
}

// fields is how many fields tab cycles through.
func (m DraftForm) fields() int {
	if m.prio != nil {
		return 3
	}
	return 2
}

// moveFocus moves between the fields, wrapping at the ends.
func (m DraftForm) moveFocus(d int) DraftForm {
	m.focus = (m.focus + d + m.fields()) % m.fields()
	m.title.Blur()
	m.body.Blur()
	switch m.focus {
	case focusTitle:
		m.title.Focus()
	case focusBody:
		m.body.Focus()
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
	if m.edit != nil {
		heading = fmt.Sprintf("edit #%d in %s", m.edit.Number, m.repo)
	} else if m.parent != nil {
		heading = fmt.Sprintf("new sub-issue of #%d in %s", m.parent.Number, m.repo)
	}
	b.WriteString(s.Title.Render(heading) + "\n")
	if m.parent != nil {
		b.WriteString(s.Help.Render("parent: "+m.parent.Title) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(m.fieldLabel("title", m.focus == focusTitle) + "\n")
	b.WriteString(m.title.View() + "\n\n")
	b.WriteString(m.fieldLabel("body", m.focus == focusBody) + "\n")
	b.WriteString(m.body.View() + "\n\n")
	if m.prio != nil {
		b.WriteString(m.fieldLabel("priority", m.focus == focusPriority) + "\n")
		b.WriteString(m.priorityRow() + "\n\n")
	}

	verb := "create"
	if m.edit != nil {
		verb = "save"
	}
	help := "tab switch · " + submitKey + " " + verb + " · esc cancel"
	if m.focus == focusPriority {
		help = "←/→ change · tab switch · " + submitKey + " " + verb + " · esc cancel"
	}
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

// priorityRow draws the choices side by side, the chosen one bracketed and in
// its board color.
func (m DraftForm) priorityRow() string {
	parts := make([]string, 0, len(m.prio.Options))
	for i, o := range m.prio.Options {
		if i == m.choice {
			parts = append(parts, m.styles.PriorityNumber(o.Color).Render("["+o.Name+"]"))
			continue
		}
		parts = append(parts, m.styles.Help.Render(" "+o.Name+" "))
	}
	return "  " + strings.Join(parts, " ")
}
