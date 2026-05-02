import {
  BedrockAgentRuntimeClient,
  RetrieveCommand,
} from "@aws-sdk/client-bedrock-agent-runtime";
import {
  BedrockRuntimeClient,
  InvokeModelCommand,
} from "@aws-sdk/client-bedrock-runtime";
import { S3Client, PutObjectCommand } from "@aws-sdk/client-s3";
import type {
  APIGatewayProxyEvent,
  APIGatewayProxyResult,
} from "aws-lambda";
import { internalError, success, validationError } from "../shared/response";
import { COVERAGE_ASSESSMENT_PROMPT } from "./prompts/coverage";

const bedrockAgent = new BedrockAgentRuntimeClient({});
const bedrockRuntime = new BedrockRuntimeClient({});
const s3 = new S3Client({});

interface RecallResult {
  uid: string;
  title: string;
  content: string;
  score: number;
}

interface CoverageResponse {
  gap_detected: boolean;
  refined_query: string | null;
}

async function retrieveFromKb(
  query: string,
  limit: number,
  excludePending: boolean,
): Promise<RecallResult[]> {
  const knowledgeBaseId = process.env.KNOWLEDGE_BASE_ID!;

  const response = await bedrockAgent.send(
    new RetrieveCommand({
      knowledgeBaseId,
      retrievalQuery: { text: query },
      retrievalConfiguration: {
        vectorSearchConfiguration: {
          numberOfResults: limit,
          filter: excludePending
            ? { equals: { key: "status", value: "active" } }
            : undefined,
        },
      },
    }),
  );

  const results: RecallResult[] = [];
  for (const r of response.retrievalResults ?? []) {
    const uid = (r.metadata?.["uid"] as string) ?? "";
    const title = (r.metadata?.["title"] as string) ?? "";
    const content = r.content?.text ?? "";
    const score = r.score ?? 0;
    results.push({ uid, title, content, score });
  }

  // Sort by descending score
  return results.sort((a, b) => b.score - a.score);
}

async function assessCoverage(
  query: string,
  results: RecallResult[],
): Promise<CoverageResponse> {
  const coverageModelId = process.env.COVERAGE_MODEL_ID!;
  const region = process.env.AWS_REGION ?? "us-east-1";
  const modelArn = `arn:aws:bedrock:${region}::foundation-model/${coverageModelId}`;

  const resultSummaries = results
    .slice(0, 5)
    .map(
      (r, i) =>
        `${i + 1}. Title: ${r.title}\nContent snippet: ${r.content.substring(0, 300)}`,
    )
    .join("\n\n");

  const userMessage = `Query: ${query}\n\nTop search results:\n${resultSummaries}`;

  const response = await bedrockRuntime.send(
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
  const text = parsed.content?.find((b: { type: string }) => b.type === "text")?.text ?? "";
  return JSON.parse(text) as CoverageResponse;
}

async function writeRecallLog(
  requestId: string,
  query: string,
  results: RecallResult[],
): Promise<void> {
  const bucketName = process.env.BUCKET_NAME!;
  const dateStr = new Date().toISOString().substring(0, 10); // YYYY-MM-DD UTC
  const key = `recall-logs/${dateStr}/${requestId}.json`;

  const recallLog = {
    timestamp: new Date().toISOString(),
    query,
    recalled_uids: results.map((r) => r.uid),
  };

  await s3.send(
    new PutObjectCommand({
      Bucket: bucketName,
      Key: key,
      Body: JSON.stringify(recallLog),
      ContentType: "application/json",
    }),
  );
}

export const handler = async (
  event: APIGatewayProxyEvent,
): Promise<APIGatewayProxyResult> => {
  try {
    let body: Record<string, unknown>;
    try {
      body = JSON.parse(event.body ?? "{}");
    } catch {
      return validationError({ body: "invalid JSON" });
    }

    // Validate query
    if (
      typeof body.query !== "string" ||
      body.query.trim().length === 0
    ) {
      return validationError({ query: "must be present and non-empty" });
    }
    const query = body.query;

    // Validate limit
    let limit = 10;
    if (body.limit !== undefined) {
      if (!Number.isInteger(body.limit) || typeof body.limit !== "number") {
        return validationError({
          limit: "must be an integer between 1 and 100",
        });
      }
      limit = body.limit as number;
      if (limit < 1 || limit > 100) {
        return validationError({
          limit: "must be an integer between 1 and 100",
        });
      }
    }

    const excludePending = process.env.EXCLUDE_PENDING === "true";
    const coverageThreshold = parseFloat(
      process.env.COVERAGE_SCORE_THRESHOLD ?? "0.3",
    );

    // Initial retrieval
    let results = await retrieveFromKb(query, limit, excludePending);

    // Coverage assessment if top score is below threshold
    const topScore = results[0]?.score ?? 0;
    if (results.length > 0 && topScore < coverageThreshold) {
      try {
        const coverage = await assessCoverage(query, results);
        if (coverage.gap_detected && coverage.refined_query) {
          const followUp = await retrieveFromKb(
            coverage.refined_query,
            limit,
            excludePending,
          );

          // Merge: deduplicate by UID, sort by score, truncate to limit
          const seenUids = new Set(results.map((r) => r.uid));
          for (const r of followUp) {
            if (!seenUids.has(r.uid)) {
              results.push(r);
              seenUids.add(r.uid);
            }
          }
          results.sort((a, b) => b.score - a.score);
          results = results.slice(0, limit);
        }
      } catch (coverageErr) {
        // Coverage assessment failure is silent — fall back to original results
        console.warn("Coverage assessment failed (falling back):", coverageErr);
      }
    }

    // Write recall log (best-effort — failure doesn't affect response)
    try {
      await writeRecallLog(event.requestContext.requestId, query, results);
    } catch (logErr) {
      console.warn("Failed to write recall log:", logErr);
    }

    return success(200, results);
  } catch (err) {
    console.error("recallKnowledge error:", err);
    return internalError();
  }
};
