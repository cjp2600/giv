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
	Branch     string
	Files      []ChangedFile
	Error      string
	AheadCount int // commits ahead of upstream (unpushed)
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
	ahead := countAhead(ctx, repoRoot)
	return Snapshot{Branch: branch, Files: files, AheadCount: ahead}
}

// countAhead returns the number of commits on the current branch that are
// not yet pushed to the upstream tracking branch.  Returns 0 when there is
// no upstream or on any error (safe default — no indicator shown).
func countAhead(ctx context.Context, repoRoot string) int {
	out, err := RunGit(ctx, repoRoot, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return 0
	}
	n := 0
	fmt.Sscanf(strings.TrimSpace(out), "%d", &n)
	return n
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

// diffArgs returns extra flags for git diff: --text when the path is a known
// source file (overrides gitattributes -diff that marks generated code as binary).
func diffTextFlag(path string) []string {
	if IsKnownTextPath(path) {
		return []string{"--text"}
	}
	return nil
}

// DiffAgainstHEAD returns unified diff for path vs HEAD (needs at least one commit).
// Uses -w to hide whitespace-only noise from IDE formatting.
func DiffAgainstHEAD(ctx context.Context, repoRoot, path string) (string, error) {
	args := append([]string{"diff", "-w"}, diffTextFlag(path)...)
	args = append(args, "HEAD", "--", path)
	return RunGit(ctx, repoRoot, args...)
}

// DiffCached returns staged changes (index), used when there is no HEAD yet.
func DiffCached(ctx context.Context, repoRoot, path string) (string, error) {
	args := append([]string{"diff", "-w"}, diffTextFlag(path)...)
	args = append(args, "--cached", "--", path)
	return RunGit(ctx, repoRoot, args...)
}

// DiffWorktree returns unstaged changes: working tree vs index.
func DiffWorktree(ctx context.Context, repoRoot, path string) (string, error) {
	args := append([]string{"diff", "-w"}, diffTextFlag(path)...)
	args = append(args, "--", path)
	return RunGit(ctx, repoRoot, args...)
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

// RunGitLong is like RunGit but uses the caller's context timeout instead of 8s.
func RunGitLong(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
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

// PushCurrentBranch runs git push for the current branch, setting upstream if needed.
func PushCurrentBranch(ctx context.Context, repoRoot string) error {
	branch, err := BranchName(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("cannot determine branch: %w", err)
	}
	// Try normal push first.
	_, err = RunGitLong(ctx, repoRoot, "push")
	if err != nil {
		// If no upstream, set it automatically.
		errMsg := err.Error()
		if strings.Contains(errMsg, "no upstream") || strings.Contains(errMsg, "has no upstream") ||
			strings.Contains(errMsg, "set-upstream") {
			_, err = RunGitLong(ctx, repoRoot, "push", "-u", "origin", branch)
		}
	}
	return err
}

// LastCommitOneLiner returns a short one-line summary of the last commit (hash + subject).
func LastCommitOneLiner(ctx context.Context, repoRoot string) string {
	out, err := RunGit(ctx, repoRoot, "log", "-1", "--format=%h %s")
	if err != nil {
		return "(no commits)"
	}
	return strings.TrimSpace(out)
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

// FetchAll runs git fetch to update all remote-tracking branches.
func FetchAll(ctx context.Context, repoRoot string) error {
	_, err := RunGitLong(ctx, repoRoot, "fetch", "--all")
	return err
}

// CheckoutBranch switches to the given branch.
func CheckoutBranch(ctx context.Context, repoRoot, branch string) error {
	_, err := RunGit(ctx, repoRoot, "checkout", branch)
	return err
}

// MergeBase returns the best common ancestor between two refs.
func MergeBase(ctx context.Context, repoRoot, ref1, ref2 string) (string, error) {
	out, err := RunGit(ctx, repoRoot, "merge-base", ref1, ref2)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// FindBranchBase finds the merge-base of the given branch against main/master.
// Returns the merge-base commit hash and the main branch name used.
func FindBranchBase(ctx context.Context, repoRoot, branch string) (base string, mainBranch string, err error) {
	// Try origin/main, then origin/master, then local main, then local master.
	candidates := []string{"origin/main", "origin/master", "main", "master"}
	for _, c := range candidates {
		if _, verr := RunGit(ctx, repoRoot, "rev-parse", "--verify", c); verr != nil {
			continue
		}
		mb, merr := MergeBase(ctx, repoRoot, c, branch)
		if merr == nil && mb != "" {
			return mb, c, nil
		}
	}
	return "", "", fmt.Errorf("cannot find merge-base for branch %s against main/master", branch)
}

// ChangedFilesBetweenRefs returns the list of changed files between two refs.
func ChangedFilesBetweenRefs(ctx context.Context, repoRoot, fromRef, toRef string) ([]ChangedFile, error) {
	raw, err := RunGit(ctx, repoRoot, "diff", "--name-status", fromRef, toRef)
	if err != nil {
		return nil, err
	}
	return parseNameStatus(raw), nil
}

// parseNameStatus parses "git diff --name-status" output into ChangedFile list.
func parseNameStatus(raw string) []ChangedFile {
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	var out []ChangedFile
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			// Try space-separated (some git versions)
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			parts = fields
		}
		status := parts[0]
		path := parts[len(parts)-1] // for renames, take the new name
		cf := ChangedFile{
			Path:        path,
			IsUntracked: false,
			HasStaged:   false,
		}
		switch {
		case strings.HasPrefix(status, "A"):
			cf.PorcelainXY = "A "
			cf.HasStaged = true // renders as "S" badge (green) for new files
		case strings.HasPrefix(status, "D"):
			cf.PorcelainXY = "D "
		case strings.HasPrefix(status, "R"):
			cf.PorcelainXY = "M "
		default:
			cf.PorcelainXY = "M "
		}
		out = append(out, cf)
	}
	return out
}

// DiffBetweenRefs returns unified diff for a specific file between two refs.
func DiffBetweenRefs(ctx context.Context, repoRoot, fromRef, toRef, path string) (string, error) {
	args := append([]string{"diff", "-w"}, diffTextFlag(path)...)
	args = append(args, fromRef, toRef, "--", path)
	return RunGit(ctx, repoRoot, args...)
}

// ShowFileAtRef reads file content at a specific git ref (commit/branch).
func ShowFileAtRef(ctx context.Context, repoRoot, ref, path string) ([]byte, error) {
	spec := ref + ":" + filepath.ToSlash(path)
	out, err := RunGit(ctx, repoRoot, "show", spec)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
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

// binarySniffSize is the number of bytes inspected for NUL to decide
// whether a file is binary (same heuristic git uses).
const binarySniffSize = 8000

// knownBinaryExts lists extensions that are always treated as binary,
// regardless of content.  Lowercase, with leading dot.
var knownBinaryExts = map[string]bool{
	// images
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".ico": true, ".webp": true, ".tiff": true,
	".tif": true, ".svg": false, // SVG is text
	// audio / video
	".mp3": true, ".mp4": true, ".wav": true, ".ogg": true,
	".flac": true, ".avi": true, ".mkv": true, ".mov": true,
	".webm": true,
	// archives
	".zip": true, ".tar": true, ".gz": true, ".bz2": true,
	".xz": true, ".zst": true, ".7z": true, ".rar": true,
	// compiled / object
	".o": true, ".a": true, ".so": true, ".dylib": true,
	".dll": true, ".exe": true, ".class": true, ".pyc": true,
	".pyo": true, ".wasm": true,
	// documents
	".pdf": true, ".doc": true, ".docx": true, ".xls": true,
	".xlsx": true, ".ppt": true, ".pptx": true,
	// fonts
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
	".eot": true,
	// databases
	".db": true, ".sqlite": true, ".sqlite3": true,
	// misc binary
	".bin": true, ".dat": true, ".pak": true, ".DS_Store": true,
}

// knownTextExts lists extensions that are always treated as text,
// overriding gitattributes -diff or binary detection heuristics.
var knownTextExts = map[string]bool{
	// Go
	".go": true,
	// Proto
	".proto": true,
	// Web
	".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".mjs": true, ".cjs": true, ".mts": true, ".cts": true,
	".vue": true, ".svelte": true,
	".html": true, ".htm": true, ".css": true, ".scss": true,
	".less": true, ".sass": true,
	// Config / data
	".json": true, ".yaml": true, ".yml": true, ".toml": true,
	".xml": true, ".csv": true, ".tsv": true, ".env": true,
	".ini": true, ".cfg": true, ".conf": true,
	// Scripting
	".py": true, ".rb": true, ".pl": true, ".pm": true,
	".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".lua": true, ".php": true, ".r": true,
	// Systems
	".c": true, ".h": true, ".cpp": true, ".hpp": true,
	".cc": true, ".hh": true, ".cxx": true, ".hxx": true,
	".m": true, ".mm": true, ".swift": true,
	".rs": true, ".zig": true, ".nim": true,
	// JVM
	".java": true, ".kt": true, ".kts": true, ".scala": true,
	".groovy": true, ".gradle": true, ".clj": true,
	// .NET
	".cs": true, ".fs": true, ".vb": true, ".csproj": true,
	// Markup / docs
	".md": true, ".rst": true, ".txt": true, ".tex": true,
	".adoc": true, ".org": true,
	// Build / CI
	".make": true, ".cmake": true, ".dockerfile": true,
	".tf": true, ".hcl": true, ".nix": true,
	// SQL
	".sql": true,
	// Other
	".graphql": true, ".gql": true, ".thrift": true,
	".avsc": true, ".fbs": true,
	".dart": true, ".elm": true, ".ex": true, ".exs": true,
	".erl": true, ".hrl": true, ".hs": true,
}

// IsKnownTextPath returns true if the file extension is a well-known source / text format.
// Such files are always diffed as text regardless of gitattributes.
func IsKnownTextPath(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	return knownTextExts[ext]
}

// IsBinaryPath returns true if the file extension is a well-known binary format.
func IsBinaryPath(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	return knownBinaryExts[ext]
}

// IsBinaryContent inspects the raw bytes and returns true when the data
// looks like a binary blob rather than human-readable text.
//
// Detection layers (in order):
//  1. NUL byte in the first 8 KB — the same heuristic git uses.
//  2. Common binary magic signatures (ELF, Mach-O, PE, gzip, PK/zip, etc.).
func IsBinaryContent(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// 1. NUL scan (same as git: first 8000 bytes).
	sniff := data
	if len(sniff) > binarySniffSize {
		sniff = sniff[:binarySniffSize]
	}
	if bytes.ContainsRune(sniff, 0) {
		return true
	}

	// 2. Magic-number signatures for common binary formats.
	if matchesBinaryMagic(data) {
		return true
	}

	return false
}

// matchesBinaryMagic checks leading bytes against well-known signatures.
func matchesBinaryMagic(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// ELF
	if data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
		return true
	}
	// Mach-O (32/64, big/little)
	m := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	switch m {
	case 0xFEEDFACE, 0xFEEDFACF, 0xCEFAEDFE, 0xCFFAEDFE, 0xCAFEBABE:
		return true
	}
	// PE (MZ)
	if data[0] == 'M' && data[1] == 'Z' {
		return true
	}
	// gzip
	if data[0] == 0x1F && data[1] == 0x8B {
		return true
	}
	// PK (zip)
	if data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04 {
		return true
	}
	// PDF (%PDF)
	if data[0] == '%' && data[1] == 'P' && data[2] == 'D' && data[3] == 'F' {
		return true
	}
	return false
}
