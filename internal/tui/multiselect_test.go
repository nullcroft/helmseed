package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/nullcroft/helmseed/internal/provider"
)

func testRepos() []provider.Repo {
	return []provider.Repo{
		{Name: "chart-nginx"},
		{Name: "chart-redis"},
		{Name: "chart-postgres"},
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case " ":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	default:
		r := rune(k[0])
		return tea.KeyPressMsg{Code: r, Text: k}
	}
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

	m = sendKey(m, "j")
	if m.cursor != 1 {
		t.Errorf("after down: cursor = %d, want 1", m.cursor)
	}

	m = sendKey(m, "j")
	if m.cursor != 2 {
		t.Errorf("after second down: cursor = %d, want 2", m.cursor)
	}

	// should not go past last item
	m = sendKey(m, "j")
	if m.cursor != 2 {
		t.Errorf("past end: cursor = %d, want 2", m.cursor)
	}

	m = sendKey(m, "k")
	if m.cursor != 1 {
		t.Errorf("after up: cursor = %d, want 1", m.cursor)
	}
}

func TestModel_CursorDoesNotGoBelowZero(t *testing.T) {
	m := newModel(testRepos())
	m = sendKey(m, "k")
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
		if !strings.Contains(view.Content, r.Name) {
			t.Errorf("view should contain repo name %q", r.Name)
		}
	}
}

func TestModel_ViewEmptyAfterDone(t *testing.T) {
	m := newModel(testRepos())
	m = sendKey(m, "enter")

	if view := m.View(); view.Content != "" {
		t.Errorf("view after done should be empty, got %q", view.Content)
	}
}

func TestUp(t *testing.T) {
	m := newModel(testRepos())
	m.cursor = 2
	m = m.up()
	if m.cursor != 1 {
		t.Errorf("up: cursor = %d, want 1", m.cursor)
	}
	m.cursor = 0
	m = m.up()
	if m.cursor != 0 {
		t.Errorf("up at zero: cursor = %d, want 0", m.cursor)
	}
}

func TestDown(t *testing.T) {
	m := newModel(testRepos())
	m = m.down()
	if m.cursor != 1 {
		t.Errorf("down: cursor = %d, want 1", m.cursor)
	}
	m.cursor = 2
	m = m.down()
	if m.cursor != 2 {
		t.Errorf("down at end: cursor = %d, want 2", m.cursor)
	}
}

func TestToggle(t *testing.T) {
	m := newModel(testRepos())
	m = m.toggle()
	if _, ok := m.selected[0]; !ok {
		t.Error("toggle should select current item")
	}
	m = m.toggle()
	if _, ok := m.selected[0]; ok {
		t.Error("toggle again should deselect current item")
	}
}

func TestSelectAll(t *testing.T) {
	m := newModel(testRepos())
	m = m.selectAll()
	if len(m.selected) != 3 {
		t.Errorf("selectAll: %d selected, want 3", len(m.selected))
	}
	m = m.selectAll()
	if len(m.selected) != 0 {
		t.Errorf("selectAll again: %d selected, want 0", len(m.selected))
	}
}

func TestEnter(t *testing.T) {
	m := newModel(testRepos())
	updated, cmd := m.enter()
	if !updated.done {
		t.Error("enter should set done=true")
	}
	if cmd == nil {
		t.Error("enter should return tea.Quit command")
	}
}

func TestQuit(t *testing.T) {
	m := newModel(testRepos())
	updated, cmd := m.quit()
	if !updated.aborted {
		t.Error("quit should set aborted=true")
	}
	if cmd == nil {
		t.Error("quit should return tea.Quit command")
	}
}
