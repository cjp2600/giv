package tui

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	ltree "github.com/charmbracelet/lipgloss/tree"
	gogit "github.com/cjp2600/giv/internal/git"
)

// enumSibs satisfies ltree.Children with only Length() wired for DefaultEnumerator.
type enumSibs int

func (n enumSibs) Length() int { return int(n) }

func (enumSibs) At(int) ltree.Node { return nil }

// treeAncestorPrefix mirrors DefaultIndenter given ancestor “last sibling” flags.
func treeAncestorPrefix(ancestorLast []bool) string {
	var b strings.Builder
	for _, last := range ancestorLast {
		// Same as DefaultIndenter: last child → spaces, else vertical bar column.
		if last {
			b.WriteString("   ")
		} else {
			b.WriteString("│  ")
		}
	}
	return b.String()
}

func treeBranchGlyph(siblingIdx, siblingCount int) string {
	if siblingCount <= 0 {
		return ""
	}
	return ltree.DefaultEnumerator(enumSibs(siblingCount), siblingIdx)
}

type fileTrieNode struct {
	name     string
	children map[string]*fileTrieNode
	file     *gogit.ChangedFile
}

func (n *fileTrieNode) insert(parts []string, idx int, f *gogit.ChangedFile) {
	if idx >= len(parts) {
		return
	}
	seg := parts[idx]
	if n.children == nil {
		n.children = make(map[string]*fileTrieNode)
	}
	ch := n.children[seg]
	if ch == nil {
		ch = &fileTrieNode{name: seg}
		n.children[seg] = ch
	}
	if idx == len(parts)-1 {
		ch.file = f
		return
	}
	ch.insert(parts, idx+1, f)
}

func buildFileTrie(files []gogit.ChangedFile) *fileTrieNode {
	root := &fileTrieNode{name: ""}
	for i := range files {
		f := &files[i]
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		if p == "" {
			continue
		}
		parts := strings.Split(p, "/")
		root.insert(parts, 0, f)
	}
	return root
}

