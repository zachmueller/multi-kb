package hook

import (
	"fmt"
	"os"
	"path/filepath"
)

const notorAutomationFilename = "multi-kb-recall.md"

// automationTemplate is the Notor automation file content.
// It registers a blocking on_conversation_start hook that calls multi-kb hook --harness notor.
const automationTemplate = `---
notor-type: automation
notor-trigger: on_conversation_start
notor-blocking: true
notor-blocking-emit-kind: multi_kb_recall
notor-blocking-timeout: 10000
notor-automation-order: 100
---

` + "```typescript" + `
const input = JSON.stringify({
  first_message: context.firstMessage,
  conversation_id: context.conversationId,
  timestamp: new Date().toISOString(),
});

const result = await utils.executeShellCommand(
  "multi-kb hook --harness notor",
  { stdin: input }
);

if (result.stdout && result.stdout.trim()) {
  await chatBlocks.emit({
    kind: "multi_kb_recall",
    content: result.stdout,
  });
}
` + "```\n"

// RegisterNotorHook writes the multi-kb automation file to the Notor automations directory.
// It is idempotent: overwrites if the file already exists.
// vaultDir is the Obsidian vault root directory.
func RegisterNotorHook(vaultDir string) error {
	automationsDir := filepath.Join(vaultDir, "notor", "automations")
	if err := os.MkdirAll(automationsDir, 0o755); err != nil {
		return fmt.Errorf("hook: create automations directory: %w", err)
	}

	automationPath := filepath.Join(automationsDir, notorAutomationFilename)
	if err := os.WriteFile(automationPath, []byte(automationTemplate), 0o644); err != nil {
		return fmt.Errorf("hook: write automation file: %w", err)
	}

	return nil
}

// UnregisterNotorHook removes the multi-kb automation file from the Notor automations directory.
func UnregisterNotorHook(vaultDir string) error {
	automationPath := filepath.Join(vaultDir, "notor", "automations", notorAutomationFilename)
	err := os.Remove(automationPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
