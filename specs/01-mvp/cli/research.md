# Research: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)
**Status:** Open (findings to be populated during Phase 0)

## R-1: Bubbletea Wizard Pattern

**Question:** Which terminal UI library best handles the multi-step setup wizard flow?

**Options:**
- `bubbletea` + `huh` (Charmbracelet form library)
- `survey` (AlecAivazis/survey)
- Raw `bubbletea` with custom components

**Evaluation Criteria:**
- Multi-step form flow support (branching based on answers)
- Terminal compatibility (macOS Terminal, iTerm2, Windows Terminal, WSL)
- Accessibility (screen readers, keyboard-only navigation)
- Maintained and well-documented

**Prototype Task:** Build a 3-step wizard: (1) select from list, (2) text input, (3) confirmation screen. Evaluate all three options.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-2: Bedrock InvokeModel Go SDK Pattern

**Question:** What is the correct Go SDK v2 pattern for calling Bedrock InvokeModel with Claude models?

**Areas to Investigate:**
- `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` — InvokeModel API
- Request body format for Claude models (Messages API vs. legacy)
- How to specify model ID, system prompt, user content
- Response parsing (JSON body extraction from InvokeModelOutput)
- Credential chain with named SSO profiles (`aws_profile` in config)
- Retry and backoff configuration (SDK-level vs. application-level)

**Prototype Task:** Make a working InvokeModel call that sends a system prompt + user message and parses a JSON array response.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-3: Claude Code Conversation Format ✅

**Question:** What is the exact schema of Claude Code conversation files?

**Location:** `~/.claude/projects/<project>/<session>.jsonl`

**Areas to Investigate:**
- How `<project>` directory name maps to the user's project path
- JSONL line schema: message roles, content block structure, tool call/result format
- How to identify conversation boundaries (one file = one conversation?)
- Presence/absence of per-message timestamps
- How to detect file modifications for re-processing

**Prototype Task:** Read a real Claude Code conversation file, document the schema, build a parser.

**Findings:**

### Project Directory Naming

The absolute filesystem path is encoded by replacing every `/` with `-`. The result always starts with a leading `-` (since absolute paths start with `/`).

| Filesystem Path | Directory Name |
|---|---|
| `/Volumes/workplace/multi-kb` | `-Volumes-workplace-multi-kb` |
| `/Users/jane/projects/foo` | `-Users-jane-projects-foo` |

**Reverse mapping:** To find the project directory for a user-configured path like `/Volumes/workplace/multi-kb`, replace all `/` with `-` to get `-Volumes-workplace-multi-kb`, then look for `~/.claude/projects/-Volumes-workplace-multi-kb/`.

### Session File Layout

Each project directory contains:
- **`<uuid>.jsonl`** — One file per conversation session. The filename is the session's UUID.
- **`<uuid>/`** — Optional companion directory containing:
  - `subagents/` — JSONL files for Agent sub-conversations (`agent-<id>.jsonl` + `agent-<id>.meta.json`)
  - `tool-results/` — Large tool outputs persisted to disk
- **`memory/`** — Project-level memory directory

**One file = one conversation.** No explicit boundary markers needed.

### JSONL Line Types

Every line is a JSON object with a top-level `type` field:

| `type` | Description | Relevant to translator? |
|---|---|---|
| `user` | User message or tool_result delivery | **Yes** |
| `assistant` | Assistant response (one content block per line) | **Yes** |
| `attachment` | File attachments, tool listings, injected context | Selective |
| `system` | System-level events | No |
| `queue-operation` | Internal queuing metadata | No |
| `permission-mode` | Permission mode changes | No |
| `file-history-snapshot` | File backup state | No |
| `last-prompt` | Truncated last user prompt | No |
| `ai-title` | AI-generated conversation title | No |

### Common Fields on Message Lines

Most message lines share these fields:

```json
{
  "type": "user|assistant|attachment",
  "uuid": "<uuid>",
  "parentUuid": "<uuid> | null",
  "timestamp": "2026-05-01T04:38:24.311Z",
  "sessionId": "<uuid>",
  "cwd": "/Volumes/workplace/multi-kb",
  "version": "2.1.123",
  "entrypoint": "claude-vscode",
  "userType": "external",
  "isSidechain": false,
  "gitBranch": "main"
}
```

### Per-Message Timestamps

**IMPORTANT CHANGE:** Every message line **does** have a `timestamp` field — ISO 8601 with millisecond precision, UTC (`Z`). This contradicts the spec's assumption that "Claude Code's native format lacks reliable per-message timestamps."

