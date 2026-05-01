# Contract: Extraction LLM Output

**Source:** CLI spec FR-5, FR-6

## Overview

The extraction sub-agent is a single Bedrock InvokeModel call that reads a translated conversation (intermediate JSONL format) and returns a JSON array of candidate knowledge notes.

## Input

### System Prompt

Composed of three parts, concatenated:

1. **Hardcoded base prompt** (versioned per CLI release) — defines the extraction task, output format, and quality guidelines
2. **Exclusion rules section** (from `config.yaml`):
   ```
   ## Content exclusion rules — never include in notes destined for non-local KBs
   - Personal opinions about individuals
   - Credentials and secrets
   - Salary or compensation details
   ```
   Only present when `exclusion_rules` is non-empty in config.
3. **User append file** (optional, from `~/.multi-kb/prompts/extraction-append.md`):
   Read fresh on each extraction run. Appended verbatim after the base prompt and exclusion rules.

### User Message

The full translated conversation in intermediate JSONL format (all lines concatenated as a single string).

For re-processed conversations, all messages are included (full context), with `previously_processed: true/false` flags. The system prompt instructs the LLM to extract knowledge only from `previously_processed: false` messages while using the full conversation for context.

### Chunked Conversations (>800K tokens)

When a translated conversation exceeds 800K tokens (measured by fast approximation):

1. Split at message boundaries (never mid-message) at the ~800K token mark
2. Process first chunk normally → extract notes
3. Summarize first chunk to ~10–20K tokens using the **extraction model** (`extraction.model_id`) with a summarization-specific prompt
4. Prepend summary to next chunk as contextual preamble
5. Process next chunk → extract notes
6. Repeat until all chunks processed
7. Combine all extracted notes from all chunks

## Output

### Success: Valid JSON Array

```json
[
  {
    "title": "DynamoDB Global Tables require specific IAM permissions",
    "content": "## Key Insight\n\nWhen configuring DynamoDB Global Tables, the IAM role needs...\n\n## Details\n\n- Permission `dynamodb:CreateGlobalTable` is required\n- The role must have access in all target regions\n- Replication lag is typically under 1 second for small items",
    "suggested_target_kbs": ["my-team-kb", "infrastructure-kb"]
  },
  {
    "title": "Go context propagation best practices",
    "content": "## Pattern\n\nAlways pass `context.Context` as the first parameter...",
    "suggested_target_kbs": ["my-team-kb"]
  }
]
```

### Output Field Schema

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `title` | string | yes | Non-empty, ≤255 characters |
| `content` | string | yes | Non-empty, Markdown |
| `suggested_target_kbs` | string[] | yes | Array of KB name strings (may be empty) |

### Empty Extraction

An empty array `[]` is valid — indicates no extractable knowledge in the conversation.

## Error Handling

### Retry Logic

| Error Type | Retries | Backoff | Action After Exhaustion |
|------------|---------|---------|------------------------|
| Bedrock API failure (throttle, timeout, network) | 3 | Exponential | Skip conversation, log to `extraction-errors.jsonl` |
| Malformed JSON output | 3 | None (fresh API call) | Skip conversation, log to `extraction-errors.jsonl` |

### Partial Acceptance

If the JSON array is partially valid (some entries parse, some don't):
- Accept all valid entries
- Log invalid entries to `extraction-errors.jsonl`
- The conversation is considered processed (timestamp advances normally)
- No retry for partial failures

### KB Name Resolution

`suggested_target_kbs` entries are resolved against the user's config:
- Names matching a configured `knowledge_bases[].name` or `local/<name>` → routed
- Names not matching any configured KB → silently dropped
- Empty array after resolution + no `always`-mode KBs for directory → fallback to `local/default`

## Routing Integration

The extraction output feeds into the routing engine:

1. For each extracted note, collect target KBs:
   - All `always`-mode KBs for the directory (unconditional)
   - `consider`-mode KBs whose names appear in `suggested_target_kbs` (LLM-recommended)
2. If no targets after resolution → route to `local/default` (fallback)
3. For each target KB, check approval mode:
   - `auto-approve` → submit immediately
   - `require-manual-approval` → stage in `~/.multi-kb/pending/`
