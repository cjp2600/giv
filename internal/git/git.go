// Package git wraps git subprocess calls for giv.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ChangedFile describes a single path reported by git status.
type ChangedFile struct {
	Path        string
	IsUntracked bool
	HasStaged   bool
	// PorcelainXY is the two-letter code from git status --porcelain (index / worktree columns).
	PorcelainXY string
}

// Snapshot is refreshed periodically for the repo root.
type Snapshot struct {
	Branch string
	Files  []ChangedFile
	Error  string
}

// RepoRoot returns absolute path of git work tree for dir (or dir itself if inside repo).
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := RunGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

// BranchName returns the current branch, "(detached)" in detached HEAD, or an error.
// For a repository with no commits yet, rev-parse --abbrev-ref HEAD fails; we use
// symbolic-ref (and then branch --show-current) so the name still resolves to the
// unborn branch (e.g. main/master) and status can be loaded.
func BranchName(ctx context.Context, repoRoot string) (string, error) {
	if out, err := RunGit(ctx, repoRoot, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		if b := strings.TrimSpace(out); b != "" {
			return b, nil
		}
	}
	out, err := RunGit(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		if out2, err2 := RunGit(ctx, repoRoot, "branch", "--show-current"); err2 == nil {
			if b := strings.TrimSpace(out2); b != "" {
				return b, nil
			}
		}
		return "", err
	}
	b := strings.TrimSpace(out)
	if b == "HEAD" {
		return "(detached)", nil
	}
	return b, nil
}

// SnapshotFromRepo gathers branch + porcelain status for changed paths.
func SnapshotFromRepo(ctx context.Context, repoRoot string) Snapshot {
	branch, err := BranchName(ctx, repoRoot)
	if err != nil {
		return Snapshot{Error: err.Error()}
	}
	// -uall: list each untracked file (not only parent directories).
	raw, err := RunGit(ctx, repoRoot, "status", "--porcelain", "-uall")
	if err != nil {
		return Snapshot{Branch: branch, Error: err.Error()}
	}
	files := parsePorcelain(raw)
	return Snapshot{Branch: branch, Files: files}
}

func parsePorcelain(raw string) []ChangedFile {
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	var out []ChangedFile
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line == "" {
			continue
		}
		if len(line) < 4 {
			continue
		}
		xy := line[0:2]
		rest := strings.TrimSpace(line[3:])

		// Rename / copy: "R  old -> new" on one line, or two-line format.
		if xy[0] == 'R' || xy[0] == 'C' {
			if strings.Contains(rest, " -> ") {
				parts := strings.Split(rest, " -> ")
				if len(parts) == 2 {
					out = append(out, fileFromXY(xy, parts[1]))
				}
				continue
			}
			if i+1 < len(lines) {
				nl := strings.TrimRight(lines[i+1], "\r")
				if len(nl) >= 4 {
					out = append(out, fileFromXY(xy, strings.TrimSpace(nl[3:])))
					i++
				}
			}
			continue
		}

		if rest == "" {
			continue
		}
		out = append(out, fileFromXY(xy, rest))
	}
	return dedupePreferStaged(out)
}

func fileFromXY(xy, path string) ChangedFile {
	if xy == "??" {
		return ChangedFile{Path: path, IsUntracked: true, HasStaged: false, PorcelainXY: xy}
	}
	idx := xy[0:1]
	hasStaged := idx != " " && idx != "?"
	return ChangedFile{Path: path, IsUntracked: false, HasStaged: hasStaged, PorcelainXY: xy}
}

func mergePorcelainXY(a, b string) string {
	if len(a) != 2 {
		return b
	}
	if len(b) != 2 {
		return a
	}
	pick := func(c1, c2 byte) byte {
		if c1 == 'D' || c2 == 'D' {
			return 'D'
		}
		if c1 != ' ' && c1 != '?' {
			return c1
		}
		if c2 != ' ' && c2 != '?' {
			return c2
		}
		return ' '
	}
	return string([]byte{pick(a[0], b[0]), pick(a[1], b[1])})
}

