package hook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/logging"
	"github.com/zmueller/multi-kb/internal/recall"
	"github.com/zmueller/multi-kb/internal/route"
)

// InjectResult is the output of the injection pipeline.
type InjectResult struct {
	Markdown string
	Output   string // harness-formatted (JSON for claude-code, raw for notor)
}

// RunInjection orchestrates the full recall injection pipeline.
// It queries all target KBs concurrently, merges results, and formats output.
func RunInjection(ctx context.Context, cfg *config.Config, query, sourceDir, harness string) (*InjectResult, error) {
	if query == "" {
		return &InjectResult{}, nil
	}

	// Find target KBs for this source directory
	var sourceEntry *config.Source
	for i := range cfg.Sources {
		if cfg.Sources[i].Directory == sourceDir {
			sourceEntry = &cfg.Sources[i]
			break
		}
	}

	if sourceEntry == nil {
		return &InjectResult{}, nil
	}

	// Build keyword client for local KB recall
	keywordClient, err := bedrock.NewClient(ctx,
		cfg.Extraction.AWSProfile,
		cfg.Extraction.AWSRegion,
		cfg.Translation.SummarizationModelID,
	)
	if err != nil {
		// Non-fatal: fall back to mechanical keywords
		keywordClient = nil
	}

	var keywords []string
	if keywordClient != nil {
		kws, err := recall.DeriveKeywords(ctx, keywordClient, query)
		if err != nil || len(kws) == 0 {
			keywords = recall.MechanicalKeywords(query)
		} else {
			keywords = kws
		}
	} else {
		// Mechanical fallback
		keywords = recall.MechanicalKeywords(query)
	}

	// Collect all target KBs (deduplicated)
	type kbTarget struct {
		name      string
		isLocal   bool
		kbName    string // for local: the local KB name; for remote: the config KB name
		kbConfig  *config.KnowledgeBase
	}

	var targets []kbTarget
	seen := make(map[string]bool)
	for _, t := range sourceEntry.Targets {
		if seen[t.KB] {
			continue
		}
		seen[t.KB] = true

		if len(t.KB) > 6 && t.KB[:6] == "local/" {
			targets = append(targets, kbTarget{name: t.KB, isLocal: true, kbName: t.KB[6:]})
		} else {
			for i := range cfg.KnowledgeBases {
				if cfg.KnowledgeBases[i].Name == t.KB {
					kb := &cfg.KnowledgeBases[i]
					targets = append(targets, kbTarget{name: t.KB, isLocal: false, kbConfig: kb})
					break
				}
			}
		}
	}

	// Query all KBs concurrently
	type kbResults struct {
		name    string
		results []recall.MergedResult
		err     error
	}

	resultsCh := make(chan kbResults, len(targets))
	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)
		go func(t kbTarget) {
			defer wg.Done()

			if t.isLocal {
				kbDir := filepath.Join(homeDir(), ".multi-kb", "local", t.kbName)
				local, err := recall.LocalRecall(kbDir, keywords)
				if err != nil {
					resultsCh <- kbResults{name: t.name, err: err}
					return
				}
				merged := recall.LocalResultsToMerged(local)
				for i := range merged {
					merged[i].SourceKB = t.name
				}
				resultsCh <- kbResults{name: t.name, results: merged}
			} else {
				remote, err := recall.RecallFromRemoteKB(ctx,
					t.kbConfig.Endpoint,
					t.kbConfig.Auth,
					t.kbConfig.AWSProfile,
					t.kbConfig.AWSRegion,
					query, 10,
				)
				if err != nil {
					resultsCh <- kbResults{name: t.name, err: err}
					return
				}
				merged := recall.RemoteResultsToMerged(remote)
				for i := range merged {
					merged[i].SourceKB = t.name
				}
				resultsCh <- kbResults{name: t.name, results: merged}
			}
		}(target)
	}

	wg.Wait()
	close(resultsCh)

	var allLists [][]recall.MergedResult
	var hasError bool
	for r := range resultsCh {
		if r.err != nil {
			hasError = true
			_ = logging.AppendHookError(logging.DefaultLogsDir(), logging.HookErrorEntry{
				Harness:   harness,
				Directory: sourceDir,
				Error:     fmt.Sprintf("KB %q: %v", r.name, r.err),
			})
			continue
		}
		if len(r.results) > 0 {
			allLists = append(allLists, r.results)
		}
	}

	merged := recall.InterleaveResults(allLists, 10)

	// Get pending count for notice
	pendingDir := filepath.Join(homeDir(), ".multi-kb", "pending")
	pendingCount, _ := route.PendingCount(pendingDir)

	markdown := recall.FormatInjection(merged, "", pendingCount)
	output := recall.FormatHookOutput(markdown, harness)

	_ = hasError // logged above; non-fatal
	return &InjectResult{Markdown: markdown, Output: output}, nil
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}
