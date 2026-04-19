package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) branchModalView() string {
	title := commitModalTitleStyle.Render("Create Branch")
	hint := commitModalHintStyle.Render("Enter — fetch + checkout main + create branch · Esc — cancel")

	parts := []string{
		title,
		"",
		m.branchInput.View(),
	}
	if strings.TrimSpace(m.branchErr) != "" {
		parts = append(parts, "", warnAccent.Render(truncateCommitInline(m.branchErr)))
	}
	parts = append(parts, "", hint)

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	boxW := m.branchInput.Width + 8
	if boxW < 42 {
		boxW = 42
	}
	return commitModalBoxStyle.Width(boxW).Render(body)
}
