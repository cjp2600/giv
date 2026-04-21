package tui

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	gogit "github.com/cjp2600/giv/internal/git"
)

const maxPreviewLines = 6000

// Expand tabs to tab stops (like ts=8 in vim / most terminals).
// Makefiles rely on this (recipe tabs, alignment before '=').
// Fixed "tab → N spaces" replacement looks wrong next to editors.
// After expansion there is no raw '\t' — viewport won't apply a second pass.
const previewTabStopWidth = 8

// Muted truecolor backgrounds; add/del rows padded to viewport wrap width.
const (
	ansiBgDiffDel = "\033[48;2;75;24;24m"
	ansiBgDiffAdd = "\033[48;2;30;58;30m"

	ansiGutterDelNums = "\033[38;2;198;150;155m"
	ansiGutterDelPipe = "\033[38;2;215;165;172m"
	ansiGutterDelMark = "\033[38;2;255;145;155m"

	ansiGutterAddNums = "\033[38;2;140;185;158m"
	ansiGutterAddPipe = "\033[38;2;160;200;175m"
	ansiGutterAddMark = "\033[38;2;165;235;190m"

	ansiGutterCtxNums = "\033[38;5;245m"
	ansiGutterCtxPipe = "\033[38;5;239m"
)

var (
	draculaStyle = styles.Get("dracula")
	reHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
	// Full-line git binary notice only — not a substring inside diff body.
	reGitBinaryFilesNotice = regexp.MustCompile(`^Binary files .+ differ\s*$`)
	lexerCache             sync.Map // map[string]chroma.Lexer
)

func gitUnifiedDiffContainsBinaryNotice(diff string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t != "" && reGitBinaryFilesNotice.MatchString(t) {
			return true
		}
	}
	return false
}

func previewDiffHint(showDeletions bool) string {
	return "\n"
}

func normWidth(w int) int {
	if w < 20 {
		return 80
	}
	return w
}

func expandPreviewTabs(s string) string {
	return expandTabsToStops(s, previewTabStopWidth)
}

// expandTabsToStops replaces '\t' up to the next tab column (classic tab stops).
// ANSI-heavy lines: tabs should be stripped from code before highlighting in our pipeline.
func expandTabsToStops(s string, tabWidth int) string {
	if tabWidth < 1 {
		tabWidth = 8
	}
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + strings.Count(s, "\t")*tabWidth)
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			n := tabWidth - col%tabWidth
			if n == 0 {
				n = tabWidth
			}
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case '\n':
			b.WriteByte('\n')
			col = 0
		case '\r':
			b.WriteByte('\r')
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}

func stripLeadingBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
}

// showDeletions: when false, omit removed-line text (working tree + green additions only).
// isBinaryFile returns true only when the path is NOT a known text extension
// AND fails the binary heuristic. Known source files (.go, .proto, .ts, …)
// are never treated as binary even if gitattributes says -diff.
func isBinaryFile(path string, body []byte) bool {
	if gogit.IsKnownTextPath(path) {
		return false
	}
	return gogit.IsBinaryPath(path) || gogit.IsBinaryContent(body)
}

