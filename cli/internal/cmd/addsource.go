package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"gopkg.in/yaml.v3"
)

func newAddSourceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-source",
		Short: "Add a new conversation source directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			return runAddSource(cfgPath)
		},
	}
}

func runAddSource(cfgPath string) error {
	cfg, errs := config.Load(cfgPath)
	if len(errs) > 0 {
		return fmt.Errorf("add-source: load config: %w (run 'multi-kb setup' first)", errs[0])
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Directory path: ")
	dir, _ := reader.ReadString('\n')
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("directory path is required")
	}

	fmt.Println("Harness (1=claude-code, 2=notor, 3=both):")
	hChoice, _ := reader.ReadString('\n')
	var harnesses []string
	switch strings.TrimSpace(hChoice) {
	case "1":
		harnesses = []string{"claude-code"}
	case "2":
		harnesses = []string{"notor"}
	case "3":
		harnesses = []string{"claude-code", "notor"}
	default:
		harnesses = []string{"claude-code"}
	}

	// Build targets: default local KB always, plus any remote KBs as consider
	targets := []config.Target{
		{KB: "local/default", Routing: "always", Approval: "auto-approve"},
	}
	for _, kb := range cfg.KnowledgeBases {
		fmt.Printf("Route to remote KB %q? (y/n): ", kb.Name)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) == "y" {
			fmt.Printf("  Routing mode for %q (always/consider): ", kb.Name)
			mode, _ := reader.ReadString('\n')
			mode = strings.TrimSpace(strings.ToLower(mode))
			if mode != "always" {
				mode = "consider"
			}
			fmt.Printf("  Approval mode (auto-approve/require-manual-approval): ")
			approval, _ := reader.ReadString('\n')
			approval = strings.TrimSpace(strings.ToLower(approval))
			if approval != "auto-approve" {
				approval = "require-manual-approval"
			}
			targets = append(targets, config.Target{KB: kb.Name, Routing: mode, Approval: approval})
		}
	}

	cfg.Sources = append(cfg.Sources, config.Source{
		Directory: dir,
		Harnesses: harnesses,
		Targets:   targets,
	})

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("add-source: marshal config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return fmt.Errorf("add-source: write config: %w", err)
	}

	fmt.Printf("Added source directory %q.\n", dir)
	return nil
}
