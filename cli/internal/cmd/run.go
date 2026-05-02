package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/dreamcycle"
	"github.com/zmueller/multi-kb/internal/lock"
	"github.com/zmueller/multi-kb/internal/logging"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run capture processing followed by dream cycle",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			ctx := cmd.Context()

			// Run capture processing first (acquires and releases its own lock)
			if err := runProcess(ctx, cfgPath, "manual"); err != nil {
				if !errors.Is(err, lock.ErrLockHeld) {
					return err
				}
				fmt.Println("Another multi-kb process is running. Skipping.")
				return nil
			}

			// Then run dream cycle (acquires its own lock)
			cfg, errs := config.Load(cfgPath)
			if len(errs) > 0 {
				return fmt.Errorf("run: load config for dream cycle: %w", errs[0])
			}

			lockPath := lock.DefaultLockPath()
			logsDir := logging.DefaultLogsDir()

			if err := dreamcycle.RunDreamCycle(ctx, cfg, lockPath, logsDir, "manual"); err != nil {
				if errors.Is(err, lock.ErrLockHeld) {
					fmt.Println("Another multi-kb process is running. Skipping dream cycle.")
					return nil
				}
				return err
			}

			return nil
		},
	}
}
