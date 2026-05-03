//go:build integration

package prompts_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zmueller/multi-kb/internal/extract/prompts"
)

// TestChunkSummarizationPrompt_LongConversation verifies the LLM produces a structured,
// coherent summary of a long conversation chunk that preserves key technical details.
func TestChunkSummarizationPrompt_LongConversation(t *testing.T) {
	client := makeClient(t) // reuses makeClient defined in extraction_validation_test.go (same package)

	// A realistic multi-turn conversation about infrastructure setup with technical decisions
	chunk := strings.Join([]string{
		"User: We need to set up a new VPC for our data platform. We're using us-east-1 and need private subnets for RDS and ECS, plus public subnets for the ALB.",
		"",
		"Assistant: Here's the VPC design for your data platform:",
		"- CIDR: 10.20.0.0/16",
		"- 3 public subnets (10.20.0.0/24, 10.20.1.0/24, 10.20.2.0/24) — one per AZ for ALB",
		"- 3 private subnets (10.20.10.0/24, 10.20.11.0/24, 10.20.12.0/24) — ECS tasks",
		"- 3 isolated subnets (10.20.20.0/24, 10.20.21.0/24, 10.20.22.0/24) — RDS (no NAT gateway route)",
		"- 1 NAT gateway in the first AZ (cost optimization — single NAT is fine for non-prod)",
		"",
		"User: Should we use NAT gateway or NAT instance for cost savings?",
		"",
		"Assistant: For production, use NAT Gateway — managed, highly available, no patching. For dev/staging where cost matters, a NAT instance (t3.nano) saves ~$32/month per AZ. If you go NAT instance, use the official Amazon NAT AMI and add a health check script. Key gotcha: NAT instances require disabling source/destination check on the ENI.",
		"",
		"User: OK, we'll use NAT Gateway for prod. What about security groups for RDS?",
		"",
		"Assistant: RDS security group should only allow inbound from the ECS task security group on port 5432 (PostgreSQL). Never open RDS to 0.0.0.0/0. Pattern:",
		"- rdsSG.addIngressRule(ecsTaskSG, Port.tcp(5432))",
		"- No outbound rules needed (RDS doesn't initiate connections)",
		"Also place RDS in the isolated subnets (no route to NAT gateway) for defense in depth.",
		"",
		"User: We also need S3 access from ECS tasks. Should we use NAT or an S3 endpoint?",
		"",
		"Assistant: Use a Gateway VPC Endpoint for S3 — it's free and routes S3 traffic within AWS's network, bypassing NAT entirely. This saves significant NAT data processing costs for high-volume S3 access. Add it in CDK: vpc.addGatewayEndpoint('S3', { service: GatewayVpcEndpointAwsService.S3 }). It automatically adds routes to all your route tables.",
		"",
		"User: Great. One more thing — we need to enable VPC Flow Logs for compliance.",
		"",
		"Assistant: Enable VPC Flow Logs to CloudWatch Logs with a 30-day retention policy. Key decisions made:",
		"1. Log destination: CloudWatch Logs (easier querying than S3 for security team)",
		"2. Retention: 30 days (compliance minimum; adjust if auditors require more)",
		"3. Traffic type: ALL (captures both ACCEPT and REJECT — important for security analysis)",
		"4. IAM role: create a dedicated role with logs:CreateLogGroup, logs:CreateLogStream, logs:PutLogEvents",
		"Cost note: Flow Logs to CloudWatch cost ~$0.50/GB ingested. High-traffic VPCs can get expensive — consider sampling or S3 destination for cost optimization.",
	}, "\n")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	summary, err := client.InvokeModel(ctx, prompts.ChunkSummarizationPrompt, chunk)
	if err != nil {
		if strings.Contains(err.Error(), "access denied") {
			t.Skipf("skipping: model access denied: %v", err)
		}
		t.Fatalf("InvokeModel: %v", err)
	}

	if strings.TrimSpace(summary) == "" {
		t.Fatal("expected non-empty summary")
	}

	// Summary should be structured — check for at least one heading or bullet
	hasStructure := strings.Contains(summary, "#") || strings.Contains(summary, "- ") || strings.Contains(summary, "* ")
	if !hasStructure {
		t.Errorf("expected structured summary with headings or bullets; got plain prose: %s", summary[:minSumLen(300, len(summary))])
	}

	// Summary must preserve key technical decisions
	lower := strings.ToLower(summary)
	expectedTopics := []string{"vpc", "nat", "rds", "s3", "subnet"}
	for _, topic := range expectedTopics {
		if !strings.Contains(lower, topic) {
			t.Errorf("summary missing key topic %q; summary (first 500 chars): %s", topic, summary[:minSumLen(500, len(summary))])
		}
	}

	// Summary should not be trivially short — must contain enough context for subsequent chunks
	if len(summary) < 200 {
		t.Errorf("summary too short (%d chars) to carry meaningful context forward", len(summary))
	}
}

func minSumLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
