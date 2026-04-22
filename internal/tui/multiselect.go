package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nullcroft/helmseed/internal/provider"
)

var (
	ErrAborted    = fmt.Errorf("selection aborted")
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	helpStyle     = lipgloss.NewStyle().Faint(true)

	keyQuit  = key.NewBinding(key.WithKeys("ctrl+c", "q"))
	keyUp    = key.NewBinding(key.WithKeys("up", "k"))
	keyDown  = key.NewBinding(key.WithKeys("down", "j"))
	keySpace = key.NewBinding(key.WithKeys(" "))
	keyAll   = key.NewBinding(key.WithKeys("a"))
	keyEnter = key.NewBinding(key.WithKeys("enter"))
)

type model struct {
	repos    []provider.Repo
	cursor   int
	selected map[int]struct{}
	done     bool
	aborted  bool
}

func newModel(repos []provider.Repo) model {
	return model{
		repos:    repos,
		selected: make(map[int]struct{}),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keyQuit):
			m.aborted = true
			return m, tea.Quit

		case key.Matches(msg, keyUp):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, keyDown):
			if m.cursor < len(m.repos)-1 {
				m.cursor++
			}

		case key.Matches(msg, keySpace):
			if _, ok := m.selected[m.cursor]; ok {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = struct{}{}
			}

		case key.Matches(msg, keyAll):
			if len(m.selected) == len(m.repos) {
				m.selected = make(map[int]struct{})
			} else {
				for i := range m.repos {
					m.selected[i] = struct{}{}
				}
			}

		case key.Matches(msg, keyEnter):
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.done || m.aborted {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Select repos (%d/%d)", len(m.selected), len(m.repos))))
	b.WriteString("\n\n")

	for i, r := range m.repos {
		cursor := "  "
		if m.cursor == i {
			cursor = cursorStyle.Render("> ")
		}

		check := "[ ]"
		name := r.Name
		if _, ok := m.selected[i]; ok {
			check = selectedStyle.Render("[x]")
			name = selectedStyle.Render(name)
		}

		fmt.Fprintf(&b, "%s%s %s\n", cursor, check, name)
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("space: toggle  a: all  enter: confirm  q: quit"))

	return b.String()
}

// Select runs an interactive multi-select TUI and returns the chosen repos.
// Returns nil (not error) if the user aborts with q/ctrl+c.
func Select(repos []provider.Repo) ([]provider.Repo, error) {
	m := newModel(repos)

	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("tui error: %w", err)
	}

	final, ok := result.(model)
	if !ok {
		return nil, fmt.Errorf("tui error: unexpected model type %T", result)
	}
	if final.aborted {
		return nil, ErrAborted
	}

	out := make([]provider.Repo, 0, len(final.selected))
	for i, r := range final.repos {
		if _, ok := final.selected[i]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}
