package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/server"
)

func newServerCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run in server mode (EC2 deployment)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				configPath = "/opt/multi-kb/config.yaml"
			}

			cfg, errs := config.Load(configPath)
			if len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(cmd.ErrOrStderr(), "config error: %v\n", e)
				}
				return fmt.Errorf("server config validation failed (%d errors)", len(errs))
			}

			if cfg.Mode != "server" {
				return fmt.Errorf("config mode is %q, expected \"server\"", cfg.Mode)
			}

			return server.RunServer(context.Background(), cfg)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "path to server config file (default /opt/multi-kb/config.yaml)")

	return cmd
}
