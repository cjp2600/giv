package tui

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenFileInVSCode runs `code <absolute path>` for a path under repoRoot.
func OpenFileInVSCode(ctx context.Context, repoRoot, relPath string) error {
	if strings.TrimSpace(relPath) == "" || strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
		return fmt.Errorf("invalid path")
	}
	full := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	cmd := exec.CommandContext(ctx, "code", full)
	return cmd.Run()
}