func BuildPreview(ctx context.Context, repoRoot string, cf gogit.ChangedFile, contentWidth int, showDeletions bool) string {
	w := normWidth(contentWidth)
	if cf.IsUntracked || (len(cf.PorcelainXY) > 0 && cf.PorcelainXY[0] == 'A') {
		if !gogit.IsKnownTextPath(cf.Path) && gogit.IsBinaryPath(cf.Path) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		body, err := gogit.ReadWorktreeFile(repoRoot, cf.Path)
		if err != nil {
			return warnAccent.Render(fmt.Sprintf("read error: %v", err))
		}
		if isBinaryFile(cf.Path, body) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		label := "Untracked file — showing full content"
		if !cf.IsUntracked {
			label = "New file — showing full content"
		}
		note := metaStyle.Render(label)
		return note + "\n\n" + renderFullContext(cf.Path, stripLeadingBOM(body), w)
	}

	type diffRes struct {
		text string
		err  error
	}
	type fileRes struct {
		body []byte
		err  error
	}
	diffCh := make(chan diffRes, 1)
	fileCh := make(chan fileRes, 1)
	go func() {
		d, err := gogit.DiffForPreview(ctx, repoRoot, cf)
		diffCh <- diffRes{text: d, err: err}
	}()
	// ...
	go func() {
		b, err := gogit.ReadWorktreeFile(repoRoot, cf.Path)
		fileCh <- fileRes{body: b, err: err}
	}()

	diffOut := <-diffCh
	diffText, err := diffOut.text, diffOut.err
	if err != nil {
		return warnAccent.Render(fmt.Sprintf("git diff: %v", err))
	}
	if strings.TrimSpace(diffText) == "" {
		if !gogit.IsKnownTextPath(cf.Path) && gogit.IsBinaryPath(cf.Path) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		readOut := <-fileCh
		if readOut.err != nil {
			return metaStyle.Render("No diff; could not read file.")
		}
		body := stripLeadingBOM(readOut.body)
		if isBinaryFile(cf.Path, body) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		note := "No uncommitted changes vs HEAD — showing file from disk:\n\n"
		if !gogit.HasHEAD(ctx, repoRoot) {
			note = "Empty diff in repo without commits — showing file from disk:\n\n"
		}
		return metaStyle.Render(note) + renderFullContext(cf.Path, body, w)
	}

	if !gogit.IsKnownTextPath(cf.Path) && (gitUnifiedDiffContainsBinaryNotice(diffText) || gogit.IsBinaryPath(cf.Path)) {
		return metaStyle.Render("Binary files differ — content not shown.")
	}
	readOut := <-fileCh
	if readOut.err != nil {
		return warnAccent.Render(fmt.Sprintf("read file: %v", readOut.err)) + "\n" + renderUnifiedDiff(cf.Path, diffText, w, showDeletions)
	}
	body := readOut.body
	body = stripLeadingBOM(body)
	if isBinaryFile(cf.Path, body) {
		return metaStyle.Render("Binary files differ — content not shown.")
	}
	hint := previewDiffHint(showDeletions)
	overlay, ok := renderFullFileWithDiff(cf.Path, body, diffText, w, showDeletions)
	if !ok {
		return hint + renderUnifiedDiff(cf.Path, diffText, w, showDeletions)
	}
	return hint + overlay
}

// BuildReviewPreview builds preview for review mode: diff between merge-base and branch HEAD.
// Since we already checked out the branch, reads file from working tree (same as normal mode).
func BuildReviewPreview(ctx context.Context, repoRoot string, cf gogit.ChangedFile, base, branch string, contentWidth int, showDeletions bool) string {
	w := normWidth(contentWidth)

	// Deleted file — show notice.
	if len(cf.PorcelainXY) >= 1 && cf.PorcelainXY[0] == 'D' {
		return metaStyle.Render("File deleted in this branch.")
	}

	// New file (added in branch) — show full content from working tree.
	if len(cf.PorcelainXY) >= 1 && cf.PorcelainXY[0] == 'A' {
		if !gogit.IsKnownTextPath(cf.Path) && gogit.IsBinaryPath(cf.Path) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		body, err := gogit.ReadWorktreeFile(repoRoot, cf.Path)
		if err != nil {
			return warnAccent.Render(fmt.Sprintf("read error: %v", err))
		}
		if isBinaryFile(cf.Path, body) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		note := metaStyle.Render("New file — showing full content")
		return note + "\n\n" + renderFullContext(cf.Path, stripLeadingBOM(body), w)
	}

	// Modified file — get diff between base and branch HEAD, plus file from working tree.
	type diffRes struct {
		text string
		err  error
	}
	type fileRes struct {
		body []byte
		err  error
	}
	diffCh := make(chan diffRes, 1)
	fileCh := make(chan fileRes, 1)
	go func() {
		d, err := gogit.DiffBetweenRefs(ctx, repoRoot, base, branch, cf.Path)
		diffCh <- diffRes{text: d, err: err}
	}()
	go func() {
		b, err := gogit.ReadWorktreeFile(repoRoot, cf.Path)
		fileCh <- fileRes{body: b, err: err}
	}()

	diffOut := <-diffCh
	if diffOut.err != nil {
		return warnAccent.Render(fmt.Sprintf("git diff: %v", diffOut.err))
	}
	if strings.TrimSpace(diffOut.text) == "" {
		readOut := <-fileCh
		if readOut.err != nil {
			return metaStyle.Render("No diff available.")
		}
		body := stripLeadingBOM(readOut.body)
		if isBinaryFile(cf.Path, body) {
			return metaStyle.Render("Binary file — content not shown.")
		}
		return metaStyle.Render("No changes detected.\n\n") + renderFullContext(cf.Path, body, w)
	}

	if !gogit.IsKnownTextPath(cf.Path) && (gitUnifiedDiffContainsBinaryNotice(diffOut.text) || gogit.IsBinaryPath(cf.Path)) {
		return metaStyle.Render("Binary files differ — content not shown.")
	}
	readOut := <-fileCh
	if readOut.err != nil {
		return warnAccent.Render(fmt.Sprintf("read file: %v", readOut.err)) + "\n" + renderUnifiedDiff(cf.Path, diffOut.text, w, showDeletions)
	}
	body := stripLeadingBOM(readOut.body)
	if isBinaryFile(cf.Path, body) {
		return metaStyle.Render("Binary files differ — content not shown.")
	}
	hint := previewDiffHint(showDeletions)
	overlay, ok := renderFullFileWithDiff(cf.Path, body, diffOut.text, w, showDeletions)
	if ok {
		return hint + overlay
	}

	// Fallback: working tree might differ from committed version (smudge filters,
	// line-ending normalization, etc.). Try the exact committed blob.
	if gitBody, err := gogit.ShowFileAtRef(ctx, repoRoot, branch, cf.Path); err == nil {
		gitBody = stripLeadingBOM(gitBody)
		if overlay2, ok2 := renderFullFileWithDiff(cf.Path, gitBody, diffOut.text, w, showDeletions); ok2 {
			return hint + overlay2
		}
	}

	return hint + renderUnifiedDiff(cf.Path, diffOut.text, w, showDeletions)
}

