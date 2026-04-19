package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// diffOp is one line inside a unified-diff hunk (marker stripped from text).
type diffOp struct {
	prefix byte // ' ', '+', '-'
	text   string
}

// diffHunk is one @@ ... @@ block with ops.
type diffHunk struct {
	oldStart int
	oldCount int
	newStart int
	newCount int
	ops      []diffOp
}

var reHunkHeaderStrict = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parseUnifiedHunks parses unified diff (e.g. git diff PATH) into hunks.
func parseUnifiedHunks(diff string) []diffHunk {
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	var hunks []diffHunk
	var cur *diffHunk

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "@@") {
			m := reHunkHeaderStrict.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if cur != nil {
				hunks = append(hunks, *cur)
			}
			h := diffHunk{}
			h.oldStart, _ = strconv.Atoi(m[1])
			h.oldCount = 1
			if m[2] != "" {
				h.oldCount, _ = strconv.Atoi(m[2])
			}
			h.newStart, _ = strconv.Atoi(m[3])
			h.newCount = 1
			if m[4] != "" {
				h.newCount, _ = strconv.Atoi(m[4])
			}
			cur = &h
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "diff --git") {
			break
		}
		if strings.HasPrefix(line, `\`) {
			continue
		}
		if len(line) == 0 {
			cur.ops = append(cur.ops, diffOp{prefix: ' ', text: ""})
			continue
		}
		c := line[0]
		if c != ' ' && c != '+' && c != '-' {
			continue
		}
		text := ""
		if len(line) >= 2 {
			text = line[1:]
		}
		cur.ops = append(cur.ops, diffOp{prefix: c, text: text})
	}
	if cur != nil {
		hunks = append(hunks, *cur)
	}
	return hunks
}

// renderFullFileWithDiff builds preview from full file on disk; with showDeletions, inserts removed lines
// and highlights +/- from hunk ops. Line numbers match fullLines (1-based).
func renderFullFileWithDiff(path string, body []byte, diff string, width int, showDeletions bool) (string, bool) {
	fullLines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	hunks := parseUnifiedHunks(diff)
	if len(hunks) == 0 {
		return "", false
	}

	sort.Slice(hunks, func(i, j int) bool {
		if hunks[i].newStart != hunks[j].newStart {
			return hunks[i].newStart < hunks[j].newStart
		}
		return hunks[i].oldStart < hunks[j].oldStart
	})

	lex := filepath.Base(path)
	var b strings.Builder
	count := 0
	pos := 1 // next new-file line for tail outside any hunk

	appendTrunc := func() {
		if count < maxPreviewLines {
			b.WriteString(metaStyle.Render(fmt.Sprintf("… truncated after %d lines …\n", maxPreviewLines)))
		}
	}

	for _, hunk := range hunks {
		for pos < hunk.newStart && pos <= len(fullLines) {
			if count >= maxPreviewLines {
				appendTrunc()
				return b.String(), true
			}
			n := fmt.Sprintf("%4d", pos)
			b.WriteString(formatCtxLine(n, n, lex, fullLines[pos-1], width, showDeletions))
			b.WriteByte('\n')
			count++
			pos++
		}

		iNew := hunk.newStart
		iOld := hunk.oldStart
		for _, op := range hunk.ops {
			if count >= maxPreviewLines {
				appendTrunc()
				return b.String(), true
			}
			switch op.prefix {
			case ' ':
				if iNew < 1 || iNew > len(fullLines) {
					return "", false
				}
				n := fmt.Sprintf("%4d", iNew)
				b.WriteString(formatCtxLine(n, n, lex, fullLines[iNew-1], width, showDeletions))
				b.WriteByte('\n')
				count++
				iNew++
				iOld++
				pos = iNew
			case '-':
				iOld++
				if !showDeletions {
					continue
				}
				o := fmt.Sprintf("%4d", iOld-1)
				b.WriteString(formatDelLine(o, "    ", lex, op.text, width))
				b.WriteByte('\n')
				count++
			case '+':
				if iNew < 1 || iNew > len(fullLines) {
					return "", false
				}
				oldS := "    "
				n := fmt.Sprintf("%4d", iNew)
				b.WriteString(formatAddLine(oldS, n, lex, fullLines[iNew-1], width, showDeletions))
				b.WriteByte('\n')
				count++
				iNew++
				pos = iNew
			}
		}
	}

	for pos <= len(fullLines) {
		if count >= maxPreviewLines {
			appendTrunc()
			return b.String(), true
		}
		n := fmt.Sprintf("%4d", pos)
		b.WriteString(formatCtxLine(n, n, lex, fullLines[pos-1], width, showDeletions))
		b.WriteByte('\n')
		count++
		pos++
	}

	return b.String(), true
}
