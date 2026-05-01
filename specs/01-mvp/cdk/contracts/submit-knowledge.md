# Contract: submitKnowledge Endpoint (Server Side)

**Source:** CDK spec FR-1, FR-2, FR-3
**Client Contract:** [CLI submitKnowledge contract](../../cli/contracts/submit-knowledge.md)

## Endpoint

```
POST /submitKnowledge
```

Deployed on API Gateway REST API with `prod` stage. `AWS_IAM` authorization (SigV4).

## Lambda Handler

**Runtime:** Node.js 22 (`nodejs22.x`), ARM64 (Graviton)
**Memory:** 256 MB
**Timeout:** 10 seconds

### Request Processing

1. Parse JSON body from API Gateway Lambda proxy event (`event.body`)
2. Validate fields:
   - `title`: present, non-empty string, ≤255 characters
   - `content`: present, non-empty string, ≤100,000 characters
   - `author`: present, non-empty string, ≤100 characters
3. On validation failure → return HTTP 400 with error details
4. Generate UID: 16-character Crockford base32 (10 bytes from `crypto.randomBytes`)
5. Generate `submitted_at`: ISO 8601 timestamp (current time)
6. Enqueue SQS message: `{ uid, title, content, author, submitted_at }`
7. Return HTTP 202: `{ uid, request_id }`

### Response Format (Lambda Proxy Integration)

**Success (HTTP 202):**

```javascript
{
  statusCode: 202,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    uid: "01H5K9QZXNM8V3PW",
    request_id: event.requestContext.requestId
  })
}
```

**Validation Error (HTTP 400):**

```javascript
{
  statusCode: 400,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    errors: {
      title: "must be present and non-empty",
      content: "must not exceed 100,000 characters"
    }
  })
}
```

Only fields that failed validation appear in the `errors` object.

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

Logged to CloudWatch with full error details; client receives generic message.

### Validation Rules

| Field | Rule | Error Message |
|-------|------|---------------|
| `title` | Missing or not a string | `"must be present and non-empty"` |
| `title` | Empty string or whitespace-only | `"must be present and non-empty"` |
| `title` | Length > 255 | `"must not exceed 255 characters"` |
| `content` | Missing or not a string | `"must be present and non-empty"` |
| `content` | Empty string or whitespace-only | `"must be present and non-empty"` |
| `content` | Length > 100,000 | `"must not exceed 100,000 characters"` |
| `author` | Missing or not a string | `"must be present and non-empty"` |
| `author` | Empty string or whitespace-only | `"must be present and non-empty"` |
| `author` | Length > 100 | `"must not exceed 100 characters"` |

### SQS Message

```json
{
  "uid": "01H5K9QZXNM8V3PW",
  "title": "Note title (validated)",
  "content": "Full content (validated)",
  "author": "submitter-identity (validated)",
  "submitted_at": "2026-05-01T10:30:00Z"
}
```

Message body is JSON-serialized. No message attributes.

### IAM Permissions

| Permission | Resource | Purpose |
|------------|----------|---------|
| `sqs:SendMessage` | Queue ARN | Enqueue validated message |

### Environment Variables

| Variable | Source | Description |
|----------|--------|-------------|
| `SQS_QUEUE_URL` | CDK construct | SQS queue URL for message enqueue |
