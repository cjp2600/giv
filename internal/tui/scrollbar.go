package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
)

// scrollbarColumn renders a vertical scrollbar strip for the preview viewport.
func scrollbarColumn(vp viewport.Model) string {
	h := vp.Height
	if h <= 0 {
		return ""
	}
	total := vp.TotalLineCount()
	if total <= vp.Height || total <= 0 {
		var b strings.Builder
		for row := 0; row < h; row++ {
			b.WriteString(scrollDim.Render("░"))
			if row < h-1 {
				b.WriteByte('\n')
			}
		}
		return b.String()
	}

	thumb := int(math.Round(float64(vp.Height) / float64(total) * float64(h)))
	if thumb < 1 {
		thumb = 1
	}
	if thumb > h {
		thumb = h
	}
	maxStart := h - thumb
	start := int(vp.ScrollPercent() * float64(maxStart))
	if start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}

	var b strings.Builder
	for row := 0; row < h; row++ {
		if row >= start && row < start+thumb {
			b.WriteString(scrollThumb.Render("█"))
		} else {
			b.WriteString(scrollDim.Render("░"))
		}
		if row < h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
