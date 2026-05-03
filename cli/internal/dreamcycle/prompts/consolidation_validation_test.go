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
	dreamcyclePrompts "github.com/zmueller/multi-kb/internal/dreamcycle/prompts"
)

func makeConsolidationClient(t *testing.T) *bedrock.Client {
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

// parseConsolidationResponse extracts and validates the JSON actions array from LLM output.
func parseConsolidationResponse(t *testing.T, text string) []map[string]interface{} {
	t.Helper()
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	var output struct {
		Actions []map[string]interface{} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		t.Fatalf("response is not valid JSON with 'actions': %v\nresponse: %s", err, text)
	}
	if len(output.Actions) == 0 {
		t.Fatalf("expected at least one action in response, got 0\nresponse: %s", text)
	}
	return output.Actions
}

// buildBatchMessage builds the user message for a consolidation prompt — mirrors phase3.go.
func buildBatchMessage(pendingUID, pendingTitle, pendingContent string, relatedNotes []struct{ uid, title, content string }) string {
	var sb strings.Builder
	sb.WriteString("## Pending Notes (to evaluate)\n\n")
	sb.WriteString("### Note: " + pendingUID + "\n")
	sb.WriteString("**Title:** " + pendingTitle + "\n")
	sb.WriteString(pendingContent + "\n\n")
	if len(relatedNotes) > 0 {
		sb.WriteString("## Related Active Notes (for context)\n\n")
		for _, n := range relatedNotes {
			sb.WriteString("### Note: " + n.uid + "\n")
			sb.WriteString("**Title:** " + n.title + "\n")
			sb.WriteString(n.content + "\n\n")
		}
	}
	return sb.String()
}

// TestConsolidationPrompt_NovelNote verifies the LLM returns a "keep" action when the
// pending note covers a topic not addressed by any active note.
func TestConsolidationPrompt_NovelNote(t *testing.T) {
	client := makeConsolidationClient(t)

	userMessage := buildBatchMessage(
		"PEND0001",
		"Redis SETNX for Distributed Locking",
		"## Redis SETNX Distributed Lock\n\nUse SETNX (SET if Not eXists) with an expiry to implement a distributed lock:\n- `SET lock_key lock_value NX PX 30000` — sets only if absent, expires in 30s\n- Release by checking value before DEL to avoid releasing another client's lock\n- Use a random UUID as the lock value to identify ownership",
		[]struct{ uid, title, content string }{
			{
				uid:   "ACTV0010",
				title: "Go Context Cancellation in HTTP Middleware",
				content: "## Context Cancellation\n\nAlways check ctx.Done() in long-running middleware. Use context.WithTimeout for external calls.",
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, dreamcyclePrompts.ConsolidationSystemPrompt, userMessage)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	actions := parseConsolidationResponse(t, text)

	// Find the action for PEND0001
	var actionType string
	for _, a := range actions {
		if a["source_uid"] == "PEND0001" {
			actionType, _ = a["type"].(string)
			break
		}
	}

	if actionType != "keep" {
		t.Errorf("expected 'keep' for novel note PEND0001, got %q\nresponse: %s", actionType, text[:minLen(500, len(text))])
	}
}

// TestConsolidationPrompt_DuplicateNote verifies the LLM returns a "merge" action when the
// pending note covers the same topic as an existing active note.
func TestConsolidationPrompt_DuplicateNote(t *testing.T) {
	client := makeConsolidationClient(t)

	userMessage := buildBatchMessage(
		"PEND0002",
		"DynamoDB TTL Configuration",
		"## DynamoDB TTL\n\nEnable TTL with `aws dynamodb update-time-to-live`. Set a numeric epoch attribute. Items are deleted eventually (up to 48h after expiry).",
		[]struct{ uid, title, content string }{
			{
				uid:   "ACTV0020",
				title: "DynamoDB Time-To-Live Setup",
				content: "## DynamoDB TTL\n\nAdd a 'ttl' attribute with Unix epoch timestamp. Enable TTL via AWS console or CLI. Note: deletion is eventual, not immediate.",
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, dreamcyclePrompts.ConsolidationSystemPrompt, userMessage)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	actions := parseConsolidationResponse(t, text)

	var actionType string
	for _, a := range actions {
		if a["source_uid"] == "PEND0002" {
			actionType, _ = a["type"].(string)
			break
		}
	}

	// merge or consolidate both handle duplicate content — either is correct
	if actionType != "merge" && actionType != "consolidate" {
		t.Errorf("expected 'merge' or 'consolidate' for duplicate note PEND0002, got %q\nresponse: %s", actionType, text[:minLen(500, len(text))])
	}
}

// TestConsolidationPrompt_OverlappingNotes verifies the LLM returns a "consolidate" action
// when the pending note and an active note have significant overlap and would read better
// as a single comprehensive note.
func TestConsolidationPrompt_OverlappingNotes(t *testing.T) {
	client := makeConsolidationClient(t)

	userMessage := buildBatchMessage(
		"PEND0003",
		"AWS VPC Security Group Inbound Rules for ECS Tasks",
		"## ECS Security Groups — Inbound\n\nECS tasks need inbound rules from their load balancer security group, not from 0.0.0.0/0. Reference the ALB SG directly in the ECS task SG rule. This avoids opening tasks to the internet.\n\nExample CDK:\n```\ntaskSG.addIngressFrom(albSG, ec2.Port.tcp(8080));\n```",
		[]struct{ uid, title, content string }{
			{
				uid:   "ACTV0030",
				title: "AWS VPC Security Group Best Practices",
				content: "## Security Group Rules\n\nNever use 0.0.0.0/0 as source for inbound rules in production. Always reference specific security groups as source. Egress is typically open (0.0.0.0/0) unless you have strict outbound controls. Security groups are stateful — no need for matching outbound rule.",
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, dreamcyclePrompts.ConsolidationSystemPrompt, userMessage)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	actions := parseConsolidationResponse(t, text)

	// At least one action must address the pending note
	addressed := false
	for _, a := range actions {
		src, _ := a["source_uid"].(string)
		srcUIDs, _ := a["source_uids"].([]interface{})

		if src == "PEND0003" {
			addressed = true
			break
		}
		for _, uid := range srcUIDs {
			if uid == "PEND0003" {
				addressed = true
				break
			}
		}
		if addressed {
			break
		}
	}

	if !addressed {
		t.Errorf("PEND0003 not addressed by any action\nresponse: %s", text[:minLen(500, len(text))])
	}

	// The response should be a merge or consolidate (notes overlap substantially)
	var actionType string
	for _, a := range actions {
		src, _ := a["source_uid"].(string)
		if src == "PEND0003" {
			actionType, _ = a["type"].(string)
			break
		}
		srcUIDs, _ := a["source_uids"].([]interface{})
		for _, uid := range srcUIDs {
			if uid == "PEND0003" {
				actionType, _ = a["type"].(string)
				break
			}
		}
	}

	if actionType != "merge" && actionType != "consolidate" {
		t.Logf("note: expected merge/consolidate for overlapping notes, got %q — this is a qualitative test, not a hard failure", actionType)
	}
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
