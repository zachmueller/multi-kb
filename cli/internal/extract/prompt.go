package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zmueller/multi-kb/internal/extract/prompts"
)

// BuildExtractionPrompt assembles the extraction system prompt from:
// 1. The hardcoded base prompt (from prompts package)
// 2. Optional exclusion rules from config
// 3. Optional append file at ~/.multi-kb/prompts/extraction-append.md
func BuildExtractionPrompt(exclusionRules []string) string {
	var sb strings.Builder
	sb.WriteString(prompts.ExtractionSystemPrompt)

	if len(exclusionRules) > 0 {
		sb.WriteString("\n\n## Content exclusion rules — never include in notes destined for non-local KBs\n\n")
		for _, rule := range exclusionRules {
			sb.WriteString(fmt.Sprintf("- %s\n", rule))
		}
	}

	appendPath := defaultAppendPath()
	if data, err := os.ReadFile(appendPath); err == nil && len(data) > 0 {
		sb.WriteString("\n\n")
		sb.Write(data)
	}

	return sb.String()
}

func defaultAppendPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".multi-kb", "prompts", "extraction-append.md")
}
