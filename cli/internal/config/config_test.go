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

// --- Server-mode tests (SRV-001) ---

func minimalValidServerConfig() map[string]interface{} {
	return map[string]interface{}{
		"mode":   "server",
		"author": "multi-kb-server",
		"sqs": map[string]interface{}{
			"queue_url":  "https://sqs.us-east-1.amazonaws.com/123456789012/multi-kb",
			"batch_size": 10,
		},
		"codecommit": map[string]interface{}{
			"repo_name": "multi-kb",
			"region":    "us-east-1",
		},
		"s3": map[string]interface{}{
			"bucket": "multi-kb-123456789012-us-east-1",
			"region": "us-east-1",
		},
		"opensearch": map[string]interface{}{
			"endpoint": "https://abc123.us-east-1.aoss.amazonaws.com",
			"region":   "us-east-1",
		},
		"bedrock_kb": map[string]interface{}{
			"knowledge_base_id": "KBXXXXXX",
			"data_source_id":    "DSXXXXXX",
		},
		"tick_interval": "5m",
		"dream_cycle": map[string]interface{}{
			"interval": "3h",
			"model_id": "anthropic.claude-sonnet-4-20250514",
		},
		"recall_log": map[string]interface{}{
			"schedule": "02:00",
		},
	}
}

func TestLoad_ValidServerConfig(t *testing.T) {
	path := writeTempConfig(t, minimalValidServerConfig())

	cfg, errs := Load(path)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Mode != "server" {
		t.Errorf("mode = %q, want %q", cfg.Mode, "server")
	}
	if cfg.SQS.QueueURL != "https://sqs.us-east-1.amazonaws.com/123456789012/multi-kb" {
		t.Errorf("sqs.queue_url = %q", cfg.SQS.QueueURL)
	}
	if cfg.CodeCommit.RepoName != "multi-kb" {
		t.Errorf("codecommit.repo_name = %q", cfg.CodeCommit.RepoName)
	}
	if cfg.S3.Bucket != "multi-kb-123456789012-us-east-1" {
		t.Errorf("s3.bucket = %q", cfg.S3.Bucket)
	}
	if cfg.OpenSearch.Endpoint != "https://abc123.us-east-1.aoss.amazonaws.com" {
		t.Errorf("opensearch.endpoint = %q", cfg.OpenSearch.Endpoint)
	}
	if cfg.BedrockKB.KnowledgeBaseID != "KBXXXXXX" {
		t.Errorf("bedrock_kb.knowledge_base_id = %q", cfg.BedrockKB.KnowledgeBaseID)
	}
	if cfg.TickInterval != "5m" {
		t.Errorf("tick_interval = %q", cfg.TickInterval)
	}
}

func TestLoad_ServerModeMissingRequiredFields(t *testing.T) {
	raw := map[string]interface{}{
		"mode":   "server",
		"author": "multi-kb-server",
	}
	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if cfg != nil {
		t.Error("expected nil config")
	}

	expectedFields := []string{
		"sqs.queue_url",
		"codecommit.repo_name",
		"s3.bucket",
		"opensearch.endpoint",
		"bedrock_kb",
		"tick_interval",
		"dream_cycle.interval",
		"recall_log.schedule",
	}
	errText := ""
	for _, e := range errs {
		errText += e.Error() + "\n"
	}
	for _, field := range expectedFields {
		if !strings.Contains(errText, field) {
			t.Errorf("expected error about %q in:\n%s", field, errText)
		}
	}
}

func TestLoad_ClientModeIgnoresServerFields(t *testing.T) {
	raw := minimalValidConfig()
	// Client mode should not require server fields
	path := writeTempConfig(t, raw)

	cfg, errs := Load(path)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	if cfg.SQS != nil {
		t.Error("expected nil SQS config in client mode")
	}
}

func TestLoad_ServerModeInvalidDurations(t *testing.T) {
	raw := minimalValidServerConfig()
	raw["tick_interval"] = "nope"
	path := writeTempConfig(t, raw)

	_, errs := Load(path)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "tick_interval") && strings.Contains(e.Error(), "invalid duration") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tick_interval duration error, got: %v", errs)
	}
}

func TestLoad_ServerModeInvalidRecallSchedule(t *testing.T) {
	raw := minimalValidServerConfig()
	raw["recall_log"] = map[string]interface{}{"schedule": "25:99"}
	path := writeTempConfig(t, raw)

	_, errs := Load(path)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "recall_log.schedule") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recall_log.schedule error, got: %v", errs)
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
