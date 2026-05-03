package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/approve"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/route"
)

func newApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve",
		Short: "Launch the approval web UI to review pending notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := cfgFile
			if cfgPath == "" {
				cfgPath = config.DefaultConfigPath()
			}
			return execApprove(cfgPath, route.DefaultPendingDir(),
				approve.StartServer, cmd.ErrOrStderr(), os.Stdout)
		},
	}
}

// startServerFn is the function signature for starting the approval server.
// Injectable for testing.
type startServerFn func(pendingDir string, cfg *config.Config) error

// execApprove is the testable core of the approve command.
func execApprove(cfgPath, pendingDir string, startServer startServerFn, stderr, stdout io.Writer) error {
	// Check for pending notes first
	count, err := route.PendingCount(pendingDir)
	if err != nil {
		return fmt.Errorf("cannot check pending notes: %w", err)
	}
	if count == 0 {
		fmt.Fprintln(stdout, "No notes awaiting approval.")
		return nil
	}

	// Load config (needed for remote KB info)
	cfg, errs := config.Load(cfgPath)
	if len(errs) > 0 {
		// Config load failed — still allow approval of local KBs
		fmt.Fprintf(stderr, "Warning: config load errors (remote KBs may not work):\n")
		for _, e := range errs {
			fmt.Fprintf(stderr, "  - %v\n", e)
		}
		cfg = &config.Config{}
	}

	fmt.Fprintf(stdout, "Found %d pending note(s). Starting approval UI...\n", count)
	return startServer(pendingDir, cfg)
}
