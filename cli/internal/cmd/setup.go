package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/git"
	"github.com/zmueller/multi-kb/internal/hook"
	"github.com/zmueller/multi-kb/internal/schedule"
	"gopkg.in/yaml.v3"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard for first-time configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup()
		},
	}
}

func runSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Welcome to multi-kb setup!")
	fmt.Println("This wizard will configure conversation capture and knowledge routing.")

	// Phase 1: Harness selection
	harnesses, err := selectHarnesses(reader)
	if err != nil {
		return err
	}

	// Phase 1: Source directory discovery per harness
	var sources []config.Source
	var selectedDirs []dirHarnessPair

	for _, h := range harnesses {
		switch h {
		case "claude-code":
			fmt.Print("Enter Claude Code project directory path: ")
			dir, _ := reader.ReadString('\n')
			dir = strings.TrimSpace(dir)
			if dir == "" {
				fmt.Println("Skipping Claude Code (no directory provided).")
				continue
			}
			if err := validateDir(dir); err != nil {
				fmt.Printf("Warning: %v\n", err)
			}
			selectedDirs = append(selectedDirs, dirHarnessPair{dir: dir, harness: "claude-code"})

		case "notor":
			fmt.Print("Enter Notor vault path: ")
			dir, _ := reader.ReadString('\n')
			dir = strings.TrimSpace(dir)
			if dir == "" {
				fmt.Println("Skipping Notor (no path provided).")
				continue
			}
			if err := validateDir(dir); err != nil {
				fmt.Printf("Warning: %v\n", err)
			}
			selectedDirs = append(selectedDirs, dirHarnessPair{dir: dir, harness: "notor"})
		}
	}

	// Phase 2: KB configuration
	fmt.Println("\n--- Knowledge Base Configuration ---")

	// Create default local KB
	defaultKBDir, err := git.LocalKBDir("default")
	if err != nil {
		return fmt.Errorf("setup: resolve default KB dir: %w", err)
	}
	if err := git.InitRepo(defaultKBDir); err != nil {
		return fmt.Errorf("setup: init default KB: %w", err)
	}
	fmt.Println("Created default local KB at", defaultKBDir)

	// Remote KBs
	var kbs []config.KnowledgeBase
	for {
		fmt.Print("\nAdd a remote knowledge base? (y/n): ")
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			break
		}

		kb, err := promptRemoteKB(reader)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		kbs = append(kbs, *kb)
		fmt.Printf("Added remote KB %q.\n", kb.Name)
	}

	// Routing for each source directory
	for _, dh := range selectedDirs {
		targets := buildDefaultTargets(kbs)
		sources = append(sources, config.Source{
			Directory: dh.dir,
			Harnesses: []string{dh.harness},
			Targets:   targets,
		})
	}

	// Phase 3: Author, exclusion rules, and finalization
	fmt.Print("\nEnter your author identity (e.g., your name): ")
	author, _ := reader.ReadString('\n')
	author = strings.TrimSpace(author)
	if author == "" {
		author = "anonymous"
	}

	fmt.Print("Enter exclusion rules (comma-separated, or blank for none): ")
	exclusionLine, _ := reader.ReadString('\n')
	var exclusionRules []string
	for _, rule := range strings.Split(strings.TrimSpace(exclusionLine), ",") {
		rule = strings.TrimSpace(rule)
		if rule != "" {
			exclusionRules = append(exclusionRules, rule)
		}
	}

	// Build config
	cfg := config.Config{
		Mode:           "client",
		Author:         author,
		KnowledgeBases: kbs,
		Extraction: config.ExtractionConfig{
			ModelID: "anthropic.claude-sonnet-4-20250514",
		},
		Translation: config.TranslationConfig{
			SummarizationModelID: "anthropic.claude-haiku-4-5-20251001",
		},
		DreamCycle: config.DreamCycleConfig{
			ModelID: "anthropic.claude-sonnet-4-20250514",
		},
		Hook: config.HookConfig{
			Timeout: "8s",
		},
		ExclusionRules: exclusionRules,
		Sources:        sources,
	}

	// Prompt for extraction AWS config if remote KBs exist
	if len(kbs) > 0 {
		fmt.Print("AWS profile for Bedrock extraction (blank for default): ")
		profile, _ := reader.ReadString('\n')
		cfg.Extraction.AWSProfile = strings.TrimSpace(profile)

		fmt.Print("AWS region for Bedrock extraction (e.g., us-west-2): ")
		region, _ := reader.ReadString('\n')
		cfg.Extraction.AWSRegion = strings.TrimSpace(region)
	}

	// Write config.yaml
	cfgPath := config.DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("setup: create config directory: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("setup: marshal config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return fmt.Errorf("setup: write config: %w", err)
	}
	fmt.Printf("\nWrote config to %s\n", cfgPath)

	// Write initial state.yaml
	statePath := config.DefaultStatePath()
	state := &config.State{Directories: make(map[string]config.DirectoryState)}
	if err := config.SaveState(statePath, state); err != nil {
		return fmt.Errorf("setup: write state: %w", err)
	}

	// WIZ-003: Hook auto-registration
	for _, h := range harnesses {
		switch h {
		case "claude-code":
			if err := hook.RegisterClaudeCodeHook(); err != nil {
				fmt.Printf("Warning: could not register Claude Code hook: %v\n", err)
			} else {
				fmt.Println("Registered Claude Code hook.")
			}
		case "notor":
			for _, dh := range selectedDirs {
				if dh.harness == "notor" {
					if err := hook.RegisterNotorHook(dh.dir); err != nil {
						fmt.Printf("Warning: could not register Notor hook: %v\n", err)
					} else {
						fmt.Println("Registered Notor hook.")
					}
				}
			}
		}
	}

	// Cron registration
	fmt.Print("\nSchedule automatic capture? (y/n): ")
	cronAnswer, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(cronAnswer)) == "y" {
		fmt.Print("Cron expression (e.g., */30 * * * * for every 30 min): ")
		cronExpr, _ := reader.ReadString('\n')
		cronExpr = strings.TrimSpace(cronExpr)
		if cronExpr != "" {
			sched := schedule.NewScheduler()
			exePath, _ := os.Executable()
			if err := sched.Install(cronExpr, exePath, cfgPath); err != nil {
				fmt.Printf("Warning: could not register cron: %v\n", err)
			} else {
				fmt.Println("Scheduled automatic capture.")
			}
		}
	}

	fmt.Println("\nSetup complete! Run 'multi-kb run' to start capturing knowledge.")
	return nil
}

