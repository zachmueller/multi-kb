# Contract: submitKnowledge Client

**Source:** CLI spec FR-3 + CDK spec FR-1, FR-2

## Endpoint

```
POST {endpoint}/submitKnowledge
```

Where `{endpoint}` is the `knowledge_bases[].endpoint` value from `config.yaml`.

## Authentication

SigV4-signed request using the configured `aws_profile` (for `iam` auth) or direct call (for `federate` auth, where the backend handles authentication transparently).

## Request

**Content-Type:** `application/json`

```json
{
  "title": "How to configure multi-region replication",
  "content": "## Overview\nWhen configuring multi-region replication...",
  "author": "zmueller"
}
```

**Field Constraints:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `title` | string | yes | Non-empty, ≤255 characters |
| `content` | string | yes | Non-empty, ≤100,000 characters |
| `author` | string | yes | Non-empty, ≤100 characters |

## Responses

### Success: HTTP 202 Accepted

```json
{
  "uid": "01H5K9QZXNM8V3PW",
  "request_id": "abc123-def456"
}
```

The returned `uid` is not stored or tracked by the CLI in MVP (fire-and-forget).

### Validation Error: HTTP 400

```json
{
  "errors": {
    "title": "must be present and non-empty",
    "content": "must not exceed 100,000 characters"
  }
}
```

A flat object keyed by field name. Only fields that failed validation appear.

**CLI behavior on 400:** Pass the error details and original note to the extraction LLM with a correction prompt. Retry submission with corrected output (up to 2 correction attempts). On persistent failure, stage the note in the pending queue for manual review.

### Auth Error: HTTP 401 / 403

**CLI behavior:** Log the error, skip remaining submissions to this KB for the current run, surface a message guiding the user to refresh credentials (e.g., `aws sso login --profile <profile>`).

### Server Error: HTTP 5xx / Network Error

**CLI behavior:** Retry up to 3 times with exponential backoff. On persistent failure, log the error and continue with remaining notes.

## Rate Limiting

CLI self-throttles to a maximum of 10 requests per second per target KB.

## Pre-flight Validation

The CLI validates field constraints locally before sending the request to surface friendly error messages without a network round-trip. This mirrors the CDK Lambda's validation (CDK FR-2).
