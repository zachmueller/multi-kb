package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/hook"
	"github.com/zmueller/multi-kb/internal/logging"
)

func newHookCmd() *cobra.Command {
	var harness string

	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Hook entry point called by AI conversation harnesses",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			return runHook(cfgPath, harness)
		},
	}

	cmd.Flags().StringVar(&harness, "harness", "", "Harness type: claude-code or notor (required)")
	_ = cmd.MarkFlagRequired("harness")

	return cmd
}

type claudeCodeStdin struct {
	UserPrompt     string `json:"user_prompt"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
}

type notorStdin struct {
	FirstMessage   string `json:"first_message"`
	ConversationID string `json:"conversation_id"`
	Timestamp      string `json:"timestamp"`
}

func runHook(cfgPath, harness string) error {
	stdinBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0)
	}

	var query, sourceDir string

	switch harness {
	case "claude-code":
		var input claudeCodeStdin
		if err := json.Unmarshal(stdinBytes, &input); err != nil {
			os.Exit(0)
		}
		if strings.TrimSpace(input.UserPrompt) == "" {
			os.Exit(0)
		}
		if input.TranscriptPath != "" && isNotFirstMessage(input.TranscriptPath) {
			os.Exit(0)
		}
		query = input.UserPrompt
		sourceDir = input.CWD
		if sourceDir == "" {
			sourceDir = os.Getenv("CLAUDE_PROJECT_DIR")
		}

	case "notor":
		var input notorStdin
		if err := json.Unmarshal(stdinBytes, &input); err != nil {
			os.Exit(0)
		}
		if strings.TrimSpace(input.FirstMessage) == "" {
			os.Exit(0)
		}
		query = input.FirstMessage
		sourceDir, _ = os.Getwd()

	default:
		fmt.Fprintf(os.Stderr, "hook: unknown harness %q\n", harness)
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hook: load config: %v\n", err)
		os.Exit(1)
	}

	timeout := 8 * time.Second
	if cfg.Hook.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Hook.Timeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := hook.RunInjection(ctx, cfg, query, sourceDir, harness)
	if err != nil {
		_ = logging.AppendHookError(logging.DefaultLogsDir(), logging.HookErrorEntry{
			Harness:   harness,
			Directory: sourceDir,
			Error:     err.Error(),
		})
		os.Exit(1)
	}

	if result.Output != "" {
		fmt.Print(result.Output)
	}

	return nil
}

// isNotFirstMessage returns true if the transcript already has ≥1 user-type messages.
func isNotFirstMessage(transcriptPath string) bool {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var line map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line["type"] == "user" {
			return true
		}
	}
	return false
}
