package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	gogit "github.com/cjp2600/giv/internal/git"
)

// topMargin adds blank lines below the terminal tab/status strip.
const topMargin = 2

type tickMsg time.Time

// rowKind classifies left-pane rows: section headers, divider, file, or tree folder.
type rowKind uint8

const (
	rowTrackedHeader rowKind = iota // "In repository (changes)" section title
	rowFileTracked
	rowDivider // visual divider between tracked / untracked sections
	rowUntrackedHeader
	rowFileUntracked
	rowTreeDir // folder row in lipgloss/tree-style branch rendering
)

func isFileRow(k rowKind) bool {
	return k == rowFileTracked || k == rowFileUntracked
}

// rowItem implements list.Item for files and chrome rows (headers / divider).
type rowItem struct {
	kind rowKind
	file gogit.ChangedFile // only for rowFile*
	// Tree branch prefix (lipgloss/tree DefaultEnumerator / DefaultIndenter style):
	treePrefix string // uncolored branch glyphs before the label
	segName    string // one path segment or slash-joined compact folder label
	// rowTreeDir:
	dirPath      string
	dirExpanded  bool
	dirUntracked bool
}

func (r rowItem) Title() string {
	switch r.kind {
	case rowTrackedHeader:
		return "tracked"
	case rowDivider:
		return "divider"
	case rowUntrackedHeader:
		return "untracked"
	case rowTreeDir:
		return r.dirPath
	default:
		return r.file.Path
	}
}

func (r rowItem) Description() string { return "" }

func (r rowItem) FilterValue() string {
	if isFileRow(r.kind) {
		return r.file.Path
	}
	if r.kind == rowTreeDir {
		return r.dirPath
	}
	return ""
}

// previewDoneMsg carries async preview content (non-blocking for Update).
type previewDoneMsg struct {
	seq      uint64
	path     string
	content  string
	isEmpty  bool
	cacheKey string
}

// commitDoneMsg is the result of background git commit -a.
type commitDoneMsg struct {
	err error
}

// fileGitOpDoneMsg is the result of background git add / revert.
type fileGitOpDoneMsg struct {
	err error
}

// branchDoneMsg is the result of background branch creation.
type branchDoneMsg struct {
	err error
}

// Model is the root Bubble Tea model.
type Model struct {
	repoRoot        string
	focusLeft       bool
	list            list.Model
	vp              viewport.Model
	snap            gogit.Snapshot
	selectedPath    string
	lastPreviewPath string
	snapSig         string

	previewSeq        uint64
	previewCache      map[string]string
	previewCacheOrder []string

	// previewShowDeletions: full unified diff with red deletions; else working tree / additions only.
	previewShowDeletions bool

	// previewJumpLines — viewport line indices with diff highlight; used by Ctrl+N.
	previewJumpLines []int

	termWidth, termH int
	leftColW         int
	rightColW        int

	// expandedDirs maps directory path (filepath.Join) → expanded.
	// Missing key defaults to expanded.
	expandedDirs map[string]bool

	commitModalOpen bool
	commitInput     textinput.Model
	commitErr       string
	fileOpErr       string

	branchModalOpen bool
	branchInput     textinput.Model
	branchErr       string
	branchCreating  bool
}

const previewCacheMaxEntries = 24

func previewCacheKey(path string, width int, snapSig string, showDeletions bool) string {
	return fmt.Sprintf("%s|%d|%s|%v", path, width, snapSig, showDeletions)
}

func (m *Model) getPreviewCache(key string) (string, bool) {
	if m.previewCache == nil {
		return "", false
	}
	v, ok := m.previewCache[key]
	return v, ok
}

func (m *Model) putPreviewCache(key, content string) {
	if key == "" || content == "" {
		return
	}
	if m.previewCache == nil {
		m.previewCache = make(map[string]string, previewCacheMaxEntries)
	}
	if _, exists := m.previewCache[key]; exists {
		m.previewCache[key] = content
		return
	}
	m.previewCache[key] = content
	m.previewCacheOrder = append(m.previewCacheOrder, key)
	if len(m.previewCacheOrder) <= previewCacheMaxEntries {
		return
	}
	evict := m.previewCacheOrder[0]
	m.previewCacheOrder = m.previewCacheOrder[1:]
	delete(m.previewCache, evict)
}

type itemDelegate struct{ list.DefaultDelegate }

func newItemDelegate() itemDelegate {
	d := list.NewDefaultDelegate()
	return itemDelegate{DefaultDelegate: d}
}

func (d itemDelegate) Height() int { return 1 }

