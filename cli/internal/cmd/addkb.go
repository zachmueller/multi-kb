package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"gopkg.in/yaml.v3"
)

func newAddKbCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-kb",
		Short: "Add a new knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			return runAddKB(cfgPath)
		},
	}
}

func runAddKB(cfgPath string) error {
	return runAddKBFrom(cfgPath, os.Stdin)
}

func runAddKBFrom(cfgPath string, stdin io.Reader) error {
	cfg, errs := config.Load(cfgPath)
	if len(errs) > 0 {
		return fmt.Errorf("add-kb: load config: %w (run 'multi-kb setup' first)", errs[0])
	}

	reader := bufio.NewReader(stdin)

	fmt.Print("KB name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("KB name is required")
	}

	// Check for duplicates
	for _, kb := range cfg.KnowledgeBases {
		if kb.Name == name {
			return fmt.Errorf("KB %q already exists in config", name)
		}
	}

	fmt.Print("Endpoint URL: ")
	endpoint, _ := reader.ReadString('\n')
	endpoint = strings.TrimSpace(endpoint)

	fmt.Print("Auth type (iam/federate): ")
	auth, _ := reader.ReadString('\n')
	auth = strings.TrimSpace(strings.ToLower(auth))
	if auth != "iam" && auth != "federate" {
		auth = "iam"
	}

	var profile string
	if auth == "iam" {
		fmt.Print("AWS profile: ")
		profile, _ = reader.ReadString('\n')
		profile = strings.TrimSpace(profile)
	}

	fmt.Print("AWS region: ")
	region, _ := reader.ReadString('\n')
	region = strings.TrimSpace(region)

	fmt.Print("Description: ")
	desc, _ := reader.ReadString('\n')
	desc = strings.TrimSpace(desc)

	cfg.KnowledgeBases = append(cfg.KnowledgeBases, config.KnowledgeBase{
		Name:       name,
		Endpoint:   endpoint,
		Auth:       auth,
		AWSProfile: profile,
		AWSRegion:  region,
		Desc:       desc,
	})

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("add-kb: marshal config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return fmt.Errorf("add-kb: write config: %w", err)
	}

	fmt.Printf("Added KB %q.\n", name)
	return nil
}
