package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/huh/v2"
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
	// Phase 1 outputs
	var selectedHarnesses []string
	var claudeCodeDir string
	var notorVaultPath string

	// Phase 1 Form: harness selection + source discovery
	phase1 := huh.NewForm(
		// Group 1: Welcome + harness selection
		huh.NewGroup(
			huh.NewNote().
				Title("multi-kb Setup").
				Description("Welcome! Let's configure your knowledge base pipeline.\n\n"+
					"This wizard will walk you through:\n"+
					"  1. Selecting your AI coding assistants\n"+
					"  2. Configuring knowledge bases and routing\n"+
					"  3. Setting your identity and preferences"),
			huh.NewMultiSelect[string]().
				Title("Which AI coding assistants do you use?").
				Options(
					huh.NewOption("Claude Code (CLI/IDE)", "claude-code"),
					huh.NewOption("Notor (Obsidian plugin)", "notor"),
				).
				Value(&selectedHarnesses).
				Validate(func(s []string) error {
					if len(s) == 0 {
						return fmt.Errorf("select at least one harness")
					}
					return nil
				}),
		),

		// Group 2: Claude Code directory (conditional)
		huh.NewGroup(
			huh.NewInput().
				Title("Claude Code: Project directory").
				Description("Enter a directory where you use Claude Code.\n"+
					"(You can add more directories later with `multi-kb add-source`.)").
				Placeholder("/Users/you/my-project").
				Value(&claudeCodeDir).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("directory path is required")
					}
					return validateDirPath(s)
				}),
		).WithHideFunc(func() bool {
			return !slices.Contains(selectedHarnesses, "claude-code")
		}),

		// Group 3: Notor vault path (conditional)
		huh.NewGroup(
			huh.NewInput().
				Title("Notor: Obsidian vault path").
				Description("Enter the path to your Obsidian vault where Notor is installed.").
				Placeholder("/Users/you/obsidian-vault").
				Value(&notorVaultPath).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("vault path is required")
					}
					return validateDirPath(s)
				}),
		).WithHideFunc(func() bool {
			return !slices.Contains(selectedHarnesses, "notor")
		}),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "")

	if err := phase1.Run(); err != nil {
		return fmt.Errorf("setup cancelled: %w", err)
	}

	// Auto-discovery summary
	var discoveryLines []string
	if slices.Contains(selectedHarnesses, "claude-code") && claudeCodeDir != "" {
		discoveryLines = append(discoveryLines, fmt.Sprintf("Claude Code: %s", claudeCodeDir))
	}
	if slices.Contains(selectedHarnesses, "notor") && notorVaultPath != "" {
		discoveryLines = append(discoveryLines, fmt.Sprintf("Notor vault: %s", notorVaultPath))
	}

	var discoveryConfirmed bool
	discoveryForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Discovered Sources").
				Description(strings.Join(discoveryLines, "\n")),
			huh.NewConfirm().
				Title("Continue with these sources?").
				Affirmative("Yes").
				Negative("No, cancel").
				Value(&discoveryConfirmed),
		),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "")

	if err := discoveryForm.Run(); err != nil {
		return fmt.Errorf("setup cancelled: %w", err)
	}
	if !discoveryConfirmed {
		fmt.Println("Setup cancelled.")
		return nil
	}

	// Create default local KB
	defaultKBDir, err := git.LocalKBDir("default")
	if err != nil {
		return fmt.Errorf("setup: resolve default KB dir: %w", err)
	}
	if err := git.InitRepo(defaultKBDir); err != nil {
		return fmt.Errorf("setup: init default KB: %w", err)
	}

	// Phase 2: Remote KB configuration via sequential forms ("Add another?" pattern)
	var kbs []config.KnowledgeBase
	for {
		var addRemoteKB bool
		addForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Add a remote knowledge base?").
					Description(fmt.Sprintf("You have %d remote KB(s) configured so far.", len(kbs))).
					Affirmative("Yes").
					Negative("No, continue").
					Value(&addRemoteKB),
			),
		).WithAccessible(os.Getenv("ACCESSIBLE") != "")

		if err := addForm.Run(); err != nil {
			break
		}
		if !addRemoteKB {
			break
		}

		kb, err := promptRemoteKBForm()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping: %v\n", err)
			continue
		}
		kbs = append(kbs, *kb)
	}

	// Phase 3: Author, exclusion rules, routing, and finalization
	var author string
	var exclusionText string
	var routingPreset string
	var cronExpr string
	var confirmed bool

	phase3 := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Author identity").
				Description("This name appears on all submitted knowledge notes.").
				Placeholder("your-name").
				Value(&author).
				Validate(huh.ValidateNotEmpty()).
				Validate(huh.ValidateMaxLength(100)),
			huh.NewText().
				Title("Exclusion rules").
				Description("Content matching these rules will never be sent to remote KBs.\nOne rule per line (optional).").
				Value(&exclusionText),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Approval mode for remote KBs").
				Options(
					huh.NewOption("Auto-approve all notes", "auto"),
					huh.NewOption("Require manual approval", "manual"),
					huh.NewOption("Auto-approve for local, manual for remote", "mixed"),
				).
				Value(&routingPreset),
		).WithHideFunc(func() bool {
			return len(kbs) == 0
		}),

		huh.NewGroup(
			huh.NewInput().
				Title("Cron schedule for automatic capture").
				Description("Standard 5-field cron expression, or leave blank to skip.\nExample: */30 * * * * (every 30 minutes)").
				Placeholder("*/30 * * * *").
				Value(&cronExpr),
		),

		huh.NewGroup(
			huh.NewNote().
				Title("Setup Summary").
				DescriptionFunc(func() string {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("Author: %s\n", author))
					sb.WriteString(fmt.Sprintf("Harnesses: %s\n", strings.Join(selectedHarnesses, ", ")))
					sb.WriteString(fmt.Sprintf("Local KB: default (at %s)\n", defaultKBDir))
					if len(kbs) > 0 {
						sb.WriteString(fmt.Sprintf("Remote KBs: %d\n", len(kbs)))
						for _, kb := range kbs {
							sb.WriteString(fmt.Sprintf("  - %s (%s)\n", kb.Name, kb.Auth))
						}
					}
					if cronExpr != "" {
						sb.WriteString(fmt.Sprintf("Schedule: %s\n", cronExpr))
					}
					return sb.String()
				}, &author),
			huh.NewConfirm().
				Title("Save this configuration?").
				Affirmative("Yes, save").
				Negative("Cancel").
				Value(&confirmed),
		),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "")

	if err := phase3.Run(); err != nil {
		return fmt.Errorf("setup cancelled: %w", err)
	}
	if !confirmed {
		fmt.Println("Setup cancelled.")
		return nil
	}

	exclusionRules := parseExclusionLines(exclusionText)

	// Build sources with routing targets
	var sources []config.Source
	if slices.Contains(selectedHarnesses, "claude-code") && claudeCodeDir != "" {
		sources = append(sources, config.Source{
			Directory: claudeCodeDir,
			Harnesses: []string{"claude-code"},
			Targets:   buildTargets(kbs, routingPreset),
		})
	}
	if slices.Contains(selectedHarnesses, "notor") && notorVaultPath != "" {
		sources = append(sources, config.Source{
			Directory: notorVaultPath,
			Harnesses: []string{"notor"},
			Targets:   buildTargets(kbs, routingPreset),
		})
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
	fmt.Printf("Wrote config to %s\n", cfgPath)

	// Write initial state.yaml
	statePath := config.DefaultStatePath()
	state := &config.State{Directories: make(map[string]config.DirectoryState)}
	if err := config.SaveState(statePath, state); err != nil {
		return fmt.Errorf("setup: write state: %w", err)
	}

	// WIZ-003: Hook auto-registration
	for _, h := range selectedHarnesses {
		switch h {
		case "claude-code":
			if err := hook.RegisterClaudeCodeHook(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not register Claude Code hook: %v\n", err)
			} else {
				fmt.Println("Registered Claude Code hook.")
			}
		case "notor":
			if notorVaultPath != "" {
				if err := hook.RegisterNotorHook(notorVaultPath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not register Notor hook: %v\n", err)
				} else {
					fmt.Println("Registered Notor hook.")
				}
			}
		}
	}

	// Cron registration
	cronExpr = strings.TrimSpace(cronExpr)
	if cronExpr != "" {
		sched := schedule.NewScheduler()
		exePath, _ := os.Executable()
		if err := sched.Install(cronExpr, exePath, cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not register cron: %v\n", err)
		} else {
			fmt.Println("Scheduled automatic capture.")
		}
	}

	fmt.Println("\nSetup complete! Run 'multi-kb run' to start capturing knowledge.")
	return nil
}

