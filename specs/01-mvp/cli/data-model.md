# Data Model: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Specification:** [spec.md](spec.md)
**Plan:** [plan.md](plan.md)

## Entities

### 1. Knowledge Note (Local KB)

**Storage:** Obsidian-flavor Markdown file in `~/.multi-kb/local/<kb-name>/<UID>.md`

**Frontmatter Schema (YAML):**

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `uid` | string | yes | 16-char Crockford base32 | Locally generated, immutable after creation |
| `title` | string | yes | ≤255 characters, non-empty | Succinct note title |
| `status` | enum | yes | `pending` \| `active` | Lifecycle state |
| `author` | string | yes | ≤100 characters, non-empty | From `config.yaml` `author` field |
| `last-updated` | ISO 8601 | yes | — | Last body content modification timestamp |
| `last-linked-to` | ISO 8601 | no | — | Not populated in MVP (schema parity with remote) |
| `last-recalled` | ISO 8601 | no | — | Not updated by local processes in MVP (schema parity) |
| `consolidated-from-notes` | string[] | no | Format: `<filename> (<commit-hash>)` | Populated by dream cycle merge/consolidate actions |

**Body:** Markdown content. May contain `[[UID|Title]]` wikilinks (inserted by dream cycle consolidation).

**State Transitions:**

```
[Created by extraction] → status: pending
         │
         ▼
[Dream cycle Phase 3]  → status: active   (keep, merge target, split, consolidate)
         │
         ▼
[Dream cycle merge]    → file deleted      (merged into another note)
```

**Validation Rules:**
- `uid` must be exactly 16 characters, valid Crockford base32 alphabet
- `title` must be present, non-empty, ≤255 characters
- `status` must be one of: `pending`, `active`
- `author` must be present, non-empty, ≤100 characters
- `last-updated` must be valid ISO 8601
- File name must match `<uid>.md`

### 2. Configuration (`config.yaml`)

**Storage:** `~/.multi-kb/config.yaml`

```yaml
# Top-level fields
mode: client                    # client | server
author: "zmueller"              # Identity for all submitKnowledge calls

# Remote KB definitions
knowledge_bases:
  - name: "my-team-kb"          # Unique reference name
    endpoint: "https://..."     # API Gateway endpoint URL
    auth: iam                   # iam | federate
    aws_profile: "my-sso"      # Required for iam auth; omit for federate
    aws_region: "us-west-2"    # AWS region for SigV4 signing
    description: "Team KB..."  # Used by extraction LLM for routing decisions

# Extraction settings
extraction:
  model_id: "anthropic.claude-sonnet-4-20250514"
  aws_profile: "bedrock-profile"
  aws_region: "us-west-2"

# Translation settings (optional overrides)
translation:
  summarization_model_id: "anthropic.claude-haiku-3-20240307"

# Dream cycle settings (optional overrides)
dream_cycle:
  model_id: "anthropic.claude-sonnet-4-20250514"

# Hook settings
hook:
  timeout: 8s                   # Default: 8 seconds

# Content exclusion rules
exclusion_rules:
  - "Personal opinions about individuals"
  - "Credentials and secrets"
  - "Salary or compensation details"

# Source directory configuration
sources:
  - directory: "/Users/zmueller/my-project"
    harnesses:
      - claude-code
    targets:
      - kb: local/default
        routing: always          # always | consider
        approval: auto-approve   # auto-approve | require-manual-approval
      - kb: my-team-kb
        routing: consider
        approval: require-manual-approval
    overrides:                   # Optional per-harness/persona refinements
      - harness: notor
        targets:
          - kb: architecture-kb
            routing: always
            approval: auto-approve
      - harness: notor
        persona: "architecture"
        targets:
          - kb: architecture-kb
            routing: always
            approval: auto-approve
```

**Validation Rules:**
- `mode` must be `client` or `server`
- `author` must be non-empty, ≤100 characters
- Each `knowledge_bases` entry must have unique `name`
- `auth` must be `iam` or `federate`
- `aws_profile` required when `auth` is `iam`
- Each `sources` entry must have a valid `directory` path
- `harnesses` must contain only `claude-code` and/or `notor`
- `routing` must be `always` or `consider`
- `approval` must be `auto-approve` or `require-manual-approval`
- `kb` must reference a valid `knowledge_bases[].name` or `local/<name>`
- `exclusion_rules` must be an array of strings (can be empty)
- `extraction.model_id` must be a non-empty string
- `hook.timeout` must be a valid duration string

