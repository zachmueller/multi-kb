package route

import (
	"testing"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/extract"
)

func buildTestConfig(sources []config.Source, kbs []config.KnowledgeBase) *config.Config {
	return &config.Config{
		Sources:        sources,
		KnowledgeBases: kbs,
	}
}

func TestAlwaysRouting(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets: []config.Target{
				{KB: "teamKB", Routing: "always", Approval: "auto-approve"},
			},
		}},
		[]config.KnowledgeBase{{Name: "teamKB"}},
	)

	notes := []extract.Note{{
		Title:              "Always Note",
		Content:            "Content",
		SuggestedTargetKBs: []string{"otherKB"}, // does not match, but routing=always
	}}

	routed := RouteNotes(cfg, notes, "/src", "claude-code", "")
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note, got %d", len(routed))
	}
	if len(routed[0].Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(routed[0].Targets))
	}
	if routed[0].Targets[0].KB != "teamKB" {
		t.Errorf("expected target KB %q, got %q", "teamKB", routed[0].Targets[0].KB)
	}
}

func TestConsiderRoutingMatch(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets: []config.Target{
				{KB: "myKB", Routing: "consider", Approval: "auto-approve"},
			},
		}},
		[]config.KnowledgeBase{{Name: "myKB"}},
	)

	notes := []extract.Note{{
		Title:              "Matching Note",
		Content:            "Content",
		SuggestedTargetKBs: []string{"myKB"},
	}}

	routed := RouteNotes(cfg, notes, "/src", "claude-code", "")
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note, got %d", len(routed))
	}
	if routed[0].Targets[0].KB != "myKB" {
		t.Errorf("expected target KB %q, got %q", "myKB", routed[0].Targets[0].KB)
	}
}

func TestConsiderRoutingNoMatch(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets: []config.Target{
				{KB: "myKB", Routing: "consider", Approval: "auto-approve"},
			},
		}},
		[]config.KnowledgeBase{{Name: "myKB"}},
	)

	notes := []extract.Note{{
		Title:              "Non-Matching Note",
		Content:            "Content",
		SuggestedTargetKBs: []string{"other"},
	}}

	routed := RouteNotes(cfg, notes, "/src", "claude-code", "")
	// The consider target does not match, so it falls through to local/default
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note (fallback), got %d", len(routed))
	}
	if routed[0].Targets[0].KB != "local/default" {
		t.Errorf("expected fallback to local/default, got %q", routed[0].Targets[0].KB)
	}
}

func TestFallbackToLocalDefault(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets:   []config.Target{}, // no targets configured
		}},
		nil,
	)

	notes := []extract.Note{{
		Title:              "Unrouted Note",
		Content:            "Content",
		SuggestedTargetKBs: nil,
	}}

	routed := RouteNotes(cfg, notes, "/src", "claude-code", "")
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note, got %d", len(routed))
	}
	target := routed[0].Targets[0]
	if target.KB != "local/default" {
		t.Errorf("expected KB %q, got %q", "local/default", target.KB)
	}
	if target.ApprovalMode != "auto-approve" {
		t.Errorf("expected auto-approve fallback, got %q", target.ApprovalMode)
	}
}

func TestOverrideHarnessMatch(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets: []config.Target{
				{KB: "baseKB", Routing: "always", Approval: "auto-approve"},
			},
			Overrides: []config.Override{{
				Harness: "claude-code",
				Targets: []config.Target{
					{KB: "overrideKB", Routing: "always", Approval: "require-manual-approval"},
				},
			}},
		}},
		[]config.KnowledgeBase{{Name: "baseKB"}, {Name: "overrideKB"}},
	)

	notes := []extract.Note{{
		Title:   "Override Note",
		Content: "Content",
	}}

	routed := RouteNotes(cfg, notes, "/src", "claude-code", "")
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note, got %d", len(routed))
	}
	if routed[0].Targets[0].KB != "overrideKB" {
		t.Errorf("expected overrideKB from override, got %q", routed[0].Targets[0].KB)
	}
}

func TestOverrideHarnessAndPersona(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets: []config.Target{
				{KB: "baseKB", Routing: "always", Approval: "auto-approve"},
			},
			Overrides: []config.Override{
				{
					Harness: "notor",
					Targets: []config.Target{
						{KB: "harnessOnlyKB", Routing: "always", Approval: "auto-approve"},
					},
				},
				{
					Harness: "notor",
					Persona: "reviewer",
					Targets: []config.Target{
						{KB: "personaKB", Routing: "always", Approval: "auto-approve"},
					},
				},
			},
		}},
		[]config.KnowledgeBase{{Name: "baseKB"}, {Name: "harnessOnlyKB"}, {Name: "personaKB"}},
	)

	notes := []extract.Note{{
		Title:   "Persona Note",
		Content: "Content",
	}}

	// With persona, the more specific harness+persona override should win
	routed := RouteNotes(cfg, notes, "/src", "notor", "reviewer")
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note, got %d", len(routed))
	}
	if routed[0].Targets[0].KB != "personaKB" {
		t.Errorf("expected personaKB from harness+persona override, got %q", routed[0].Targets[0].KB)
	}
}

func TestLocalKBTarget(t *testing.T) {
	cfg := buildTestConfig(
		[]config.Source{{
			Directory: "/src",
			Targets: []config.Target{
				{KB: "local/myKB", Routing: "always", Approval: "auto-approve"},
				{KB: "local/otherKB", Routing: "consider", Approval: "auto-approve"},
			},
		}},
		nil,
	)

	notes := []extract.Note{{
		Title:              "Local Note",
		Content:            "Content",
		SuggestedTargetKBs: []string{"otherKB"}, // matches "local/otherKB" after prefix strip
	}}

	routed := RouteNotes(cfg, notes, "/src", "claude-code", "")
	if len(routed) != 1 {
		t.Fatalf("expected 1 routed note, got %d", len(routed))
	}
	// Should have 2 targets: always-routed local/myKB and consider-matched local/otherKB
	if len(routed[0].Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(routed[0].Targets))
	}
	kbs := map[string]bool{}
	for _, tgt := range routed[0].Targets {
		kbs[tgt.KB] = true
	}
	if !kbs["local/myKB"] {
		t.Error("expected local/myKB in targets")
	}
	if !kbs["local/otherKB"] {
		t.Error("expected local/otherKB in targets")
	}
}
