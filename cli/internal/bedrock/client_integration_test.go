//go:build integration

package bedrock

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInvokeModel_RealBedrock(t *testing.T) {
	profile := os.Getenv("MULTI_KB_AWS_PROFILE")
	region := os.Getenv("MULTI_KB_AWS_REGION")
	modelID := os.Getenv("MULTI_KB_MODEL_ID")

	if region == "" {
		region = "us-east-1"
	}
	if modelID == "" {
		modelID = "anthropic.claude-3-haiku-20240307-v1:0"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := NewClient(ctx, profile, region, modelID)
	if err != nil {
		if strings.Contains(err.Error(), "SSO") || strings.Contains(err.Error(), "credentials") {
			t.Skipf("skipping: credentials unavailable: %v", err)
		}
		t.Fatalf("NewClient: %v", err)
	}

	systemPrompt := "You are a JSON extractor. Return a JSON array of keywords from the user's message."
	userMessage := "How do I configure AWS VPC peering for multi-region deployment?"

	text, err := client.InvokeModel(ctx, systemPrompt, userMessage)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") || strings.Contains(err.Error(), "credentials") {
			t.Skipf("skipping: access issue: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	if text == "" {
		t.Fatal("expected non-empty response")
	}

	// Should contain something about VPC or AWS
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "vpc") && !strings.Contains(lower, "aws") && !strings.Contains(lower, "[") {
		t.Errorf("response doesn't seem relevant: %s", text[:min(200, len(text))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