func renderAddedFile(name string, body []byte, width int) string {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	if len(lines) > maxPreviewLines {
		lines = lines[:maxPreviewLines]
		lines = append(lines, fmt.Sprintf("… truncated after %d lines …", maxPreviewLines))
	}
	lex := filepath.Base(name)
	newNum := 1
	var b strings.Builder
	for _, line := range lines {
		oldS := "    "
		newS := fmt.Sprintf("%4d", newNum)
		newNum++
		b.WriteString(formatAddLine(oldS, newS, lex, line, width, false))
		b.WriteByte('\n')
	}
	return b.String()
}

func renderFullContext(name string, body []byte, width int) string {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	if len(lines) > maxPreviewLines {
		lines = lines[:maxPreviewLines]
	}
	lex := filepath.Base(name)
	var b strings.Builder
	for i, line := range lines {
		n := i + 1
		oldS := fmt.Sprintf("%4d", n)
		newS := fmt.Sprintf("%4d", n)
		b.WriteString(formatCtxLine(oldS, newS, lex, line, width, false))
		b.WriteByte('\n')
	}
	return b.String()
}

func renderUnifiedDiff(name string, diff string, width int, showDeletions bool) string {
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	var b strings.Builder
	lex := filepath.Base(name)
	count := 0
	oldLine, newLine := 0, 0

	for _, line := range lines {
		if count >= maxPreviewLines {
			b.WriteString(metaStyle.Render(fmt.Sprintf("… truncated after %d lines …\n", maxPreviewLines)))
			break
		}

		switch {
		case strings.HasPrefix(line, "\\"):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "diff --git"):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "index "):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "--- "):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "+++ "):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "Binary files"):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "similarity index"):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "rename "):
			b.WriteString(metaStyle.Render(line) + "\n")
		case strings.HasPrefix(line, "@@"):
			if m := reHunkHeader.FindStringSubmatch(line); len(m) == 3 {
				oldLine, _ = strconv.Atoi(m[1])
				newLine, _ = strconv.Atoi(m[2])
			}
			// Hide @@ hunk headers — noise when reading code.
			continue
		case strings.HasPrefix(line, "+"):
			if strings.HasPrefix(line, "+++") {
				b.WriteString(metaStyle.Render(line) + "\n")
				continue
			}
			oldS := "    "
			newS := fmt.Sprintf("%4d", newLine)
			newLine++
			code := strings.TrimPrefix(line, "+")
			b.WriteString(formatAddLine(oldS, newS, lex, code, width, showDeletions))
			b.WriteByte('\n')
			count++
		case strings.HasPrefix(line, "-"):
			if strings.HasPrefix(line, "---") {
				b.WriteString(metaStyle.Render(line) + "\n")
				continue
			}
			code := strings.TrimPrefix(line, "-")
			oldLine++
			if !showDeletions {
				continue
			}
			oldS := fmt.Sprintf("%4d", oldLine-1)
			newS := "    "
			b.WriteString(formatDelLine(oldS, newS, lex, code, width))
			b.WriteByte('\n')
			count++
		default:
			// context line: leading space or empty line inside hunk body
			if len(line) == 0 || line[0] == ' ' {
				code := strings.TrimPrefix(line, " ")
				oldS := fmt.Sprintf("%4d", oldLine)
				newS := fmt.Sprintf("%4d", newLine)
				oldLine++
				newLine++
				b.WriteString(formatCtxLine(oldS, newS, lex, code, width, showDeletions))
				b.WriteByte('\n')
				count++
				continue
			}
			b.WriteString(metaStyle.Render(line) + "\n")
		}
	}
	return b.String()
}

