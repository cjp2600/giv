package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PrintHotkeyHelp writes a formatted hotkey reference table to w.
func PrintHotkeyHelp(w io.Writer) {
	const (
		colKey = 22
		colSep = " │ "
	)

	hotkeyTitleStyle := headerAccent.Copy().Bold(true)

	// Keys — orange accent; description — light text; sep — muted.
	hotkeyKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "#fab387", Light: "#c2410c"}).
		Bold(true)
	hotkeyDescStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: "#cdd6f4", Light: "#475569"})
	hotkeySepStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#6c7086", Light: "#94a3b8"})

	printRow := func(key, desc string) {
		key = padRightVisible(key, colKey)
		_, _ = fmt.Fprintf(w, "%s%s%s\n",
			hotkeyKeyStyle.Render(key),
			hotkeySepStyle.Render(colSep),
			hotkeyDescStyle.Render(desc))
	}

	printSection := func(title string) {
		_, _ = fmt.Fprintf(w, "\n%s\n", sectionTitleStyle.Render(title))
		u := strings.Repeat("─", colKey+len(colSep)+60)
		_, _ = fmt.Fprintln(w, dividerStyle.Render(u))
	}

	_, _ = fmt.Fprintln(w, hotkeyTitleStyle.Render("giv — keyboard shortcuts"))

	printSection("General")
	printRow("Tab", "Switch focus: file tree ↔ preview")
	printRow("q, Esc", "Quit")
	printRow("Ctrl+C", "Quit (also exits when the commit modal is open)")
	printRow("Ctrl+N", "Jump to next changed line in preview (wraps from bottom to top)")
	printRow("d", "Preview mode: working tree only / full diff including deletions")
	printRow("m", "Toggle rendered Markdown preview (for .md files)")

	printSection("Files & Git")
	printRow("Ctrl+P", "git push current branch")
	printRow("Ctrl+O", "Open selected file in external editor (VS Code if available)")
	printRow("Ctrl+A", "git add selected file (focus on file list)")
	printRow("Ctrl+U", "Revert selected file per giv rules (focus on file list)")
	printRow("Ctrl+B", "Create new branch from latest main")

	printSection("File tree (left pane)")
	printRow("↑ ↓", "Move through tree rows")
	printRow("Enter", "On a folder row — expand or collapse children")
	printRow("j, k", "Down / up (vim-style)")
	printRow("g", "Go to start of list")
	printRow("G", "Go to end of list")
	printRow("Mouse", "Click row to select; wheel to scroll")

	printSection("Preview (right pane; Ctrl+N works from either focus)")
	printRow("↑ ↓ PgUp PgDn", "Scroll preview")
	printRow("Space, f", "Page down")
	printRow("b", "Page up")
	printRow("h, l, ← →", "Horizontal scroll for long lines")
	printRow("Mouse", "Wheel (and drag when terminal sends motion events)")

	printSection("Commit (Ctrl+G)")
	printRow("Ctrl+G", "Open commit message input (git commit -a)")
	printRow("Enter", "Run commit with the typed message")
	printRow("Esc", "Close modal without committing")

	printSection("Branch (Ctrl+B)")
	printRow("Ctrl+B", "Open branch creation input (fetch + checkout main + create branch)")
	printRow("Enter", "Create branch from latest main")
	printRow("Esc", "Close modal without creating")

	_, _ = fmt.Fprintln(w)
}

func padRightVisible(s string, width int) string {
	// Simple rune padding (no ANSI in these strings).
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(r))
}
