package tui

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FileCandidate struct {
	Path     string
	Resolved bool
}

type fileItem struct {
	path     string
	resolved bool
}

func (f fileItem) Title() string {
	return f.path
}

func (f fileItem) Description() string {
	return ""
}

func (f fileItem) FilterValue() string {
	return f.path
}

type fileItemDelegate struct{}

type programRunner interface {
	Run() (tea.Model, error)
}

var (
	resolvedLabelStyle   lipgloss.Style
	unresolvedLabelStyle lipgloss.Style
	selectProgram        = func(model tea.Model, ctx context.Context) programRunner {
		return tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	}
)

func (d fileItemDelegate) Height() int {
	return 1
}

func (d fileItemDelegate) Spacing() int {
	return 0
}

func (d fileItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d fileItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	file, ok := item.(fileItem)
	if !ok {
		return
	}
	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}
	label := "unresolved"
	labelStyle := unresolvedLabelStyle
	if file.resolved {
		label = "resolved"
		labelStyle = resolvedLabelStyle
	}
	labelWidth := len("unresolved")
	labelText := fmt.Sprintf("%*s", labelWidth, label)
	fmt.Fprint(w, cursor+labelStyle.Render(labelText)+"  "+file.path)
}

type fileSelectModel struct {
	list     list.Model
	selected string
	err      error
}

var ErrSelectorQuit = fmt.Errorf("selector quit")

const (
	keyNextUnresolvedFile = keyNextConflict
	keyPrevUnresolvedFile = keyPrevConflict
)

// SelectFile opens a TUI selector and returns the chosen repo-relative path.
func SelectFile(ctx context.Context, candidates []FileCandidate) (string, error) {
	if err := ensureThemeLoaded(); err != nil {
		return "", err
	}
	items := make([]list.Item, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, fileItem{path: candidate.Path, resolved: candidate.Resolved})
	}

	model := fileSelectModel{list: list.New(items, fileItemDelegate{}, 0, 0)}
	selectFirstUnresolvedFile(&model.list)
	model.list.Title = "Select conflicted file"
	model.list.SetShowHelp(false)
	model.list.SetShowStatusBar(false)
	model.list.SetShowPagination(false)
	model.list.SetFilteringEnabled(false)

	program := selectProgram(model, ctx)
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("file selector TUI error: %w", err)
	}

	result, ok := finalModel.(fileSelectModel)
	if !ok {
		return "", fmt.Errorf("file selector returned unexpected model")
	}
	if result.err != nil {
		return "", result.err
	}
	if result.selected == "" {
		return "", fmt.Errorf("no file selected")
	}
	return result.selected, nil
}

func (m fileSelectModel) Init() tea.Cmd {
	return nil
}

func (m fileSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.err = ErrSelectorQuit
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(fileItem); ok {
				m.selected = item.path
				return m, tea.Quit
			}
		case keyNextUnresolvedFile:
			selectAdjacentUnresolvedFile(&m.list, 1)
			return m, nil
		case keyPrevUnresolvedFile:
			selectAdjacentUnresolvedFile(&m.list, -1)
			return m, nil
		}
	case tea.WindowSizeMsg:
		width := msg.Width
		height := msg.Height
		if height < 5 {
			height = 5
		}
		m.list.SetSize(width, height-2)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m fileSelectModel) View() string {
	return m.list.View() + "\n" + "up/down: move, n/p: unresolved, enter: select, q: quit"
}

func selectFirstUnresolvedFile(model *list.Model) {
	for index, item := range model.Items() {
		file, ok := item.(fileItem)
		if ok && !file.resolved {
			model.Select(index)
			return
		}
	}
}

func selectAdjacentUnresolvedFile(model *list.Model, direction int) {
	items := model.Items()
	for index := model.Index() + direction; index >= 0 && index < len(items); index += direction {
		file, ok := items[index].(fileItem)
		if ok && !file.resolved {
			model.Select(index)
			return
		}
	}
}
