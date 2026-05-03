package extract

import (
	"strings"
	"testing"

	"github.com/zmueller/multi-kb/internal/extract/prompts"
)

func TestBuildExtractionPrompt_NoExclusions(t *testing.T) {
	result := BuildExtractionPrompt(nil)
	if !strings.Contains(result, prompts.ExtractionSystemPrompt) {
		t.Error("expected base prompt in output")
	}
	if strings.Contains(result, "Content exclusion rules") {
		t.Error("should not have exclusion section when no rules")
	}
}

func TestBuildExtractionPrompt_WithExclusions(t *testing.T) {
	rules := []string{"No internal URLs", "No API keys"}
	result := BuildExtractionPrompt(rules)
	if !strings.Contains(result, "Content exclusion rules") {
		t.Error("expected exclusion rules section")
	}
	if !strings.Contains(result, "- No internal URLs") {
		t.Error("expected first exclusion rule")
	}
	if !strings.Contains(result, "- No API keys") {
		t.Error("expected second exclusion rule")
	}
}