func formatNumCol(s string) string {
	if strings.TrimSpace(s) == "" {
		return "    "
	}
	return s
}

func formatCtxLine(oldS, newS, lex, code string, viewportCols int, dualGutter bool) string {
	var line string
	if dualGutter {
		line = buildCtxLineCore(oldS, newS, lex, code)
	} else {
		line = buildCtxLineCoreSingle(newS, lex, code)
	}
	return padAnsiLineToMinWidth(line, viewportWrapMinCols(viewportCols))
}

func buildCtxLineCore(oldS, newS, lex, code string) string {
	gutter := ansiGutterCtxNums + fmt.Sprintf("%4s %4s ", formatNumCol(oldS), formatNumCol(newS)) + ansiGutterCtxPipe + "│" + ansiGutterCtxNums + "   " + "\033[0m"
	hl, err := highlightLine(lex, code)
	if err != nil {
		hl = code
	}
	return gutter + hl
}

func buildCtxLineCoreSingle(newS, lex, code string) string {
	gutter := ansiGutterCtxNums + fmt.Sprintf("%4s ", formatNumCol(newS)) + ansiGutterCtxPipe + "│" + ansiGutterCtxNums + "   " + "\033[0m"
	hl, err := highlightLine(lex, code)
	if err != nil {
		hl = code
	}
	return gutter + hl
}

func formatDelLine(oldS, newS, lex, code string, viewportCols int) string {
	core := buildDelLineCore(oldS, newS, lex, code)
	target := viewportWrapMinCols(viewportCols)
	core = padDiffColoredToWidth(core, ansiBgDiffDel, target)
	return finalizeDiffNeutralTail(core, target)
}

func buildDelLineCore(oldS, newS, lex, code string) string {
	var buf strings.Builder
	buf.WriteString(ansiBgDiffDel)
	buf.WriteString(ansiGutterDelNums)
	fmt.Fprintf(&buf, "%4s %4s ", formatNumCol(oldS), formatNumCol(newS))
	buf.WriteString(ansiGutterDelPipe)
	buf.WriteString("│")
	buf.WriteString(ansiGutterDelMark)
	buf.WriteString(" - ")
	appendDraculaTokensNoReset(&buf, lex, code)
	return buf.String()
}

func formatAddLine(oldS, newS, lex, code string, viewportCols int, dualGutter bool) string {
	core := buildAddLineCore(oldS, newS, lex, code, dualGutter)
	target := viewportWrapMinCols(viewportCols)
	core = padDiffColoredToWidth(core, ansiBgDiffAdd, target)
	return finalizeDiffNeutralTail(core, target)
}

func buildAddLineCore(oldS, newS, lex, code string, dualGutter bool) string {
	var buf strings.Builder
	buf.WriteString(ansiBgDiffAdd)
	buf.WriteString(ansiGutterAddNums)
	if dualGutter {
		fmt.Fprintf(&buf, "%4s %4s ", formatNumCol(oldS), formatNumCol(newS))
	} else {
		fmt.Fprintf(&buf, "%4s ", formatNumCol(newS))
	}
	buf.WriteString(ansiGutterAddPipe)
	buf.WriteString("│")
	buf.WriteString(ansiGutterAddMark)
	buf.WriteString(" + ")
	appendDraculaTokensNoReset(&buf, lex, code)
	return buf.String()
}

// viewportWrapMinCols — minimum logical line width for cellbuf.Wrap(viewport).
func viewportWrapMinCols(viewportCols int) int {
	if viewportCols <= 0 {
		return 0
	}
	v := viewportCols - 1
	if v < 1 {
		v = viewportCols
	}
	return v
}

func padDiffColoredToWidth(core, rowBg string, target int) string {
	core = expandPreviewTabs(core)
	if target < 1 {
		return core
	}
	w := ansi.StringWidth(core)
	if w >= target {
		return core
	}
	return core + rowBg + strings.Repeat(" ", target-w)
}

func finalizeDiffNeutralTail(core string, minCols int) string {
	core = expandPreviewTabs(core)
	w := ansi.StringWidth(core)
	if w >= minCols {
		return core + "\033[0m"
	}
	return core + "\033[0m" + strings.Repeat(" ", minCols-w)
}

