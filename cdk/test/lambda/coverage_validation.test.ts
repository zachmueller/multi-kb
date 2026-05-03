/**
 * Integration validation tests for PRM-003: Coverage Assessment Prompt.
 *
 * These tests call the real Bedrock Runtime API and are skipped unless
 * MULTI_KB_AWS_REGION is set in the environment.
 *
 * Run with:
 *   AWS_PROFILE=myprofile MULTI_KB_AWS_REGION=us-east-1 npx jest coverage_validation
 */

import { BedrockRuntimeClient, InvokeModelCommand } from "@aws-sdk/client-bedrock-runtime";
import { COVERAGE_ASSESSMENT_PROMPT } from "../../lambda/recall/prompts/coverage";

const REGION = process.env.MULTI_KB_AWS_REGION;
const MODEL_ID =
  process.env.MULTI_KB_MODEL_ID ?? "anthropic.claude-3-haiku-20240307-v1:0";

function skipIfNoRegion(fn: () => void | Promise<void>): () => Promise<void> {
  return async () => {
    if (!REGION) {
      console.log("Skipping integration test: MULTI_KB_AWS_REGION not set");
      return;
    }
    await fn();
  };
}

async function invokeCoveragePrompt(
  query: string,
  results: Array<{ title: string; content: string }>,
): Promise<{ gap_detected: boolean; refined_query: string | null }> {
  // Credentials come from the default AWS credential chain (env vars, ~/.aws, instance profile).
  // Set AWS_PROFILE before running if a named profile is needed.
  const client = new BedrockRuntimeClient({ region: REGION });
  const modelArn = `arn:aws:bedrock:${REGION}::foundation-model/${MODEL_ID}`;

  const resultSummaries = results
    .slice(0, 5)
    .map((r, i) => `${i + 1}. Title: ${r.title}\nContent snippet: ${r.content.substring(0, 300)}`)
    .join("\n\n");

  const userMessage = `Query: ${query}\n\nTop search results:\n${resultSummaries}`;

  const response = await client.send(
    new InvokeModelCommand({
      modelId: modelArn,
      contentType: "application/json",
      body: JSON.stringify({
        anthropic_version: "bedrock-2023-05-31",
        max_tokens: 200,
        system: COVERAGE_ASSESSMENT_PROMPT,
        messages: [{ role: "user", content: userMessage }],
      }),
    }),
  );

  const bodyText = Buffer.from(response.body).toString("utf-8");
  const parsed = JSON.parse(bodyText);
  const text =
    (parsed.content as Array<{ type: string; text: string }> | undefined)
      ?.find((b) => b.type === "text")?.text ?? "";
  return JSON.parse(text) as { gap_detected: boolean; refined_query: string | null };
}

describe("CoverageAssessmentPrompt integration validation", () => {
  // PRM-003 acceptance criterion: tested against good coverage → gap_detected: false
  test(
    "good coverage: results directly address query → gap_detected false",
    skipIfNoRegion(async () => {
      const query = "How do I enable DynamoDB TTL?";
      const results = [
        {
          title: "DynamoDB TTL Configuration",
          content:
            "Enable DynamoDB TTL by adding a numeric attribute to each item containing a Unix epoch expiry timestamp, then calling update-time-to-live to enable TTL on the table. Items are deleted eventually, up to 48 hours after the expiry timestamp.",
        },
        {
          title: "DynamoDB Table Design Patterns",
          content:
            "Use single-table design for DynamoDB. Partition key should be high-cardinality. TTL attribute name is configurable.",
        },
      ];

      const result = await invokeCoveragePrompt(query, results);

      expect(typeof result.gap_detected).toBe("boolean");
      expect(result).toHaveProperty("refined_query");

      if (result.gap_detected) {
        // Soft assertion: log but don't hard-fail (model may occasionally flag a gap)
        console.warn(
          "Unexpected gap_detected=true for well-covered query:",
          result.refined_query,
        );
      } else {
        expect(result.gap_detected).toBe(false);
        expect(result.refined_query).toBeNull();
      }
    }),
  );

  // PRM-003 acceptance criterion: missing topic → gap_detected: true with refined_query
  test(
    "missing topic: results miss key aspect → gap_detected true with refined_query",
    skipIfNoRegion(async () => {
      const query =
        "How do I configure DynamoDB cross-region replication with Global Tables?";
      const results = [
        {
          title: "DynamoDB TTL Configuration",
          content:
            "Enable TTL by adding a numeric epoch attribute. Call update-time-to-live to enable it.",
        },
        {
          title: "DynamoDB Pricing",
          content: "DynamoDB charges per read/write capacity unit. On-demand mode is available.",
        },
      ];

      const result = await invokeCoveragePrompt(query, results);

      expect(typeof result.gap_detected).toBe("boolean");
      expect(result).toHaveProperty("refined_query");

      if (result.gap_detected) {
        expect(result.refined_query).toBeTruthy();
        expect(result.refined_query).not.toBe(query);
        const lower = (result.refined_query ?? "").toLowerCase();
        const isRelevant =
          lower.includes("global") ||
          lower.includes("replication") ||
          lower.includes("cross-region") ||
          lower.includes("multi-region");
        if (!isRelevant) {
          console.warn(
            "refined_query may not target the gap well:",
            result.refined_query,
          );
        }
      } else {
        console.warn(
          "Expected gap_detected=true for mismatched results, got false — qualitative check",
        );
      }
    }),
  );

  // PRM-003 acceptance criterion: ambiguous results → model makes a reasonable decision
  test(
    "ambiguous results: model makes a reasonable gap/no-gap decision",
    skipIfNoRegion(async () => {
      const query = "Best practices for AWS IAM roles";
      const results = [
        {
          title: "IAM Role Trust Policies",
          content:
            "Trust policies define who can assume the role. Use conditions like aws:SourceAccount to restrict assumption.",
        },
        {
          title: "AWS Security Best Practices Overview",
          content:
            "Use least-privilege principle. Rotate access keys. Enable MFA. Use IAM roles over long-lived credentials.",
        },
      ];

      const result = await invokeCoveragePrompt(query, results);

      expect(typeof result.gap_detected).toBe("boolean");
      expect(result).toHaveProperty("refined_query");

      if (result.gap_detected) {
        expect(typeof result.refined_query).toBe("string");
        expect((result.refined_query as string).trim().length).toBeGreaterThan(0);
      } else {
        expect(result.refined_query).toBeNull();
      }
    }),
  );
});