func (d itemDelegate) Spacing() int { return 0 }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	r, ok := li.(rowItem)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, li)
		return
	}
	lw := m.Width()
	if lw < 4 {
		lw = 4
	}
	switch r.kind {
	case rowTrackedHeader:
		txt := sectionTitleStyle.Width(lw).Render(" In repository (changes) ")
		_, _ = fmt.Fprint(w, txt)
	case rowDivider:
		// Match panel border (styles.panelBorder) or light hyphens look like a streak.
		line := strings.Repeat("─", lw)
		_, _ = fmt.Fprint(w, dividerStyle.Width(lw).Render(line))
	case rowUntrackedHeader:
		txt := sectionTitleStyle.Width(lw).Render(" Untracked (not git add) ")
		_, _ = fmt.Fprint(w, txt)
	case rowFileTracked, rowFileUntracked:
		sel := index == m.Index()
		var line string
		if r.treePrefix != "" {
			seg := r.segName
			if seg == "" {
				seg = filepath.Base(r.file.Path)
			}
			line = formatTreeFileRow(lw, r.treePrefix, r.file, seg, sel)
		} else {
			line = formatFileRow(lw, r.file, sel)
		}
		_, _ = fmt.Fprint(w, line)
	case rowTreeDir:
		sel := index == m.Index()
		line := formatTreeDirRow(lw, r.treePrefix, r.segName, r.dirExpanded, sel, r.dirUntracked)
		_, _ = fmt.Fprint(w, line)
	}
}

func isDeletionPorcelain(f gogit.ChangedFile) bool {
	if len(f.PorcelainXY) != 2 {
		return false
	}
	return f.PorcelainXY[0] == 'D' || f.PorcelainXY[1] == 'D'
}

func fileBadge(f gogit.ChangedFile, selected bool) string {
	if isDeletionPorcelain(f) {
		if selected {
			return lipgloss.NewStyle().Foreground(listSelectedPathFg).Bold(true).Render("D")
		}
		return fileListBadgeDeleted.Render("D")
	}
	switch {
	case f.IsUntracked:
		return fileListBadgeUntracked.Render("U")
	case f.HasStaged:
		return fileListBadgeStaged.Render("S")
	default:
		return fileListBadgeModified.Render("M")
	}
}

// List selection marker: U+276F HEAVY RIGHT-POINTING ANGLE BRACKET (IDE-style caret).
const listSelectionMark = "\u276f"

func formatFileRow(listWidth int, f gogit.ChangedFile, selected bool) string {
	if listWidth < 8 {
		listWidth = 8
	}
	badge := fileBadge(f, selected)
	gap := " "
	var lead string
	if selected {
		lead = listSelectionArrow.Render(listSelectionMark) + " "
	}
	prefix := lead + badge + gap
	prefixW := lipgloss.Width(prefix)
	avail := listWidth - prefixW
	if avail < 4 {
		avail = 4
	}
	path := ansi.TruncateWc(f.Path, avail, "…")

	var pathStyle lipgloss.Style
	if selected {
		pathStyle = lipgloss.NewStyle().
			Foreground(listSelectedPathFg).
			Bold(true)
	} else if isDeletionPorcelain(f) {
		pathStyle = lipgloss.NewStyle().Foreground(listDeletedFg)
	} else if f.IsUntracked {
		pathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("210"))
	} else if f.HasStaged {
		pathStyle = lipgloss.NewStyle().Foreground(listStagedFg)
	} else {
		pathStyle = lipgloss.NewStyle().Foreground(listFileFg)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Left, prefix, pathStyle.Render(path))

	return lipgloss.NewStyle().Width(listWidth).PaddingLeft(0).Render(row)
}

