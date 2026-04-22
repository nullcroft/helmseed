package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nullcroft/helmseed/internal/provider"
)

func testRepos() []provider.Repo {
	return []provider.Repo{
		{Name: "chart-nginx"},
		{Name: "chart-redis"},
		{Name: "chart-postgres"},
	}
}

var keyTypes = map[string]tea.KeyType{
	"up":    tea.KeyUp,
	"down":  tea.KeyDown,
	"enter": tea.KeyEnter,
	" ":     tea.KeySpace,
	"q":     tea.KeyRunes,
	"a":     tea.KeyRunes,
	"j":     tea.KeyRunes,
	"k":     tea.KeyRunes,
}

func keyMsg(k string) tea.KeyMsg {
	kt, ok := keyTypes[k]
	if !ok {
		kt = tea.KeyRunes
	}
	msg := tea.KeyMsg{Type: kt}
	if kt == tea.KeyRunes {
		msg.Runes = []rune(k)
	}
	return msg
}

func sendKey(m model, k string) model {
	updated, _ := m.Update(keyMsg(k))
	return updated.(model)
}

func TestModel_CursorMovement(t *testing.T) {
	m := newModel(testRepos())

	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	m = sendKey(m, "down")
	if m.cursor != 1 {
		t.Errorf("after down: cursor = %d, want 1", m.cursor)
	}

	m = sendKey(m, "down")
	if m.cursor != 2 {
		t.Errorf("after second down: cursor = %d, want 2", m.cursor)
	}

	// should not go past last item
	m = sendKey(m, "down")
	if m.cursor != 2 {
		t.Errorf("past end: cursor = %d, want 2", m.cursor)
	}

	m = sendKey(m, "up")
	if m.cursor != 1 {
		t.Errorf("after up: cursor = %d, want 1", m.cursor)
	}
}

func TestModel_CursorDoesNotGoBelowZero(t *testing.T) {
	m := newModel(testRepos())
	m = sendKey(m, "up")
	if m.cursor != 0 {
		t.Errorf("cursor went below zero: %d", m.cursor)
	}
}

func TestModel_ToggleSelection(t *testing.T) {
	m := newModel(testRepos())

	m = sendKey(m, " ")
	if _, ok := m.selected[0]; !ok {
		t.Error("item 0 should be selected after space")
	}

	m = sendKey(m, " ")
	if _, ok := m.selected[0]; ok {
		t.Error("item 0 should be deselected after second space")
	}
}

func TestModel_SelectAll(t *testing.T) {
	m := newModel(testRepos())

	m = sendKey(m, "a")
	if len(m.selected) != 3 {
		t.Errorf("select all: %d selected, want 3", len(m.selected))
	}

	// toggle all off
	m = sendKey(m, "a")
	if len(m.selected) != 0 {
		t.Errorf("deselect all: %d selected, want 0", len(m.selected))
	}
}

func TestModel_EnterSetsDone(t *testing.T) {
	m := newModel(testRepos())
	m = sendKey(m, " ")
	m = sendKey(m, "enter")

	if !m.done {
		t.Error("enter should set done=true")
	}
}

func TestModel_QuitSetsAborted(t *testing.T) {
	m := newModel(testRepos())
	m = sendKey(m, "q")

	if !m.aborted {
		t.Error("q should set aborted=true")
	}
}

func TestModel_VimKeys(t *testing.T) {
	m := newModel(testRepos())

	m = sendKey(m, "j")
	if m.cursor != 1 {
		t.Errorf("j: cursor = %d, want 1", m.cursor)
	}

	m = sendKey(m, "k")
	if m.cursor != 0 {
		t.Errorf("k: cursor = %d, want 0", m.cursor)
	}
}

func TestModel_ViewContainsRepoNames(t *testing.T) {
	m := newModel(testRepos())
	view := m.View()

	for _, r := range testRepos() {
		if !strings.Contains(view, r.Name) {
			t.Errorf("view should contain repo name %q", r.Name)
		}
	}
}

func TestModel_ViewEmptyAfterDone(t *testing.T) {
	m := newModel(testRepos())
	m = sendKey(m, "enter")

	if view := m.View(); view != "" {
		t.Errorf("view after done should be empty, got %q", view)
	}
}
