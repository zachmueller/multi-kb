package cmd

import (
	"fmt"

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
			pendingDir := route.DefaultPendingDir()

			// Check for pending notes first
			count, err := route.PendingCount(pendingDir)
			if err != nil {
				return fmt.Errorf("cannot check pending notes: %w", err)
			}
			if count == 0 {
				fmt.Println("No notes awaiting approval.")
				return nil
			}

			// Load config (needed for remote KB info)
			cfgPath := cfgFile
			if cfgPath == "" {
				cfgPath = config.DefaultConfigPath()
			}
			cfg, errs := config.Load(cfgPath)
			if len(errs) > 0 {
				// Config load failed — still allow approval of local KBs
				// by using an empty config
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: config load errors (remote KBs may not work):\n")
				for _, e := range errs {
					fmt.Fprintf(cmd.ErrOrStderr(), "  - %v\n", e)
				}
				cfg = &config.Config{}
			}

			fmt.Printf("Found %d pending note(s). Starting approval UI...\n", count)
			return approve.StartServer(pendingDir, cfg)
		},
	}
}
