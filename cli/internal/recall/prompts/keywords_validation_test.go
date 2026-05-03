//go:build integration

package prompts_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/recall/prompts"
)

func makeKeywordsClient(t *testing.T) *bedrock.Client {
	t.Helper()
	profile := os.Getenv("MULTI_KB_AWS_PROFILE")
	region := os.Getenv("MULTI_KB_AWS_REGION")
	modelID := os.Getenv("MULTI_KB_MODEL_ID")
	if region == "" {
		region = "us-east-1"
	}
	if modelID == "" {
		modelID = "anthropic.claude-3-haiku-20240307-v1:0"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := bedrock.NewClient(ctx, profile, region, modelID)
	if err != nil {
		if strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "SSO") {
			t.Skipf("skipping: credentials unavailable: %v", err)
		}
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// parseKeywords calls the LLM with KeywordDerivationPrompt and returns the parsed keyword array.
func parseKeywords(t *testing.T, client *bedrock.Client, query string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, prompts.KeywordDerivationPrompt, query)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	var keywords []string
	if err := json.Unmarshal([]byte(text), &keywords); err != nil {
		t.Fatalf("response is not a valid JSON array of strings: %v\nresponse: %s", err, text)
	}
	return keywords
}

// TestKeywordDerivationPrompt_TechnicalQuestion verifies the LLM extracts specific technical
// terms from a detailed technical question.
func TestKeywordDerivationPrompt_TechnicalQuestion(t *testing.T) {
	client := makeKeywordsClient(t)

	query := "How do I configure DynamoDB Global Tables for cross-region replication with consistent reads?"

	keywords := parseKeywords(t, client, query)

	if len(keywords) < 2 || len(keywords) > 7 {
		t.Errorf("expected 2-7 keywords, got %d: %v", len(keywords), keywords)
	}

	// At least one keyword should reference DynamoDB, Global Tables, or replication
	combined := strings.ToLower(strings.Join(keywords, " "))
	hasTech := strings.Contains(combined, "dynamodb") ||
		strings.Contains(combined, "global") ||
		strings.Contains(combined, "replication") ||
		strings.Contains(combined, "cross-region") ||
		strings.Contains(combined, "cross region")
	if !hasTech {
		t.Errorf("expected keywords to reference the specific technical topic; got: %v", keywords)
	}

	// No generic stop words
	stopWords := []string{"how", "do", "i", "for", "with", "the", "a", "an"}
	for _, kw := range keywords {
		lower := strings.ToLower(kw)
		for _, stop := range stopWords {
			if lower == stop {
				t.Errorf("keyword %q is a generic stop word; should be filtered out", kw)
			}
		}
	}
}

// TestKeywordDerivationPrompt_BroadRequest verifies the LLM produces reasonable keywords
// for a broader, less-specific request.
func TestKeywordDerivationPrompt_BroadRequest(t *testing.T) {
	client := makeKeywordsClient(t)

	query := "What are the best practices for building microservices?"

	keywords := parseKeywords(t, client, query)

	if len(keywords) < 2 || len(keywords) > 7 {
		t.Errorf("expected 2-7 keywords, got %d: %v", len(keywords), keywords)
	}

	// Keywords should reference microservices-related concepts
	combined := strings.ToLower(strings.Join(keywords, " "))
	hasTech := strings.Contains(combined, "microservice") ||
		strings.Contains(combined, "service") ||
		strings.Contains(combined, "architecture") ||
		strings.Contains(combined, "api") ||
		strings.Contains(combined, "deployment")
	if !hasTech {
		t.Errorf("expected keywords related to microservices; got: %v", keywords)
	}
}

// TestKeywordDerivationPrompt_ShortAmbiguousQuery verifies the LLM produces best-effort
// keywords for a short, ambiguous query without hallucinating.
func TestKeywordDerivationPrompt_ShortAmbiguousQuery(t *testing.T) {
	client := makeKeywordsClient(t)

	query := "fix the timeout"

	keywords := parseKeywords(t, client, query)

	if len(keywords) == 0 {
		t.Fatal("expected at least one keyword even for a short query, got 0")
	}
	if len(keywords) > 7 {
		t.Errorf("expected at most 7 keywords, got %d: %v", len(keywords), keywords)
	}

	// At minimum, "timeout" should appear (or a close variant)
	combined := strings.ToLower(strings.Join(keywords, " "))
	if !strings.Contains(combined, "timeout") && !strings.Contains(combined, "time") {
		t.Logf("note: short query produced keywords without 'timeout': %v — qualitative check only", keywords)
	}

	// Keywords should be non-empty strings
	for i, kw := range keywords {
		if strings.TrimSpace(kw) == "" {
			t.Errorf("keyword[%d] is empty or whitespace", i)
		}
	}
}