type dirHarnessPair struct {
	dir     string
	harness string
}

func selectHarnesses(reader *bufio.Reader) ([]string, error) {
	fmt.Println("Which AI coding assistants do you use?")
	fmt.Println("  1) Claude Code")
	fmt.Println("  2) Notor")
	fmt.Println("  3) Both")
	fmt.Print("Enter choice (1/2/3): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		return []string{"claude-code"}, nil
	case "2":
		return []string{"notor"}, nil
	case "3":
		return []string{"claude-code", "notor"}, nil
	default:
		return []string{"claude-code"}, nil
	}
}

func promptRemoteKB(reader *bufio.Reader) (*config.KnowledgeBase, error) {
	fmt.Print("  KB name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	fmt.Print("  Endpoint URL: ")
	endpoint, _ := reader.ReadString('\n')
	endpoint = strings.TrimSpace(endpoint)

	fmt.Print("  Auth type (iam/federate): ")
	auth, _ := reader.ReadString('\n')
	auth = strings.TrimSpace(strings.ToLower(auth))
	if auth != "iam" && auth != "federate" {
		auth = "iam"
	}

	var profile string
	if auth == "iam" {
		fmt.Print("  AWS profile: ")
		profile, _ = reader.ReadString('\n')
		profile = strings.TrimSpace(profile)
	}

	fmt.Print("  AWS region: ")
	region, _ := reader.ReadString('\n')
	region = strings.TrimSpace(region)

	fmt.Print("  Description: ")
	desc, _ := reader.ReadString('\n')
	desc = strings.TrimSpace(desc)

	return &config.KnowledgeBase{
		Name:       name,
		Endpoint:   endpoint,
		Auth:       auth,
		AWSProfile: profile,
		AWSRegion:  region,
		Desc:       desc,
	}, nil
}

func buildDefaultTargets(kbs []config.KnowledgeBase) []config.Target {
	targets := []config.Target{
		{KB: "local/default", Routing: "always", Approval: "auto-approve"},
	}
	for _, kb := range kbs {
		targets = append(targets, config.Target{
			KB:       kb.Name,
			Routing:  "consider",
			Approval: "require-manual-approval",
		})
	}
	return targets
}

func validateDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("directory %q does not exist", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	return nil
}
