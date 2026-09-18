package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/version"
)

// Action is what the user chose in the picker.
type Action int

const (
	// ActionCancel means the user pressed esc; there is nothing to do.
	ActionCancel Action = iota
	// ActionNew means the user picked the "new issue" row.
	ActionNew
	// ActionGrab means the user picked an existing issue.
	ActionGrab
	// ActionNewSub means the user asked for a sub-issue of the highlighted
	// issue, which Result.Issue carries.
	ActionNewSub
)

// Result is the picker's outcome. Issue is only meaningful for ActionGrab.
type Result struct {
	Action Action
	Issue  ghclient.Issue
}

// defaultVisible is how many issue rows to show when the terminal height is
// unknown.
const defaultVisible = 10

// Picker is a bubbletea model listing issues. `enzo list` pins a "new" row on
// top; the parent picker used by `enzo new sub` does not.
type Picker struct {
	repo   string
	nodes  []ghclient.Node
	styles Styles

	heading string
	// newRow pins the "+ new issue" row above the list.
	newRow bool
	// newSub allows ctrl+n to open a sub-issue of the highlighted row.
	newSub bool

	cursor  int // 0 is the "new" row; issue i is at cursor i+1
	offset  int // first visible issue index, for scrolling
	visible int

	result Result
	done   bool
}

var _ tea.Model = Picker{}

// NewPicker builds the `enzo list` picker, with "new" pinned on top.
func NewPicker(repo string, issues []ghclient.Issue, styles Styles) Picker {
	return Picker{
		repo:    repo,
		nodes:   ghclient.Arrange(repo, issues),
		styles:  styles,
		visible: defaultVisible,
		heading: "open issues assigned to you in " + repo,
		newRow:  true,
		newSub:  true,
	}
}

// NewParentPicker builds the picker `enzo new sub` uses to choose a parent. It
// has no "new" row: you are choosing among issues that already exist.
func NewParentPicker(repo string, issues []ghclient.Issue, styles Styles) Picker {
	return Picker{
		repo:    repo,
		nodes:   ghclient.Arrange(repo, issues),
		styles:  styles,
		visible: defaultVisible,
		heading: "choose a parent issue in " + repo,
	}
}

// Result returns what the user chose. It is only valid once the program exits.
func (m Picker) Result() Result { return m.result }

// Done reports whether the picker has been resolved.
func (m Picker) Done() bool { return m.done }

// rows is the number of selectable rows, including "new" when it is shown.
func (m Picker) rows() int { return len(m.nodes) + m.offsetForNewRow() }

// offsetForNewRow is 1 when the "new" row occupies cursor position 0.
func (m Picker) offsetForNewRow() int {
	if m.newRow {
		return 1
	}
	return 0
}

// issueAt returns the issue under the cursor, and whether one is there at all.
func (m Picker) issueAt(cursor int) (ghclient.Issue, bool) {
	i := cursor - m.offsetForNewRow()
	if i < 0 || i >= len(m.nodes) {
		return ghclient.Issue{}, false
	}
	return m.nodes[i].Issue, true
}

