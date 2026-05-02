package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newProcessCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "process",
		Short: "Scan conversations, extract knowledge, and route to KBs",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
