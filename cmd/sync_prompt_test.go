package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestJobInfoBoxFitsTerminal(t *testing.T) {
	command := "/var/cronitor/src/cronitor/manage.py prune_events --days 30 --batch-size 5000 >> /var/log/cronitor/prune.log 2>&1"
	for _, width := range []int{60, 120} {
		box := renderJobInfoBox("Prune events", "0 1 * * *", command, true, width)
		lines := strings.Split(box, "\n")
		for i, l := range lines {
			if w := lipgloss.Width(l); w > width {
				t.Fatalf("width %d: line %d is %d wide:\n%s", width, i, w, box)
			}
			trimmed := strings.TrimLeft(l, " ")
			first := []rune(trimmed)[0]
			switch {
			case i == 0 && first != '╭', i == len(lines)-1 && first != '╰', i > 0 && i < len(lines)-1 && first != '│':
				t.Fatalf("width %d: border broken at line %d:\n%s", width, i, box)
			}
		}
		// The whole command is in the box, wrapped but not cut.
		var text strings.Builder
		for _, l := range lines {
			text.WriteString(strings.Trim(l, " │╭╮╰╯─"))
		}
		if !strings.Contains(strings.ReplaceAll(text.String(), " ", ""), strings.ReplaceAll(command, " ", "")) {
			t.Fatalf("width %d: command was cut:\n%s", width, box)
		}
	}
}

func TestSudoHint(t *testing.T) {
	if got := sudoHint(0); got != "" {
		t.Fatalf("sudoHint(0) = %q", got)
	}
	if got := sudoHint(1); got != "1 job in system crontabs was not checked. Run sync with sudo to include it." {
		t.Fatalf("sudoHint(1) = %q", got)
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
