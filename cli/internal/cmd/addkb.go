package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAddKbCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-kb",
		Short: "Add a new knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
