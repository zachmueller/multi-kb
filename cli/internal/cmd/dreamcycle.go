package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDreamCycleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dream-cycle",
		Short: "Run the dream cycle to consolidate pending notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not implemented")
			return nil
		},
	}
}
