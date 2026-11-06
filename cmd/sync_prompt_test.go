package cmd

import (
	"strings"
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

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestPromptKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		key    tea.KeyMsg
		exited bool
	}{
		{"s skips", runes("s"), false},
		{"q quits", runes("q"), true},
		{"esc quits", tea.KeyMsg{Type: tea.KeyEsc}, true},
		{"ctrl+c quits", tea.KeyMsg{Type: tea.KeyCtrlC}, true},
		{"ctrl+d quits", tea.KeyMsg{Type: tea.KeyCtrlD}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, quit := press(t, initialNameInputModel("job"), tc.key)
			if !quit || m.done || m.ignore || m.exited != tc.exited {
				t.Fatalf("quit=%v done=%v ignore=%v exited=%v; want quit with exited=%v", quit, m.done, m.ignore, m.exited, tc.exited)
			}
		})
	}
}

func TestPromptOtherKeysDoNothing(t *testing.T) {
	for _, key := range []tea.KeyMsg{runes("/"), runes("?"), runes("x")} {
		m, quit := press(t, initialNameInputModel("job"), key)
		if quit || m.state != "choosing" {
			t.Fatalf("%q: quit=%v state=%s; want no effect", key.String(), quit, m.state)
		}
	}
}

func TestPromptNamingKeys(t *testing.T) {
	m := selectItem(t, initialNameInputModel("job"), EnterCustomName)
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != "naming" {
		t.Fatalf("state = %s", m.state)
	}
	for _, r := range "sq" {
		var quit bool
		if m, quit = press(t, m, runes(string(r))); quit {
			t.Fatalf("%q quit while typing a name", r)
		}
	}
	if m.textInput.Value() != "sq" {
		t.Fatalf("typed %q; want sq", m.textInput.Value())
	}
	m, quit := press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if quit || m.state != "choosing" {
		t.Fatalf("esc: quit=%v state=%s; want back to the menu", quit, m.state)
	}
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // EnterCustomName is still selected
	m, quit = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !quit || !m.exited {
		t.Fatalf("ctrl+c while naming: quit=%v exited=%v", quit, m.exited)
	}
}

func TestPromptShowsEveryItemAndFooter(t *testing.T) {
	m, _ := press(t, initialNameInputModel("job"), runes("x"))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	view := next.(nameInputModel).View()
	for _, want := range []string{UseDefaultName, EnterCustomName, SkipJob, IgnoreJob, "enter select · s skip · q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}