func formatTreeFileRow(listWidth int, treePrefix string, f gogit.ChangedFile, segName string, selected bool) string {
	if listWidth < 8 {
		listWidth = 8
	}
	pfxW := lipgloss.Width(treePrefix)
	if pfxW >= listWidth-4 {
		treePrefix = ansi.TruncateWc(treePrefix, max(4, listWidth-8), "…")
		pfxW = lipgloss.Width(treePrefix)
	}
	badge := fileBadge(f, selected)
	gap := " "
	var lead string
	if selected {
		lead = listSelectionArrow.Render(listSelectionMark) + " "
	}
	prefix := treeBranchStyle.Render(treePrefix) + lead + badge + gap
	prefixW := lipgloss.Width(prefix)
	avail := listWidth - prefixW
	if avail < 4 {
		avail = 4
	}
	label := segName
	if strings.TrimSpace(label) == "" {
		label = filepath.Base(f.Path)
	}
	pathDisp := ansi.TruncateWc(label, avail, "…")
	var pathStyle lipgloss.Style
	if selected {
		pathStyle = lipgloss.NewStyle().Foreground(listSelectedPathFg).Bold(true)
	} else if isDeletionPorcelain(f) {
		pathStyle = lipgloss.NewStyle().Foreground(listDeletedFg)
	} else if f.IsUntracked {
		pathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("210"))
	} else if f.HasStaged {
		pathStyle = lipgloss.NewStyle().Foreground(listStagedFg)
	} else {
		pathStyle = lipgloss.NewStyle().Foreground(listFileFg)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Left, prefix, pathStyle.Render(pathDisp))
	return lipgloss.NewStyle().Width(listWidth).Render(row)
}

func formatTreeDirRow(listWidth int, treePrefix string, seg string, expanded, selected, untrackedSection bool) string {
	if listWidth < 8 {
		listWidth = 8
	}
	pfxW := lipgloss.Width(treePrefix)
	if pfxW >= listWidth-6 {
		treePrefix = ansi.TruncateWc(treePrefix, max(4, listWidth-10), "…")
	}
	chev := "▸ "
	if expanded {
		chev = "▾ "
	}
	if untrackedSection {
		chev = lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Render(chev)
	} else {
		chev = treeFolderGlyphStyle.Render(chev)
	}
	var lead string
	if selected {
		lead = listSelectionArrow.Render(listSelectionMark) + " "
	}
	prefix := treeBranchStyle.Render(treePrefix) + lead + chev
	prefixW := lipgloss.Width(prefix)
	avail := listWidth - prefixW
	if avail < 4 {
		avail = 4
	}
	name := ansi.TruncateWc(seg, avail, "…")
	st := treeFolderNameStyle
	if selected {
		st = lipgloss.NewStyle().Foreground(listSelectedPathFg).Bold(true)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Left, prefix, st.Render(name))
	return lipgloss.NewStyle().Width(listWidth).Render(row)
}

func snapshotSignature(repoRoot string, s gogit.Snapshot) string {
	var b strings.Builder
	b.WriteString(s.Branch)
	b.WriteByte('|')
	b.WriteString(s.Error)
	for _, f := range s.Files {
		b.WriteString(f.Path)
		b.WriteByte(':')
		b.WriteString(f.PorcelainXY)
		if f.IsUntracked {
			b.WriteByte('u')
		}
		if f.HasStaged {
			b.WriteByte('s')
		}
		// Include mtime so edits to already-modified files invalidate the cache.
		if info, err := os.Stat(filepath.Join(repoRoot, f.Path)); err == nil {
			fmt.Fprintf(&b, "@%d", info.ModTime().UnixNano())
		}
		b.WriteByte(';')
	}
	return b.String()
}

func changedFileByPath(snap gogit.Snapshot, path string) (gogit.ChangedFile, bool) {
	for _, f := range snap.Files {
		if f.Path == path {
			return f, true
		}
	}
	return gogit.ChangedFile{}, false
}

// previewHeaderStatic builds the preview header without touching list (safe for goroutines).
func previewHeaderStatic(snap gogit.Snapshot, filePath string) string {
	branch := snap.Branch
	if branch == "" {
		branch = "—"
	}
	errPart := ""
	if strings.TrimSpace(snap.Error) != "" {
		errPart = warnAccent.Render(" · ") + warnAccent.Render(snap.Error)
	}
	line1 := titleBarStyle.Render(headerAccent.Render("⎇ "+branch) + errPart)
	if filePath == "" {
		return line1
	}
	return line1 + "\n" + mutedPathStyle.Render(filePath)
}