func promptRemoteKBForm() (*config.KnowledgeBase, error) {
	var name, endpoint, auth, profile, region, desc string

	kbForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("KB name").
				Description("A unique name to reference this knowledge base.").
				Value(&name).
				Validate(huh.ValidateNotEmpty()),
			huh.NewInput().
				Title("Endpoint URL").
				Description("The API Gateway endpoint for this KB.").
				Placeholder("https://xxx.execute-api.us-west-2.amazonaws.com/prod").
				Value(&endpoint).
				Validate(huh.ValidateNotEmpty()),
			huh.NewSelect[string]().
				Title("Auth type").
				Options(
					huh.NewOption("IAM (SigV4 signing with AWS profile)", "iam"),
					huh.NewOption("Federate (network-layer identity)", "federate"),
				).
				Value(&auth),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("AWS profile").
				Description("The named AWS SSO profile for SigV4 signing.").
				Value(&profile).
				Validate(huh.ValidateNotEmpty()),
		).WithHideFunc(func() bool {
			return auth != "iam"
		}),

		huh.NewGroup(
			huh.NewInput().
				Title("AWS region").
				Placeholder("us-west-2").
				Value(&region),
			huh.NewInput().
				Title("Description").
				Description("Brief description of what this KB stores (helps LLM routing).").
				Value(&desc),
		),
	).WithAccessible(os.Getenv("ACCESSIBLE") != "")

	if err := kbForm.Run(); err != nil {
		return nil, err
	}

	return &config.KnowledgeBase{
		Name:       name,
		Endpoint:   endpoint,
		Auth:       auth,
		AWSProfile: profile,
		AWSRegion:  region,
		Desc:       desc,
	}, nil
}

func buildTargets(kbs []config.KnowledgeBase, routingPreset string) []config.Target {
	targets := []config.Target{
		{KB: "local/default", Routing: "always", Approval: "auto-approve"},
	}
	for _, kb := range kbs {
		approval := "require-manual-approval"
		switch routingPreset {
		case "auto":
			approval = "auto-approve"
		case "mixed":
			approval = "require-manual-approval"
		}
		targets = append(targets, config.Target{
			KB:       kb.Name,
			Routing:  "consider",
			Approval: approval,
		})
	}
	return targets
}

func parseExclusionLines(text string) []string {
	var rules []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			rules = append(rules, line)
		}
	}
	return rules
}

func validateDirPath(s string) error {
	info, err := os.Stat(s)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", s)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", s)
	}
	return nil
}
