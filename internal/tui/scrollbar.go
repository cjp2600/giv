package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var scrollChange = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))

// scrollbarColumn renders a vertical scrollbar strip for the preview viewport.
// changeLines contains 0-based line indices that have diff highlights (additions/deletions).
func scrollbarColumn(vp viewport.Model, changeLines []int) string {
	h := vp.Height
	if h <= 0 {
		return ""
	}
	total := vp.TotalLineCount()
	if total <= 0 {
		total = 1
	}

	// Build a set of scrollbar rows that have at least one change line mapped to them.
	changeRows := make([]bool, h)
	if total > 0 && len(changeLines) > 0 {
		for _, ln := range changeLines {
			row := int(math.Round(float64(ln) / float64(total) * float64(h-1)))
			if row < 0 {
				row = 0
			}
			if row >= h {
				row = h - 1
			}
			changeRows[row] = true
		}
	}

	noScroll := total <= vp.Height
	thumb, start := 0, 0
	if !noScroll {
		thumb = int(math.Round(float64(vp.Height) / float64(total) * float64(h)))
		if thumb < 1 {
			thumb = 1
		}
		if thumb > h {
			thumb = h
		}
		maxStart := h - thumb
		start = int(vp.ScrollPercent() * float64(maxStart))
		if start > maxStart {
			start = maxStart
		}
		if start < 0 {
			start = 0
		}
	}

	var b strings.Builder
	for row := 0; row < h; row++ {
		isThumb := !noScroll && row >= start && row < start+thumb
		hasChange := changeRows[row]

		switch {
		case isThumb && hasChange:
			// Thumb + change: show change color on thumb character.
			b.WriteString(scrollChange.Render("█"))
		case isThumb:
			b.WriteString(scrollThumb.Render("█"))
		case hasChange:
			// Change marker on track.
			b.WriteString(scrollChange.Render("┃"))
		default:
			if noScroll {
				b.WriteString(scrollDim.Render("░"))
			} else {
				b.WriteString(scrollDim.Render("░"))
			}
		}
		if row < h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
