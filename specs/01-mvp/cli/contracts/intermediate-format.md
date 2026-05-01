# Contract: Intermediate Conversation Format

**Source:** CLI spec FR-4

## Overview

Before extraction, conversations from any harness are translated into a standardized JSONL intermediate representation. This decouples the extraction sub-agent from harness-specific formats.

## Format

One JSONL file (or in-memory buffer) per conversation. First line is a header, subsequent lines are messages.

### Conversation Header (Line 1)

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

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Always `"conversation"` |
| `id` | string | yes | Unique conversation identifier (derived from session file) |
| `source_harness` | string | yes | `"claude-code"` or `"notor"` |
| `source_path` | string | yes | Absolute path to the original conversation file |
| `started_at` | ISO 8601 | yes | Conversation start timestamp |
| `metadata.persona` | string \| null | no | Notor persona name (null for Claude Code) |
| `metadata.workflow` | string \| null | no | Notor workflow name (null for Claude Code) |
| `metadata.project_dir` | string | yes | User-configured project directory path |

### Message Lines (Lines 2+)

```json
{
  "type": "message",
  "role": "assistant",
  "content": "Here's how to configure the replication settings...",
  "timestamp": "2026-05-01T09:01:30Z",
  "previously_processed": false,
  "tool_uses": [
    {
      "tool_name": "Read",
      "summary": "Read file src/config/replication.go (142 lines)"
    },
    {
      "tool_name": "Bash",
      "summary": "Ran `aws dynamodb describe-table --table-name users` — returned table config showing no global tables enabled"
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Always `"message"` |
| `role` | string | yes | `"user"`, `"assistant"`, or `"system"` |
| `content` | string | yes | Plain text (flattened from content block arrays) |
| `timestamp` | ISO 8601 | yes | Per-message (Notor) or file-level (Claude Code) |
| `previously_processed` | boolean | yes | `true` if message was included in a prior extraction run |
| `tool_uses` | array | yes | Tool interactions (empty array if none) |

### Tool Use Entry

```json
{
  "tool_name": "Read",
  "summary": "Read file src/main.go (245 lines)"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tool_name` | string | yes | Name of the tool called |
| `summary` | string | yes | Human-readable summary of the interaction |

## Translation Rules

### Role Normalization
- Map harness-specific roles to `user`, `assistant`, `system`

### Content Flattening
- Content block arrays (e.g., `[{type: "text", text: "..."}]`) are flattened to a single plain text string
- Image blocks are replaced with `[Image]` placeholder
- Non-text blocks are omitted or described briefly

### Tool Call Collapsing
- Tool call/result pairs on an assistant message are collapsed into `tool_uses` entries
- **Small interactions** (<~1K tokens in the tool result): summarized via mechanical template (no LLM call)
  - Pattern: `"{tool_name}: {brief action description} — {brief result description}"`
  - Examples: `"Read file src/main.go (245 lines)"`, `"Bash: ran 'ls -la' — listed 12 files"`
- **Large interactions** (≥~1K tokens): summarized via the translation summarization model (`translation.summarization_model_id`)

### Previously Processed Flag

**Notor (per-message timestamps available):**
- Compare each message's timestamp to the directory's `last-processed` timestamp
- Messages with timestamp ≤ `last-processed` → `previously_processed: true`

**Claude Code (no per-message timestamps):**
- Use conversation file's last-modified time as proxy
- If the file was previously processed, all prior messages are flagged `previously_processed: true`
- The entire conversation is re-translated (extraction prompt focuses on `previously_processed: false` messages)

### Timestamp Handling

**Notor:** Per-message timestamps from the native format.

**Claude Code:** File last-modified time used as the timestamp for all messages in the conversation.
