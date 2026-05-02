package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// helper: write a YAML config to a temp file, return its path.
func writeTempConfig(t *testing.T, cfg map[string]interface{}) string {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal temp config: %v", err)
	}
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

// minimalValidConfig returns a map that represents a minimal valid config.
func minimalValidConfig() map[string]interface{} {
	return map[string]interface{}{
		"mode":   "client",
		"author": "tester",
		"knowledge_bases": []map[string]interface{}{
			{
				"name":        "my-kb",
				"endpoint":    "https://example.com",
				"auth":        "iam",
				"aws_profile": "default",
			},
		},
		"sources": []map[string]interface{}{
			{
				"directory": "/tmp/src",
				"harnesses": []string{"claude-code"},
				"targets": []map[string]interface{}{
					{
						"kb":       "my-kb",
						"routing":  "always",
						"approval": "auto-approve",
					},
				},
			},
		},
	}
}

func TestLoad_ValidMinimalConfig(t *testing.T) {
	path := writeTempConfig(t, minimalValidConfig())

	cfg, errs := Load(path)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Mode != "client" {
		t.Errorf("mode = %q, want %q", cfg.Mode, "client")
	}
	if cfg.Author != "tester" {
		t.Errorf("author = %q, want %q", cfg.Author, "tester")
	}
	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 KB, got %d", len(cfg.KnowledgeBases))
	}
	if cfg.KnowledgeBases[0].Name != "my-kb" {
		t.Errorf("kb name = %q, want %q", cfg.KnowledgeBases[0].Name, "my-kb")
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(cfg.Sources))
	}
	if cfg.Sources[0].Targets[0].KB != "my-kb" {
		t.Errorf("target kb = %q, want %q", cfg.Sources[0].Targets[0].KB, "my-kb")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	// Config with no mode and no author
	raw := map[string]interface{}{
		"knowledge_bases": []map[string]interface{}{
			{
				"name":        "kb1",
				"auth":        "iam",
				"aws_profile": "p",
			},
		},
		"sources": []map[string]interface{}{
			{
				"directory": "/d",
				"harnesses": []string{"claude-code"},
				"targets": []map[string]interface{}{
					{"kb": "kb1", "routing": "always", "approval": "auto-approve"},
				},
			},
		},
	}
	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config when validation fails")
	}
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors (mode + author), got %d: %v", len(errs), errs)
	}

	var foundMode, foundAuthor bool
	for _, e := range errs {
		msg := e.Error()
		if strings.Contains(msg, "mode") {
			foundMode = true
		}
		if strings.Contains(msg, "author") {
			foundAuthor = true
		}
	}
	if !foundMode {
		t.Error("expected an error about mode")
	}
	if !foundAuthor {
		t.Error("expected an error about author")
	}
}

func TestLoad_InvalidEnumValues(t *testing.T) {
	raw := map[string]interface{}{
		"mode":   "invalid",
		"author": "tester",
		"knowledge_bases": []map[string]interface{}{
			{
				"name":        "kb1",
				"auth":        "magic",
				"aws_profile": "p",
			},
		},
		"sources": []map[string]interface{}{
			{
				"directory": "/d",
				"harnesses": []string{"claude-code"},
				"targets": []map[string]interface{}{
					{"kb": "kb1", "routing": "maybe", "approval": "reject"},
				},
			},
		},
	}
	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config on validation failure")
	}

	errText := ""
	for _, e := range errs {
		errText += e.Error() + "\n"
	}

	for _, want := range []string{"mode", "auth", "routing", "approval"} {
		if !strings.Contains(errText, want) {
			t.Errorf("expected error mentioning %q in:\n%s", want, errText)
		}
	}
}

func TestLoad_DanglingKBReference(t *testing.T) {
	raw := minimalValidConfig()
	// Point the target at a KB that doesn't exist and is not local/...
	sources := raw["sources"].([]map[string]interface{})
	targets := sources[0]["targets"].([]map[string]interface{})
	targets[0]["kb"] = "nonexistent-kb"

	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "nonexistent-kb") && strings.Contains(e.Error(), "does not reference") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected dangling KB reference error, got: %v", errs)
	}
}

