package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/logging"
	"github.com/zmueller/multi-kb/internal/route"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Display configuration summary, run history, and pending count",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	configPath := cfgFile
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, errs := config.Load(configPath)
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "Configuration errors:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  • %s\n", e)
		}
		fmt.Fprintln(os.Stderr, "\nRun 'multi-kb setup' to configure.")
		return nil
	}
	if cfg == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "No configuration found. Run 'multi-kb setup' to get started.")
		return nil
	}

	out := cmd.OutOrStdout()

	// Config summary
	fmt.Fprintln(out, "=== Configuration ===")
	fmt.Fprintf(out, "Mode:   %s\n", cfg.Mode)
	fmt.Fprintf(out, "Author: %s\n", cfg.Author)

	if len(cfg.KnowledgeBases) > 0 {
		fmt.Fprintf(out, "\nRemote Knowledge Bases (%d):\n", len(cfg.KnowledgeBases))
		for _, kb := range cfg.KnowledgeBases {
			fmt.Fprintf(out, "  • %s  (%s, %s)\n", kb.Name, kb.Auth, kb.AWSRegion)
		}
	}

	if len(cfg.Sources) > 0 {
		fmt.Fprintf(out, "\nTracked Directories (%d):\n", len(cfg.Sources))
		for _, src := range cfg.Sources {
			fmt.Fprintf(out, "  • %s  harnesses: %v\n", src.Directory, src.Harnesses)
		}
	}

	// Run history
	fmt.Fprintln(out, "\n=== Recent Runs ===")
	logsDir := logging.DefaultLogsDir()
	runs, err := logging.ReadRunLog(logsDir, 10)
	if err != nil || len(runs) == 0 {
		fmt.Fprintln(out, "No runs recorded yet.")
	} else {
		for _, r := range runs {
			status := "ok"
			if r.Errors > 0 {
				status = fmt.Sprintf("%d errors", r.Errors)
			}
			if r.Type == "capture" {
				fmt.Fprintf(out, "  [%s] %s  trigger=%-6s  conversations=%d  notes=%d  %s  (%dms)\n",
					r.Timestamp, r.Type, r.Trigger,
					r.ConversationsProcessed, r.NotesExtracted, status, r.DurationMS)
			} else {
				fmt.Fprintf(out, "  [%s] %s  trigger=%-6s  batches=%d  %s  (%dms)\n",
					r.Timestamp, r.Type, r.Trigger,
					r.BatchesProcessed, status, r.DurationMS)
			}
		}
	}

	// Pending approval count
	pendingDir := route.DefaultPendingDir()
	count, _ := route.PendingCount(pendingDir)
	if count > 0 {
		fmt.Fprintf(out, "\n⚠  %d note(s) awaiting approval. Run 'multi-kb approve' to review.\n", count)
	}

	// Next scheduled run (placeholder until cron parsing in WIZ-005)
	fmt.Fprintln(out, "\n=== Scheduled Run ===")
	fmt.Fprintln(out, "Next run: (run 'multi-kb setup' to configure cron schedule)")

	return nil
}