### 3. Runtime State (`state.yaml`)

**Storage:** `~/.multi-kb/state.yaml`

```yaml
directories:
  "/Users/zmueller/my-project":
    last_processed: "2026-05-01T10:00:00Z"   # File last-modified time of final processed conversation
  "/Users/zmueller/other-project":
    last_processed: "2026-04-30T15:30:00Z"

last_dream_cycle: "2026-05-01T08:00:00Z"     # Timestamp of last completed dream cycle
```

**Managed exclusively by CLI** — never user-edited. Written atomically (write temp file → rename).

**Validation Rules:**
- Directory paths must be absolute
- Timestamps must be valid ISO 8601
- CLI never writes to `config.yaml` after initial setup (except via explicit commands like `add-kb`)

### 4. Pending Queue Entry

**Storage:** `~/.multi-kb/pending/<timestamp>-<hash>.json`

**Naming Convention:** `<YYYYMMDDTHHMMSS>-<8-char-hex-hash>.json`
- `<timestamp>` is the `extracted_at` value
- `<hash>` is first 8 hex chars of SHA-256 of `title + content`

```json
{
  "title": "Note title",
  "content": "Full Markdown content",
  "author": "configured-author-identity",
  "target_kbs": ["kb-name-1", "kb-name-2"],
  "source_conversation": "~/.claude/projects/my-project/abc123.jsonl",
  "extracted_at": "2026-05-01T10:30:00Z"
}
```

**Lifecycle:**
- Created: extraction routes a note with `require-manual-approval` targets
- Modified: approval UI removes individual targets from `target_kbs` (approve or reject)
- Deleted: all targets resolved (array empty)

**No `uid` field** — UIDs are generated at submission time.
**No `status` field** — existence in directory = pending; deletion = resolved.

**Validation Rules:**
- `title` must be non-empty, ≤255 characters
- `content` must be non-empty, ≤100,000 characters
- `author` must be non-empty, ≤100 characters
- `target_kbs` must be non-empty array of strings (each referencing a valid KB)
- `source_conversation` must be a non-empty string
- `extracted_at` must be valid ISO 8601

### 5. Intermediate Conversation Format

**Storage:** In-memory during processing (not persisted to disk)

**Format:** JSONL — one header line followed by message lines

**Header Line:**
```json
{
  "type": "conversation",
  "id": "session-abc123",
  "source_harness": "claude-code",
  "source_path": "~/.claude/projects/my-project/abc123.jsonl",
  "started_at": "2026-05-01T09:00:00Z",
  "metadata": {
    "persona": null,
    "workflow": null,
    "project_dir": "/Users/zmueller/my-project"
  }
}
```

**Message Line:**
```json
{
  "type": "message",
  "role": "user",
  "content": "Plain text string (flattened from content blocks)",
  "timestamp": "2026-05-01T09:00:01Z",
  "previously_processed": false,
  "tool_uses": []
}
```

**Tool Use Entry (on assistant messages):**
```json
{
  "tool_name": "Read",
  "summary": "Read file src/main.go (245 lines)"
}
```

**Validation Rules:**
- `role` must be `user`, `assistant`, or `system`
- `content` must be a plain string (no content block arrays)
- `previously_processed` is boolean, derived from `last-processed` timestamp comparison
- Small tool interactions (<~1K tokens): mechanical template summary (no LLM call)
- Large tool interactions (≥~1K tokens): LLM-summarized via `translation.summarization_model_id`

### 6. Extraction Output

**Format:** JSON array returned by Bedrock InvokeModel

```json
[
  {
    "title": "How to configure multi-region replication",
    "content": "## Overview\nWhen configuring multi-region...",
    "suggested_target_kbs": ["my-team-kb", "architecture-kb"]
  }
]
```

**Validation Rules (parsed by CLI):**
- Must be a valid JSON array
- Each entry must have `title` (string, non-empty), `content` (string, non-empty), `suggested_target_kbs` (string array)
- Entries with missing/invalid fields are silently dropped (partial acceptance)
- `suggested_target_kbs` entries that don't match a configured KB name are silently dropped
- Empty array is valid (no knowledge extracted)

### 7. Run Log Entry

**Storage:** `~/.multi-kb/logs/runs.jsonl` (one JSON object per line, append-only)

