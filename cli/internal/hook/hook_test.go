package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterNotorHook(t *testing.T) {
	dir := t.TempDir()
	if err := RegisterNotorHook(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	automationPath := filepath.Join(dir, "notor", "automations", notorAutomationFilename)
	data, err := os.ReadFile(automationPath)
	if err != nil {
		t.Fatalf("cannot read automation file: %v", err)
	}

	content := string(data)
	if content != automationTemplate {
		t.Errorf("automation file content doesn't match template")
	}
}

func TestRegisterNotorHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := RegisterNotorHook(dir); err != nil {
		t.Fatal(err)
	}
	if err := RegisterNotorHook(dir); err != nil {
		t.Fatal(err)
	}
	// File should exist and have same content
	data, err := os.ReadFile(filepath.Join(dir, "notor", "automations", notorAutomationFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != automationTemplate {
		t.Error("content changed after second registration")
	}
}

func TestUnregisterNotorHook(t *testing.T) {
	dir := t.TempDir()
	if err := RegisterNotorHook(dir); err != nil {
		t.Fatal(err)
	}
	if err := UnregisterNotorHook(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	automationPath := filepath.Join(dir, "notor", "automations", notorAutomationFilename)
	if _, err := os.Stat(automationPath); !os.IsNotExist(err) {
		t.Error("expected automation file to be removed")
	}
}

func TestUnregisterNotorHook_NotExist(t *testing.T) {
	dir := t.TempDir()
	if err := UnregisterNotorHook(dir); err != nil {
		t.Fatalf("unregister of non-existent hook should not error: %v", err)
	}
}

func TestIsMultiKBEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry interface{}
		want  bool
	}{
		{
			"multi-kb hook",
			map[string]interface{}{
				"matcher": "*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": "multi-kb hook --harness claude-code",
					},
				},
			},
			true,
		},
		{
			"other hook",
			map[string]interface{}{
				"matcher": "*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": "other-tool --flag",
					},
				},
			},
			false,
		},
		{
			"non-map entry",
			"just a string",
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isMultiKBEntry(tc.entry)
			if got != tc.want {
				t.Errorf("isMultiKBEntry() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadSettings_NonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	settings, err := readSettings(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("expected empty map for non-existent file, got %v", settings)
	}
}

func TestReadSettings_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{"hooks":{"UserPromptSubmit":[]}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := readSettings(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := settings["hooks"]; !ok {
		t.Error("expected hooks key in settings")
	}
}

func TestWriteSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "settings.json")

	settings := map[string]interface{}{
		"key": "value",
	}
	if err := writeSettings(path, settings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON written: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
}

func TestEnsureHooksSection(t *testing.T) {
	// With existing hooks
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{"UserPromptSubmit": []interface{}{}},
	}
	h := ensureHooksSection(settings)
	if h == nil {
		t.Fatal("expected hooks section")
	}

	// Without hooks
	settings2 := map[string]interface{}{}
	h2 := ensureHooksSection(settings2)
	if h2 == nil {
		t.Fatal("expected new empty hooks section")
	}
	if len(h2) != 0 {
		t.Errorf("expected empty map, got %v", h2)
	}
}

func TestGetPromptSubmitHooks(t *testing.T) {
	// With hooks
	hooks := map[string]interface{}{
		"UserPromptSubmit": []interface{}{"entry1"},
	}
	result := getPromptSubmitHooks(hooks)
	if len(result) != 1 {
		t.Errorf("expected 1 hook, got %d", len(result))
	}

	// Without key
	empty := map[string]interface{}{}
	result2 := getPromptSubmitHooks(empty)
	if result2 != nil {
		t.Errorf("expected nil for missing key, got %v", result2)
	}
}