// collectPreviewJumpLineIndices returns 0-based line indices (split on \n) where the preview
// line shows addition or deletion highlighting.
func collectPreviewJumpLineIndices(fullPreviewContent string) []int {
	s := strings.ReplaceAll(fullPreviewContent, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	var out []int
	for i, line := range lines {
		if isPreviewDiffHighlightLine(line) {
			out = append(out, i)
		}
	}
	return out
}

func isPreviewDiffHighlightLine(line string) bool {
	if strings.Contains(line, ansiBgDiffAdd) || strings.Contains(line, ansiBgDiffDel) {
		return true
	}
	// Fallback detection without relying on truecolor 48;2 sequences.
	if strings.Contains(line, ansiGutterAddNums) && strings.Contains(line, " + ") {
		return true
	}
	if strings.Contains(line, ansiGutterDelNums) && strings.Contains(line, " - ") {
		return true
	}
	return false
}

// padAnsiLineToMinWidth pads with spaces to minVisual (viewport width minus wrap slack).
func padAnsiLineToMinWidth(line string, minVisual int) string {
	line = expandPreviewTabs(line)
	if minVisual <= 0 {
		return line
	}
	w := ansi.StringWidth(line)
	if w >= minVisual {
		return line
	}
	return line + strings.Repeat(" ", minVisual-w)
}

func pickStyleEntry(s *chroma.Style, ty chroma.TokenType) chroma.StyleEntry {
	e := s.Get(ty)
	if !e.IsZero() {
		return e
	}
	e = s.Get(ty.SubCategory())
	if !e.IsZero() {
		return e
	}
	e = s.Get(ty.Category())
	if !e.IsZero() {
		return e
	}
	return s.Get(chroma.Text)
}

func styleEntryFG(ent chroma.StyleEntry) string {
	if ent.IsZero() {
		return ""
	}
	var b strings.Builder
	if ent.Bold == chroma.Yes {
		b.WriteString("\033[1m")
	}
	if ent.Underline == chroma.Yes {
		b.WriteString("\033[4m")
	}
	if ent.Italic == chroma.Yes {
		b.WriteString("\033[3m")
	}
	if ent.Colour.IsSet() {
		c := ent.Colour
		fmt.Fprintf(&b, "\033[38;2;%d;%d;%dm", c.Red(), c.Green(), c.Blue())
	}
	return b.String()
}

func appendDraculaTokensNoReset(b *strings.Builder, filename, line string) {
	line = expandPreviewTabs(line)
	if line == "" {
		return
	}
	l := lexerForFile(filename)
	it, err := l.Tokenise(nil, line)
	if err != nil {
		b.WriteString(line)
		return
	}
	for t := it(); t != chroma.EOF; t = it() {
		ent := pickStyleEntry(draculaStyle, t.Type)
		b.WriteString(styleEntryFG(ent))
		b.WriteString(t.Value)
	}
}

func lexerForFile(filename string) chroma.Lexer {
	if v, ok := lexerCache.Load(filename); ok {
		if lx, ok2 := v.(chroma.Lexer); ok2 && lx != nil {
			return lx
		}
	}
	l := lexers.Match(filepath.Base(filename))
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)
	lexerCache.Store(filename, l)
	return l
}

func highlightLine(filename, line string) (string, error) {
	line = expandPreviewTabs(line)
	if line == "" {
		return "", nil
	}
	var b strings.Builder
	appendDraculaTokensNoReset(&b, filename, line)
	return b.String(), nil
}

// isMarkdownFile returns true for .md and .markdown extensions.
func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".mdown" || ext == ".mkd"
}

// renderMarkdownPreview reads the file content and renders it with glamour.
func renderMarkdownPreview(ctx context.Context, repoRoot string, cf gogit.ChangedFile, mode ViewMode, reviewBranch string, contentWidth int) string {
	var raw []byte
	var err error
	if mode == ModeReview {
		raw, err = gogit.ShowFileAtRef(ctx, repoRoot, reviewBranch, cf.Path)
	} else {
		raw, err = gogit.ReadWorktreeFile(repoRoot, cf.Path)
	}
	if err != nil {
		return metaStyle.Render(fmt.Sprintf("Error reading file: %v", err))
	}

	w := normWidth(contentWidth)
	if w < 20 {
		w = 80
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dracula"),
		glamour.WithWordWrap(w),
	)
	if err != nil {
		return metaStyle.Render(fmt.Sprintf("Markdown render error: %v", err))
	}

	rendered, err := renderer.Render(string(raw))
	if err != nil {
		return metaStyle.Render(fmt.Sprintf("Markdown render error: %v", err))
	}
	return rendered
}
