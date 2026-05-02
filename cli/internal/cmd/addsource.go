package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAddSourceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-source",
		Short: "Add a new conversation source directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
