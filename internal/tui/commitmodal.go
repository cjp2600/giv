package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	commitModalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(1, 2)

	commitModalTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("252"))

	commitModalHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))
)

func (m *Model) commitModalView() string {
	title := commitModalTitleStyle.Render("Commit")
	hint := commitModalHintStyle.Render("Enter — git commit -a · Esc — cancel · tracked files only")

	parts := []string{
		title,
		"",
		m.commitInput.View(),
	}
	if strings.TrimSpace(m.commitErr) != "" {
		parts = append(parts, "", warnAccent.Render(truncateCommitInline(m.commitErr)))
	}
	parts = append(parts, "", hint)

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	boxW := m.commitInput.Width + 8
	if boxW < 42 {
		boxW = 42
	}
	return commitModalBoxStyle.Width(boxW).Render(body)
}

func truncateCommitInline(s string) string {
	const max = 180
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
