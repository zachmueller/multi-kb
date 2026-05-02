package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve",
		Short: "Launch the approval web UI to review pending notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