// New builds a model for repo root; default focus is the file tree.
func New(repoRoot string, width, height int) *Model {
	del := newItemDelegate()
	del.Styles.NormalTitle = lipgloss.NewStyle()
	del.Styles.SelectedTitle = lipgloss.NewStyle()

	l := list.New([]list.Item{}, del, 28, 18)
	l.SetShowTitle(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	l.Styles.NoItems = metaStyle

	vp := viewport.New(72, 18)

	ci := textinput.New()
	ci.Placeholder = "Commit message (git commit -a — tracked files only)…"
	ci.CharLimit = 500
	ci.Width = 50

	bi := textinput.New()
	bi.Placeholder = "Branch name…"
	bi.CharLimit = 200
	bi.Width = 50

	m := &Model{
		repoRoot:     repoRoot,
		focusLeft:    true,
		list:         l,
		vp:           vp,
		termWidth:    width,
		termH:        height,
		commitInput:  ci,
		branchInput:  bi,
		previewCache: make(map[string]string, previewCacheMaxEntries),
	}
	m.applyWindowSize(width, height)
	m.refreshGitState()
	return m
}

func (m *Model) Init() tea.Cmd {
	// Preview builds asynchronously — UI thread never blocks on chroma/git.
	return tea.Batch(tickCmd(), m.previewCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(450*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func layoutHeight(termH int) int {
	h := termH - topMargin
	if h < 4 {
		h = 4
	}
	return h
}

// panelLayH is framed panel height: one line below layoutHeight;
// top terminal row is reserved for global status (branch · files · preview).
func panelLayH(termH int) int {
	x := layoutHeight(termH) - 1
	if x < 4 {
		x = 4
	}
	return x
}

// First content row of left/right panel in mouse coords (after marginTop and status line).
func panelMouseTopY() int {
	return topMargin + 2
}

func (m *Model) mouseInLeftPanel(msg tea.MouseMsg) bool {
	w := m.termWidth
	if w <= 0 {
		w = 80
	}
	h := m.termH
	if h <= 0 {
		h = 24
	}
	if msg.X < 1 || msg.X > m.leftColW {
		return false
	}
	if msg.X > w {
		return false
	}
	layH := panelLayH(h)
	top := panelMouseTopY()
	bottom := topMargin + 1 + layH
	return msg.Y >= top && msg.Y <= bottom
}

func (m *Model) mouseInRightPanel(msg tea.MouseMsg) bool {
	w := m.termWidth
	if w <= 0 {
		w = 80
	}
	h := m.termH
	if h <= 0 {
		h = 24
	}
	if msg.X <= m.leftColW || msg.X > w {
		return false
	}
	layH := panelLayH(h)
	top := panelMouseTopY()
	bottom := topMargin + 1 + layH
	return msg.Y >= top && msg.Y <= bottom
}

func (m *Model) scrollFileListWheel(msg tea.MouseMsg) tea.Cmd {
	prev := m.selectedPath
	switch msg.Type {
	case tea.MouseWheelUp, tea.MouseWheelLeft:
		m.list.CursorUp()
		m.skipNonFileRows(-1)
	case tea.MouseWheelDown, tea.MouseWheelRight:
		m.list.CursorDown()
		m.skipNonFileRows(1)
	default:
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelLeft:
			m.list.CursorUp()
			m.skipNonFileRows(-1)
		case tea.MouseButtonWheelDown, tea.MouseButtonWheelRight:
			m.list.CursorDown()
			m.skipNonFileRows(1)
		default:
			return nil
		}
	}
	if m.selectedPath != prev {
		return m.previewCmd()
	}
	return nil
}

func (m *Model) applyWindowSize(width, height int) {
	m.termWidth = width
	m.termH = height

	layH := panelLayH(height)

	// Column split + room for vertical borders so the preview right edge
	// stays visible with the scrollbar.
	left := width * 24 / 100
	if left < 14 {
		left = 14
	}
	maxLeft := width * 44 / 100
	if maxLeft < 22 {
		maxLeft = 22
	}
	if left > maxLeft {
		left = maxLeft
	}
	if left >= width-12 {
		left = width / 4
		if left < 12 {
			left = 12
		}
	}
	const seamW = 4
	right := width - left - seamW
	if right < 12 {
		right = 12
		left = width - right - seamW
		if left < 12 {
			left = 12
			right = width - left - seamW
		}
	}
	m.leftColW = left
	m.rightColW = right

	listInnerW := m.leftColW
	if listInnerW < 6 {
		listInnerW = 6
	}

	innerH := layH
	if innerH < 3 {
		innerH = 3
	}
	// Status strip is outside panels — left column is only the file tree.
	listH := innerH
	if listH < 1 {
		listH = 1
	}
	m.list.SetSize(listInnerW, listH)

	rightInner := m.rightColW
	vpW := rightInner - 1
	if vpW < 10 {
		vpW = 10
	}
	vpH := innerH
	if vpH < 1 {
		vpH = 1
	}
	m.vp.Width = vpW
	m.vp.Height = vpH
	// Otherwise horizontalStep = 0 and arrows / h l won't scroll the preview horizontally.
	step := vpW / 5
	if step < 4 {
		step = 4
	}
	if step > 24 {
		step = 24
	}
	m.vp.SetHorizontalStep(step)

	iw := width - 14
	if iw > 72 {
		iw = 72
	}
	if iw < 24 {
		iw = 24
	}
	m.commitInput.Width = iw
	m.branchInput.Width = iw
}

func (m *Model) commitCmd(message string) tea.Cmd {
	repo := m.repoRoot
	msg := strings.TrimSpace(message)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := gogit.CommitWorkingTree(ctx, repo, msg)
		return commitDoneMsg{err: err}
	}
}

func (m *Model) createBranchCmd(name string) tea.Cmd {
	repo := m.repoRoot
	n := strings.TrimSpace(name)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := gogit.CreateBranchFromMain(ctx, repo, n)
		return branchDoneMsg{err: err}
	}
}

func (m *Model) gitAddPathCmd(path string) tea.Cmd {
	repo := m.repoRoot
	p := path
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := gogit.AddPath(ctx, repo, p)
		return fileGitOpDoneMsg{err: err}
	}
}