// Init implements tea.Model.
func (m Picker) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Leave room for the title, blank line and help line.
		if n := msg.Height - 4; n > 0 {
			m.visible = n
		}
		m.clampOffset()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.result = Result{Action: ActionCancel}
			m.done = true
			return m, tea.Quit

		case "up", "k", "shift+tab":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = m.rows() - 1
			}
			m.clampOffset()
			return m, nil

		case "down", "j", "tab":
			m.cursor++
			if m.cursor >= m.rows() {
				m.cursor = 0
			}
			m.clampOffset()
			return m, nil

		case "home", "g":
			m.cursor = 0
			m.clampOffset()
			return m, nil

		case "end", "G":
			m.cursor = m.rows() - 1
			m.clampOffset()
			return m, nil

		case "ctrl+n":
			if !m.newSub {
				return m, nil
			}
			iss, ok := m.issueAt(m.cursor)
			if !ok {
				// On the pinned "new" row there is nothing to parent to, so
				// ctrl+n opens a top-level issue, same as enter.
				m.result = Result{Action: ActionNew}
			} else {
				m.result = Result{Action: ActionNewSub, Issue: iss}
			}
			m.done = true
			return m, tea.Quit

		case "enter":
			iss, ok := m.issueAt(m.cursor)
			if !ok {
				// Only the pinned "new" row has no issue behind it.
				if !m.newRow {
					return m, nil
				}
				m.result = Result{Action: ActionNew}
			} else {
				m.result = Result{Action: ActionGrab, Issue: iss}
			}
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// clampOffset keeps the cursor inside the visible window.
func (m *Picker) clampOffset() {
	if len(m.nodes) == 0 {
		m.offset = 0
		return
	}
	// The "new" row is pinned, so only issue rows scroll.
	idx := m.cursor - m.offsetForNewRow()
	if idx < 0 {
		m.offset = 0
		return
	}
	if idx < m.offset {
		m.offset = idx
	}
	if idx >= m.offset+m.visible {
		m.offset = idx - m.visible + 1
	}
	if max := len(m.nodes) - m.visible; m.offset > max {
		if max < 0 {
			max = 0
		}
		m.offset = max
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// View implements tea.Model.
func (m Picker) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	s := m.styles
	numW := m.numberWidth()

	fmt.Fprintf(&b, "%s\n\n", s.Title.Render(m.heading))
	if m.newRow {
		b.WriteString(m.cursorFor(0) + s.Selected.Render("+ new issue") + "\n")
	}

	if len(m.nodes) == 0 {
		b.WriteString("\n" + s.Empty.Render("  "+m.emptyMessage()) + "\n")
	}

	end := m.offset + m.visible
	if end > len(m.nodes) {
		end = len(m.nodes)
	}
	for i := m.offset; i < end; i++ {
		row := i + m.offsetForNewRow()
		b.WriteString(m.cursorFor(row) + m.issueRow(m.nodes[i], row == m.cursor, numW) + "\n")
	}

	if more := len(m.nodes) - end; more > 0 {
		fmt.Fprintf(&b, "%s\n", s.Help.Render(fmt.Sprintf("  … %d more", more)))
	}

	b.WriteString("\n" + s.Help.Render(m.helpLine()))
	return tea.NewView(b.String())
}

// helpLine lists the keys this picker responds to, signed with the banner so
// the list says which enzo drew it. version.Banner is the one place that
// string is built; `enzo --version`, the usage text and the commit `enzo
// start` writes all use the same call.
func (m Picker) helpLine() string {
	keys := "↑/↓ move · enter select · esc cancel"
	if m.newSub {
		keys = "↑/↓ move · enter select · ctrl+n sub-issue · esc cancel"
	}
	return version.Banner() + " · " + keys
}

// emptyMessage explains an empty list in the terms of whichever picker this is.
func (m Picker) emptyMessage() string {
	if m.newRow {
		return "nothing assigned to you here"
	}
	return "no open issues here to parent this one"
}

// numberWidth sizes the issue-number column to the widest number on screen, so
// the titles line up.
func (m Picker) numberWidth() (w int) {
	for _, n := range m.nodes {
		if l := len(fmt.Sprintf("#%d", n.Issue.Number)); l > w {
			w = l
		}
	}
	return w
}

// cursorFor returns the gutter for a row: the marker when selected, matching
// blank space otherwise.
func (m Picker) cursorFor(idx int) string {
	if idx == m.cursor {
		return m.styles.Cursor.Render(m.styles.CursorStr) + " "
	}
	return strings.Repeat(" ", len([]rune(m.styles.CursorStr))) + " "
}

// issueRow renders one issue as "#12  title [labels]", indented by its depth in
// the sub-issue tree. Label colors stay on the selected row rather than being
// flattened by the highlight.
func (m Picker) issueRow(n ghclient.Node, selected bool, numW int) string {
	s := m.styles
	iss := n.Issue

	cols := []string{Indent(n.Depth) + s.Number.Render(padRight(fmt.Sprintf("#%d", iss.Number), numW))}

	title := s.Normal.Render(iss.Title)
	if selected {
		title = s.Selected.Render(iss.Title)
	}
	cols = append(cols, title)

	if labels := renderLabels(s, iss.Labels); labels != "" {
		cols = append(cols, labels)
	}
	return strings.Join(cols, " ")
}

// IndentWidth is how far each level of the sub-issue tree is indented.
const IndentWidth = 2

// Indent returns the leading space for a tree depth.
func Indent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat(" ", depth*IndentWidth)
}

func padRight(s string, w int) string {
	if n := w - len(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func renderLabels(s Styles, labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, s.Label.Render("["+l+"]"))
	}
	return strings.Join(parts, " ")
}