**Impact:** The translator can use per-message timestamps for the `previously_processed` flag (same as Notor), rather than the file-level fallback strategy described in the spec. This simplifies re-processing: only messages with timestamps ≤ `last_processed` are flagged `previously_processed: true`.

### User Message Schema (`type: "user"`)

**Human-typed messages:**
```json
{
  "type": "user",
  "promptId": "<uuid>",
  "permissionMode": "default",
  "message": {
    "role": "user",
    "content": [
      { "type": "text", "text": "the user's message" }
    ]
  }
}
```

**Content is always an array of content blocks**, never a bare string. Multiple `text` blocks may exist (e.g., IDE-injected file context alongside user text).

**Tool result messages** (`type: "user"` with `tool_result` content block):
```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [
      {
        "type": "tool_result",
        "tool_use_id": "toolu_bdrk_01...",
        "content": "<string or [{type, text}]>",
        "is_error": true
      }
    ]
  },
  "toolUseResult": { ... },
  "sourceToolAssistantUUID": "<uuid>"
}
```

**`toolUseResult`** provides rich metadata beyond the `message.content`:
- `Bash`: `{stdout, stderr, interrupted, ...}`
- `Read`: `{type, file: {filePath, content, numLines, ...}}`
- `Write`/`Edit`: `{filePath, content, structuredPatch, ...}`
- `Agent`: `{status, prompt, agentId, agentType, content, totalDurationMs, totalTokens, ...}`

### Assistant Message Schema (`type: "assistant"`)

**Critical:** A single API response is **split across multiple JSONL lines, one content block per line.** All lines from the same API response share the same `message.id`.

```json
{
  "type": "assistant",
  "message": {
    "model": "claude-opus-4-6",
    "id": "msg_bdrk_013...",
    "role": "assistant",
    "content": [
      { "type": "text", "text": "response text" }
    ],
    "stop_reason": "tool_use|end_turn",
    "usage": { "input_tokens": 3, "output_tokens": 359, ... }
  }
}
```

Content block types:
- `thinking`: `{type: "thinking", thinking: "<text>", signature: "<base64>"}`
- `text`: `{type: "text", text: "<text>"}`
- `tool_use`: `{type: "tool_use", id: "<tool_use_id>", name: "<tool_name>", input: {...}}`

### Attachment Schema (`type: "attachment"`)

Key subtypes:
- `file`: File contents from `@file` references
- `hook_additional_context`: Context injected by hooks
- `deferred_tools_delta`: Tool palette changes
- `skill_listing`: Available slash commands

### Message Threading

Messages form a linked list via `parentUuid` → `uuid`. First message has `parentUuid: null`. The `promptId` groups a user prompt with its responses. `isSidechain: true` indicates branched conversations.

### Subagent Files

Agent tool calls produce companion files under `<session-uuid>/subagents/`:
- `agent-<id>.jsonl` — Sub-conversation (same JSONL format, no queue-operation preamble)
- `agent-<id>.meta.json` — Metadata: `{agentType, description}`

**Decision:**

1. **Per-message timestamps are available and should be used.** The spec's file-level `previously_processed` fallback is unnecessary for Claude Code. The translator should compare each message's `timestamp` to `last_processed`, same as Notor. This change simplifies the translator and improves re-processing precision (only new messages get `previously_processed: false`).

2. **Project directory discovery:** To map a user-configured directory path to the Claude Code project directory, replace all `/` with `-`. No complex path resolution needed.

3. **Translator must reassemble split assistant messages.** Group consecutive `type: "assistant"` lines with the same `message.id` into a single logical assistant message.

4. **Tool call/result pairing:** Match `tool_use` content blocks (on assistant lines) to `tool_result` content blocks (on user lines) via `tool_use_id` ↔ `tool_use_id`. The `toolUseResult` field on the user line provides richer metadata for summarization.

5. **Ignore non-message line types** (`queue-operation`, `permission-mode`, `file-history-snapshot`, `last-prompt`, `ai-title`) during translation. Only process `user`, `assistant`, and selectively `attachment` lines.

6. **Subagent conversations** should be skipped in MVP — they are subsidiary context that would complicate the translator without proportional value. The Agent tool's result is already captured in the parent conversation's tool result.

---

## R-4: Notor Conversation Format

**Question:** What is the exact schema of Notor chat history files?

**Location:** `{vault}/notor/history/`

