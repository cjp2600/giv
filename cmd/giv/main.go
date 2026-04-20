package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	gogit "github.com/cjp2600/giv/internal/git"
	"github.com/cjp2600/giv/internal/tui"
)

var (
	flagHotkey bool
	flagReview string
)

var rootCmd = &cobra.Command{
	Use:   "giv",
	Short: "Interactive git change viewer with syntax highlighting and diffs",
	Long: `Run from a repository root: two panes—a navigable tree of changed files and a smart,
syntax-highlighted preview / diff.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagHotkey {
			tui.PrintHotkeyHelp(os.Stdout)
			return nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		ctx := context.Background()
		root, err := gogit.RepoRoot(ctx, cwd)
		if err != nil {
			return fmt.Errorf("not a git repository (or git unavailable): %w", err)
		}

		p := tea.NewProgram(
			tui.New(root, 100, 30, flagReview),
			tea.WithAltScreen(),
			// AllMotion emits an event on every pixel move → extra full redraws and
			// visible background artifacts in the preview (ANSI/BCE). CellMotion:
			// clicks, wheel, drag — enough for us.
			tea.WithMouseCellMotion(),
		)
		_, err = p.Run()
		return err
	},
}

func init() {
	rootCmd.Flags().BoolVar(&flagHotkey, "hotkey", false, "Print keyboard shortcuts and exit")
	rootCmd.Flags().StringVar(&flagReview, "review", "", "Review changes from a branch (compared to main)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