func TestLoad_LocalKBReferenceIsValid(t *testing.T) {
	raw := minimalValidConfig()
	// Point the target at a local/... KB, which should be accepted without being in knowledge_bases.
	sources := raw["sources"].([]map[string]interface{})
	targets := sources[0]["targets"].([]map[string]interface{})
	targets[0]["kb"] = "local/my-notes"

	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if len(errs) > 0 {
		t.Fatalf("expected no errors for local/ KB reference, got: %v", errs)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoad_EmptySourceHarnesses(t *testing.T) {
	raw := minimalValidConfig()
	sources := raw["sources"].([]map[string]interface{})
	sources[0]["harnesses"] = []string{}

	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "harnesses") && strings.Contains(e.Error(), "non-empty") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected harnesses error, got: %v", errs)
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
	}{
		{"unparseable string", "5 minutes"},
		{"bare integer", "5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := minimalValidConfig()
			raw["hook"] = map[string]interface{}{"timeout": tc.timeout}

			path := writeTempConfig(t, raw)

			cfg, errs := Load(path)
			if cfg != nil {
				t.Error("expected nil config")
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), "hook.timeout") && strings.Contains(e.Error(), "invalid duration") {
					found = true
				}
			}
			if !found {
				t.Errorf("expected hook.timeout duration error for %q, got: %v", tc.timeout, errs)
			}
		})
	}
}

func TestLoad_InvalidSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
	}{
		{"out of range hour", "25:00"},
		{"non-HH:MM format", "2pm"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := minimalValidConfig()
			raw["recall_log"] = map[string]interface{}{"schedule": tc.schedule}

			path := writeTempConfig(t, raw)

			cfg, errs := Load(path)
			if cfg != nil {
				t.Error("expected nil config")
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), "recall_log.schedule") {
					found = true
				}
			}
			if !found {
				t.Errorf("expected recall_log.schedule error for %q, got: %v", tc.schedule, errs)
			}
		})
	}
}

func TestLoad_ValidSchedule(t *testing.T) {
	raw := minimalValidConfig()
	raw["recall_log"] = map[string]interface{}{"schedule": "02:00"}

	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid schedule, got: %v", errs)
	}
	if cfg.RecallLog == nil || cfg.RecallLog.Schedule != "02:00" {
		t.Errorf("expected recall_log.schedule = %q, got %+v", "02:00", cfg.RecallLog)
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	raw := minimalValidConfig()
	// Do NOT set hook.timeout; it should default to "8s".

	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if cfg.Hook.Timeout != "8s" {
		t.Errorf("hook.timeout = %q, want %q", cfg.Hook.Timeout, "8s")
	}
	if cfg.Extraction.ModelID == "" {
		t.Error("extraction.model_id should have a default value")
	}
}

func TestLoad_StructuredErrorsReturnAll(t *testing.T) {
	// Config with multiple independent problems: bad mode, empty author,
	// bad auth, empty harnesses, dangling KB, and bad duration.
	raw := map[string]interface{}{
		"mode":   "bad-mode",
		"author": "",
		"knowledge_bases": []map[string]interface{}{
			{
				"name": "kb1",
				"auth": "notathing",
			},
		},
		"sources": []map[string]interface{}{
			{
				"directory": "/d",
				"harnesses": []string{},
				"targets": []map[string]interface{}{
					{"kb": "missing-kb", "routing": "always", "approval": "auto-approve"},
				},
			},
		},
		"hook": map[string]interface{}{"timeout": "nope"},
	}

	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config")
	}
	// We expect at least 4 distinct errors (mode, author, auth, harnesses, hook.timeout).
	if len(errs) < 4 {
		t.Errorf("expected at least 4 errors, got %d: %v", len(errs), errs)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config")
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "cannot read config file") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}
