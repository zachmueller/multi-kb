package token

import (
	"strings"
	"testing"
)

func TestEstimateTokensEmpty(t *testing.T) {
	got := EstimateTokens("")
	if got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTokensShortProse(t *testing.T) {
	s := "hello world"
	got := EstimateTokens(s)

	// Prose: ~4 chars/token → "hello world" (11 chars) ≈ 2-3 tokens
	expectedApprox := len(s) / 4
	// Allow reasonable tolerance: ±1 token
	if got < expectedApprox-1 || got > expectedApprox+1 {
		t.Errorf("EstimateTokens(%q) = %d, expected approximately %d (len/4)", s, got, expectedApprox)
	}
}

func TestEstimateTokensCodeBlock(t *testing.T) {
	code := "func main() { fmt.Println(\"hello\") }"
	s := "Some prose text here.\n```\n" + code + "\n```\nMore prose after."

	got := EstimateTokens(s)

	// The code portion should be counted at ~3.5 chars/token (denser)
	// Prose portion at ~4 chars/token
	// Total should be > pure prose estimate (all at 4 chars/token)
	pureProseEstimate := int(float64(len(s))/4.0 + 0.5)
	if got <= pureProseEstimate-1 {
		t.Errorf("EstimateTokens with code block = %d, expected > %d (code should be denser)", got, pureProseEstimate-1)
	}

	// Also verify it's reasonable: not wildly different
	if got > len(s) {
		t.Errorf("EstimateTokens = %d, should not exceed string length %d", got, len(s))
	}
}

func TestExceedsChunkingThresholdTrue(t *testing.T) {
	// 700001 chars → should exceed threshold (ChunkingThreshold = 700_000 tokens)
	// At ~4 chars/token, we need ~2.8M chars to exceed 700K tokens.
	// But the threshold is in tokens, and 700001 chars ≈ 175K tokens, which is < 700K.
	// We need chars ≈ 700_000 * 4 = 2_800_000 to exceed.
	bigString := strings.Repeat("a", 2_800_005)
	if !ExceedsChunkingThreshold(bigString) {
		t.Errorf("ExceedsChunkingThreshold should return true for %d chars (~%d tokens)", len(bigString), EstimateTokens(bigString))
	}
}

func TestExceedsChunkingThresholdFalse(t *testing.T) {
	smallString := strings.Repeat("a", 100)
	if ExceedsChunkingThreshold(smallString) {
		t.Error("ExceedsChunkingThreshold should return false for a 100-char string")
	}
}
