package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zmueller/multi-kb/internal/config"
	"gopkg.in/yaml.v3"
)

// --- parseExclusionLines tests (AUD-011: exclusion rules) ---

func TestParseExclusionLines_Basic(t *testing.T) {
	got := parseExclusionLines("^node_modules/\n\\.pyc$\n.DS_Store")
	want := []string{"^node_modules/", "\\.pyc$", ".DS_Store"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseExclusionLines_Empty(t *testing.T) {
	if got := parseExclusionLines(""); len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestParseExclusionLines_BlankLinesSkipped(t *testing.T) {
	got := parseExclusionLines("\n\nrule1\n\nrule2\n\n")
	if len(got) != 2 || got[0] != "rule1" || got[1] != "rule2" {
		t.Errorf("expected [rule1 rule2], got %v", got)
	}
}

func TestParseExclusionLines_WhitespaceTrimmed(t *testing.T) {
	got := parseExclusionLines("  rule1  \n  rule2  ")
	if len(got) != 2 || got[0] != "rule1" || got[1] != "rule2" {
		t.Errorf("expected trimmed rules, got %v", got)
	}
}

func TestParseExclusionLines_SingleRule(t *testing.T) {
	got := parseExclusionLines("^secret/")
	if len(got) != 1 || got[0] != "^secret/" {
		t.Errorf("expected [^secret/], got %v", got)
	}
}

// --- validateDirPath tests (AUD-010: directory validation) ---

func TestValidateDirPath_Exists(t *testing.T) {
	dir := t.TempDir()
	if err := validateDirPath(dir); err != nil {
		t.Errorf("expected nil for existing directory, got %v", err)
	}
}

func TestValidateDirPath_NotExist(t *testing.T) {
	err := validateDirPath(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateDirPath_FileNotDir(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := validateDirPath(f.Name()); err == nil {
		t.Error("expected error for file path, got nil")
	}
}

// --- buildTargets tests (AUD-011: routing presets) ---

func TestBuildTargets_NoRemoteKBs(t *testing.T) {
	targets := buildTargets(nil, "auto")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target (local/default), got %d", len(targets))
	}
	if targets[0].KB != "local/default" {
		t.Errorf("expected kb=local/default, got %q", targets[0].KB)
	}
}

func TestBuildTargets_AutoPreset(t *testing.T) {
	kbs := []config.KnowledgeBase{{Name: "my-kb"}}
	targets := buildTargets(kbs, "auto")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	remote := targets[1]
	if remote.KB != "my-kb" {
		t.Errorf("expected kb=my-kb, got %q", remote.KB)
	}
	if remote.Approval != "auto-approve" {
		t.Errorf("expected auto-approve, got %q", remote.Approval)
	}
	if remote.Routing != "consider" {
		t.Errorf("expected routing=consider, got %q", remote.Routing)
	}
}

func TestBuildTargets_ManualPreset(t *testing.T) {
	kbs := []config.KnowledgeBase{{Name: "my-kb"}}
	targets := buildTargets(kbs, "manual")
	if targets[1].Approval != "require-manual-approval" {
		t.Errorf("expected require-manual-approval, got %q", targets[1].Approval)
	}
}

func TestBuildTargets_MixedPreset(t *testing.T) {
	kbs := []config.KnowledgeBase{{Name: "my-kb"}}
	targets := buildTargets(kbs, "mixed")
	// Local target always auto-approve.
	if targets[0].Approval != "auto-approve" {
		t.Errorf("local target: expected auto-approve, got %q", targets[0].Approval)
	}
	// Remote target in mixed = manual.
	if targets[1].Approval != "require-manual-approval" {
		t.Errorf("remote target in mixed: expected require-manual-approval, got %q", targets[1].Approval)
	}
}

func TestBuildTargets_MultipleKBs(t *testing.T) {
	kbs := []config.KnowledgeBase{{Name: "kb1"}, {Name: "kb2"}}
	targets := buildTargets(kbs, "auto")
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
}

// --- runAddSourceFrom tests (AUD-015: WIZ-006 add-source) ---

func writeMinimalConfig(t *testing.T, dir string) string {
	t.Helper()
	cfg := `mode: client
author: tester
extraction:
  model_id: anthropic.claude-sonnet-4-20250514
hook:
  timeout: 8s
dream_cycle:
  model_id: anthropic.claude-sonnet-4-20250514
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRunAddSource_AppendsSource(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMinimalConfig(t, dir)
	srcDir := t.TempDir()

	stdin := strings.NewReader(srcDir + "\n1\n")
	if err := runAddSourceFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddSourceFrom: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(cfg.Sources))
	}
	if cfg.Sources[0].Directory != srcDir {
		t.Errorf("expected directory %q, got %q", srcDir, cfg.Sources[0].Directory)
	}
	if len(cfg.Sources[0].Harnesses) != 1 || cfg.Sources[0].Harnesses[0] != "claude-code" {
		t.Errorf("expected harness [claude-code], got %v", cfg.Sources[0].Harnesses)
	}
}

func TestRunAddSource_BothHarnesses(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMinimalConfig(t, dir)
	srcDir := t.TempDir()

	stdin := strings.NewReader(srcDir + "\n3\n")
	if err := runAddSourceFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddSourceFrom: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg config.Config
	yaml.Unmarshal(data, &cfg) //nolint
	if len(cfg.Sources[0].Harnesses) != 2 {
		t.Errorf("expected 2 harnesses, got %v", cfg.Sources[0].Harnesses)
	}
}

func TestRunAddSource_PreservesExistingSources(t *testing.T) {
	dir := t.TempDir()
	// Config with an existing source.
	srcDir1 := t.TempDir()
	cfg := config.Config{
		Mode:   "client",
		Author: "tester",
		Extraction: config.ExtractionConfig{
			ModelID: "anthropic.claude-sonnet-4-20250514",
		},
		Hook:      config.HookConfig{Timeout: "8s"},
		DreamCycle: config.DreamCycleConfig{ModelID: "anthropic.claude-sonnet-4-20250514"},
		Sources: []config.Source{
			{Directory: srcDir1, Harnesses: []string{"claude-code"}, Targets: []config.Target{{KB: "local/default", Routing: "always", Approval: "auto-approve"}}},
		},
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0o600)

	srcDir2 := t.TempDir()
	stdin := strings.NewReader(srcDir2 + "\n1\n")
	if err := runAddSourceFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddSourceFrom: %v", err)
	}

	data, _ = os.ReadFile(cfgPath)
	var result config.Config
	yaml.Unmarshal(data, &result) //nolint
	if len(result.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(result.Sources))
	}
}

func TestRunAddSource_MissingConfig(t *testing.T) {
	stdin := strings.NewReader("")
	err := runAddSourceFrom("/nonexistent/config.yaml", stdin)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("expected 'load config' in error, got %q", err)
	}
}

func TestRunAddSource_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMinimalConfig(t, dir)

	// Send empty directory path.
	stdin := strings.NewReader("\n")
	err := runAddSourceFrom(cfgPath, stdin)
	if err == nil {
		t.Fatal("expected error for empty directory path")
	}
}

// --- runAddKBFrom tests (AUD-015: WIZ-006 add-kb) ---

func TestRunAddKB_AppendsKB(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMinimalConfig(t, dir)

	// KB name, endpoint, auth=iam, profile, region, description
	stdin := strings.NewReader("my-kb\nhttps://example.com\niam\nmyprofile\nus-east-1\nMy KB description\n")
	if err := runAddKBFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddKBFrom: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg config.Config
	yaml.Unmarshal(data, &cfg) //nolint
	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 KB, got %d", len(cfg.KnowledgeBases))
	}
	kb := cfg.KnowledgeBases[0]
	if kb.Name != "my-kb" {
		t.Errorf("expected name=my-kb, got %q", kb.Name)
	}
	if kb.Auth != "iam" {
		t.Errorf("expected auth=iam, got %q", kb.Auth)
	}
	if kb.AWSProfile != "myprofile" {
		t.Errorf("expected profile=myprofile, got %q", kb.AWSProfile)
	}
}

func TestRunAddKB_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		Mode:   "client",
		Author: "tester",
		Extraction: config.ExtractionConfig{
			ModelID: "anthropic.claude-sonnet-4-20250514",
		},
		Hook:      config.HookConfig{Timeout: "8s"},
		DreamCycle: config.DreamCycleConfig{ModelID: "anthropic.claude-sonnet-4-20250514"},
		KnowledgeBases: []config.KnowledgeBase{
			{Name: "existing-kb", Endpoint: "https://old.example.com", Auth: "federate"},
		},
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0o600)

	stdin := strings.NewReader("new-kb\nhttps://new.example.com\nfederate\n\nus-east-1\nNew KB\n")
	if err := runAddKBFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddKBFrom: %v", err)
	}

	data, _ = os.ReadFile(cfgPath)
	var result config.Config
	yaml.Unmarshal(data, &result) //nolint
	if len(result.KnowledgeBases) != 2 {
		t.Errorf("expected 2 KBs, got %d", len(result.KnowledgeBases))
	}
}

func TestRunAddKB_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		Mode:   "client",
		Author: "tester",
		Extraction: config.ExtractionConfig{
			ModelID: "anthropic.claude-sonnet-4-20250514",
		},
		Hook:      config.HookConfig{Timeout: "8s"},
		DreamCycle: config.DreamCycleConfig{ModelID: "anthropic.claude-sonnet-4-20250514"},
		KnowledgeBases: []config.KnowledgeBase{
			{Name: "my-kb", Endpoint: "https://example.com", Auth: "federate"},
		},
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0o600)

	stdin := strings.NewReader("my-kb\n")
	err := runAddKBFrom(cfgPath, stdin)
	if err == nil {
		t.Fatal("expected error for duplicate KB name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got %q", err)
	}
}

func TestRunAddKB_EmptyName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMinimalConfig(t, dir)

	stdin := strings.NewReader("\n")
	err := runAddKBFrom(cfgPath, stdin)
	if err == nil {
		t.Fatal("expected error for empty KB name")
	}
}

func TestRunAddKB_MissingConfig(t *testing.T) {
	stdin := strings.NewReader("kb\n")
	err := runAddKBFrom("/nonexistent/config.yaml", stdin)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("expected 'load config' in error, got %q", err)
	}
}

// --- overrides roundtrip tests (AUD-011: directory-specific routing overrides) ---

func TestRunAddSource_PreservesExistingOverrides(t *testing.T) {
	dir := t.TempDir()
	srcDir1 := t.TempDir()
	cfg := config.Config{
		Mode:   "client",
		Author: "tester",
		Extraction: config.ExtractionConfig{
			ModelID: "anthropic.claude-sonnet-4-20250514",
		},
		Hook:       config.HookConfig{Timeout: "8s"},
		DreamCycle: config.DreamCycleConfig{ModelID: "anthropic.claude-sonnet-4-20250514"},
		KnowledgeBases: []config.KnowledgeBase{
			{Name: "remote-sec", Endpoint: "https://sec.example.com", Auth: "iam", AWSProfile: "default"},
		},
		Sources: []config.Source{
			{
				Directory: srcDir1,
				Harnesses: []string{"claude-code"},
				Targets:   []config.Target{{KB: "local/default", Routing: "always", Approval: "auto-approve"}},
				Overrides: []config.Override{
					{
						Harness: "claude-code",
						Persona: "security",
						Targets: []config.Target{
							{KB: "remote-sec", Routing: "always", Approval: "require-manual-approval"},
						},
					},
				},
			},
		},
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	data, _ := yaml.Marshal(cfg)
	os.WriteFile(cfgPath, data, 0o600)

	srcDir2 := t.TempDir()
	stdin := strings.NewReader(srcDir2 + "\n1\n")
	if err := runAddSourceFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddSourceFrom: %v", err)
	}

	data, _ = os.ReadFile(cfgPath)
	var result config.Config
	yaml.Unmarshal(data, &result) //nolint
	if len(result.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(result.Sources))
	}
	if len(result.Sources[0].Overrides) != 1 {
		t.Fatalf("expected 1 override on first source, got %d", len(result.Sources[0].Overrides))
	}
	ov := result.Sources[0].Overrides[0]
	if ov.Harness != "claude-code" || ov.Persona != "security" {
		t.Errorf("override mutated: harness=%q persona=%q", ov.Harness, ov.Persona)
	}
	if len(ov.Targets) != 1 || ov.Targets[0].KB != "remote-sec" {
		t.Errorf("override targets mutated: %v", ov.Targets)
	}
}

func TestOverrides_YAMLRoundtrip(t *testing.T) {
	src := config.Source{
		Directory: "/test/project",
		Harnesses: []string{"claude-code", "notor"},
		Targets:   []config.Target{{KB: "local/default", Routing: "always", Approval: "auto-approve"}},
		Overrides: []config.Override{
			{
				Harness: "claude-code",
				Targets: []config.Target{{KB: "team-kb", Routing: "consider", Approval: "auto-approve"}},
			},
			{
				Harness: "notor",
				Persona: "research",
				Targets: []config.Target{{KB: "research-kb", Routing: "always", Approval: "require-manual-approval"}},
			},
		},
	}

	data, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got config.Source
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(got.Overrides))
	}
	if got.Overrides[0].Harness != "claude-code" {
		t.Errorf("expected harness=claude-code, got %q", got.Overrides[0].Harness)
	}
	if got.Overrides[1].Persona != "research" {
		t.Errorf("expected persona=research, got %q", got.Overrides[1].Persona)
	}
	if got.Overrides[1].Targets[0].KB != "research-kb" {
		t.Errorf("expected KB=research-kb, got %q", got.Overrides[1].Targets[0].KB)
	}
}

func TestRunAddKB_FederateAuthNoProfile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMinimalConfig(t, dir)

	// federate auth — no profile prompt
	stdin := strings.NewReader("fed-kb\nhttps://example.com\nfederate\nus-west-2\nFederate KB\n")
	if err := runAddKBFrom(cfgPath, stdin); err != nil {
		t.Fatalf("runAddKBFrom: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg config.Config
	yaml.Unmarshal(data, &cfg) //nolint
	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 KB, got %d", len(cfg.KnowledgeBases))
	}
	if cfg.KnowledgeBases[0].Auth != "federate" {
		t.Errorf("expected auth=federate, got %q", cfg.KnowledgeBases[0].Auth)
	}
	if cfg.KnowledgeBases[0].AWSProfile != "" {
		t.Errorf("expected no profile for federate auth, got %q", cfg.KnowledgeBases[0].AWSProfile)
	}
}
