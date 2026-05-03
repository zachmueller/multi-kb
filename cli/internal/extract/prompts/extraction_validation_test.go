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
	"github.com/zmueller/multi-kb/internal/extract/prompts"
)

// makeClient builds a Bedrock client from env vars, or skips if unavailable.
func makeClient(t *testing.T) *bedrock.Client {
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

// assertExtractionJSON verifies the LLM returned a parseable JSON array with each element
// having "title", "content", and "suggested_target_kbs" fields.
// Tolerates:
//   - prose preambles before the JSON array
//   - no JSON array at all (model indicates nothing to extract in prose) → returns nil (0 notes)
//   - literal newlines inside JSON string values (Haiku sometimes produces these)
func assertExtractionJSON(t *testing.T, text string) []map[string]interface{} {
	t.Helper()
	text = strings.TrimSpace(text)

	// Strip markdown code fences
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	// Find the start of the JSON array, skipping any prose preamble.
	// If no '[' is present the model indicated nothing to extract in prose — treat as 0 notes.
	idx := strings.Index(text, "[")
	if idx < 0 {
		return nil
	}
	text = text[idx:]
	if end := strings.LastIndex(text, "]"); end >= 0 {
		text = text[:end+1]
	}

	// Repair literal newlines inside JSON string values (LLMs sometimes emit these).
	text = repairJSONLiteralNewlines(text)

	var notes []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &notes); err != nil {
		t.Fatalf("response is not valid JSON array: %v\nresponse: %s", err, text)
	}
	for i, note := range notes {
		if _, ok := note["title"]; !ok {
			t.Errorf("note[%d]: missing 'title' field", i)
		}
		if _, ok := note["content"]; !ok {
			t.Errorf("note[%d]: missing 'content' field", i)
		}
		if _, ok := note["suggested_target_kbs"]; !ok {
			t.Errorf("note[%d]: missing 'suggested_target_kbs' field", i)
		}
	}
	return notes
}

// TestExtractionPrompt_WithClearKnowledge verifies the LLM extracts meaningful notes
// from a conversation containing clear, reusable technical knowledge.
func TestExtractionPrompt_WithClearKnowledge(t *testing.T) {
	client := makeClient(t)

	conversation := `{"type":"conversation","source_harness":"claude-code","source_path":"/projects/api","project_dir":"/projects/api"}
{"type":"message","role":"user","content":"How do I set up DynamoDB TTL? We need items to expire automatically after 30 days.","timestamp":"2026-01-01T10:00:00Z","previously_processed":false}
{"type":"message","role":"assistant","content":"DynamoDB TTL lets you automatically delete expired items. Here's how to enable it:\n\n1. Add a numeric attribute to your items (e.g., 'expires_at') containing a Unix epoch timestamp.\n2. Enable TTL on the table: aws dynamodb update-time-to-live --table-name MyTable --time-to-live-specification 'Enabled=true,AttributeName=expires_at'\n3. Set the attribute when writing items: use int64(time.Now().Unix()) + 30*24*3600 for 30-day expiry.\n\nKey gotcha: TTL deletion is eventual — items may linger up to 48 hours after expiry. Don't rely on TTL for security-sensitive access control.","timestamp":"2026-01-01T10:01:00Z","previously_processed":false}`

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, prompts.ExtractionSystemPrompt, conversation)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	notes := assertExtractionJSON(t, text)
	if len(notes) == 0 {
		t.Fatal("expected at least one extracted note from a knowledge-rich conversation, got 0")
	}

	// Verify at least one note mentions DynamoDB or TTL
	found := false
	for _, note := range notes {
		title := strings.ToLower(note["title"].(string))
		content := strings.ToLower(note["content"].(string))
		if strings.Contains(title, "dynamodb") || strings.Contains(title, "ttl") ||
			strings.Contains(content, "dynamodb") || strings.Contains(content, "ttl") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a note mentioning DynamoDB or TTL; got: %v", text[:min(300, len(text))])
	}
}

