package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// press sends one key to the prompt model and returns the new model and
// whether it quit.
func press(t *testing.T, m nameInputModel, key tea.KeyMsg) (nameInputModel, bool) {
	t.Helper()
	next, cmd := m.Update(key)
	quit := false
	if cmd != nil {
		_, quit = cmd().(tea.QuitMsg)
	}
	return next.(nameInputModel), quit
}

func selectItem(t *testing.T, m nameInputModel, title string) nameInputModel {
	t.Helper()
	for i, it := range m.list.Items() {
		if it.(item).title == title {
			m.list.Select(i)
			return m
		}
	}
	t.Fatalf("no menu item %q", title)
	return m
}

func TestPromptIgnoreItem(t *testing.T) {
	m := selectItem(t, initialNameInputModel("job"), IgnoreJob)
	m, quit := press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !quit || !m.ignore || m.done {
		t.Fatalf("quit=%v ignore=%v done=%v; want quit with ignore set", quit, m.ignore, m.done)
	}
}