```json
{
  "timestamp": "2026-05-01T10:05:00Z",
  "type": "capture",
  "trigger": "cron",
  "directories_scanned": 2,
  "conversations_processed": 5,
  "notes_extracted": 8,
  "notes_routed": {
    "local/default": 8,
    "my-team-kb": 3
  },
  "errors": 0,
  "duration_ms": 45200
}
```

Dream cycle variant:
```json
{
  "timestamp": "2026-05-01T10:06:00Z",
  "type": "dream_cycle",
  "trigger": "cron",
  "batches_processed": 4,
  "actions": {
    "keep": 2,
    "merge": 1,
    "split": 0,
    "consolidate": 1
  },
  "errors": 0,
  "duration_ms": 120000
}
```

### 8. Extraction Error Log Entry

**Storage:** `~/.multi-kb/logs/extraction-errors.jsonl`

```json
{
  "timestamp": "2026-05-01T10:03:00Z",
  "conversation_id": "session-abc123",
  "source_path": "~/.claude/projects/my-project/abc123.jsonl",
  "error": "malformed JSON after 3 retries: unexpected token at position 1234",
  "retries": 3
}
```

### 9. Hook Error Log Entry

**Storage:** `~/.multi-kb/logs/hook-errors.jsonl`

```json
{
  "timestamp": "2026-05-01T09:00:05Z",
  "harness": "claude-code",
  "directory": "/Users/zmueller/my-project",
  "error": "remote KB 'my-team-kb' timed out after 8s",
  "partial_results": true
}
```

### 10. Lock File

**Storage:** `~/.multi-kb/lock`

```json
{
  "pid": 12345,
  "started_at": "2026-05-01T10:00:00Z",
  "heartbeat": "2026-05-01T10:04:00Z",
  "activity": "run"
}
```

**Behavior:**
- TTL: 30 minutes from last `heartbeat`
- Heartbeat update: every 60 seconds
- Stale lock (heartbeat > 30 min old): force-acquire
- Active lock: skip (cron) or print message and exit (manual)

## Entity Relationships

```
config.yaml
  ├── knowledge_bases[] ──────────────── referenced by sources[].targets[].kb
  ├── sources[]
  │     ├── directory ────────────────── key in state.yaml.directories
  │     ├── targets[].kb ─────────────── local/<name> or knowledge_bases[].name
  │     └── overrides[].targets[].kb ─── local/<name> or knowledge_bases[].name
  └── exclusion_rules[] ──────────────── appended to extraction system prompt

state.yaml
  └── directories[].last_processed ───── compared to conversation file mtime

Conversation file (harness-native)
  → [Translator] → Intermediate JSONL
  → [Extractor] → Extraction Output (JSON array)
  → [Router]    → Knowledge Note (local KB) or Pending Queue Entry

Pending Queue Entry
  → [Approval UI] → Knowledge Note (local KB) or Remote submitKnowledge

Knowledge Note (local, status: pending)
  → [Dream Cycle] → Knowledge Note (local, status: active) or deleted (merged)
```

## Server-Mode Entities (FR-12)

Server mode reuses all above entities plus:

### SQS Message (Ingestion)
```json
{
  "uid": "01H5K9QZXNM8V3PW",
  "title": "Note title",
  "content": "Markdown content",
  "author": "submitter-identity",
  "submitted_at": "2026-05-01T10:30:00Z"
}
```

### Recall Log (S3 Object)
**Path:** `recall-logs/<YYYY-MM-DD>/<request-id>.json`
```json
{
  "timestamp": "2026-05-01T10:30:05Z",
  "query": "how to configure replication",
  "recalled_uids": ["01H5K9QZXNM8V3PW", "01H5KABCDEF12345"]
}
```

### Server Config Extensions (in `config.yaml` when `mode: server`)
```yaml
sqs:
  queue_url: "https://sqs.us-west-2.amazonaws.com/123456789/multi-kb-queue"
  batch_size: 10
codecommit:
  repo_name: "multi-kb"
  region: "us-west-2"
s3:
  bucket: "multi-kb-notes-123456"
  region: "us-west-2"
opensearch:
  endpoint: "https://abc123.us-west-2.aoss.amazonaws.com"
  region: "us-west-2"
bedrock_kb:
  knowledge_base_id: "KBID123"
  data_source_id: "DSID456"
tick_interval: 5m
dream_cycle:
  interval: 3h
  model_id: "anthropic.claude-sonnet-4-20250514"
recall_log:
  schedule: "02:00"
```
