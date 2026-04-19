# giv

**Terminal UI for reviewing git changes without leaving the shell.** Built for workflows where an LLM edits your repo and you want a fast, readable diff next to a navigable file tree—no context switch to a GUI or browser.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)

## Why giv?

- **Stay in the terminal** while you skim what changed: modified paths in a **collapsible tree** (single-child paths are **compacted** with `/`), syntax-highlighted preview, and optional full unified diff.
- **LLM-assisted coding**: after generated patches land in your working tree, open `giv` in the repo root and walk changes file-by-file with **Ctrl+N** jumping between edited hunks.
- **Lightweight**: one binary, [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss); works anywhere you have git and a decent terminal.

## Features

- Two-pane layout: **file tree** (tracked vs untracked sections) + **preview** with line numbers and Dracula-style highlighting (via Chroma).
- Toggle preview mode **`d`**: working-tree view vs full diff including deleted lines.
- Git shortcuts: **Ctrl+A** (add), **Ctrl+U** (revert per giv rules), **Ctrl+P** (push), **Ctrl+G** (commit changed tracked files with a message), **Ctrl+O** (open file in editor—VS Code `code` if available).
- Mouse: wheel scroll on each pane when the terminal reports cell motion events.

## Requirements

- **Go 1.26+** (see `go.mod`) to build from source.
- **Git** on `PATH`.
- A terminal with **alternate screen** and ideally **mouse reporting** for wheel support.

## Install

### From source

```bash
git clone https://github.com/cjp2600/giv.git
cd giv
go build -o giv ./cmd/giv
```

Put the binary on your `PATH`, for example:

```bash
go install github.com/cjp2600/giv/cmd/giv@latest
```

(Adjust the module path if you fork or change `go.mod`.)

### Verify

```bash
cd /path/to/a/git/repo
giv --hotkey   # print keybindings and exit
giv            # full-screen TUI
```

## Usage

Run **`giv` from the repository root** (or any directory inside it; the tool resolves the git root).

| Action | Keys |
|--------|------|
| Focus list ↔ preview | **Tab** |
| Move in tree / preview | **↑↓**, **j/k**, **PgUp/PgDn** |
| Expand/collapse folder | **Enter** on a directory row |
| Jump to next changed line in preview | **Ctrl+N** |
| Toggle diff mode (with/without deletions) | **d** |
| Commit tracked changes | **Ctrl+G**, then type message, **Enter** |
| Quit | **q**, **Esc**, or **Ctrl+C** |

Full table: `giv --hotkey`.

### Example: review LLM output

```bash
# You asked an LLM to refactor a package; patches applied to the working tree
cd my-service
giv
```

1. Use **↑↓** or the mouse to select a file in the left tree.
2. Read the highlighted preview on the right; press **`d`** if you want to see removed lines too.
3. Press **Ctrl+N** repeatedly to jump between colored change regions.
4. **Ctrl+A** to stage a file you accept, **Ctrl+U** to revert one you don’t, **Ctrl+G** to commit when ready.

This matches the original motivation: **review AI-generated edits entirely from the command line**, without opening a separate diff tool or IDE for every pass.

## Configuration

There is no config file yet; behavior is opinionated defaults (e.g. whitespace-insensitive diff for preview via `git diff -w`, see `internal/git`).

## Contributing

- **Commit messages**: **English**, imperative mood (e.g. `Add tree compaction for single-child dirs`).
- **Cleaning up existing history** before open-sourcing: rename or squash old commits with `git rebase -i --root`, or create a fresh repo with an initial English commit containing the current tree. Rewriting history changes SHAs—inform collaborators before force-pushing.

## License

Released under the [MIT License](./LICENSE).

## Acknowledgements

Built with [Charm](https://charm.sh/) libraries and [Chroma](https://github.com/alecthomas/chroma) for syntax highlighting.
