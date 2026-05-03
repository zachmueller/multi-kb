package dreamcycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/lock"
	"github.com/zmueller/multi-kb/internal/logging"
)

// storeFactory creates a NoteStore for a given KB directory.
// Injected in tests to avoid filesystem operations.
type storeFactory func(kbDir string) NoteStore

// RunDreamCycle executes the full dream cycle (phases 0-4) for all local KBs.
// Phase 0: no-op for local mode.
// Phase 1: find pending notes, create singleton batches.
// Phase 2: find related active notes via git grep.
// Phase 3: LLM consolidation + action application.
// Phase 4: no-op for local mode.
func RunDreamCycle(ctx context.Context, cfg *config.Config, lockPath, logsDir string, trigger string) error {
	bedrockClient, err := bedrock.NewClient(ctx,
		cfg.Extraction.AWSProfile,
		cfg.Extraction.AWSRegion,
		cfg.DreamCycle.ModelID,
	)
	if err != nil {
		return fmt.Errorf("dream-cycle: create Bedrock client: %w", err)
	}

	sf := func(kbDir string) NoteStore { return &localNoteStore{kbDir: kbDir} }
	return runDreamCycle(ctx, cfg, lockPath, logsDir, trigger, bedrockClient, sf)
}

// runDreamCycle is the testable core: it accepts injectable client and store factory.
func runDreamCycle(ctx context.Context, cfg *config.Config, lockPath, logsDir string, trigger string, client llmInvoker, sf storeFactory) error {
	start := time.Now()

	l, err := lock.Acquire(lockPath, "dream_cycle")
	if err != nil {
		return fmt.Errorf("dream-cycle: %w", err)
	}
	defer l.Release()

	var (
		batchesProcessed int
		actions          = map[string]int{"keep": 0, "merge": 0, "split": 0, "consolidate": 0}
		errorCount       int
	)

	// Collect unique local KBs to process
	seen := make(map[string]bool)
	for _, source := range cfg.Sources {
		for _, target := range source.Targets {
			if !isLocalKB(target.KB) || seen[target.KB] {
				continue
			}
			seen[target.KB] = true

			kbName := target.KB[len("local/"):]
			kbDir, err := localKBDir(kbName)
			if err != nil {
				errorCount++
				continue
			}

			store := sf(kbDir)

			// Phase 1: find pending notes and create singleton batches
			batches, err := CreateBatches(kbDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dream-cycle: phase 1 error for KB %q: %v\n", kbName, err)
				errorCount++
				continue
			}
			if len(batches) == 0 {
				continue
			}

			// Phase 2: find related active notes for each batch
			for i := range batches {
				related, err := FindRelatedNotes(kbDir, batches[i])
				if err != nil {
					fmt.Fprintf(os.Stderr, "dream-cycle: phase 2 error for batch: %v\n", err)
					errorCount++
					continue
				}
				batches[i].RelatedNotes = related
			}

			// Phase 3: LLM consolidation + action application per batch
			for _, batch := range batches {
				batchesProcessed++
				batchActions, err := ConsolidateBatch(ctx, client, store, batch)
				if err != nil {
					fmt.Fprintf(os.Stderr, "dream-cycle: phase 3 error for batch: %v\n", err)
					errorCount++
					continue
				}
				for k, v := range batchActions {
					actions[k] += v
				}
			}
		}
	}

	_ = logging.AppendRunLog(logsDir, logging.RunEntry{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Type:             "dream_cycle",
		Trigger:          trigger,
		BatchesProcessed: batchesProcessed,
		Actions:          actions,
		Errors:           errorCount,
		DurationMS:       time.Since(start).Milliseconds(),
	})

	statePath := config.DefaultStatePath()
	state, _ := config.LoadState(statePath)
	if state != nil {
		state.LastDreamCycle = time.Now().UTC().Format(time.RFC3339)
		_ = config.SaveState(statePath, state)
	}

	return nil
}

func isLocalKB(kb string) bool {
	return len(kb) > 6 && kb[:6] == "local/"
}

func localKBDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("dream-cycle: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".multi-kb", "local", name), nil
}