func dedupePreferStaged(files []ChangedFile) []ChangedFile {
	byPath := map[string]ChangedFile{}
	for _, f := range files {
		cur, ok := byPath[f.Path]
		if !ok {
			byPath[f.Path] = f
			continue
		}
		combined := cur
		combined.IsUntracked = cur.IsUntracked || f.IsUntracked
		combined.HasStaged = cur.HasStaged || f.HasStaged
		combined.PorcelainXY = mergePorcelainXY(cur.PorcelainXY, f.PorcelainXY)
		byPath[f.Path] = combined
	}
	out := make([]ChangedFile, 0, len(byPath))
	for _, f := range byPath {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// HasHEAD reports whether the repository has at least one commit (HEAD resolves).
func HasHEAD(ctx context.Context, repoRoot string) bool {
	_, err := RunGit(ctx, repoRoot, "rev-parse", "-q", "--verify", "HEAD")
	return err == nil
}

// DiffAgainstHEAD returns unified diff for path vs HEAD (needs at least one commit).
// Uses -w to hide whitespace-only noise from IDE formatting.
func DiffAgainstHEAD(ctx context.Context, repoRoot, path string) (string, error) {
	return RunGit(ctx, repoRoot, "diff", "-w", "HEAD", "--", path)
}

// DiffCached returns staged changes (index), used when there is no HEAD yet.
func DiffCached(ctx context.Context, repoRoot, path string) (string, error) {
	return RunGit(ctx, repoRoot, "diff", "-w", "--cached", "--", path)
}

// DiffWorktree returns unstaged changes: working tree vs index.
func DiffWorktree(ctx context.Context, repoRoot, path string) (string, error) {
	return RunGit(ctx, repoRoot, "diff", "-w", "--", path)
}

// DiffForPreview returns unified diff text for the preview pane.
// With -w: omit whitespace-only changes (see git diff -w).
// With HEAD: working tree vs last commit. Without commits: staged + unstaged chunks.
func DiffForPreview(ctx context.Context, repoRoot string, cf ChangedFile) (string, error) {
	if HasHEAD(ctx, repoRoot) {
		return DiffAgainstHEAD(ctx, repoRoot, cf.Path)
	}
	var chunks []string
	if cf.HasStaged {
		d, err := DiffCached(ctx, repoRoot, cf.Path)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(d) != "" {
			chunks = append(chunks, d)
		}
	}
	d2, err := DiffWorktree(ctx, repoRoot, cf.Path)
	if err != nil {
		if len(chunks) > 0 {
			return strings.Join(chunks, "\n"), nil
		}
		return "", err
	}
	if strings.TrimSpace(d2) != "" {
		chunks = append(chunks, d2)
	}
	return strings.Join(chunks, "\n"), nil
}

func validateRepoRelPath(relPath string) error {
	if strings.TrimSpace(relPath) == "" || strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
		return fmt.Errorf("invalid path")
	}
	return nil
}

// AddPath runs git add -- path (stages new paths and changes).
func AddPath(ctx context.Context, repoRoot, relPath string) error {
	if err := validateRepoRelPath(relPath); err != nil {
		return err
	}
	_, err := RunGit(ctx, repoRoot, "add", "--", relPath)
	return err
}

// RemoveUntrackedPath removes an untracked file from the working tree (git clean -f).
func RemoveUntrackedPath(ctx context.Context, repoRoot, relPath string) error {
	if err := validateRepoRelPath(relPath); err != nil {
		return err
	}
	_, err := RunGit(ctx, repoRoot, "clean", "-f", "--", relPath)
	return err
}

// CheckoutPathFromHEAD resets a tracked file to HEAD in the index and working tree.
func CheckoutPathFromHEAD(ctx context.Context, repoRoot, relPath string) error {
	if err := validateRepoRelPath(relPath); err != nil {
		return err
	}
	_, err := RunGit(ctx, repoRoot, "checkout", "HEAD", "--", relPath)
	return err
}

// RmCachedPath removes path from the index only (file stays on disk).
func RmCachedPath(ctx context.Context, repoRoot, relPath string) error {
	if err := validateRepoRelPath(relPath); err != nil {
		return err
	}
	_, err := RunGit(ctx, repoRoot, "rm", "--cached", "--", relPath)
	return err
}

// pathExistsInTree reports whether relPath exists at treeRef (e.g. HEAD).
// Avoids checkout when a newly staged file has never been committed (no blob at HEAD).
func pathExistsInTree(ctx context.Context, repoRoot, treeRef, relPath string) bool {
	p := filepath.ToSlash(strings.TrimPrefix(relPath, "./"))
	spec := treeRef + ":" + p
	_, err := RunGit(ctx, repoRoot, "rev-parse", "-q", "--verify", spec)
	return err == nil
}

// RevertFileSelection:
//   - untracked file — remove from working tree (git clean);
//   - tracked and path exists at HEAD — reset to last commit (checkout HEAD);
//   - tracked but not in HEAD (added then never committed) — rm --cached, keep file as untracked;
//   - tracked with no commits in repo — rm --cached only.
func RevertFileSelection(ctx context.Context, repoRoot string, cf ChangedFile) error {
	if err := validateRepoRelPath(cf.Path); err != nil {
		return err
	}
	if cf.IsUntracked {
		return RemoveUntrackedPath(ctx, repoRoot, cf.Path)
	}
	if !HasHEAD(ctx, repoRoot) {
		return RmCachedPath(ctx, repoRoot, cf.Path)
	}
	if pathExistsInTree(ctx, repoRoot, "HEAD", cf.Path) {
		return CheckoutPathFromHEAD(ctx, repoRoot, cf.Path)
	}
	return RmCachedPath(ctx, repoRoot, cf.Path)
}

// PushCurrentBranch runs git push for the current branch (upstream from remote config).
func PushCurrentBranch(ctx context.Context, repoRoot string) error {
	_, err := RunGit(ctx, repoRoot, "push")
	return err
}

// CreateBranchFromMain fetches origin, checks out main (or master), pulls latest,
// then creates and switches to a new branch.
func CreateBranchFromMain(ctx context.Context, repoRoot, branchName string) error {
	if strings.TrimSpace(branchName) == "" {
		return fmt.Errorf("empty branch name")
	}

	// Fetch latest from origin.
	if _, err := RunGit(ctx, repoRoot, "fetch", "origin"); err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	// Determine main branch name (main or master).
	mainBranch := "main"
	if _, err := RunGit(ctx, repoRoot, "rev-parse", "--verify", "origin/main"); err != nil {
		if _, err2 := RunGit(ctx, repoRoot, "rev-parse", "--verify", "origin/master"); err2 != nil {
			return fmt.Errorf("cannot find origin/main or origin/master")
		}
		mainBranch = "master"
	}

	// Switch to main branch.
	if _, err := RunGit(ctx, repoRoot, "checkout", mainBranch); err != nil {
		return fmt.Errorf("checkout %s: %w", mainBranch, err)
	}

	// Pull latest.
	if _, err := RunGit(ctx, repoRoot, "pull"); err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Create and switch to new branch.
	if _, err := RunGit(ctx, repoRoot, "checkout", "-b", branchName); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	return nil
}

// CommitWorkingTree runs git commit -a -m message (tracked paths only; git add new files first).
func CommitWorkingTree(ctx context.Context, repoRoot, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("empty commit message")
	}
	_, err := RunGit(ctx, repoRoot, "commit", "-a", "-m", message)
	return err
}

// ReadWorktreeFile reads bytes from the working tree.
func ReadWorktreeFile(repoRoot, relPath string) ([]byte, error) {
	if strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("invalid path")
	}
	full := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	return os.ReadFile(full)
}
