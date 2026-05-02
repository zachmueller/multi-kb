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

func newDreamCycleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dream-cycle",
		Short: "Run the dream cycle to consolidate pending notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")

			cfg, errs := config.Load(cfgPath)
			if len(errs) > 0 {
				return fmt.Errorf("dream-cycle: load config: %w", errs[0])
			}

			lockPath := lock.DefaultLockPath()
			logsDir := logging.DefaultLogsDir()

			err := dreamcycle.RunDreamCycle(cmd.Context(), cfg, lockPath, logsDir, "manual")
			if err != nil {
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
