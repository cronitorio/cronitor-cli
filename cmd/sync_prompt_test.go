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

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestPromptKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(*testing.T, nameInputModel) nameInputModel
		key    tea.KeyMsg
		exited bool
		ignore bool
	}{
		{"s skips", nil, runes("s"), false, false},
		{"q quits", nil, runes("q"), true, false},
		{"esc quits", nil, tea.KeyMsg{Type: tea.KeyEsc}, true, false},
		{"ctrl+c quits", nil, tea.KeyMsg{Type: tea.KeyCtrlC}, true, false},
		{"ctrl+d quits", nil, tea.KeyMsg{Type: tea.KeyCtrlD}, true, false},
		{"enter on ignore", func(t *testing.T, m nameInputModel) nameInputModel {
			return selectItem(t, m, IgnoreJob)
		}, tea.KeyMsg{Type: tea.KeyEnter}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := initialNameInputModel("job")
			if tc.setup != nil {
				m = tc.setup(t, m)
			}
			m, quit := press(t, m, tc.key)
			if !quit || m.done || m.ignore != tc.ignore || m.exited != tc.exited {
				t.Fatalf("quit=%v done=%v ignore=%v exited=%v; want quit with ignore=%v exited=%v", quit, m.done, m.ignore, m.exited, tc.ignore, tc.exited)
			}
		})
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

// The Windows task flow has no save prompt and does not read the ignore
// choice, so its menu keeps the old keys: q, esc and ctrl+c skip one task,
// ctrl+d stops.
func TestWindowsPromptKeepsOldKeys(t *testing.T) {
	m := initialWindowsNameInputModel("task")
	for _, it := range m.list.Items() {
		if it.(item).title == IgnoreJob {
			t.Fatal("the Windows menu offers \"don't ask again\", which it cannot save")
		}
	}
	for _, key := range []tea.KeyMsg{runes("q"), {Type: tea.KeyEsc}, {Type: tea.KeyCtrlC}, runes("s")} {
		got, quit := press(t, m, key)
		if !quit || got.exited || got.done {
			t.Fatalf("%q: quit=%v exited=%v done=%v; want skip", key.String(), quit, got.exited, got.done)
		}
	}
	if got, quit := press(t, m, tea.KeyMsg{Type: tea.KeyCtrlD}); !quit || !got.exited {
		t.Fatalf("ctrl+d: quit=%v exited=%v; want stop", quit, got.exited)
	}
}
