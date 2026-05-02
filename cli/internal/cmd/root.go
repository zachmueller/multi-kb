package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
	version = "dev"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "multi-kb",
		Short:   "Multi-KB — capture, consolidate, and recall knowledge across AI conversations",
		Version: version,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.multi-kb/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")

	rootCmd.AddCommand(
		newSetupCmd(),
		newRunCmd(),
		newProcessCmd(),
		newDreamCycleCmd(),
		newApproveCmd(),
		newStatusCmd(),
		newAddSourceCmd(),
		newAddKbCmd(),
		newHookCmd(),
		newServerCmd(),
	)

	return rootCmd
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
