package tui

import (
	"context"
	"strings"
	"time"

	gogit "github.com/cjp2600/giv/internal/git"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) pushModalView() string {
	title := commitModalTitleStyle.Render("Push")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	branch := m.snap.Branch
	commit := gogit.LastCommitOneLiner(ctx, m.repoRoot)

	parts := []string{
		title,
		"",
		commitModalHintStyle.Render("branch") + "  " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render(branch),
		commitModalHintStyle.Render("commit") + "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(commit),
	}

	if m.pushPushing {
		parts = append(parts, "", commitModalHintStyle.Render("Pushing…"))
	} else if strings.TrimSpace(m.pushErr) != "" {
		parts = append(parts, "", warnAccent.Render(truncateCommitInline(m.pushErr)))
		parts = append(parts, "", commitModalHintStyle.Render("Enter — retry · Esc — cancel"))
	} else {
		parts = append(parts, "", commitModalHintStyle.Render("Enter — push · Esc — cancel"))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	boxW := 50
	return commitModalBoxStyle.Width(boxW).Render(body)
}
