package route

import (
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/extract"
)

// RoutedNote is a note paired with its resolved routing targets.
type RoutedNote struct {
	Note      extract.Note
	Targets   []ResolvedTarget
}

// ResolvedTarget is a routing target with its approval mode resolved.
type ResolvedTarget struct {
	KB           string // KB name or "local/<name>"
	ApprovalMode string // "auto-approve" or "require-manual-approval"
}

// RouteNotes applies routing rules to extracted notes, resolving which KBs receive each note
// and whether each target requires approval.
//
// sourceDir is the source directory being processed.
// harness is the harness that produced the conversation (e.g. "claude-code", "notor").
// persona is the Notor persona name, if any (empty string for Claude Code).
func RouteNotes(
	cfg *config.Config,
	notes []extract.Note,
	sourceDir, harness, persona string,
) []RoutedNote {
	// Find the source config entry for this directory
	var sourceEntry *config.Source
	for i := range cfg.Sources {
		if cfg.Sources[i].Directory == sourceDir {
			sourceEntry = &cfg.Sources[i]
			break
		}
	}

	// Build a map of KB name → KnowledgeBase config for quick lookup
	kbIndex := make(map[string]config.KnowledgeBase, len(cfg.KnowledgeBases))
	for _, kb := range cfg.KnowledgeBases {
		kbIndex[kb.Name] = kb
	}

	var result []RoutedNote
	for _, note := range notes {
		targets := resolveTargets(cfg, sourceEntry, kbIndex, note, harness, persona)
		if len(targets) > 0 {
			result = append(result, RoutedNote{Note: note, Targets: targets})
		}
	}
	return result
}

// resolveTargets determines the final set of routing targets for a single note.
func resolveTargets(
	cfg *config.Config,
	src *config.Source,
	kbIndex map[string]config.KnowledgeBase,
	note extract.Note,
	harness, persona string,
) []ResolvedTarget {
	if src == nil {
		return []ResolvedTarget{{KB: "local/default", ApprovalMode: "auto-approve"}}
	}

	// Check for override: most specific first (harness+persona), then (harness only)
	baseTargets := src.Targets
	for _, override := range src.Overrides {
		if override.Harness == harness {
			if persona != "" && override.Persona == persona {
				baseTargets = override.Targets
				break
			}
			if override.Persona == "" {
				baseTargets = override.Targets
				// don't break — a more-specific harness+persona override may follow
			}
		}
	}

	// Build a set of suggested KB names from the LLM output
	suggestedSet := make(map[string]bool, len(note.SuggestedTargetKBs))
	for _, s := range note.SuggestedTargetKBs {
		suggestedSet[s] = true
	}

	var resolved []ResolvedTarget
	for _, target := range baseTargets {
		switch target.Routing {
		case "always":
			resolved = append(resolved, ResolvedTarget{
				KB:           target.KB,
				ApprovalMode: target.Approval,
			})
		case "consider":
			kbName := kbNameFromTarget(target.KB)
			if suggestedSet[kbName] {
				resolved = append(resolved, ResolvedTarget{
					KB:           target.KB,
					ApprovalMode: target.Approval,
				})
			}
		}
	}

	// Fallback: if no targets resolved, route to local/default with auto-approve
	if len(resolved) == 0 {
		resolved = append(resolved, ResolvedTarget{KB: "local/default", ApprovalMode: "auto-approve"})
	}

	return resolved
}

// kbNameFromTarget extracts the KB name from a target string.
// For "local/<name>" targets, returns just "<name>". For remote KB names, returns the name as-is.
func kbNameFromTarget(target string) string {
	if len(target) > 6 && target[:6] == "local/" {
		return target[6:]
	}
	return target
}
