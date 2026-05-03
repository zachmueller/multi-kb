package extract

import (
	"testing"

	"github.com/zmueller/multi-kb/internal/token"
)

func TestSplitAtMessageBoundaries_SingleChunk(t *testing.T) {
	input := "line1\nline2\nline3"
	chunks := splitAtMessageBoundaries(input, token.ChunkingThreshold)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for small input, got %d", len(chunks))
	}
	if chunks[0] != input {
		t.Errorf("expected chunk to be whole input")
	}
}

func TestSplitAtMessageBoundaries_MultipleChunks(t *testing.T) {
	// Force splits by using a very low threshold
	input := "line1\nline2\nline3\nline4"
	chunks := splitAtMessageBoundaries(input, 2) // ~2 tokens per chunk → each line is its own chunk
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks with low threshold, got %d", len(chunks))
	}
}

func TestSplitAtMessageBoundaries_EmptyInput(t *testing.T) {
	chunks := splitAtMessageBoundaries("", 1000)
	if chunks != nil {
		t.Fatalf("expected nil for empty input, got %v", chunks)
	}
}

func TestSplitAtMessageBoundaries_PreservesLines(t *testing.T) {
	// Each line gets its own chunk at threshold=1
	input := "alpha\nbeta\ngamma"
	chunks := splitAtMessageBoundaries(input, 1)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks at threshold=1, got %d", len(chunks))
	}
	if chunks[0] != "alpha" || chunks[1] != "beta" || chunks[2] != "gamma" {
		t.Errorf("chunks don't match expected lines: %v", chunks)
	}
}
