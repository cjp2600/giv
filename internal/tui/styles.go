package tui

import "github.com/charmbracelet/lipgloss"

// panelBorder is a thin full box-drawing border.
var panelBorder = lipgloss.Border{
	Top:         "─",
	Bottom:      "─",
	Left:        "│",
	Right:       "│",
	TopLeft:     "┌",
	TopRight:    "┐",
	BottomLeft:  "└",
	BottomRight: "┘",
}

// PanelFrame is the left column with a full border.
func PanelFrame(active bool) lipgloss.Style {
	return panelBorderStyle(active, true, true)
}

// PanelFrameRight is the right column; both sides use a border for a clear split.
func PanelFrameRight(active bool) lipgloss.Style {
	return panelBorderStyle(active, true, true)
}

func panelBorderStyle(active bool, leftBorder, rightBorder bool) lipgloss.Style {
	fg := lipgloss.Color("239")
	if active {
		fg = lipgloss.Color("255")
	}
	s := lipgloss.NewStyle().
		Border(panelBorder).
		BorderForeground(fg).
		Padding(0, 0)
	if !leftBorder {
		s = s.BorderLeft(false)
	}
	if !rightBorder {
		s = s.BorderRight(false)
	}
	return s
}

var (
	appStyle = lipgloss.NewStyle().Padding(0)

	titleBarStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	listHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true)

	headerAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	warnAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("210"))

	// giv logo: orange; Bold after Foreground is more reliable in terminals.
	logoLetterG = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "#FF8C00", Light: "#E07000"}).
			Bold(true)
	logoLetterI = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "#FF8C00", Light: "#E07000"}).
			Bold(true)
	logoLetterV = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Dark: "#FF8C00", Light: "#E07000"}).
			Bold(true)
	// Status bar text after the logo (single gray tone).
	topBarMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true)

	metaStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	mutedPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))

	sectionTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Bold(true)
	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	scrollThumb = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	scrollDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// List badges: M modified (blue); S staged (green); U untracked (red/orange);
	// D deletion in index or tree (git status D in column 1 or 2), gray.
	listFileFg            = lipgloss.AdaptiveColor{Dark: "#89b4fa", Light: "#3358c4"}
	listStagedFg          = lipgloss.AdaptiveColor{Dark: "#a6e3a1", Light: "#1a7f37"}
	listDeletedFg         = lipgloss.AdaptiveColor{Dark: "#6c7086", Light: "#737373"}
	fileListBadgeModified = lipgloss.NewStyle().
				Foreground(listFileFg).
				Bold(true)
	fileListBadgeStaged = lipgloss.NewStyle().
				Foreground(listStagedFg).
				Bold(true)
	fileListBadgeUntracked = lipgloss.NewStyle().
				Foreground(lipgloss.Color("210")).
				Bold(true)
	fileListBadgeDeleted = lipgloss.NewStyle().
				Foreground(listDeletedFg).
				Bold(true)

	// Selected row: bold path; marker is U+276F HEAVY RIGHT-POINTING ANGLE BRACKET.
	listSelectedPathFg = lipgloss.AdaptiveColor{Dark: "#ffffff", Light: "#121820"}
	listSelectionArrow = lipgloss.NewStyle().
				Foreground(listFileFg).
				Bold(true)

	// File tree branch glyphs (lipgloss/tree style).
	treeBranchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	treeFolderNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	treeFolderGlyphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
)