func (m *Model) gitRevertFileCmd(cf gogit.ChangedFile) tea.Cmd {
	repo := m.repoRoot
	file := cf
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := gogit.RevertFileSelection(ctx, repo, file)
		return fileGitOpDoneMsg{err: err}
	}
}

func (m *Model) gitPushCmd() tea.Cmd {
	repo := m.repoRoot
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := gogit.PushCurrentBranch(ctx, repo)
		return fileGitOpDoneMsg{err: err}
	}
}

func (m *Model) openEditorCmd(relPath string) tea.Cmd {
	repo := m.repoRoot
	p := relPath
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := OpenFileInVSCode(ctx, repo, p)
		return fileGitOpDoneMsg{err: err}
	}
}

// previewCmd runs expensive preview assembly in a Bubble Tea command (outside Update).
func (m *Model) previewCmd() tea.Cmd {
	m.previewSeq++
	seq := m.previewSeq
	path := m.selectedPath
	snap := m.snap
	repo := m.repoRoot
	cacheKey := previewCacheKey(path, m.vp.Width, m.snapSig, m.previewShowDeletions)

	if cached, ok := m.getPreviewCache(cacheKey); ok {
		return func() tea.Msg {
			return previewDoneMsg{
				seq:      seq,
				path:     path,
				content:  cached,
				cacheKey: cacheKey,
			}
		}
	}

	return func() tea.Msg {
		if len(snap.Files) == 0 {
			return previewDoneMsg{seq: seq, path: path, isEmpty: true}
		}
		if path == "" {
			hdr := previewHeaderStatic(snap, "")
			body := metaStyle.Render("Select a file (↑↓ · Enter on a folder — expand or collapse).")
			return previewDoneMsg{
				seq:      seq,
				path:     "",
				content:  hdr + "\n\n" + body,
				cacheKey: previewCacheKey("", m.vp.Width, m.snapSig, m.previewShowDeletions),
			}
		}
		cf, ok := changedFileByPath(snap, path)
		if !ok {
			return previewDoneMsg{seq: seq, path: path, isEmpty: true}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		hdr := previewHeaderStatic(snap, cf.Path)
		body := BuildPreview(ctx, repo, cf, m.vp.Width, m.previewShowDeletions)
		return previewDoneMsg{
			seq:      seq,
			path:     path,
			content:  hdr + "\n" + body,
			cacheKey: cacheKey,
		}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case commitDoneMsg:
		m.commitModalOpen = false
		m.commitInput.Blur()
		if msg.err != nil {
			m.commitErr = msg.err.Error()
			return m, nil
		}
		m.commitErr = ""
		m.commitInput.SetValue("")
		m.refreshGitState()
		return m, m.previewCmd()

	case branchDoneMsg:
		m.branchCreating = false
		m.branchModalOpen = false
		m.branchInput.Blur()
		if msg.err != nil {
			m.branchErr = msg.err.Error()
			return m, nil
		}
		m.branchErr = ""
		m.branchInput.SetValue("")
		m.refreshGitState()
		return m, m.previewCmd()

	case fileGitOpDoneMsg:
		if msg.err != nil {
			m.fileOpErr = msg.err.Error()
			return m, nil
		}
		m.fileOpErr = ""
		m.refreshGitState()
		return m, m.previewCmd()

	case previewDoneMsg:
		if msg.seq != m.previewSeq {
			return m, nil
		}
		if msg.path != m.selectedPath {
			return m, nil
		}
		if msg.isEmpty {
			m.vp.SetContent(wrapPreview(metaStyle.Render("No changed files."), m.vp.Width))
			m.vp.GotoTop()
			m.vp.SetXOffset(0)
			m.lastPreviewPath = ""
			m.previewJumpLines = nil
			return m, nil
		}
		m.vp.SetContent(wrapPreview(msg.content, m.vp.Width))
		m.previewJumpLines = collectPreviewJumpLineIndices(msg.content)
		m.putPreviewCache(msg.cacheKey, msg.content)
		if msg.path != m.lastPreviewPath {
			m.vp.GotoTop()
			m.vp.SetXOffset(0)
			m.lastPreviewPath = msg.path
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.applyWindowSize(msg.Width, msg.Height)
		var vcmd tea.Cmd
		m.vp, vcmd = m.vp.Update(msg)
		return m, tea.Batch(vcmd, m.previewCmd())

	case tickMsg:
		var pcmd tea.Cmd
		if m.refreshGitState() {
			pcmd = m.previewCmd()
		}
		return m, tea.Batch(tickCmd(), pcmd)

	case tea.KeyMsg:
		if m.commitModalOpen {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.commitModalOpen = false
				m.commitInput.Blur()
				m.commitErr = ""
				return m, nil
			case "enter":
				t := strings.TrimSpace(m.commitInput.Value())
				if t == "" {
					m.commitErr = "empty commit message"
					return m, nil
				}
				m.commitErr = ""
				return m, m.commitCmd(t)
			}
			if msg.Type != tea.KeyEnter && m.commitErr != "" {
				m.commitErr = ""
			}
			var icmd tea.Cmd
			m.commitInput, icmd = m.commitInput.Update(msg)
			return m, icmd
		}

		if m.branchModalOpen {
			if m.branchCreating {
				if msg.String() == "ctrl+c" {
					return m, tea.Quit
				}
				return m, nil
			}
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.branchModalOpen = false
				m.branchInput.Blur()
				m.branchErr = ""
				return m, nil
			case "enter":
				n := strings.TrimSpace(m.branchInput.Value())
				if n == "" {
					m.branchErr = "empty branch name"
					return m, nil
				}
				m.branchErr = ""
				m.branchCreating = true
				return m, m.createBranchCmd(n)
			}
			if msg.Type != tea.KeyEnter && m.branchErr != "" {
				m.branchErr = ""
			}
			var icmd tea.Cmd
			m.branchInput, icmd = m.branchInput.Update(msg)
			return m, icmd
		}

		if m.focusLeft && (msg.String() == "enter" || msg.Type == tea.KeyEnter) {
			if it, ok := m.list.SelectedItem().(rowItem); ok && it.kind == rowTreeDir {
				m.toggleDirExpandedAndRefresh(it.dirPath)
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+p":
			m.fileOpErr = ""
			return m, m.gitPushCmd()
		}

		m.captureSelection()
		switch msg.String() {
		case "ctrl+o":
			if m.selectedPath == "" {
				m.fileOpErr = "no file selected"
				return m, nil
			}
			m.fileOpErr = ""
			return m, m.openEditorCmd(m.selectedPath)
		}

		if m.focusLeft {
			m.captureSelection()
			if m.selectedPath != "" {
				switch msg.String() {
				case "ctrl+a":
					m.fileOpErr = ""
					cf, ok := changedFileByPath(m.snap, m.selectedPath)
					if ok {
						return m, m.gitAddPathCmd(cf.Path)
					}
					return m, nil
				case "ctrl+u":
					m.fileOpErr = ""
					cf, ok := changedFileByPath(m.snap, m.selectedPath)
					if ok {
						return m, m.gitRevertFileCmd(cf)
					}
					return m, nil
				}
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			return m, tea.Quit
		case "ctrl+g":
			m.commitModalOpen = true
			m.commitErr = ""
			m.fileOpErr = ""
			m.commitInput.SetValue("")
			return m, m.commitInput.Focus()
		case "ctrl+b":
			m.branchModalOpen = true
			m.branchErr = ""
			m.fileOpErr = ""
			m.branchInput.SetValue("")
			return m, m.branchInput.Focus()
		case "tab":
			m.focusLeft = !m.focusLeft
			return m, nil
		case "d":
			m.previewShowDeletions = !m.previewShowDeletions
			return m, m.previewCmd()
		}

		// Jump between changed lines in preview — Ctrl+N (either focus).
		if msg.Type == tea.KeyCtrlN || msg.String() == "ctrl+n" {
			m.jumpPreviewToNextChange()
			return m, nil
		}

		if m.focusLeft {
			prev := m.selectedPath
			k := msg.String()
			msg2 := msg
			navBias := 0 // +1 down, -1 up, 0 neutral (Home/End, etc.)
			switch k {
			case "j":
				msg2 = tea.KeyMsg{Type: tea.KeyDown}
				navBias = 1
			case "k":
				msg2 = tea.KeyMsg{Type: tea.KeyUp}
				navBias = -1
			case "g":
				msg2 = tea.KeyMsg{Type: tea.KeyHome}
			case "G":
				msg2 = tea.KeyMsg{Type: tea.KeyEnd}
			}
			if navBias == 0 {
				switch msg.Type {
				case tea.KeyDown:
					navBias = 1
				case tea.KeyUp:
					navBias = -1
				}
			}
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg2)
			m.skipNonFileRows(navBias)
			if m.selectedPath != prev {
				return m, tea.Batch(cmd, m.previewCmd())
			}
			return m, cmd
		}

		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.MouseMsg:
		if m.commitModalOpen {
			return m, nil
		}
		if tea.MouseEvent(msg).IsWheel() {
			switch {
			case m.mouseInLeftPanel(msg):
				if cmd := m.scrollFileListWheel(msg); cmd != nil {
					return m, cmd
				}
				return m, nil
			case m.mouseInRightPanel(msg):
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		if m.focusLeft {
			prev := m.selectedPath
			prevIdx := m.list.Index()
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			nav := 0
			if m.list.Index() > prevIdx {
				nav = 1
			} else if m.list.Index() < prevIdx {
				nav = -1
			}
			m.skipNonFileRows(nav)
			if m.selectedPath != prev {
				return m, tea.Batch(cmd, m.previewCmd())
			}
			return m, cmd
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	default:
		if m.commitModalOpen {
			return m, nil
		}
		if m.focusLeft && isXTermCtrlDelete(msg) {
			m.captureSelection()
			if m.selectedPath != "" {
				cf, ok := changedFileByPath(m.snap, m.selectedPath)
				if ok {
					m.fileOpErr = ""
					return m, m.gitRevertFileCmd(cf)
				}
			}
		}
	}

	return m, nil
}

func previewNextJumpOffset(cur int, targets []int) int {
	if len(targets) == 0 {
		return cur
	}
	for _, t := range targets {
		if t > cur {
			return t
		}
	}
	return targets[0]
}

func (m *Model) jumpPreviewToNextChange() {
	if len(m.previewJumpLines) == 0 {
		return
	}
	next := previewNextJumpOffset(m.vp.YOffset, m.previewJumpLines)
	m.vp.SetYOffset(next)
}

// isXTermCtrlDelete: xterm sends Ctrl+Delete as CSI 3;5 ~; Bubble Tea often surfaces it as unknownCSI.
func isXTermCtrlDelete(msg tea.Msg) bool {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return false
	}
	return string(v.Bytes()) == "\x1b[3;5~"
}

func (m *Model) captureSelection() {
	it, ok := m.list.SelectedItem().(rowItem)
	if !ok {
		m.selectedPath = ""
		return
	}
	if isFileRow(it.kind) {
		m.selectedPath = it.file.Path
		return
	}
	m.selectedPath = ""
}

// skipNonFileRows moves the cursor off headers/dividers onto a navigable row.
// navBias: after moving down (+1) prefer scanning downward; after up (-1) prefer upward;
// 0 for Home/End / no direction.
func (m *Model) skipNonFileRows(navBias int) {
	items := m.list.Items()
	if len(items) == 0 {
		m.selectedPath = ""
		return
	}
	idx := m.list.Index()
	if idx < 0 {
		idx = 0
	}
	try := func(i int) bool {
		if i < 0 || i >= len(items) {
			return false
		}
		r, ok := items[i].(rowItem)
		if !ok {
			return false
		}
		switch r.kind {
		case rowTrackedHeader, rowDivider, rowUntrackedHeader:
			return false
		default:
			m.list.Select(i)
			m.captureSelection()
			return true
		}
	}
	if try(idx) {
		return
	}
	scanDown := func() bool {
		for i := idx + 1; i < len(items); i++ {
			if try(i) {
				return true
			}
		}
		return false
	}
	scanUp := func() bool {
		for i := idx - 1; i >= 0; i-- {
			if try(i) {
				return true
			}
		}
		return false
	}
	found := false
	switch {
	case navBias > 0:
		found = scanDown()
		if !found {
			found = scanUp()
		}
	case navBias < 0:
		found = scanUp()
		if !found {
			found = scanDown()
		}
	default:
		found = scanDown()
		if !found {
			found = scanUp()
		}
	}
	if !found {
		m.selectedPath = ""
	}
}

func (m *Model) refreshGitState() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	snap := gogit.SnapshotFromRepo(ctx, m.repoRoot)
	sig := snapshotSignature(m.repoRoot, snap)
	if sig == m.snapSig {
		return false
	}
	m.snapSig = sig
	m.snap = snap

	prev := m.selectedPath
	m.pruneExpandedDirs(snap.Files)
	items := m.buildFileListItems()
	m.list.SetItems(items)

	if prev != "" {
		for i, it := range items {
			ri, ok := it.(rowItem)
			if ok && isFileRow(ri.kind) && ri.file.Path == prev {
				m.list.Select(i)
				break
			}
		}
	}
	m.skipNonFileRows(0)
	return true
}

// previewStatusPlain is one physical line — avoid lipgloss.Width here (narrow column wraps).
// Truncate only in the top bar via terminal width.
func (m *Model) previewStatusPlain() string {
	total := m.vp.TotalLineCount()
	if total == 0 {
		return "no data"
	}
	vis := m.vp.Height
	top := m.vp.YOffset + 1
	visibleLines := vis
	if m.vp.YOffset+visibleLines > total {
		visibleLines = total - m.vp.YOffset
	}
	if visibleLines < 0 {
		visibleLines = 0
	}
	bottom := m.vp.YOffset + visibleLines
	if total <= vis {
		return fmt.Sprintf("all visible · %d ln", total)
	}
	return fmt.Sprintf("lines %d–%d", top, bottom)
}

func wrapPreview(s string, maxW int) string {
	if maxW <= 10 {
		return s
	}
	_ = maxW
	return s
}

func (m *Model) View() string {
	w := m.termWidth
	h := m.termH
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	if m.commitModalOpen {
		return lipgloss.Place(
			w, h,
			lipgloss.Center, lipgloss.Center,
			m.commitModalView(),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("232")),
		)
	}
	if m.branchModalOpen {
		return lipgloss.Place(
			w, h,
			lipgloss.Center, lipgloss.Center,
			m.branchModalView(),
			lipgloss.WithWhitespaceBackground(lipgloss.Color("232")),
		)
	}
	return m.mainLayoutView()
}

func (m *Model) mainLayoutView() string {
	w := m.termWidth
	h := m.termH
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	layH := panelLayH(h)

	innerH := layH

	branch := m.snap.Branch
	if branch == "" {
		branch = "—"
	}
	diffHint := ""
	if len(m.snap.Files) > 0 {
		if m.previewShowDeletions {
			diffHint = " · preview ±"
		} else {
			diffHint = " · preview +"
		}
	}
	logoBlock := " " + logoLetterG.Render("g") + logoLetterI.Render("i") + logoLetterV.Render("v")
	rest := fmt.Sprintf(" ⎇ %s · %d · (%s)%s", branch, len(m.snap.Files), m.previewStatusPlain(), diffHint)
	statusLine := logoBlock + topBarMuted.Render(rest)
	if ce := strings.TrimSpace(m.commitErr); ce != "" {
		statusLine += topBarMuted.Render(" · " + ce)
	}
	if fe := strings.TrimSpace(m.fileOpErr); fe != "" {
		statusLine += topBarMuted.Render(" · " + fe)
	}
	topBar := lipgloss.NewStyle().
		Width(w).
		Render(ansi.TruncateWc(statusLine, w, "…"))

	leftStack := m.list.View()
	leftBox := PanelFrame(m.focusLeft).
		Width(m.leftColW).
		Height(layH).
		Render(leftStack)

	vpStr := lipgloss.NewStyle().
		Width(m.vp.Width).
		Height(innerH).
		Render(m.vp.View())
	scr := scrollbarColumn(m.vp, m.previewJumpLines)
	rightTop := lipgloss.JoinHorizontal(lipgloss.Top, vpStr, scr)
	rightPadded := rightTop

	rightBox := PanelFrameRight(!m.focusLeft).
		Width(m.rightColW).
		Height(layH).
		Render(rightPadded)

	row := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	content := lipgloss.JoinVertical(lipgloss.Left, topBar, row)

	body := lipgloss.NewStyle().MarginTop(topMargin).Render(content)
	// Horizontal place only: PlaceVertical would pad bottom when lipgloss height < h,
	// showing blank rows on the alt screen; we fix panel row height via panelLayH.
	return lipgloss.PlaceHorizontal(w, lipgloss.Left, body)
}
