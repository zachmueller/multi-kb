# Contract: recallKnowledge Endpoint (Server Side)

**Source:** CDK spec FR-1, FR-9
**Client Contract:** [CLI recallKnowledge contract](../../cli/contracts/recall-knowledge.md)

## Endpoint

```
POST /recallKnowledge
```

Deployed on API Gateway REST API with `prod` stage. `AWS_IAM` authorization (SigV4).

## Lambda Handler

**Runtime:** Node.js 22 (`nodejs22.x`), ARM64 (Graviton)
**Memory:** 1024 MB
**Timeout:** 30 seconds

### Request Processing

1. Parse JSON body from API Gateway Lambda proxy event (`event.body`)
2. Extract `query` (required, string) and `limit` (optional, integer in range [1, 100], default 10)
3. Call Bedrock Knowledge Base Retrieve API with `query`
4. Optionally filter results to `status: active` notes only (if `EXCLUDE_PENDING` is `true`)
5. If top result score < `COVERAGE_SCORE_THRESHOLD`:
   a. Send query + result summaries to coverage assessment LLM (InvokeModel)
   b. If gap detected, execute follow-up Retrieve query
   c. Deduplicate combined results by UID
   d. Truncate to `limit`
   e. On any failure in coverage step, fall back to original results silently
6. Write recall log to S3 (synchronous, best-effort)
7. Return results array

### Response Format (Lambda Proxy Integration)

**Success (HTTP 200):**

```javascript
{
  statusCode: 200,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify([
    {
      uid: "01H5K9QZXNM8V3PW",
      title: "DynamoDB Global Tables Configuration",
      content: "## Overview\nGlobal Tables provide...",
      score: 0.87
    },
    {
      uid: "01H5KABCDEF12345",
      title: "Cross-Region Replication Patterns",
      content: "## Replication Strategies\n...",
      score: 0.72
    }
  ])
}
```

Results sorted by descending `score`. Array length ≤ `limit`.

**Empty Results (HTTP 200):**

```javascript
{
  statusCode: 200,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify([])
}
```

**Validation Error (HTTP 400):**

```javascript
// Missing or empty query
{
  statusCode: 400,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    errors: {
      query: "must be present and non-empty"
    }
  })
}

// Out-of-range limit
{
  statusCode: 400,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    errors: {
      limit: "must be an integer between 1 and 100"
    }
  })
}
```

**Internal Error (HTTP 500):**

```javascript
{
  statusCode: 500,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    message: "Internal server error"
  })
}
```

### Coverage Assessment Flow

```
[Initial Retrieve results]
         │
         ├── top score ≥ threshold → return results (no coverage step)
         │
         └── top score < threshold
               │
               ├── InvokeModel (coverage LLM):
               │     Input: query + result summaries
               │     Output: gap detected? + optional refined query
               │
               ├── gap detected = false → return original results
               │
               └── gap detected = true
                     │
                     ├── Follow-up Retrieve with refined query
                     ├── Deduplicate by UID (original ∪ follow-up)
                     ├── Sort by score descending
                     ├── Truncate to limit
                     └── Return merged results
```

On any failure in the coverage step (LLM error, follow-up Retrieve error), silently fall back to original results.

### Recall Log Writing

After computing results, before returning:

```javascript
// Best-effort — failure doesn't affect response
const recallLog = {
  timestamp: new Date().toISOString(),
  query: query,
  recalled_uids: results.map(r => r.uid)
};

await s3.putObject({
  Bucket: BUCKET_NAME,
  Key: `recall-logs/${dateStr}/${requestId}.json`, // dateStr = UTC date (YYYY-MM-DD)
  Body: JSON.stringify(recallLog),
  ContentType: 'application/json'
});
```

S3 write adds ~20-50ms. On failure, log error and continue.

### Bedrock Retrieve API Call

```javascript
const response = await bedrockAgent.retrieve({
  knowledgeBaseId: KNOWLEDGE_BASE_ID,
  retrievalQuery: { text: query },
  retrievalConfiguration: {
    vectorSearchConfiguration: {
      numberOfResults: limit,
      // Optional: filter by status
      filter: excludePending ? {
        equals: { key: "status", value: "active" }
      } : undefined
    }
  }
});
```

### Response Field Mapping

| Retrieve API Field | Response Field | Transformation |
|-------------------|----------------|----------------|
| `retrievalResults[].content.text` | `content` | Raw Markdown content |
| `retrievalResults[].score` | `score` | Relevance score (0.0–1.0) |
| `retrievalResults[].metadata.uid` | `uid` | From note frontmatter |
| `retrievalResults[].metadata.title` | `title` | From note frontmatter |

**ASSUMPTION (verify in Phase 0):** Bedrock KB extracts YAML frontmatter fields as queryable metadata when indexing Markdown files from S3. The `uid` and `title` fields must appear in `retrievalResults[].metadata` for this mapping to work. If Bedrock does not extract frontmatter as metadata, a fallback approach is needed (e.g., parse the S3 object directly to extract frontmatter). A Phase 0 verification task must deploy a minimal KB, submit a note with frontmatter, call Retrieve, and confirm these field paths.

### IAM Permissions

| Permission | Resource | Purpose |
|------------|----------|---------|
| `bedrock:Retrieve` | Knowledge Base ARN | Semantic search |
| `bedrock:InvokeModel` | Coverage model ARN | Coverage assessment LLM call |
| `s3:PutObject` | `{bucket-arn}/recall-logs/*` | Write recall logs |

### Environment Variables

| Variable | Source | Description |
|----------|--------|-------------|
| `KNOWLEDGE_BASE_ID` | CDK construct | Bedrock KB ID for Retrieve API |
| `BUCKET_NAME` | CDK construct | S3 bucket for recall logs |
| `COVERAGE_MODEL_ID` | CDK context | LLM model for coverage assessment (short-form model ID) |
| `COVERAGE_SCORE_THRESHOLD` | CDK context | Score threshold for coverage (default: `0.3`) |
| `EXCLUDE_PENDING` | CDK context | Whether to filter pending notes (default: `true`) |

### Coverage Model ARN Construction

The Lambda constructs the Bedrock model ARN at runtime from environment variables:

```
arn:aws:bedrock:${AWS_REGION}::foundation-model/${COVERAGE_MODEL_ID}
```

- The account ID field is empty (`::`) because foundation models are AWS-owned resources
- Example: given `COVERAGE_MODEL_ID=anthropic.claude-haiku-3-20240307` and `AWS_REGION=us-east-1`, the ARN is `arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-haiku-3-20240307`
- IAM permission `bedrock:InvokeModel` must be granted on this constructed ARN