**Areas to Investigate:**
- File format (JSON, JSONL, Markdown, other?)
- Message schema (roles, content, timestamps, tool calls)
- Persona/workflow metadata (where stored, how to extract)
- Per-message timestamps (confirmed available per spec — verify format)
- Conversation boundaries

**Prototype Task:** Read a real Notor history directory, document the schema, build a parser.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-5: Claude Code Hook Registration ✅

**Question:** How to programmatically register a `user_prompt_submit` hook in Claude Code?

**Areas to Investigate:**
- Hook configuration file location and format
- Registration API (file edit? CLI command? JSON schema?)
- How multiple hooks at the same trigger point coexist
- Context provided to the hook at runtime (env vars, stdin, args)
- How the hook's stdout is consumed (prepended to system context? shown to user?)
- First-message detection: what signals are available to determine if this is the first message in a conversation?

**Prototype Task:** Register a test hook that prints "Hello from multi-kb" on first message only.

**Findings:**

### Hook Configuration Location

Hooks are configured in JSON settings files under the top-level `"hooks"` key:

| Location | Scope |
|----------|-------|
| `~/.claude/settings.json` | Global (all projects) |
| `~/.claude/settings.local.json` | Global (not checked in) |
| `<project>/.claude/settings.json` | Per-project |
| `<project>/.claude/settings.local.json` | Per-project (not checked in) |

**Recommended for multi-kb:** Use `~/.claude/settings.json` (global scope) since multi-kb hooks should fire for all projects.

### Hook JSON Schema

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "multi-kb hook --harness claude-code",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

**Event-level entry** (each element in the `UserPromptSubmit` array):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `matcher` | string | No (omit = match all) | Regex pattern; `"*"` for all |
| `hooks` | array | Yes | Array of hook actions (all run in **parallel**) |

**Hook action** (each element in the inner `hooks` array):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `"command"` or `"prompt"` | Yes | Execution type |
| `command` | string | Yes (if command) | Shell command to execute |
| `timeout` | integer (seconds) | No | Default: 60s. Max: 600s. |

### Available Hook Events

| Event | When | Supports prompt hooks? |
|-------|------|----------------------|
| **`UserPromptSubmit`** | When user submits a prompt | Yes |
| `PreToolUse` | Before tool executes | Yes |
| `PostToolUse` | After tool completes | No |
| `Stop` / `SubagentStop` | Agent considers stopping | Yes |
| `SessionStart` / `SessionEnd` | Session lifecycle | No (command only) |
| `PreCompact` | Before context compaction | No |
| `Notification` | Notifications sent | No |

### Multiple Hook Coexistence

Hooks use a **two-level array**:
1. **Outer array**: Multiple matcher groups per event. All entries whose `matcher` matches the context fire.
2. **Inner `hooks` array**: Multiple actions per matcher group. All run **in parallel**.

Adding a multi-kb hook alongside existing hooks is safe — append a new entry to the outer array.

### Runtime Context (stdin)

`UserPromptSubmit` hooks receive JSON on stdin:

```json
{
  "session_id": "abc123",
  "transcript_path": "/path/to/transcript.txt",
  "cwd": "/current/working/dir",
  "permission_mode": "ask",
  "hook_event_name": "UserPromptSubmit",
  "user_prompt": "The actual text the user typed"
}
```

**Environment variables:**
- `$CLAUDE_PROJECT_DIR` — Project root path

### Hook Output Format

**IMPORTANT CHANGE:** Hook output is **not** raw Markdown to stdout as the spec assumed. It is structured JSON:

```json
{
  "continue": true,
  "suppressOutput": false,
  "systemMessage": "Injected context text here"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `continue` | bool | `true` | If false, halt processing |
| `suppressOutput` | bool | `false` | If true, hide output from transcript |
| `systemMessage` | string | — | Message injected into Claude's system context |

**Impact:** The CLI must output a JSON object with the `systemMessage` field containing the formatted Markdown, not raw Markdown to stdout. The spec's description of "raw Markdown to stdout" is incorrect.

**Exit codes:**
- `0`: Success. stdout parsed for structured output.
- `2`: Blocking error. stderr fed back to Claude.
- Other: Non-blocking error.

### First-Message Detection

No explicit `is_first_message` field exists in the hook input. Detection strategies:

1. **Transcript-based (recommended):** Check `transcript_path` for prior `user` entries. If the transcript has no prior user messages, this is the first message:
   ```bash
   # Read JSON from stdin, extract transcript_path
   # If transcript file is empty or has no prior "role":"user" entries → first message
   ```

2. **Session state file:** Create a flag file keyed by `session_id`. If absent, it's the first message; touch the flag and proceed. Requires cleanup logic.

3. **SessionStart + env:** Use a `SessionStart` hook to write to `$CLAUDE_ENV_FILE`, then check the env var in `UserPromptSubmit`. More complex but avoids transcript parsing.

**Recommended approach:** Transcript-based detection. The `transcript_path` is reliably provided, and checking for prior user messages is deterministic. The Go binary reads the transcript file, counts `user`-type lines, and if count ≤ 1 (current prompt only), treats it as first message.

### Hooks Load at Session Start Only

Changing hook configuration requires restarting Claude Code. This is fine for `multi-kb setup` — hooks are registered once and loaded on next session start.

**Decision:**

1. **Registration target:** Write to `~/.claude/settings.json` under `hooks.UserPromptSubmit`. Read the existing file, parse JSON, append a new entry to the `UserPromptSubmit` array (create if absent), write back.

2. **Idempotency:** Before appending, check if an entry with a command containing `multi-kb hook` already exists. If so, update it rather than duplicating.

3. **Hook command:** `multi-kb hook --harness claude-code`. The CLI binary must be on PATH (documented in setup).

4. **Timeout:** Set to 10 seconds (slightly above the 8-second hook timeout in the CLI config, to avoid Claude Code killing the process before the CLI's internal timeout fires).

5. **Output format:** Return `{"systemMessage": "<formatted markdown>"}` on stdout. The spec's references to "raw Markdown to stdout" need updating — the Markdown content goes inside the `systemMessage` field of a JSON object.

6. **First-message guard:** Use transcript-based detection. The hook reads the `transcript_path` from stdin JSON, checks for prior user entries, and exits with code 0 and no output if this is not the first message.

7. **Input parsing:** The hook reads the user's prompt from stdin JSON (`user_prompt` field), not from args or env vars. The Go binary must parse JSON from stdin.

8. **Exit code semantics:** Exit 0 with empty stdout (or `{}`) for non-first-message cases. Exit 0 with `{"systemMessage": "..."}` for first-message injection. Exit non-0/non-2 for errors (non-blocking).

9. **Settings file editing:** Use read-modify-write with JSON parsing (not string concatenation). Handle the case where `settings.json` doesn't exist yet, or where `hooks` key doesn't exist, or where `UserPromptSubmit` key doesn't exist.

---

## R-6: Notor Hook Registration

**Question:** How to programmatically register a conversation-start hook in Notor?

**Areas to Investigate:**
- Hook configuration mechanism (Obsidian plugin settings? Config file?)
- Registration format
- How multiple hooks at the same trigger point coexist
- Context provided to the hook at runtime
- How hook output is injected into the conversation system context

**Prototype Task:** Register a test hook that injects a test string at conversation start.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-7: Crockford Base32 UID Generation

**Question:** Best approach for generating 16-character Crockford base32 UIDs in Go?

**Areas to Investigate:**
- Existing Go libraries for Crockford base32 (vs. standard base32)
- Crockford base32 alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ` (excludes I, L, O, U)
- Input: 10 bytes from `crypto/rand` = 80 bits → 16 Crockford base32 characters (5 bits each)
- Collision probability at scale (80 bits ≈ 1.2 × 10²⁴ possible values — more than sufficient)

**Prototype Task:** Implement and test the function. Verify output is always exactly 16 chars, valid alphabet, and passes round-trip decode.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-8: Cross-Platform Cron Registration

**Question:** How to safely register scheduled tasks on macOS/Linux (crontab) and Windows (Task Scheduler)?

**macOS/Linux — crontab:**
- Read existing crontab: `crontab -l`
- Append entry with a marker comment (e.g., `# multi-kb scheduled run`)
- Write back: `crontab -` (pipe new content)
- Idempotency: check for marker comment before appending
- Removal: filter out lines with marker comment

**Windows — Task Scheduler:**
- Use `schtasks.exe /Create` with appropriate flags
- XML task definition for more control
- Idempotency: check for existing task by name before creating
- Removal: `schtasks.exe /Delete /TN "multi-kb-run" /F`

**Areas to Investigate:**
- Does `crontab -l` fail on empty crontab? (Yes on some systems — handle gracefully)
- Windows permissions requirements (does it need admin?)
- How to parse the cron expression back for `multi-kb status` next-run display

**Prototype Task:** Implement register/unregister/check on macOS. Implement register/unregister/check on Windows (if available).

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_
