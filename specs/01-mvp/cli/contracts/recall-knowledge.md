# Contract: recallKnowledge Client

**Source:** CLI spec FR-7 + CDK spec FR-1, FR-9

## Endpoint

```
POST {endpoint}/recallKnowledge
```

Where `{endpoint}` is the `knowledge_bases[].endpoint` value from `config.yaml`.

## Authentication

SigV4-signed request using the configured `aws_profile` (for `iam` auth) or direct call (for `federate` auth).

## Request

**Content-Type:** `application/json`

```json
{
  "query": "How do I configure multi-region replication for DynamoDB?",
  "limit": 10
}
```

**Field Constraints:**

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `query` | string | yes | — | Non-empty |
| `limit` | integer | no | 10 | Positive integer, 1–100 |

For hook-based injection, the `query` is the user's first message verbatim.

## Response

### Success: HTTP 200

```json
[
  {
    "uid": "01H5K9QZXNM8V3PW",
    "title": "DynamoDB Global Tables Configuration",
    "content": "## Overview\nGlobal Tables provide...",
    "score": 0.87
  },
  {
    "uid": "01H5KABCDEF12345",
    "title": "Cross-Region Replication Patterns",
    "content": "## Replication Strategies\n...",
    "score": 0.72
  }
]
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `uid` | string | 16-char Crockford base32 UID |
| `title` | string | Note title |
| `content` | string | Full Markdown content |
| `score` | number | Relevance score (0.0–1.0), descending order |

Results are pre-sorted by descending `score`. Only `status: active` notes are returned (by default).

## CLI Usage Context

### Hook Injection (FR-7)

1. All target KBs for the current directory are queried **concurrently**
2. Remote results are sorted by descending `score`
3. Local KB results (from `git grep`) are sorted by descending match count
4. Results from all KBs are merged via **rank-based interleaving** (top-ranked from each KB first, then second-ranked, etc.)
5. Top 10 notes selected from merged results
6. Formatted as Markdown and written to stdout

### Error Responses

- **HTTP 400:** `{ "errors": { "query": "must be present and non-empty" } }` — returned when `query` is missing or empty. The CLI should log a warning and treat as empty results (no injection). Do not retry.
- **HTTP 401/403:** Authentication or authorization failure. Log error, skip this KB for the remainder of the hook invocation.
- **HTTP 5xx:** Server error. Retry per standard retry policy; on exhaustion, treat as empty results from this KB.

### Timeout Handling

- Configurable timeout (default: 8 seconds) covers the entire injection path
- Partial results from responsive KBs are used if other KBs time out
- If no KBs respond, conversation proceeds with no injection; warning logged to `hook-errors.jsonl`
