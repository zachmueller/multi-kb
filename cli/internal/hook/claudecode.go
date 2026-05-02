package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const claudeCodeSettingsPath = ".claude/settings.json"

// claudeCodeHookEntry is the hook entry added to settings.json.
var claudeCodeHookEntry = map[string]interface{}{
	"matcher": "*",
	"hooks": []map[string]interface{}{
		{
			"type":    "command",
			"command": "multi-kb hook --harness claude-code",
			"timeout": 10,
		},
	},
}

// RegisterClaudeCodeHook adds the multi-kb hook to ~/.claude/settings.json.
// It is idempotent: if a multi-kb hook entry already exists it updates it.
func RegisterClaudeCodeHook() error {
	settingsPath, err := resolveClaudeSettingsPath()
	if err != nil {
		return err
	}

	settings, err := readSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("hook: read claude settings: %w", err)
	}

	hooksSection := ensureHooksSection(settings)
	promptSubmitHooks := getPromptSubmitHooks(hooksSection)

	// Idempotency: update existing entry if found
	found := false
	for i, entry := range promptSubmitHooks {
		if isMultiKBEntry(entry) {
			promptSubmitHooks[i] = claudeCodeHookEntry
			found = true
			break
		}
	}
	if !found {
		promptSubmitHooks = append(promptSubmitHooks, claudeCodeHookEntry)
	}

	hooksSection["UserPromptSubmit"] = promptSubmitHooks
	settings["hooks"] = hooksSection

	return writeSettings(settingsPath, settings)
}

// UnregisterClaudeCodeHook removes the multi-kb hook from ~/.claude/settings.json.
func UnregisterClaudeCodeHook() error {
	settingsPath, err := resolveClaudeSettingsPath()
	if err != nil {
		return err
	}

	settings, err := readSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("hook: read claude settings: %w", err)
	}

	hooksSection, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return nil // nothing to remove
	}

	promptSubmitHooks := getPromptSubmitHooks(hooksSection)
	filtered := promptSubmitHooks[:0]
	for _, entry := range promptSubmitHooks {
		if !isMultiKBEntry(entry) {
			filtered = append(filtered, entry)
		}
	}
	hooksSection["UserPromptSubmit"] = filtered
	settings["hooks"] = hooksSection

	return writeSettings(settingsPath, settings)
}

func resolveClaudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("hook: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, claudeCodeSettingsPath), nil
}

func readSettings(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, err
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("malformed settings.json: %w", err)
	}
	return settings, nil
}

func writeSettings(path string, settings map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func ensureHooksSection(settings map[string]interface{}) map[string]interface{} {
	if h, ok := settings["hooks"].(map[string]interface{}); ok {
		return h
	}
	return make(map[string]interface{})
}

func getPromptSubmitHooks(hooks map[string]interface{}) []interface{} {
	v, ok := hooks["UserPromptSubmit"]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	return arr
}

func isMultiKBEntry(entry interface{}) bool {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	hooks, ok := m["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, h := range hooks {
		hm, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "multi-kb hook") {
			return true
		}
	}
	return false
}
