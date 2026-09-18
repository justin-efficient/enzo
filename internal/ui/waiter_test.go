package ui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

var errPoll = errors.New("poll failed")

// readyAfter returns a poll that reports true on the nth call.
func readyAfter(n int32) (func() (bool, error), *atomic.Int32) {
	var calls atomic.Int32
	return func() (bool, error) {
		return calls.Add(1) >= n, nil
	}, &calls
}

// drain runs a model's command chain until it quits, so the waiter can be
// tested without a terminal.
func drain(t *testing.T, m Waiter, cmd tea.Cmd) Waiter {
	t.Helper()
	var model tea.Model = m
	for i := 0; i < 500 && cmd != nil; i++ {
		msg := cmd()
		if _, quitting := msg.(tea.QuitMsg); quitting {
			break
		}
		model, cmd = model.Update(msg)
	}
	w, ok := model.(Waiter)
	if !ok {
		t.Fatalf("model is %T, want Waiter", model)
	}
	return w
}

// fast keeps the suite quick; the real timings are in PollInterval/PollTimeout.
func fast(m Waiter) Waiter { return m.WithTiming(time.Millisecond, 200*time.Millisecond) }

func TestWaiterFinishesWhenReady(t *testing.T) {
	poll, calls := readyAfter(1)
	m := fast(NewWaiter("waiting", poll, PlainStyles()))

	w := drain(t, m, m.pollNow())
	if w.Err() != nil {
		t.Fatalf("Err = %v", w.Err())
	}
	if w.TimedOut() {
		t.Error("should not have timed out")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("polled %d times, want 1 — a ready condition should not be re-polled", got)
	}
}

// It keeps polling while the condition is false, which is the whole point.
func TestWaiterRetriesUntilReady(t *testing.T) {
	poll, calls := readyAfter(4)
	m := fast(NewWaiter("waiting", poll, PlainStyles()))

	w := drain(t, m, m.pollNow())
	if w.Err() != nil {
		t.Fatalf("Err = %v", w.Err())
	}
	if w.TimedOut() {
		t.Error("should not have timed out before the condition held")
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("polled %d times, want 4", got)
	}
}

// A condition that never holds must not spin forever.
func TestWaiterTimesOut(t *testing.T) {
	never := func() (bool, error) { return false, nil }
	m := NewWaiter("waiting", never, PlainStyles()).WithTiming(time.Millisecond, 5*time.Millisecond)

	w := drain(t, m, m.pollNow())
	if !w.TimedOut() {
		t.Error("a condition that never holds should time out")
	}
	if w.Err() != nil {
		t.Errorf("a timeout is not an error, got %v", w.Err())
	}
}

func TestWaiterReportsPollErrors(t *testing.T) {
	m := fast(NewWaiter("waiting", func() (bool, error) { return false, errPoll }, PlainStyles()))

	w := drain(t, m, m.pollNow())
	if !errors.Is(w.Err(), errPoll) {
		t.Errorf("Err = %v, want the poll error", w.Err())
	}
	if w.TimedOut() {
		t.Error("an error is not a timeout")
	}
}

func TestWaiterCancelKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyEscape},
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		t.Run(key.String(), func(t *testing.T) {
			never := func() (bool, error) { return false, nil }
			m := fast(NewWaiter("waiting", never, PlainStyles()))

			model, cmd := m.Update(key)
			w := model.(Waiter)
			if !w.Canceled() {
				t.Errorf("%s should cancel the wait", key.String())
			}
			if cmd == nil {
				t.Fatal("canceling should quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Errorf("canceling should quit, got %T", cmd())
			}
			if w.Err() != nil {
				t.Errorf("canceling is not an error, got %v", w.Err())
			}
		})
	}
}

func TestWaiterViewShowsMessage(t *testing.T) {
	never := func() (bool, error) { return false, nil }
	out := NewWaiter("waiting for GitHub to list #99", never, PlainStyles()).View().Content

	if !strings.Contains(out, "waiting for GitHub to list #99") {
		t.Errorf("view should show the message:\n%s", out)
	}
	if !strings.Contains(out, "esc") {
		t.Errorf("view should say how to stop waiting:\n%s", out)
	}
}

func TestWaiterViewEmptyOnceDone(t *testing.T) {
	poll, _ := readyAfter(1)
	m := fast(NewWaiter("waiting", poll, PlainStyles()))
	w := drain(t, m, m.pollNow())
	if out := w.View().Content; out != "" {
		t.Errorf("view after finishing = %q, want empty", out)
	}
}

// The spinner animates: the frame changes as ticks arrive.
func TestWaiterSpinnerAnimates(t *testing.T) {
	never := func() (bool, error) { return false, nil }
	m := fast(NewWaiter("waiting", never, PlainStyles()))

	first := m.View().Content
	model, _ := m.Update(m.spin.Tick())
	second := model.(Waiter).View().Content

	if first == second {
		t.Errorf("the spinner frame should change on a tick:\n%q\n%q", first, second)
	}
}

func TestWaiterInitStartsSpinnerAndPolls(t *testing.T) {
	poll, calls := readyAfter(1)
	m := fast(NewWaiter("waiting", poll, PlainStyles()))
	if m.Init() == nil {
		t.Fatal("Init should start the spinner and the first poll")
	}
	// The condition may already hold, so the first poll happens immediately
	// rather than after an interval.
	drain(t, m, m.pollNow())
	if calls.Load() == 0 {
		t.Error("Init should poll without waiting for the first interval")
	}
}

// Driven through a real terminal.
func TestWaiterProgram(t *testing.T) {
	poll, calls := readyAfter(3)
	m := NewWaiter("waiting for GitHub", poll, PlainStyles()).WithTiming(10*time.Millisecond, 5*time.Second)

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Waiter)
	if final.Err() != nil {
		t.Fatalf("Err = %v", final.Err())
	}
	if final.TimedOut() {
		t.Error("should have finished before the timeout")
	}
	if got := calls.Load(); got < 3 {
		t.Errorf("polled %d times, want at least 3", got)
	}
}

// Defaults should be sane without a caller overriding them.
func TestWaiterDefaultTiming(t *testing.T) {
	never := func() (bool, error) { return false, nil }
	m := NewWaiter("waiting", never, PlainStyles())
	if m.interval != PollInterval {
		t.Errorf("interval = %v, want %v", m.interval, PollInterval)
	}
	if m.timeout != PollTimeout {
		t.Errorf("timeout = %v, want %v", m.timeout, PollTimeout)
	}
	// The measured listing lag is a few seconds, so the timeout has to clear it
	// comfortably.
	if m.timeout < 10*time.Second {
		t.Errorf("timeout %v is too short for GitHub's listing lag", m.timeout)
	}
}