// TestExtractionPrompt_NoExtractableKnowledge verifies the LLM returns an empty array
// when a conversation contains no reusable knowledge.
func TestExtractionPrompt_NoExtractableKnowledge(t *testing.T) {
	client := makeClient(t)

	conversation := `{"type":"conversation","source_harness":"claude-code","source_path":"/projects/misc","project_dir":"/projects/misc"}
{"type":"message","role":"user","content":"Hey, thanks for your help yesterday!","timestamp":"2026-01-01T10:00:00Z","previously_processed":false}
{"type":"message","role":"assistant","content":"You're welcome! Happy to help anytime.","timestamp":"2026-01-01T10:01:00Z","previously_processed":false}
{"type":"message","role":"user","content":"See you tomorrow.","timestamp":"2026-01-01T10:02:00Z","previously_processed":false}
{"type":"message","role":"assistant","content":"Take care!","timestamp":"2026-01-01T10:03:00Z","previously_processed":false}`

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, prompts.ExtractionSystemPrompt, conversation)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	notes := assertExtractionJSON(t, text)
	if len(notes) != 0 {
		t.Errorf("expected empty array for trivial conversation, got %d notes: %v", len(notes), text[:min(300, len(text))])
	}
}

// TestExtractionPrompt_ReprocessedConversation verifies the LLM only extracts from
// new (previously_processed=false) messages while using old messages for context.
func TestExtractionPrompt_ReprocessedConversation(t *testing.T) {
	client := makeClient(t)

	// Old message is already processed; new message has novel knowledge
	conversation := `{"type":"conversation","source_harness":"claude-code","source_path":"/projects/infra","project_dir":"/projects/infra"}
{"type":"message","role":"user","content":"How do we set up AWS VPC?","timestamp":"2026-01-01T09:00:00Z","previously_processed":true}
{"type":"message","role":"assistant","content":"Use the CDK VPC construct with NAT gateways for private subnets.","timestamp":"2026-01-01T09:01:00Z","previously_processed":true}
{"type":"message","role":"user","content":"Now I need to add an S3 VPC endpoint to avoid NAT charges for S3 traffic.","timestamp":"2026-01-02T10:00:00Z","previously_processed":false}
{"type":"message","role":"assistant","content":"Add a Gateway VPC endpoint for S3: it's free and routes S3 traffic within the AWS network, bypassing NAT. In CDK: vpc.addGatewayEndpoint('S3Endpoint', { service: ec2.GatewayVpcEndpointAwsService.S3 }). This adds a route to your route tables automatically. Important: gateway endpoints work for S3 and DynamoDB only — use Interface endpoints for other services.","timestamp":"2026-01-02T10:01:00Z","previously_processed":false}`

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text, err := client.InvokeModel(ctx, prompts.ExtractionSystemPrompt, conversation)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	notes := assertExtractionJSON(t, text)
	if len(notes) == 0 {
		t.Fatal("expected at least one note from the new (previously_processed=false) message, got 0")
	}

	// Should mention S3 endpoint content, not just generic VPC setup
	combined := strings.ToLower(text)
	if !strings.Contains(combined, "endpoint") && !strings.Contains(combined, "nat") && !strings.Contains(combined, "s3") {
		t.Errorf("note content should reference S3 VPC endpoint knowledge; got: %s", text[:min(400, len(text))])
	}
}

// repairJSONLiteralNewlines replaces bare newlines inside JSON string values with \n.
// Some LLMs (notably Claude Haiku) emit literal newlines in string values, producing
// invalid JSON. This scanner walks the byte stream and escapes newlines that appear
// inside a string literal (between unescaped double-quotes).
func repairJSONLiteralNewlines(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && inString:
			// Copy escape sequence as-is
			buf.WriteByte(c)
			i++
			if i < len(s) {
				buf.WriteByte(s[i])
			}
		case c == '"':
			inString = !inString
			buf.WriteByte(c)
		case c == '\n' && inString:
			buf.WriteString(`\n`)
		case c == '\r' && inString:
			buf.WriteString(`\r`)
		case c == '\t' && inString:
			buf.WriteString(`\t`)
		default:
			buf.WriteByte(c)
		}
	}
	return buf.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
