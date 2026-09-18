package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// PollInterval is how often the waiter re-checks its condition.
const PollInterval = 400 * time.Millisecond

// PollTimeout is how long the waiter keeps checking before giving up.
const PollTimeout = 30 * time.Second

// Waiter shows a spinner while it polls for something to become true. GitHub's
// issue listing lags a few seconds behind a creation, so enzo waits for the
// listing to catch up rather than remembering what it made.
type Waiter struct {
	spin    spinner.Model
	styles  Styles
	message string
	poll    func() (bool, error)

	interval time.Duration
	timeout  time.Duration
	elapsed  time.Duration

	err      error
	timedOut bool
	canceled bool
	done     bool
}

var _ tea.Model = Waiter{}

// pollResultMsg carries the outcome of one poll.
type pollResultMsg struct {
	ready bool
	err   error
}

// NewWaiter builds a waiter that polls until poll reports true.
func NewWaiter(message string, poll func() (bool, error), styles Styles) Waiter {
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(styles.Cursor))
	return Waiter{
		spin:     s,
		styles:   styles,
		message:  message,
		poll:     poll,
		interval: PollInterval,
		timeout:  PollTimeout,
	}
}

// WithTiming overrides the poll interval and timeout. Tests use it to keep the
// suite fast.
func (m Waiter) WithTiming(interval, timeout time.Duration) Waiter {
	m.interval, m.timeout = interval, timeout
	return m
}

// Err returns the error a poll reported, if any.
func (m Waiter) Err() error { return m.err }

// TimedOut reports whether the waiter gave up before the condition held.
func (m Waiter) TimedOut() bool { return m.timedOut }

// Canceled reports whether the user pressed esc rather than waiting.
func (m Waiter) Canceled() bool { return m.canceled }

// Init implements tea.Model.
func (m Waiter) Init() tea.Cmd {
	// Poll straight away: the condition may already hold.
	return tea.Batch(m.spin.Tick, m.pollNow())
}

// pollNow runs the caller's check off the update loop.
func (m Waiter) pollNow() tea.Cmd {
	poll := m.poll
	return func() tea.Msg {
		ready, err := poll()
		return pollResultMsg{ready: ready, err: err}
	}
}

// pollAfter waits one interval, then polls again.
func (m Waiter) pollAfter() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return retryMsg{} })
}

// retryMsg asks for another poll once the interval has passed.
type retryMsg struct{}

// Update implements tea.Model.
func (m Waiter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			m.canceled, m.done = true, true
			return m, tea.Quit
		}
		return m, nil

	case pollResultMsg:
		if msg.err != nil {
			m.err, m.done = msg.err, true
			return m, tea.Quit
		}
		if msg.ready {
			m.done = true
			return m, tea.Quit
		}
		m.elapsed += m.interval
		if m.elapsed >= m.timeout {
			m.timedOut, m.done = true, true
			return m, tea.Quit
		}
		return m, m.pollAfter()

	case retryMsg:
		return m, m.pollNow()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m Waiter) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", m.spin.View(), m.styles.Normal.Render(m.message))
	b.WriteString("\n\n" + m.styles.Help.Render("esc to stop waiting"))
	return tea.NewView(b.String())
}