func sortedTrieKeys(n *fileTrieNode) []string {
	if n == nil || len(n.children) == 0 {
		return nil
	}
	var dirs, files []string
	for k, ch := range n.children {
		if ch.file != nil && len(ch.children) == 0 {
			files = append(files, k)
		} else {
			dirs = append(dirs, k)
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)
	return append(dirs, files...)
}

func dirPathJoin(parent, seg string) string {
	if parent == "" {
		return seg
	}
	return filepath.Join(parent, seg)
}

// collapseSingleChildDirChain merges consecutive dirs that each have exactly one child directory
// (not a file). Stops at a branch or at a lone file child.
// Returns the tip node (children render from it), full path to the tip, and slash-joined label.
func collapseSingleChildDirChain(parentPath, firstSeg string, first *fileTrieNode) (tip *fileTrieNode, mergedPath, mergedLabel string) {
	segs := []string{firstSeg}
	cur := first
	mergedPath = dirPathJoin(parentPath, firstSeg)
	for {
		kids := sortedTrieKeys(cur)
		if len(kids) != 1 {
			break
		}
		nextKey := kids[0]
		next := cur.children[nextKey]
		if next == nil {
			break
		}
		if next.file != nil && len(next.children) == 0 {
			break
		}
		segs = append(segs, nextKey)
		mergedPath = dirPathJoin(mergedPath, nextKey)
		cur = next
	}
	tip = cur
	mergedLabel = strings.Join(segs, "/")
	return tip, mergedPath, mergedLabel
}

func (m *Model) appendTreeSection(out []list.Item, root *fileTrieNode, parentPath string, ancestorLast []bool, untrackedSection bool) []list.Item {
	if root == nil {
		return out
	}
	keys := sortedTrieKeys(root)
	for i, key := range keys {
		ch := root.children[key]
		if ch == nil {
			continue
		}
		isLast := i == len(keys)-1
		prefix := treeAncestorPrefix(ancestorLast)
		branch := treeBranchGlyph(i, len(keys))

		if ch.file != nil && len(ch.children) == 0 {
			kind := rowFileTracked
			if untrackedSection {
				kind = rowFileUntracked
			}
			out = append(out, rowItem{
				kind:       kind,
				file:       *ch.file,
				treePrefix: prefix + branch,
				segName:    key,
			})
			continue
		}

		tip, mergedPath, mergedLabel := collapseSingleChildDirChain(parentPath, key, ch)
		exp := m.dirExpanded(mergedPath)
		out = append(out, rowItem{
			kind:         rowTreeDir,
			treePrefix:   prefix + branch,
			segName:      mergedLabel,
			dirPath:      mergedPath,
			dirExpanded:  exp,
			dirUntracked: untrackedSection,
		})
		if exp {
			nextLast := append(append([]bool(nil), ancestorLast...), isLast)
			out = m.appendTreeSection(out, tip, mergedPath, nextLast, untrackedSection)
		}
	}
	return out
}

func (m *Model) dirExpanded(dirPath string) bool {
	if m.expandedDirs == nil {
		return true
	}
	v, ok := m.expandedDirs[dirPath]
	if !ok {
		return true
	}
	return v
}

func (m *Model) setDirExpanded(dirPath string, expanded bool) {
	if m.expandedDirs == nil {
		m.expandedDirs = make(map[string]bool)
	}
	m.expandedDirs[dirPath] = expanded
}

func (m *Model) toggleDirExpanded(dirPath string) {
	m.setDirExpanded(dirPath, !m.dirExpanded(dirPath))
}

func (m *Model) pruneExpandedDirs(files []gogit.ChangedFile) {
	valid := make(map[string]struct{})
	for i := range files {
		p := filepath.ToSlash(strings.TrimSpace(files[i].Path))
		if p == "" {
			continue
		}
		dir := filepath.Dir(p)
		for dir != "." && dir != "" {
			valid[dir] = struct{}{}
			dir = filepath.Dir(dir)
		}
	}
	for k := range m.expandedDirs {
		if _, ok := valid[k]; !ok {
			delete(m.expandedDirs, k)
		}
	}
}

func (m *Model) buildFileListItems() []list.Item {
	var tracked, untracked []gogit.ChangedFile
	for _, f := range m.snap.Files {
		if f.IsUntracked {
			untracked = append(untracked, f)
		} else {
			tracked = append(tracked, f)
		}
	}
	var out []list.Item
	if len(tracked) > 0 {
		out = append(out, rowItem{kind: rowTrackedHeader})
		out = m.appendTreeSection(out, buildFileTrie(tracked), "", nil, false)
	}
	if len(tracked) > 0 && len(untracked) > 0 {
		out = append(out, rowItem{kind: rowDivider})
	}
	if len(untracked) > 0 {
		out = append(out, rowItem{kind: rowUntrackedHeader})
		out = m.appendTreeSection(out, buildFileTrie(untracked), "", nil, true)
	}
	return out
}

// toggleDirExpandedAndRefresh rebuilds the list and keeps the cursor on the same folder row.
func (m *Model) toggleDirExpandedAndRefresh(dirPath string) {
	m.toggleDirExpanded(dirPath)
	prevSel := m.selectedPath
	items := m.buildFileListItems()
	m.list.SetItems(items)
	for i, it := range items {
		ri, ok := it.(rowItem)
		if ok && ri.kind == rowTreeDir && ri.dirPath == dirPath {
			m.list.Select(i)
			m.captureSelection()
			return
		}
	}
	if prevSel != "" {
		for i, it := range items {
			ri, ok := it.(rowItem)
			if ok && isFileRow(ri.kind) && ri.file.Path == prevSel {
				m.list.Select(i)
				m.captureSelection()
				return
			}
		}
	}
	if len(items) > 0 {
		m.list.Select(0)
		m.skipNonFileRows(0)
	}
}
