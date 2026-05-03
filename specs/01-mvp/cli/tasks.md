# Task Breakdown: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Implementation Plan:** [plan.md](plan.md)
**Specification:** [spec.md](spec.md)
**Status:** In Progress

## Task Summary

**Total Tasks:** 87 (82 original + 5 prompt tasks)
**Phases:** 10 + cross-cutting prompt authoring + research ordering
**Estimated Complexity:** High
**Parallel Execution Opportunities:** 18 task groups + prompt tasks (all parallel)

## Dependency Legend

- **[P]** — Can execute in parallel with other [P] tasks in the same phase section
- **Dependencies** — Tasks that must complete before this task can start
- **FR-N** — References to spec functional requirements
- **Contract** — References to files in `contracts/`

---

## Phase 0: Setup & Environment

### ENV-001: Go Module Initialization
**Description:** Initialize the Go module, install core dependencies, and establish the project directory structure from plan.md.
**Files:**
- `go.mod`, `go.sum`
- `cmd/multi-kb/main.go`
- `internal/` directory tree (empty packages with `package` declarations)
**Dependencies:** None
**Acceptance Criteria:**
- [x] `go mod init` creates module (e.g., `github.com/<org>/multi-kb`)
- [x] All directories from plan.md Module Architecture exist with `package` declaration files
- [x] `go build ./cmd/multi-kb/` succeeds with a no-op `main.go`
- [x] `go test ./...` passes (no tests yet, but no compilation errors)

### ENV-002 [P]: Cobra Root Command and Subcommand Stubs
**Description:** Wire up the Cobra command tree with all subcommands as stubs that print "not implemented".
**Files:**
- `internal/cmd/root.go` — root command, global flags (`--config`, `--verbose`)
- `internal/cmd/setup.go` — `multi-kb setup`
- `internal/cmd/run.go` — `multi-kb run`
- `internal/cmd/process.go` — `multi-kb process`
- `internal/cmd/dreamcycle.go` — `multi-kb dream-cycle`
- `internal/cmd/approve.go` — `multi-kb approve`
- `internal/cmd/status.go` — `multi-kb status`
- `internal/cmd/addsource.go` — `multi-kb add-source`
- `internal/cmd/addkb.go` — `multi-kb add-kb`
- `internal/cmd/hook.go` — `multi-kb hook`
- `internal/cmd/server.go` — `multi-kb server`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] `multi-kb --help` lists all subcommands
- [x] Each subcommand prints "not implemented" and exits 0
- [x] `multi-kb --version` prints version string
- [x] Global `--config` flag accepts a path (default `~/.multi-kb/config.yaml`)

### ENV-003 [P]: Development Tooling Configuration
**Description:** Set up linting, formatting, and test infrastructure per quickstart.md.
**Files:**
- `.golangci.yml` — linter configuration
- `Makefile` — build, test, lint targets
- `.gitignore` — Go binaries, editor files
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] `make build` produces `multi-kb` binary
- [x] `make test` runs `go test ./...`
- [x] `make lint` runs golangci-lint with no errors on empty project
- [x] `make build-all` cross-compiles for all target platforms (linux/darwin amd64+arm64, windows amd64)
- [x] `CGO_ENABLED=0` enforced in all build targets

---

## Phase 1: Foundation (No LLM, No Network)

_Corresponds to plan.md Phase A. Builds local-only infrastructure._

### FND-001: Config YAML Loading and Validation
**Description:** Implement `config.yaml` schema, loader, and validation per data-model.md Entity 2.
**Files:**
- `internal/config/config.go` — Go struct matching full config schema, YAML tags, loader function
- `internal/config/validate.go` — validation rules from data-model.md
- `internal/config/config_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Parses all fields from data-model.md Entity 2 (mode, author, knowledge_bases, extraction, translation, dream_cycle, hook, exclusion_rules, sources with targets and overrides)
- [x] Validates: mode ∈ {client, server}, author non-empty ≤100 chars, unique KB names, auth ∈ {iam, federate}, aws_profile required when auth=iam, routing ∈ {always, consider}, approval ∈ {auto-approve, require-manual-approval}, kb references resolve to knowledge_bases or local/<name>
- [x] Returns structured errors listing all validation failures (not just first)
- [x] Defaults: hook.timeout=8s, extraction/translation/dream_cycle model IDs have sensible defaults
- [x] Duration fields (`tick_interval`, `dream_cycle.interval`, `hook.timeout`) validated using Go's `time.ParseDuration`; rejects bare integers and invalid strings with structured error
- [x] `recall_log.schedule` validated as `HH:MM` 24-hour UTC format (regex: `^([01]\d|2[0-3]):[0-5]\d$`); rejects invalid values
- [x] Test cases: valid config, missing required fields, invalid enum values, dangling KB references, empty sources, invalid duration string (e.g., `5 minutes`, `5`), invalid schedule (e.g., `25:00`, `2pm`)

### FND-002: State YAML Loading and Writing
**Description:** Implement `state.yaml` schema, loader, and atomic writer per data-model.md Entity 3.
**Files:**
- `internal/config/state.go` — Go struct, YAML tags, loader, atomic writer (write temp → rename)
- `internal/config/state_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Parses per-directory `last_processed` timestamps and `last_dream_cycle` timestamp
- [x] Atomic write: writes to temp file then renames (no partial writes on crash)
- [x] Validates: directory paths are absolute, timestamps are valid ISO 8601
- [x] Creates file with empty state if it doesn't exist
- [x] Test cases: load existing, create new, atomic write verification, concurrent read safety

### FND-003 [P]: Lock File with Heartbeat
**Description:** Implement lock file acquisition, heartbeat goroutine, release, and stale detection per data-model.md Entity 10.
**Files:**
- `internal/lock/lock.go` — Acquire, Release, IsHeld, heartbeat goroutine
- `internal/lock/lock_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Lock file at `~/.multi-kb/lock` contains JSON: pid, started_at, heartbeat, activity
- [x] Acquire succeeds if no lock file or stale heartbeat (>30 min old)
- [x] Acquire fails with structured info (pid, activity, last heartbeat) if lock is active
- [x] Heartbeat goroutine updates `heartbeat` field every 60 seconds
- [x] Release deletes the lock file and stops the heartbeat goroutine
- [x] Stale lock detection: heartbeat older than 30 minutes → force-acquire
- [x] Test cases: acquire fresh, acquire stale, fail on active, release, heartbeat updates

### FND-004 [P]: Crockford Base32 UID Generation
**Description:** Implement 16-character Crockford base32 UID generation per [research.md R-7](research.md#r-7-crockford-base32-uid-generation).
**Files:**
- `internal/submit/uid.go` — `GenerateUID` function + `EncodeCrockford` function
- `internal/submit/uid_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Uses `crypto/rand` for 10 random bytes (80 bits of entropy)
- [x] Encodes to exactly 16 characters using Crockford base32 alphabet (`0123456789ABCDEFGHJKMNPQRSTVWXYZ`)
- [x] Uses bit-buffer encoding algorithm (R-7): accumulate 8 bits per byte, extract 5-bit groups MSB-first via `(buf >> bits) & 0x1F`
- [x] No I, L, O, U characters in output
- [x] Uppercase output (Crockford canonical form — use uppercase alphabet constant directly, no post-processing)
- [x] Zero external dependencies — no third-party Crockford library
- [x] `EncodeCrockford([]byte)` exported separately from `GenerateUID()` for deterministic testing
- [x] Test cases: length=16, valid alphabet, uniqueness over 10K generations, deterministic encoding of 5 shared test vectors from R-7:
  - `[0x00 × 10]` → `"0000000000000000"`
  - `[0xFF × 10]` → `"ZZZZZZZZZZZZZZZZ"`
  - `[0x00..0x09]` → `"000G40R40M30E209"`
  - `[0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x42]` → `"VTPVXVYAZTXBW022"`
  - `"HelloWorld"` bytes → `"91JPRV3FAXQQ4V34"`

### FND-005: Local KB Git Repository Creation
**Description:** Implement local KB directory and git repository initialization.
**Files:**
- `internal/git/repo.go` — InitRepo, CommitFiles, IsRepo functions
- `internal/git/repo_test.go`
**Dependencies:** ENV-001
**Implementation Note:** Use `os/exec` shell-out to the `git` binary for all git operations. Sanitize all user-derived inputs (directory names, file names) to prevent command injection. Requires `git` on PATH.
**Acceptance Criteria:**
- [x] Creates `~/.multi-kb/local/<kb-name>/` directory
- [x] Runs `git init` in the directory
- [x] Creates initial commit (empty or with `.gitkeep`)
- [x] IsRepo detects whether a path is an initialized git repo
- [x] Default KB created at `~/.multi-kb/local/default/`
- [x] All git arguments properly escaped/quoted to prevent command injection
- [x] Test cases: init fresh repo, detect existing repo, commit files

### FND-006: Local KB Note File Writing
**Description:** Implement writing knowledge notes as Obsidian-flavor Markdown files with YAML frontmatter per data-model.md Entity 1.
**Files:**
- `internal/submit/local.go` — WriteNote function (generates UID, writes file, commits)
- `internal/submit/local_test.go`
**Dependencies:** FND-004, FND-005
**Implementation Note:** Uses git shell-out (FND-005) for commits. Sanitize title and author strings before use in file content or git commit messages.
**Acceptance Criteria:**
- [x] Writes `<UID>.md` to the appropriate local KB directory
- [x] YAML frontmatter includes all fields: uid, title, status (pending), author, last-updated, last-linked-to, last-recalled, consolidated-from-notes — all present with YAML null values for unpopulated fields (bare key, no value — e.g., `last-recalled:` on its own line, per data-model.md convention)
- [x] Body contains the note's Markdown content
- [x] Generates UID via FND-004
- [x] Commits the new file with a descriptive git commit message
- [x] Validates note constraints: title ≤255 chars, content ≤100K chars, author ≤100 chars
- [x] Test cases: write note, verify frontmatter (all keys present; unpopulated fields have null values, not omitted), verify round-trip parse produces empty string or nil for null fields, verify file naming, validation failures

### FND-007: Git Grep Recall with Match-Count Ranking
**Description:** Implement local KB recall using `git grep` with match-count ranking and title weighting per spec FR-8.
**Files:**
- `internal/git/grep.go` — GrepNotes function (keyword search, match counting, title 3x weighting)
- `internal/recall/local.go` — LocalRecall orchestrator (keyword input, grep execution, ranking)
- `internal/git/grep_test.go`
- `internal/recall/local_test.go`
**Dependencies:** FND-005, FND-006
**Implementation Note:** Uses `os/exec` shell-out to `git grep -c`. Sanitize keyword inputs to prevent command injection (keywords come from LLM output in the hook path, or from note titles in the dream cycle path).
**Acceptance Criteria:**
- [x] Runs `git grep -c <keyword>` per keyword against the working tree
- [x] Filters results to `status: active` notes only (parses frontmatter to check status)
- [x] Counts matches per note across all keywords
- [x] Title matches weighted 3x (matches in YAML `title:` frontmatter line count triple)
- [x] Returns results sorted by descending match count
- [x] Returns structured results: uid, title, content, match_count
- [x] Test cases: single keyword, multiple keywords, title weighting, status filtering, empty results

### FND-008 [P]: Pending Queue File Management
**Description:** Implement create, list, read, update, and delete for pending queue entries per data-model.md Entity 4.
**Files:**
- `internal/route/pending.go` — CreatePending, ListPending, ReadPending, UpdatePending (remove target), DeletePending
- `internal/route/pending_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Creates `~/.multi-kb/pending/` directory if not exists
- [x] Writes JSON files named `<YYYYMMDDTHHMMSS>-<8-char-hex-hash>.json`
- [x] Hash is first 8 hex chars of SHA-256 of `title + content`
- [x] JSON schema matches data-model.md Entity 4 (title, content, author, target_kbs, source_conversation, extracted_at)
- [x] ListPending returns all `.json` files in the directory
- [x] UpdatePending removes a specific target from `target_kbs` array
- [x] DeletePending removes the file when `target_kbs` is empty
- [x] PendingCount returns count of pending files (for status display and hook notice)
- [x] Test cases: create, list, read, remove single target, remove last target (auto-delete), concurrent access safety

### FND-009 [P]: Run Log and Error Log Appending
**Description:** Implement structured JSONL logging for runs, extraction errors, and hook errors per data-model.md Entities 7, 8, 9.
**Files:**
- `internal/logging/runs.go` — AppendRunLog (capture and dream cycle variants)
- `internal/logging/errors.go` — AppendExtractionError, AppendHookError
- `internal/logging/runs_test.go`
- `internal/logging/errors_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Creates `~/.multi-kb/logs/` directory if not exists
- [x] `runs.jsonl`: appends one JSON line per run with all fields from data-model.md Entity 7 (timestamp, type, trigger, directories_scanned, conversations_processed, notes_extracted, notes_routed map, errors, duration_ms)
- [x] `runs.jsonl` dream cycle variant: timestamp, type, trigger, batches_processed, actions map, errors, duration_ms
- [x] `extraction-errors.jsonl`: timestamp, conversation_id, source_path, error, retries
- [x] `hook-errors.jsonl`: timestamp, harness, directory, error, partial_results
- [x] All writes are append-only (open file with O_APPEND)
- [x] Test cases: append single entry, append multiple entries, file creation, valid JSON per line

### FND-010: Status Command Implementation
**Description:** Implement `multi-kb status` displaying config summary, run history, pending count, and next scheduled run per spec FR-11.
**Files:**
- `internal/cmd/status.go` — full implementation replacing stub
**Dependencies:** FND-001, FND-002, FND-008, FND-009
**Acceptance Criteria:**
- [x] Displays current configuration summary: tracked directories, configured KBs (names, auth types), author
- [x] Displays last N runs (default 10) from `runs.jsonl` with success/failure status and key counts
- [x] Displays pending approval queue count when non-empty (e.g., "3 notes awaiting approval")
- [x] Displays next scheduled run time (placeholder until cron parsing in WIZ-005)
- [x] Handles missing config gracefully (suggests running `multi-kb setup`)
- [x] Handles empty run log gracefully ("No runs recorded yet")
- [x] Test cases: with full data, with empty logs, with no config

### FND-011 [P]: Token Counting Approximation
**Description:** Implement fast token estimation for determining when conversations exceed the chunking threshold per spec FR-5.
**Files:**
- `internal/token/count.go` — EstimateTokens function, ChunkingThreshold constant
- `internal/token/count_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Estimates tokens from a string using a fast heuristic (e.g., ~4 chars per token for English text, ~3.5 for code-heavy content)
- [x] Does not use an external tokenizer library (speed priority)
- [x] Accuracy within ±20% for typical conversation content
- [x] Exports `ChunkingThreshold = 700_000` constant (intentionally below the spec's ~800K target to provide a safety margin for the approximate token counter — actual model context windows are larger)
- [x] Handles empty strings, very long strings, and mixed content
- [x] Test cases: known calibration strings, edge cases (empty, single char, 1M chars)

---

## Phase 2: Translation Layer

_Corresponds to plan.md Phase B. Builds harness-specific conversation translators._

### TRN-001: Intermediate JSONL Format Types
**Description:** Define Go types for the intermediate conversation format per contracts/intermediate-format.md.
**Files:**
- `internal/translate/intermediate.go` — ConversationHeader, Message, ToolUse structs; Writer that produces JSONL buffer
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] ConversationHeader: type, id, source_harness, source_path, started_at, metadata (persona, workflow, project_dir)
- [x] Message: type, role, content, timestamp, previously_processed, tool_uses
- [x] ToolUse: tool_name, summary
- [x] Writer serializes header + messages as JSONL (one JSON object per line)
- [x] Role enum: user, assistant, system
- [x] Test cases: serialize header, serialize messages with tool uses, round-trip parse

### TRN-002: Claude Code Translator
**Description:** Implement translator that reads Claude Code conversation files from `~/.claude/projects/` and produces intermediate format per spec FR-4. Schema fully documented in [research.md R-3](research.md#r-3-claude-code-conversation-format).
**Files:**
- `internal/translate/claudecode.go` — ClaudeCodeTranslator: discover sessions, translate messages, flag previously_processed
- `internal/translate/claudecode_test.go`
**Dependencies:** TRN-001, FND-002
**Acceptance Criteria:**
- [x] **Project directory mapping (R-3):** Converts user-configured directory path to project directory name by replacing `/` with `-` (e.g., `/Volumes/workplace/multi-kb` → `-Volumes-workplace-multi-kb`). Reads from `~/.claude/projects/<encoded-path>/`.
- [x] Discovers all `.jsonl` session files in the project directory (filenames are UUIDs, one file = one conversation)
- [x] Filters JSONL lines by `type` field: processes only `user`, `assistant`, and selectively `attachment` lines. Ignores `queue-operation`, `permission-mode`, `file-history-snapshot`, `last-prompt`, `ai-title`.
- [x] **Reassembles split assistant messages (R-3):** Groups consecutive `type: "assistant"` lines with the same `message.id` into a single logical assistant message (Claude Code splits one content block per JSONL line)
- [x] Parses content block arrays (always arrays, never bare strings): `text` blocks → plain text, `thinking` blocks → omit or summarize, `tool_use` blocks → pair with results
- [x] Flattens content block arrays to plain text strings (images → `[Image]` placeholder)
- [x] **Tool call/result pairing (R-3):** Matches `tool_use` blocks (on assistant lines) to `tool_result` blocks (on `type: "user"` lines) via `tool_use_id`. Uses `toolUseResult` outer field for richer summarization metadata when available.
- [x] Collapses tool call/result pairs into `tool_uses` entries on assistant messages
- [x] **Per-message timestamps (R-3 — CHANGED from spec):** Uses the `timestamp` field present on every message line (ISO 8601 with ms precision). Compares each message's `timestamp` to `last_processed` to set `previously_processed` flag individually — NOT at file level. This is the same approach as Notor, replacing the file-level fallback.
- [x] Populates conversation header with source_harness="claude-code", source_path, project_dir
- [x] **Subagent files (R-3):** Skips `<session-uuid>/subagents/` companion directories in MVP. Agent tool results are already captured in the parent conversation's tool result.
- [x] Test cases: single conversation, re-processed conversation with per-message timestamps, assistant message reassembly across split lines, tool call/result pairing via tool_use_id, content block flattening, project path mapping

### TRN-003 [P]: Notor Translator
**Description:** Implement translator that reads Notor chat history and produces intermediate format per spec FR-4. Schema fully documented in [research.md R-4](research.md#r-4-notor-conversation-format).
**Files:**
- `internal/translate/notor.go` — NotorTranslator: discover conversations, translate messages, per-message previously_processed
- `internal/translate/notor_test.go`
**Dependencies:** TRN-001, FND-002
**Acceptance Criteria:**
- [x] **History path discovery (R-4):** Reads `{vault}/.obsidian/plugins/notor/data.json`, parses JSON, extracts `history_path` field (vault-relative). Resolves relative to vault root. Default: `{vault}/.obsidian/plugins/notor/history/`. The spec's placeholder `{vault}/notor/history/` is incorrect.
- [x] **File discovery (R-4):** Lists all `*.jsonl` files in the history directory. Filters out sub-agent files (filenames containing `_subagent_`). Each remaining file is one conversation.
- [x] **JSONL parsing (R-4):** Line 1 parsed as conversation header (`_type: "conversation"` — contains id, created_at, updated_at, provider_id, model_id, persona/workflow metadata). Lines 2+ parsed as messages (`_type: "message"`). Uses `_type` field to discriminate.
- [x] **Role normalization (R-4):** Notor uses 6 roles: `user`, `assistant`, `tool_call`, `tool_result`, `system`, `extension_block`. Maps `user` → user, `assistant` → assistant. Skips `extension_block` messages (internal plugin state). Skips `system` messages that are compaction records (`JSON.parse(content).type === "compaction"`). `tool_call` and `tool_result` are collapsed into tool summaries on the preceding assistant message.
- [x] **Tool call/result pairing (R-4):** Pairs adjacent `tool_call` + `tool_result` messages via `tool_call.id` ↔ `tool_result.tool_call_id`. Generates summary using tool_name, parameters, success/error status, and result. Collapses into `tool_uses` entries on the preceding assistant message.
- [x] **Content extraction (R-4):** Content field can be plain string OR ContentBlock array. If string, use directly. If array, filter to `type: "text"` blocks and join with newline. Skip image/document/custom_block content blocks.
- [x] **Per-message timestamps (R-4):** Uses the `timestamp` field on every message line (ISO 8601 with ms precision, UTC). Compares each message's `timestamp` to `last_processed` to set `previously_processed` flag individually — same approach as Claude Code translator.
- [x] **Persona/workflow metadata (R-4):** Extracts `workflow_name`, `workflow_path`, `persona_name`, and `is_background` from conversation header (line 1). Surfaces in intermediate format conversation header metadata for routing decisions.
- [x] **Workflow instruction filtering (R-4):** Messages with `is_workflow_message: true` can be flagged so the extraction prompt can optionally skip verbose workflow instruction text.
- [x] Populates conversation header with source_harness="notor", source_path, persona_name, workflow_name, project_dir (vault root)
- [x] **Sub-agent conversations (R-4):** Skips files with `_subagent_` in filename. Sub-agent output already captured in parent conversation's `tool_result`.
- [x] Test cases: single conversation, re-processed conversation with per-message timestamps, persona extraction from header, workflow conversation, tool call/result pairing via tool_call_id, content as string vs ContentBlock array, sub-agent file filtering, compaction record skipping, extension_block skipping

### TRN-004: Tool Interaction Summarization
**Description:** Implement summarization of tool call/result pairs — mechanical templates for small interactions, LLM for large ones per contracts/intermediate-format.md.
**Files:**
- `internal/translate/summarize.go` — SummarizeTool function (dispatches to template or LLM)
- `internal/translate/summarize_test.go`
**Dependencies:** TRN-001, FND-011
**Acceptance Criteria:**
- [x] Small interactions (<~1K tokens by estimate): uses mechanical template `"{tool_name}: {brief action} — {brief result}"`
- [x] Large interactions (≥~1K tokens): delegates to Bedrock LLM via `translation.summarization_model_id` (interface dependency; actual Bedrock call wired in EXT-001)
- [x] Template patterns: Read → "Read file X (N lines)", Bash → "Ran 'cmd' — brief result", Write → "Wrote file X", Edit → "Edited file X"
- [x] LLM summarization produces a 1-2 sentence summary of the tool interaction
- [x] Test cases: small Read interaction, small Bash interaction, large interaction (mocked LLM), threshold boundary

---

## Phase 3: Extraction Pipeline

_Corresponds to plan.md Phase C. Builds LLM-powered extraction and routing._

### EXT-001: Bedrock Client Wrapper
**Description:** Implement reusable Bedrock InvokeModel client with retry, backoff, and credential profile resolution per [research.md R-2](research.md#r-2-bedrock-invokemodel-go-sdk-pattern).
**Files:**
- `internal/bedrock/client.go` — Client struct: NewClient (profile, region, model), InvokeModel (system prompt, user message → string response)
- `internal/bedrock/models.go` — Request/response types for Claude Messages API format (R-2 Section 4)
- `internal/bedrock/errors.go` — Error classification with sentinel errors (R-2 Section 6)
- `internal/bedrock/retry.go` — Application-level RetryWithBackoff generic function (R-2 Section 6)
- `internal/bedrock/client_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Creates AWS config with named SSO profile support via `config.WithSharedConfigProfile()` (R-2 Section 1)
- [x] Configures BedrockRuntime client for specified region via `config.WithRegion()`
- [x] Sets HTTP client timeout to 5 minutes via `config.WithHTTPClient()` (R-2 Section 8 — Claude models can take minutes for large context windows)
- [x] InvokeModel sends `anthropic_version: "bedrock-2023-05-31"` + system prompt + user message in Claude Messages API format (R-2 Section 3)
- [x] Model ID set in `InvokeModelInput.ModelId`, NOT in the JSON body; `ContentType` set to `"application/json"` (R-2 Section 2)
- [x] Parses response body JSON, extracts text from `content` array blocks where `type == "text"` (R-2 Section 3)
- [x] SDK-level retry: 5 attempts via `config.WithRetryMaxAttempts(5)` — handles ThrottlingException, ServiceUnavailableException, InternalServerException, network errors automatically (R-2 Section 6)
- [x] Application-level retry: generic `RetryWithBackoff[T]` function for malformed JSON output and ModelTimeoutException (R-2 Section 6)
- [x] Error classification via `classifyError()` using `errors.As()` to match 12 Bedrock error types + SSO `InvalidTokenError` and wrap with sentinel errors (R-2 Section 6)
- [x] Detects expired SSO credentials (`ssocreds.InvalidTokenError`) and surfaces user-friendly "run `aws sso login`" message (R-2 Section 7)
- [x] Supports configurable model ID (e.g., `anthropic.claude-sonnet-4-20250514`, cross-region `us.anthropic.claude-sonnet-4-20250514`)
- [x] Test cases: successful invocation (mocked), retry on throttle, retry exhaustion, profile resolution, SSO expired detection, malformed JSON retry, ModelTimeout retry

### EXT-002: Extraction System Prompt Construction
**Description:** Build the extraction system prompt from hardcoded base + exclusion rules + optional append file per contracts/extraction-output.md.
**Files:**
- `internal/extract/prompt.go` — BuildExtractionPrompt function
- `internal/extract/prompt_test.go`
**Dependencies:** FND-001
**Acceptance Criteria:**
- [x] Hardcoded base prompt defines: extraction task, output format (JSON array of {title, content, suggested_target_kbs}), quality guidelines, previously_processed handling
- [x] Appends exclusion rules section when `exclusion_rules` is non-empty: header "Content exclusion rules — never include in notes destined for non-local KBs" + bulleted list of strings
- [x] Reads and appends `~/.multi-kb/prompts/extraction-append.md` if it exists (fresh read each call)
- [x] Returns combined prompt string
- [x] Test cases: base only, with exclusion rules, with append file, with both, append file missing (no error)

### EXT-003: Single-Pass Extraction
**Description:** Implement single-pass extraction: send translated conversation to Bedrock, parse JSON array output per contracts/extraction-output.md.
**Files:**
- `internal/extract/extract.go` — Extract function (conversation → []Note)
- `internal/extract/parse.go` — ParseExtractionOutput (JSON parsing with partial acceptance)
- `internal/extract/extract_test.go`
- `internal/extract/parse_test.go`
**Dependencies:** EXT-001, EXT-002, TRN-001
**Acceptance Criteria:**
- [x] Sends full intermediate JSONL conversation as user message with constructed system prompt
- [x] Parses JSON array response into []Note structs (title, content, suggested_target_kbs)
- [x] Partial acceptance: valid entries accepted, invalid entries logged and dropped
- [x] Empty array is valid (no knowledge extracted)
- [x] Validates extracted notes: title non-empty ≤255 chars, content non-empty ≤100,000 characters, suggested_target_kbs is string array
- [x] Notes with content exceeding 100K characters are logged as extraction warnings and dropped (not included in results)
- [x] Test cases: successful extraction, empty result, partial valid JSON, completely invalid JSON, field validation

### EXT-004: Chunked Extraction for Oversized Conversations
**Description:** Implement conversation chunking for >700K token conversations per spec FR-5.
**Files:**
- `internal/extract/extract.go` — ExtractChunked (splits, summarizes, iterates)
**Dependencies:** EXT-003, FND-011, PRM-005
**Acceptance Criteria:**
- [x] Detects when translated conversation exceeds 700K tokens (conservative threshold from FND-011's `ChunkingThreshold` constant, leaves headroom for token estimation error)
- [x] Splits at message boundaries (never mid-message) near the 700K mark
- [x] First chunk processed normally via Extract
- [x] Each processed chunk summarized to ~10-20K tokens using extraction model with summarization-specific prompt (PRM-005)
- [x] **Latest summary only** carried forward: each chunk's summary replaces the previous chunk's summary as the preamble for the next chunk (not accumulated). Keeps preamble bounded at ~10-20K regardless of conversation length.
- [x] Context preamble formatted as a leading section in the user message (before the JSONL conversation content)
- [x] All extracted notes from all chunks combined into single result
- [x] Test cases: conversation under threshold (no chunking), over threshold (2 chunks), very large (3+ chunks), split at message boundary, verify preamble replacement (not accumulation)

### EXT-005: Extraction Error Handling
**Description:** Implement retry logic and error logging for extraction failures per spec FR-6 and contracts/extraction-output.md.
**Files:**
- `internal/extract/extract.go` — retry wrapper around extraction calls
**Dependencies:** EXT-003, FND-009
**Acceptance Criteria:**
- [x] Bedrock API failures (throttle, timeout, network): retry up to 3 times with exponential backoff
- [x] Malformed JSON output: retry up to 3 times (fresh API call each retry)
- [x] Partially valid JSON arrays: accept valid entries, log invalid ones
- [x] After 3 retries with no usable output: skip conversation, log to `extraction-errors.jsonl`
- [x] Error log entry includes: conversation_id, source_path, error details, retry count
- [x] Test cases: successful after retry, exhausted retries, partial acceptance

### EXT-006: Routing Engine
**Description:** Implement routing rules that determine which KBs receive each extracted note per spec FR-3 and contracts/extraction-output.md Routing Integration.
**Files:**
- `internal/route/route.go` — RouteNotes function: applies routing rules (always/consider, overrides, fallback)
- `internal/route/route_test.go`
**Dependencies:** FND-001, FND-008
**Acceptance Criteria:**
- [x] For each note, collects target KBs:
  - All `always`-mode KBs for the directory → unconditionally added
  - `consider`-mode KBs whose names appear in `suggested_target_kbs` → added
- [x] Applies per-harness and per-harness+persona overrides (overrides replace, not merge)
- [x] Suggested KB names not matching any configured KB → silently dropped
- [x] If no targets after resolution + no always-mode KBs → fallback to `local/default`
- [x] For each target: checks approval mode → dispatches to auto-approve or pending queue
- [x] Test cases: always-mode routing, consider-mode routing, override resolution, empty suggestions (fallback), mixed approval modes, unknown KB names dropped

### EXT-007: Remote KB submitKnowledge Client
**Description:** Implement the remote KB submission client per contracts/submit-knowledge.md.
**Files:**
- `internal/submit/remote.go` — SubmitToRemoteKB function (SigV4-signed POST, throttle, retry, error handling)
- `internal/submit/remote_test.go`
**Dependencies:** EXT-001, FND-001
**Acceptance Criteria:**
- [x] Sends SigV4-signed POST to `{endpoint}/submitKnowledge` with {title, content, author}
- [x] For `iam` auth: signs with configured AWS profile via SigV4; for `federate` auth: sends plain HTTP POST with no auth headers (the network layer handles identity transparently)
- [x] Pre-flight validation: title ≤255 chars, content ≤100K chars, author ≤100 chars
- [x] Self-throttle: max 10 requests/second per target KB
- [x] HTTP 202: success (UID logged but not stored)
- [x] HTTP 400: pass error + original note to extraction LLM for correction, retry up to 2 times; on persistent failure, stage in pending queue. **Note:** This LLM correction only applies in the capture processing pipeline (EXT-008 path). The approval web server (APR-003) does NOT use LLM correction — it returns 502 immediately.
- [x] HTTP 401/403: log error, skip remaining submissions to this KB for the run, surface credential refresh guidance
- [x] HTTP 5xx / network: retry 3 times with exponential backoff; on persistent failure, log and continue
- [x] Test cases: successful submission, throttling, 400 correction flow, 401 skip behavior, 5xx retry

### EXT-008: Capture Processing Orchestrator (`multi-kb process`)
**Description:** Wire up the full capture pipeline: scan directories → translate → extract → route → submit/stage per spec FR-3.
**Files:**
- `internal/cmd/process.go` — full implementation replacing stub
**Dependencies:** FND-001, FND-002, FND-003, FND-006, FND-009, TRN-002, TRN-003, TRN-004, EXT-003, EXT-004, EXT-005, EXT-006, EXT-007
**Acceptance Criteria:**
- [x] Acquires lock (exits with message if held)
- [x] Reads config and state
- [x] For each tracked directory: identifies conversations modified since `last_processed`
- [x] For each modified conversation: translates → extracts → routes → submits/stages
- [x] Updates `last_processed` to the last-modified time of the final processed file per directory
- [x] Writes atomic state update
- [x] Appends run log entry with all counts
- [x] Releases lock
- [x] Handles partial failures: continues processing remaining conversations/directories after errors
- [x] Test cases: end-to-end with mocked Bedrock (happy path), no new conversations (no-op), lock contention, extraction failure + continue

---

## Phase 4: Hook Injection

_Corresponds to plan.md Phase D. Builds harness hook system._

### HKI-001: Claude Code Hook Registration
**Description:** Implement programmatic registration of a `UserPromptSubmit` hook in Claude Code per [research.md R-5](research.md#r-5-claude-code-hook-registration) and spec FR-7.
**Files:**
- `internal/hook/claudecode.go` — RegisterClaudeCodeHook, UnregisterClaudeCodeHook
- `internal/hook/claudecode_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] **Settings file (R-5):** Reads and writes `~/.claude/settings.json` using JSON read-modify-write. Handles cases: file doesn't exist, `hooks` key absent, `UserPromptSubmit` key absent.
- [x] **Hook entry format (R-5):** Adds entry to `hooks.UserPromptSubmit` array:
  ```json
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
  ```
- [x] Appends alongside any pre-existing hooks in the `UserPromptSubmit` array (never overwrites other entries)
- [x] **Idempotency (R-5):** Before appending, scans existing entries for a command containing `multi-kb hook`. If found, updates the existing entry rather than duplicating.
- [x] UnregisterClaudeCodeHook removes only the multi-kb hook entry from the array
- [x] Preserves all other settings in the file (permissions, env, model, other hook events)
- [x] Test cases: register to empty settings, register alongside existing hooks, idempotent re-register, unregister, file doesn't exist yet, malformed settings file

### HKI-002 [P]: Notor Hook Registration
**Description:** Implement programmatic registration of a conversation-start hook in Notor per [research.md R-6](research.md#r-6-notor-hook-registration) and spec FR-7. Uses the user automation approach: writes a Markdown automation file to the vault.
**Files:**
- `internal/hook/notor.go` — RegisterNotorHook, UnregisterNotorHook
- `internal/hook/notor_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] **Automation file approach (R-6):** Writes a Markdown automation file to `{vault}/notor/automations/multi-kb-recall.md`. Does NOT modify Notor's `data.json` plugin settings.
- [x] **Automation file contents:** Frontmatter with `notor-type: automation`, `notor-trigger: on_conversation_start`, `notor-blocking: true`, `notor-blocking-emit-kind: multi_kb_recall`, `notor-blocking-timeout: 10000`, `notor-automation-order: 100`. Code fence with TypeScript that calls `utils.executeShellCommand("multi-kb hook --harness notor", ...)` passing `context.firstMessage` and `context.conversationId` via stdin JSON, then calls `chatBlocks.emit()` with the CLI's stdout.
- [x] Creates `{vault}/notor/automations/` directory if it doesn't exist
- [x] **Idempotency (R-6):** Checks if `multi-kb-recall.md` already exists. If so, overwrites it (filename is the unique key). Re-running setup produces the same result.
- [x] Coexists with any pre-existing automations (each automation is a separate file; no interference)
- [x] UnregisterNotorHook deletes `{vault}/notor/automations/multi-kb-recall.md` if it exists
- [x] The vault path is resolved from the source directory in `config.yaml`
- [x] Test cases: register to empty automations dir, register when dir doesn't exist (creates it), idempotent re-register (overwrite), unregister, coexistence with other automation files

### HKI-003: Remote KB recallKnowledge Client
**Description:** Implement the remote KB recall client per contracts/recall-knowledge.md.
**Files:**
- `internal/recall/remote.go` — RecallFromRemoteKB function (SigV4-signed POST, timeout)
- `internal/recall/remote_test.go`
**Dependencies:** EXT-001, FND-001
**Acceptance Criteria:**
- [x] Sends SigV4-signed POST to `{endpoint}/recallKnowledge` with {query, limit}
- [x] Parses response: JSON array of {uid, title, content, score}
- [x] Handles `iam` vs `federate` auth: `iam` uses SigV4 signing; `federate` sends plain HTTP POST with no auth headers
- [x] Respects configurable timeout (from hook.timeout)
- [x] Returns partial results on timeout (context cancellation)
- [x] HTTP 400 response: logs warning with error body, returns empty result set (no injection). Does not retry.
- [x] Test cases: successful recall, timeout, auth error, empty results, HTTP 400 (invalid query)

### HKI-004: LLM-Derived Keyword Generation
**Description:** Implement keyword derivation from natural language queries for local KB recall per spec FR-8.
**Files:**
- `internal/recall/keywords.go` — DeriveKeywords function (LLM call → 3-5 keywords)
- `internal/recall/keywords_test.go`
**Dependencies:** EXT-001
**Acceptance Criteria:**
- [x] Calls `translation.summarization_model_id` (e.g., Claude Haiku) to derive 3-5 search keywords from user's first message
- [x] System prompt instructs: extract 3-5 key search terms, return as JSON array of strings
- [x] Parses response as string array
- [x] Fallback: if LLM call fails, use mechanical keyword extraction (split on whitespace, remove stop words)
- [x] Test cases: successful derivation (mocked), fallback on failure, various query types

### HKI-005: Rank-Based Result Interleaving
**Description:** Implement merging of results from multiple KBs via rank-based interleaving per spec FR-7.
**Files:**
- `internal/recall/merge.go` — InterleaveResults function
- `internal/recall/merge_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] Accepts ranked result lists from multiple KBs (remote sorted by score, local by match count)
- [x] Interleaves by rank: top-ranked from each KB first, then second-ranked, etc.
- [x] If one KB has fewer results, remaining slots filled from KBs with results remaining
- [x] Truncates to 10 total notes
- [x] Deduplicates by UID (if same note appears from multiple KBs)
- [x] Test cases: equal-length lists, unequal lengths, single KB, no results, duplicate UIDs, more than 10 total

### HKI-006: Markdown Injection Formatting
**Description:** Format recalled notes for injection into harness hook output, including pending notice per spec FR-7.
**Files:**
- `internal/recall/format.go` — FormatInjection function (returns Markdown string), FormatHookOutput function (wraps for harness-specific output)
- `internal/recall/format_test.go`
**Dependencies:** FND-008
**Acceptance Criteria:**
- [x] FormatInjection produces a Markdown string with each note's title (as heading), source KB name, and full content
- [x] When pending queue is non-empty: includes notice at the end (e.g., "3 notes awaiting approval — run `multi-kb approve` to review")
- [x] Empty results produce empty string (silent — no "no results found" message)
- [x] **Claude Code output format (R-5):** FormatHookOutput for `claude-code` harness wraps the Markdown in a JSON object: `{"systemMessage": "<markdown>"}`. The `systemMessage` field is what Claude Code injects into the system context.
- [x] **Notor output format (R-6):** FormatHookOutput for `notor` harness outputs raw Markdown to stdout (no JSON wrapper). The Notor automation TypeScript code reads this stdout and calls `chatBlocks.emit()` to inject it as an extension block visible to the LLM.
- [x] Test cases: multiple notes from multiple KBs, single note, pending notice, no pending, empty results, Claude Code JSON wrapping, Notor raw output

### HKI-007: Hook Entry Point Command (`multi-kb hook`)
**Description:** Wire up the hook subcommand that orchestrates the full injection flow per spec FR-7. Behavior reshaped by [research.md R-5](research.md#r-5-claude-code-hook-registration) and [R-6](research.md#r-6-notor-hook-registration) findings.
**Files:**
- `internal/cmd/hook.go` — full implementation replacing stub
- `internal/hook/inject.go` — core injection orchestrator
**Dependencies:** FND-001, FND-007, FND-009, HKI-003, HKI-004, HKI-005, HKI-006
**Acceptance Criteria:**
- [x] Accepts `--harness` flag (claude-code or notor)
- [x] **Claude Code stdin parsing (R-5):** Reads JSON from stdin containing `user_prompt`, `session_id`, `transcript_path`, `cwd`. Parses the `user_prompt` field as the query text. Uses `cwd` (or `$CLAUDE_PROJECT_DIR` env var) to identify the current directory for routing config.
- [x] **Notor stdin parsing (R-6):** Reads JSON from stdin containing `first_message`, `conversation_id`, `timestamp` (passed by the Notor automation wrapper). Parses the `first_message` field as the query text. Uses the vault root (CWD, since Notor sets CWD to vault root) to identify the current directory for routing config.
- [x] **First-message guard (R-5):** For Claude Code only — reads the `transcript_path` file and counts `user`-type JSONL lines. If count >= 1 (prior user messages exist), this is not the first message — exit immediately with code 0 and no stdout output. If count is 0 (transcript empty, missing, or contains no user-type entries), this is the first message — proceed with injection. Note: the hook fires before the current prompt is written to the transcript, so count=0 means the current prompt is the first message. **No first-message guard needed for Notor** — the `on_conversation_start` trigger only fires on the first message.
- [x] **Pre-flight validation:** If the extracted query (user's first message) is empty or whitespace-only, skip recall entirely — exit with code 0 and no stdout output. Do not send empty queries to KBs.
- [x] Identifies target KBs for the current directory from config
- [x] Queries all target KBs concurrently (local via git grep with LLM keywords, remote via recallKnowledge)
- [x] Merges results via rank-based interleaving → top 10
- [x] **Output format (R-5, R-6):** For Claude Code, writes JSON to stdout: `{"systemMessage": "<formatted markdown>"}`. For Notor, writes raw Markdown to stdout (no JSON wrapper — the automation TypeScript code handles `chatBlocks.emit()`). Exit code 0 on success.
- [x] Enforces configurable timeout (default 8s) — partial results from responsive KBs used if others time out
- [x] On complete timeout (no KBs respond): exit code 0 with no stdout output, warning logged to `hook-errors.jsonl`
- [x] **Error handling (R-5):** On fatal error, exit with non-0/non-2 code (non-blocking error — both Claude Code and Notor proceed without injection). Never exit with code 2 for Claude Code (blocking error). For Notor, any non-zero exit from the shell command is handled by the automation wrapper.
- [x] Test cases: full injection path (mocked), first-message guard via transcript for Claude Code (pass/block), stdin JSON parsing for both harnesses, JSON output format for Claude Code, raw Markdown output for Notor, timeout handling, partial results, error exit codes

---

## Phase 5: Dream Cycle (Local)

_Corresponds to plan.md Phase E. Builds client-mode dream cycle._

### DRM-001: Dream Cycle Orchestrator
**Description:** Implement the Phase 0-4 sequencing for local dream cycles per spec FR-8.
**Files:**
- `internal/dreamcycle/cycle.go` — RunDreamCycle function (phase sequencing, lock management)
- `internal/dreamcycle/cycle_test.go`
**Dependencies:** FND-003, FND-002
**Acceptance Criteria:**
- [x] Acquires lock with activity="dream_cycle"
- [x] Executes phases 0-4 in sequence
- [x] Phase 0: no-op for local (returns immediately)
- [x] Releases lock on completion or error
- [x] Records run log entry (dream_cycle type) on completion
- [x] If any phase fails: already-committed batches preserved, remaining pending notes left for next run
- [ ] Test cases: successful full cycle (mocked phases), failure mid-cycle, lock acquisition failure

### DRM-002: Phase 1 — Singleton Batch Creation
**Description:** Implement Phase 1: find all `status: pending` notes and create singleton batches per spec FR-8.
**Files:**
- `internal/dreamcycle/phase1.go` — CreateBatches function
- `internal/dreamcycle/phase1_test.go`
**Dependencies:** FND-005, FND-006
**Acceptance Criteria:**
- [x] Scans local KB for all notes with `status: pending` in frontmatter
- [x] Each pending note becomes its own batch (singleton — no similarity grouping for local)
- [x] Returns list of batches, each containing one note
- [x] Empty list if no pending notes (dream cycle exits early)
- [x] Test cases: multiple pending notes, no pending notes, mixed pending/active notes

### DRM-003: Phase 2 — Git Grep Related Note Retrieval
**Description:** Implement Phase 2: for each batch, find related `status: active` notes via keyword-based `git grep` per spec FR-8.
**Files:**
- `internal/dreamcycle/phase2.go` — FindRelatedNotes function
- `internal/dreamcycle/phase2_test.go`
**Dependencies:** FND-007
**Acceptance Criteria:**
- [x] Derives keywords mechanically from the note's title and key terms (no LLM call — unlike hook recall)
- [x] Runs `git grep` per keyword against the local KB
- [x] Filters to `status: active` notes only
- [x] Returns up to 10 related active notes per batch
- [x] Ranks by match count (same as recall ranking)
- [x] Test cases: note with matching active notes, no related notes, keyword derivation from title

### DRM-004: Phase 3 — LLM Consolidation and Action Application
**Description:** Implement Phase 3: send each batch (pending note + related notes) to LLM for consolidation, parse actions, apply them per spec FR-8 and contracts/consolidation-output.md.
**Files:**
- `internal/dreamcycle/phase3.go` — ConsolidateBatch function (LLM call, action parsing)
- `internal/dreamcycle/actions.go` — ApplyActions function (keep, merge, split, consolidate)
- `internal/dreamcycle/notestore.go` — `NoteStore` interface definition
- `internal/dreamcycle/phase3_test.go`
- `internal/dreamcycle/actions_test.go`
**Dependencies:** EXT-001, FND-006, FND-005, PRM-002
**Contract:** [consolidation-output.md](contracts/consolidation-output.md)
**Acceptance Criteria:**
- [x] **Interface abstraction:** Action application (file reads, writes, deletes, commits) uses a `NoteStore` interface with methods: `ReadNote(uid) -> Note`, `WriteNote(note)`, `DeleteNote(uid)`, `CommitBatch(message)`. Local mode implements with `internal/git/repo.go` operations; server mode implements with `internal/server/codecommit.go` operations. DRM-004 depends on the interface, not the concrete implementation.
- [x] Constructs consolidation prompt with: the pending note (singleton batch) + related active notes
- [x] Calls `dream_cycle.model_id` via Bedrock
- [x] Parses LLM response into action types: keep, merge, split, consolidate
- [x] **keep:** flip pending note to `status: active`, update `last-updated`
- [x] **merge:** merge content into target note, delete source note, update `consolidated-from-notes` on target
- [x] **split:** create multiple new active notes from one pending note, delete original
- [x] **consolidate:** create one new active note from multiple notes, delete originals, update `consolidated-from-notes`
- [x] **Content length heuristic for consolidate:** If a `consolidate` action references active notes, warn-log if the new `content` length is less than 80% of the combined source content length. The action still proceeds (it's a warning, not a block), but the warning is recorded in the run log for auditability.
- [x] Per-batch git commit after applying all actions
- [x] Git commit message for consolidate actions explicitly lists deleted active note UIDs (e.g., `dream-cycle: consolidate — deleted active notes ABC123, DEF456`)
- [x] Test cases: keep action, merge action, split action, consolidate action, consolidate with active notes (verify warning heuristic), LLM failure (skip batch)

### DRM-005: Dream Cycle Commands
**Description:** Wire up `multi-kb dream-cycle` (standalone) and integrate into `multi-kb run` (combined capture + dream cycle).
**Files:**
- `internal/cmd/dreamcycle.go` — full implementation replacing stub
- `internal/cmd/run.go` — full implementation replacing stub
**Dependencies:** DRM-001, EXT-008
**Acceptance Criteria:**
- [x] `multi-kb dream-cycle`: acquires lock, runs dream cycle, releases lock
- [x] `multi-kb run`: acquires lock once, runs capture processing then dream cycle sequentially, releases lock
- [x] Both respect lock file: if held, manual commands print message with lock holder info and exit; scheduled runs skip silently
- [x] Both append appropriate run log entries
- [ ] Test cases: standalone dream cycle, combined run, lock contention

---

## Phase 6: Setup Wizard & Scheduling

_Corresponds to plan.md Phase F. Builds interactive setup and cron integration._

### WIZ-001: Terminal Wizard — Harness Selection and Source Discovery
**Description:** Implement the first portion of the setup wizard: select harnesses, point to directories, discover sources per spec FR-2 and [research.md R-1](research.md#r-1-bubbletea-wizard-pattern).
**Files:**
- `internal/cmd/setup.go` — partial implementation (wizard flow Phase 1 form + async discovery)
**Dependencies:** ENV-002, FND-001
**Acceptance Criteria:**
- [x] **Uses `charm.land/huh/v2` + `charm.land/bubbletea/v2` (R-1).** Form components from `huh`, parent bubbletea program manages inter-step async logic.
- [x] **Phase 1 Form** — `huh.NewForm` with Groups:
  - Group 1: `huh.NewNote` welcome + `huh.NewMultiSelect` for harness selection (Notor, Claude Code)
  - Group 2 (conditional via `WithHideFunc`): Claude Code directory path input (`huh.NewInput` with path validation via `Validate(func(s string) error {...})`)
  - Group 3 (conditional via `WithHideFunc`): Notor vault path input (`huh.NewInput` or `huh.NewFilePicker` with `DirAllowed(true)`)
- [x] **Conditional group visibility (R-1):** Use `WithHideFunc` closures watching the `selectedHarnesses` variable — only show Claude Code directory group if Claude Code selected, only show Notor group if Notor selected
- [x] **Async discovery step** (between Phase 1 and Phase 2 forms): Run inside bubbletea parent program with spinner. Auto-discovers chat history locations:
  - Claude Code: reads from `~/.claude/projects/`, matches user-pointed directory to project subdirectory (R-3 path mapping), presents summary via `huh.NewNote`
  - Notor: reads `{vault}/.obsidian/plugins/notor/data.json` for `history_path` (R-4), checks path exists, presents summary
- [x] User confirms discovered sources via `huh.NewConfirm`
- [x] **Accessibility (R-1):** Support `WithAccessible(true)` for screen readers (falls back to sequential prompts)
- [ ] Test cases: single harness selection, both harnesses, directory validation, source discovery, conditional group hiding

### WIZ-002: Terminal Wizard — KB Configuration and Routing
**Description:** Implement the second and third portions: local KB creation, remote KB addition, routing configuration, approval mode, author, exclusion rules per spec FR-2 and [research.md R-1](research.md#r-1-bubbletea-wizard-pattern).
**Files:**
- `internal/cmd/setup.go` — partial implementation (wizard flow Phases 2 and 3)
**Dependencies:** WIZ-001, FND-005
**Acceptance Criteria:**
- [x] **Phase 2 Form** (R-1 — `huh.NewForm`):
  - `huh.NewConfirm` for discovery summary
  - `huh.NewConfirm` to add remote KB (loop via sequential forms — "Add another?" pattern)
  - For each remote KB: `huh.NewInput` (name, endpoint URL), `huh.NewSelect` (auth type: iam/federate), conditional `huh.NewInput` (AWS profile, shown via `WithHideFunc` only when auth=iam), `huh.NewInput` (region, description)
  - `huh.NewSelect` per directory-KB pair for routing mode (always/consider) and approval mode presets (`huh.NewSelect` with 3 presets: auto-approve always, always require manual, select per group)
- [x] **Phase 3 Form** (R-1 — `huh.NewForm`):
  - `huh.NewInput` for author identity string with `ValidateNotEmpty()` + `ValidateMaxLength(100)`
  - `huh.NewText` for exclusion rules (multi-line, optional — user can leave empty)
  - `huh.NewNote` showing setup summary
  - `huh.NewConfirm` to finalize
- [x] **Dynamic fields (R-1):** Use `OptionsFunc` for routing target selection — options regenerate based on configured KBs
- [x] Creates default local KB automatically (`~/.multi-kb/local/default/`)
- [x] Writes complete `config.yaml` and initial empty `state.yaml`
- [x] **No looping within single form (R-1 gotcha):** "Add another KB?" uses sequential form invocations, not in-form loops
- [ ] Test cases: minimal setup (local only), with remote KB, with overrides, with exclusion rules, accessible mode

### WIZ-003: Hook Auto-Registration During Setup
**Description:** During setup, automatically register hooks for each selected harness per spec FR-2.
**Files:**
- `internal/cmd/setup.go` — hook registration step
**Dependencies:** WIZ-001, HKI-001, HKI-002
**Acceptance Criteria:**
- [x] For each selected harness, calls the appropriate hook registration function
- [x] Claude Code: registers `user_prompt_submit` hook
- [x] Notor: registers conversation-start hook
- [x] Appends alongside existing hooks (never overwrites)
- [x] Reports registration status to user
- [ ] Test cases: single harness hook registration, both harnesses, pre-existing hooks preserved

### WIZ-004: Cron Registration
**Description:** Implement OS-native cron registration for `multi-kb run` per [research.md R-8](research.md#r-8-cross-platform-cron-registration) and spec FR-3.
**Files:**
- `internal/schedule/cron_unix.go` — Install, Uninstall, IsInstalled (macOS/Linux) — build tag `//go:build unix`
- `internal/schedule/cron_windows.go` — Install, Uninstall, IsInstalled (Windows) — build tag `//go:build windows`
- `internal/schedule/cron_unix_test.go`
- `internal/schedule/cron_windows_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] **macOS/Linux crontab (R-8):** Read-modify-write pattern via `crontab -l` → modify in memory → `crontab -` (stdin replacement). Never directly edit crontab files.
- [x] **Empty crontab handling (R-8):** `crontab -l` exits code 1 on empty crontab — treat as empty (not error). Check `exitErr.ExitCode() == 1`.
- [x] **Inline marker (R-8):** Append `# multi-kb scheduled run` as trailing comment on the cron entry line. Use `strings.Contains(line, marker)` for idempotency detection.
- [x] **Idempotency (R-8):** Before appending, filter out any existing line containing the marker. Read-filter-append-write guarantees exactly one multi-kb entry regardless of how many times setup runs.
- [x] **Absolute binary path (R-8):** Use `os.Executable()` + `filepath.EvalSymlinks()` to resolve the binary's absolute path. Cron runs with minimal PATH — binary may not be found otherwise.
- [x] **Output redirection (R-8):** Append `>> ~/.multi-kb/logs/cron.log 2>&1` to prevent cron mail accumulation. Expand `~` to absolute path at registration time via `os.UserHomeDir()`.
- [x] **Cron entry format:** `{cronExpr} {absPath} run --config {absConfigPath} >> {absLogPath} 2>&1 # multi-kb scheduled run`
- [x] **Uninstall (R-8):** Filter out marker line and write back. If result is empty, use `crontab -r`.
- [x] **Windows Task Scheduler (R-8):** `schtasks.exe /Create /SC MINUTE /MO {interval} /TN "multi-kb-run" /TR "cmd /c \"{absPath} run --config {configPath} >> {logPath} 2>&1\"" /F /NP /RL LIMITED`. `/F` provides idempotency. `/NP` prevents password prompting. No admin needed.
- [x] **Windows uninstall:** `schtasks.exe /Delete /TN "multi-kb-run" /F`
- [x] **Build tags:** `//go:build unix` and `//go:build windows` with file naming convention `_unix.go` / `_windows.go`
- [x] **Scheduler interface:** Define `Scheduler` interface with `Install(cronExpr, binaryPath, configPath)`, `Uninstall()`, `IsInstalled()` — platform-specific `New()` factory
- [ ] Test cases: register fresh, idempotent re-register, unregister, existing crontab preserved, empty crontab edge case

### WIZ-005: Cron Expression Parsing for Status Display
**Description:** Parse the crontab entry to compute and display the next scheduled run time per spec FR-11 and [research.md R-8](research.md#r-8-cross-platform-cron-registration).
**Files:**
- `internal/schedule/parse.go` — ParseNextRun function (reads crontab, finds multi-kb entry, computes next occurrence)
- `internal/schedule/parse_test.go`
**Dependencies:** WIZ-004
**Acceptance Criteria:**
- [x] **macOS/Linux (R-8):** Reads user's crontab via `crontab -l`, finds the `multi-kb run` entry by marker comment `# multi-kb scheduled run`, extracts the cron expression (first 5 space-separated fields)
- [x] **Next-run computation (R-8):** Uses `github.com/robfig/cron/v3` library — `cron.ParseStandard(expr).Next(time.Now())` to compute next occurrence
- [x] **Windows (R-8):** Reads schedule via `schtasks.exe /Query /TN "multi-kb-run" /FO CSV /NH`, parses the next run time from the CSV output
- [x] Returns absolute timestamp (e.g., "2026-05-01 14:30:00")
- [x] Returns nil/error if no cron entry or scheduled task found
- [x] **Go dependency:** `github.com/robfig/cron/v3` (well-maintained, 15k+ stars, supports standard 5-field expressions)
- [ ] Test cases: common intervals (every 30 min, hourly, daily), next occurrence calculation, missing entry, Windows CSV parsing

### WIZ-006 [P]: Standalone Subcommands — add-source and add-kb
**Description:** Implement post-setup subcommands for adding sources and KBs per spec FR-2.
**Files:**
- `internal/cmd/addsource.go` — full implementation (interactive prompts for new source directory)
- `internal/cmd/addkb.go` — full implementation (interactive prompts for new remote KB)
**Dependencies:** FND-001, WIZ-001
**Acceptance Criteria:**
- [x] `multi-kb add-source`: prompts for directory, harness(es), routing targets, writes to existing config.yaml
- [x] `multi-kb add-kb`: prompts for name, endpoint, auth type, profile, region, description, writes to existing config.yaml
- [x] Both validate input against config validation rules
- [x] Both handle the case where config doesn't exist (suggests running setup first)
- [ ] Test cases: add source to existing config, add KB to existing config, validation failures

---

## Phase 7: Approval Web UI

_Corresponds to plan.md Phase G. Builds on-demand approval web server._

### APR-001: Embedded HTML/CSS/JS Assets
**Description:** Create the single-page approval UI and embed it in the Go binary via `embed.FS` per spec FR-9.
**Files:**
- `internal/approve/assets/index.html` — single-page approval UI
- `internal/approve/assets/styles.css` — styling
- `internal/approve/assets/app.js` — client-side logic (fetch notes, approve/reject, inline edit)
- `internal/approve/assets/embed.go` — `embed.FS` declaration
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] UI lists all pending notes with: title, content preview, target KBs, source conversation, extraction timestamp
- [x] Each target KB shown as individual approval target (approve/reject per KB)
- [x] Inline edit: title and content fields are editable before approving
- [x] Approve button sends POST to `/api/notes/:filename/approve`
- [x] Reject button sends POST to `/api/notes/:filename/reject`
- [x] UI updates dynamically after each action (removes resolved targets, removes fully resolved notes)
- [x] `embed.FS` embeds all assets at compile time
- [x] Responsive, functional design (not polished — visual design deferred per spec Out of Scope)

### APR-002: HTTP Server Lifecycle
**Description:** Implement the approval web server lifecycle: auto-port, browser launch, idle timeout, shutdown per spec FR-9.
**Files:**
- `internal/approve/server.go` — StartServer, activity tracking, idle timeout, shutdown
- `internal/approve/server_test.go`
**Dependencies:** APR-001
**Acceptance Criteria:**
- [x] Binds to `localhost` on auto-selected available port
- [x] Prints URL to terminal (e.g., "Approval UI running at http://localhost:52431")
- [x] Opens user's default browser to the URL
- [x] Tracks activity: any HTTP request resets the idle timer
- [x] Shuts down after configurable idle timeout (default 5 minutes)
- [x] Shuts down immediately when all pending notes are resolved
- [x] Ctrl+C terminates immediately (graceful shutdown)
- [ ] Test cases: server starts, idle timeout fires, all-resolved shutdown, manual shutdown

### APR-003: API Handlers
**Description:** Implement the REST API endpoints per contracts/approval-api.md.
**Files:**
- `internal/approve/handlers.go` — GET /api/notes, POST approve, POST reject
- `internal/approve/handlers_test.go`
**Dependencies:** APR-002, FND-008, FND-006, EXT-007
**Acceptance Criteria:**
- [x] `GET /` — serves embedded HTML
- [x] `GET /api/notes` — returns JSON array of all pending notes from `~/.multi-kb/pending/`
- [x] `POST /api/notes/:filename/approve` — body: {target_kb, title, content}
  - Submits to target KB:
    - **Local KB:** generates UID at approval time via GenerateUID() (FND-004), writes `<UID>.md` file with frontmatter + content
    - **Remote KB:** calls submitKnowledge API (server generates UID)
  - On successful submission: removes target from pending file's target_kbs; deletes file if no targets remain
  - Edits to title/content persist in pending file for remaining targets
  - Returns {remaining_targets}
- [x] `POST /api/notes/:filename/reject` — body: {target_kb}
  - Removes target from target_kbs
  - Deletes file if no targets remain
  - Returns {remaining_targets}
- [x] Error responses: 404 (file not found), 400 (target not in array), 502 (submission failed — pending file left unchanged, target NOT removed, user can retry)
- [x] **Approval error handling:** On remote KB submission failure (400/401/5xx), return HTTP 502 with error details to the UI. Leave the pending file unchanged (target not removed). No retry logic, no LLM correction — keep the approval flow simple. User can retry from the UI.
- [x] Test cases: list notes, approve for one target (local KB — verify UID generated), approve for remote KB, approve last target (file deleted), reject, edit before approve, 404, 400, remote submission failure (502 returned, pending unchanged)

### APR-004: Approve Command Wiring
**Description:** Wire up `multi-kb approve` to launch the web server per spec FR-9.
**Files:**
- `internal/cmd/approve.go` — full implementation replacing stub
**Dependencies:** APR-002, APR-003
**Acceptance Criteria:**
- [x] Starts the approval web server
- [x] Checks for pending notes first; if none, prints "No notes awaiting approval" and exits
- [x] Prints server URL to terminal
- [x] Blocks until server shuts down (idle timeout, all resolved, or Ctrl+C)
- [ ] Test cases: launch with pending notes, launch with no pending notes

---

## Phase 8: Server Mode

_Corresponds to plan.md Phase H. Builds server-mode operation (FR-12)._

### SRV-001: Server Config Loading and Validation
**Description:** Extend config loading to support server-mode configuration sections per data-model.md Server Config Extensions.
**Files:**
- `internal/config/config.go` — add server-mode fields (sqs, codecommit, s3, opensearch, bedrock_kb, tick_interval, recall_log)
- `internal/config/validate.go` — add server-mode validation rules
- `internal/config/config_test.go` — add server-mode test cases
**Dependencies:** FND-001
**Contract:** CDK [server-config.md](../cdk/contracts/server-config.md) — defines the fields the CDK user data script templates; CLI validation must match
**Acceptance Criteria:**
- [x] Parses all server-mode fields: sqs.queue_url, sqs.batch_size, codecommit.repo_name/region, s3.bucket/region, opensearch.endpoint/region, bedrock_kb.knowledge_base_id/data_source_id, tick_interval, dream_cycle.interval/model_id, recall_log.schedule
- [x] Server-mode fields only validated when `mode: server`
- [x] Required fields for server mode: sqs.queue_url, codecommit.repo_name, s3.bucket, opensearch.endpoint, bedrock_kb.knowledge_base_id/data_source_id
- [x] Duration fields (`tick_interval`, `dream_cycle.interval`) validated using `time.ParseDuration` (same rules as FND-001); `recall_log.schedule` validated as `HH:MM` UTC
- [x] Test cases: valid server config, missing required server fields, client mode ignores server fields, invalid duration for tick_interval, invalid schedule format

### SRV-002: Tick Loop and Activity Dispatch
**Description:** Implement the server-mode main loop: periodic tick, dream cycle vs ingestion dispatch per spec FR-12.
**Files:**
- `internal/server/server.go` — RunServer function (tick loop, activity selection, systemd-friendly)
- `internal/server/server_test.go`
**Dependencies:** SRV-001, FND-003
**Acceptance Criteria:**
- [x] Runs as long-lived process (not short-lived)
- [x] Wakes every `tick_interval` (default 5 minutes)
- [x] On each tick: checks if dream cycle is due (elapsed > dream_cycle.interval, default 3h)
  - If due: runs dream cycle
  - Otherwise: runs SQS ingestion + recall log processing (if daily threshold crossed)
- [x] If previous tick still running, skips current tick (no concurrent processing)
- [x] Maintains lock file with heartbeat
- [x] Handles SIGTERM/SIGINT for graceful shutdown (systemd integration)
- [x] Test cases: tick dispatching logic, dream cycle scheduling, skip-on-busy, graceful shutdown

### SRV-003 [P]: SQS Polling and Batching
**Description:** Implement SQS message polling, batching, and acknowledgement per spec FR-12 SQS Ingestion.
**Files:**
- `internal/server/ingest.go` — PollSQS, ProcessBatch, DeleteMessages
- `internal/server/ingest_test.go`
**Dependencies:** SRV-001
**Acceptance Criteria:**
- [x] Polls configured SQS queue via AWS SDK
- [x] Batches ~5-10 messages (configurable batch_size)
- [x] Parses each message: {uid, title, content, author, submitted_at}
- [x] Hands batch to CodeCommit operations for commit
- [x] Deletes successfully processed messages from queue
- [x] Failed messages left in queue for SQS retry (up to 3 attempts before DLQ)
- [x] Test cases: successful batch, partial failure, empty queue, malformed message

### SRV-004 [P]: CodeCommit Git Operations
**Description:** Implement CodeCommit git operations for server mode: clone, commit batches of note files per spec FR-12.
**Files:**
- `internal/server/codecommit.go` — CloneRepo, CommitBatch
**Dependencies:** FND-005
**Implementation Note:** Uses `os/exec` shell-out to `git` binary. CodeCommit credential helper configured via `git config` (set up by CDK user data script). Sanitize SQS message fields (UID, title, author) before use in file names or git commit messages.
**Acceptance Criteria:**
- [x] Clones CodeCommit repository via HTTPS (git-codecommit VPC endpoint)
- [x] Creates `<UID>.md` Markdown files with full frontmatter per data-model.md Entity 1: uid, title, status: pending, author, last-updated (from submitted_at), empty last-linked-to/last-recalled/consolidated-from-notes
- [x] Commits entire batch as a single git commit
- [x] Handles git push to CodeCommit
- [x] Test cases: clone, commit single note, commit batch, push

### SRV-005 [P]: Incremental S3 Sync
**Description:** Implement incremental S3 sync after each git commit per spec FR-12 S3 Incremental Sync.
**Files:**
- `internal/server/s3sync.go` — SyncToS3 function (git diff → upload/delete)
- `internal/server/s3sync_test.go`
**Dependencies:** SRV-004
**Acceptance Criteria:**
- [x] Uses `git diff` between previous and current commit to determine changeset
- [x] Files added/modified → S3 PutObject
- [x] Files deleted → S3 DeleteObject
- [x] Not a full repo comparison — incremental only
- [x] Retry up to 3 times with exponential backoff on sync failure
- [x] On persistent failure: log error and continue (next sync or Phase 0 catches missed files)
- [x] Test cases: upload new files, delete files, retry on failure, empty diff (no-op)

### SRV-006: Server Dream Cycle (OpenSearch-Backed Phases)
**Description:** Implement server-mode dream cycle with OpenSearch for Phase 1/2 and Bedrock KB sync for Phase 0/4 per spec FR-12.
**Files:**
- `internal/server/dreamcycle.go` — ServerDreamCycle: Phase 0 (sync+reindex), Phase 1 (OpenSearch pending query + similarity grouping), Phase 2 (OpenSearch related query), Phase 4 (final sync + reindex)
- `internal/server/dreamcycle_test.go`
**Dependencies:** DRM-004, SRV-004, SRV-005
**Contract:** [consolidation-output.md](contracts/consolidation-output.md) — Phase 3 shares logic with DRM-004
**Acceptance Criteria:**
- [x] Phase 0: syncs CodeCommit→S3, triggers `StartIngestionJob`, polls `GetIngestionJob` with 10-min hard cutoff (proceeds best-effort if timeout)
- [x] Phase 1: queries OpenSearch (via VPC endpoint) for `status: pending` notes; groups into batches by similarity (max 10 per batch): pick ungrouped seed, query for similar pending notes, form batch, repeat
- [x] Phase 2: for each batch, queries OpenSearch for related `status: active` notes (max 10 per batch)
- [x] Phase 3: identical to local mode (DRM-004 — shared logic)
- [x] Phase 4: final S3 sync, triggers StartIngestionJob, polls for completion, updates dream cycle timestamp, releases lock
- [x] Test cases: full cycle with mocked OpenSearch/Bedrock, Phase 0 timeout, similarity grouping

### SRV-007: Daily Recall Log Processing
**Description:** Implement daily recall log processing per spec FR-12 Recall Log Processing.
**Files:**
- `internal/server/recalllog.go` — ProcessRecallLogs function
- `internal/server/recalllog_test.go`
**Dependencies:** SRV-004, SRV-005
**Acceptance Criteria:**
- [x] Runs once per day (tracked by last-run timestamp; first eligible non-dream-cycle tick after daily threshold)
- [x] Scans S3 objects under `recall-logs/<previous-day-YYYY-MM-DD>/` prefix
- [x] Parses each recall log JSON: {timestamp, query, recalled_uids}
- [x] For each unique UID: updates `last-recalled` frontmatter in the CodeCommit note
- [x] Silently skips UIDs for notes that no longer exist
- [x] Commits all `last-recalled` updates as a single git commit
- [x] Test cases: process logs with multiple UIDs, missing notes, empty log day, already processed

---

## Phase 9: Build & Distribution

_Corresponds to plan.md Phase I._

### BLD-001: Cross-Platform Build Matrix
**Description:** Configure the build system for all target platforms per spec NFR-1.
**Files:**
- `Makefile` — update with full build matrix
- `goreleaser.yml` (optional) — if using goreleaser
**Dependencies:** ENV-003
**Acceptance Criteria:**
- [x] Builds for: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
- [x] All builds use `CGO_ENABLED=0` for fully static binaries
- [x] Binary naming convention: `multi-kb-<os>-<arch>[.exe]`
- [x] `make build-all` produces all 5 binaries
- [x] Each binary runs `--version` successfully on its target platform (or via cross-testing where available)

### BLD-002 [P]: Binary Size Optimization
**Description:** Audit and optimize binary size using ldflags and build tags.
**Files:**
- `Makefile` — add optimization flags
**Dependencies:** BLD-001
**Acceptance Criteria:**
- [x] Uses `-ldflags="-s -w"` to strip debug info and DWARF symbols
- [x] Evaluates `upx` compression (optional, document trade-offs)
- [x] Documents final binary sizes per platform
- [x] Binary size is reasonable for a Go CLI with embedded web assets (<30MB target)

---

## Cross-Cutting: LLM Prompt Authoring

_These tasks produce the core LLM prompts that drive the system's intelligence. Each prompt should be authored, tested against sample data, and iterated before the code that invokes it is finalized. Prompts can be worked on in parallel with each other._

### PRM-001 [P]: Extraction System Prompt
**Description:** Author the hardcoded extraction system prompt that instructs the LLM to extract knowledge notes from translated conversations per contracts/extraction-output.md.
**Files:**
- `internal/extract/prompts/extraction.go` — prompt text as Go constant or embedded file
- Sample test conversations for validation
**Dependencies:** None (can start early)
**Acceptance Criteria:**
- [x] Defines the extraction task, output JSON format (title/content/suggested_target_kbs), and quality guidelines
- [x] Instructs LLM to focus on `previously_processed: false` messages while using full conversation for context
- [x] Instructs LLM to avoid extracting trivial or obvious information
- [x] Instructs LLM to keep individual note content concise and focused (generally under 5,000 characters per note; hard limit of 100,000 characters enforced by the parser)
- [x] Specifies that empty array `[]` is valid when no knowledge is extractable
- [ ] Tested against ≥3 sample conversations: one with clear knowledge, one with no extractable knowledge, one re-processed conversation with mixed flags
- [x] Prompt length reasonable (under ~2K tokens)

### PRM-002 [P]: Dream Cycle Consolidation Prompt
**Description:** Author the consolidation system prompt for dream cycle Phase 3 per contracts/consolidation-output.md.
**Files:**
- `internal/dreamcycle/prompts/consolidation.go` — prompt text
- Sample test batches for validation
**Dependencies:** contracts/consolidation-output.md
**Acceptance Criteria:**
- [x] Instructs LLM to evaluate each pending note against related active notes
- [x] Defines all four action types (keep, merge, split, consolidate) with clear criteria for when to use each
- [x] Specifies the JSON output schema from consolidation-output.md
- [x] Instructs that every pending note UID must appear in exactly one action
- [x] Instructs to preserve information — never silently discard content
- [x] Explicitly instructs LLM that consolidating active notes is a high-stakes operation: the new note must contain ALL information from all source notes. The LLM should prefer `keep` over `consolidate` when uncertain about information preservation.
- [x] Distinguishes merge (absorb into existing) from consolidate (create new from multiple)
- [ ] Tested against ≥3 sample batches: one with a novel note (keep), one with a duplicate (merge), one with overlapping notes (consolidate)
- [x] Prompt length reasonable (under ~3K tokens)

### PRM-003 [P]: Coverage Assessment Prompt (CDK)
**Description:** Author the coverage gap detection prompt for the recallKnowledge Lambda's coverage assessment flow.
**Files:**
- `lambda/recall/prompts/coverage.ts` — prompt text (or inline in handler)
- Sample test inputs for validation
**Dependencies:** R-2 research (Bedrock KB metadata extraction)
**Acceptance Criteria:**
- [x] Input format: user's original query + summaries of top Retrieve results (title + content snippet)
- [x] Output format: JSON `{ "gap_detected": boolean, "refined_query": string | null }`
- [x] When gap detected, `refined_query` contains a reformulated query targeting the gap
- [x] When no gap, `refined_query` is null
- [ ] Tested against ≥3 sample scenarios: good coverage (no gap), missing topic (gap + refined query), ambiguous results
- [x] Prompt is concise — designed for a fast model (Haiku-class)

### PRM-004 [P]: Keyword Derivation Prompt
**Description:** Author the prompt for deriving 3-5 search keywords from a user's first message for local KB recall per spec FR-7.
**Files:**
- `internal/recall/prompts/keywords.go` — prompt text
**Dependencies:** None
**Acceptance Criteria:**
- [x] System prompt instructs: extract 3-5 key search terms from the user's message, return as JSON array of strings
- [x] Keywords should be specific technical terms, not generic words
- [x] Output format: `["keyword1", "keyword2", "keyword3"]`
- [ ] Tested against ≥3 sample first messages: a technical question, a broad request, a short ambiguous query
- [x] Prompt is concise — designed for a fast model (Haiku-class)

### PRM-005 [P]: Chunk Summarization Prompt
**Description:** Author the prompt for summarizing processed conversation chunks to ~10-20K tokens for context carry-forward per spec FR-5.
**Files:**
- `internal/extract/prompts/summarize_chunk.go` — prompt text
**Dependencies:** None
**Acceptance Criteria:**
- [x] Instructs LLM to summarize the conversation chunk preserving: key topics discussed, decisions made, technical details, and context needed for understanding subsequent messages
- [x] Target output length: ~10-20K tokens
- [x] Specifies that only the latest summary is carried forward (not accumulated from all prior chunks)
- [ ] Tested against ≥1 sample long conversation chunk
- [x] Uses extraction model (`extraction.model_id`), not the cheaper translation model

---

## Research Ordering

_Research items must complete before their dependent implementation phases._

| Research | Must Complete Before | Blocks | Status |
|----------|---------------------|--------|--------|
| **R-1:** Bubbletea wizard pattern | CLI Phase 6 (Setup Wizard) | WIZ-001, WIZ-002 | **✅ Complete** |
| **R-2:** Bedrock InvokeModel Go SDK | CLI Phase 3 (Extraction Pipeline) | EXT-001 | **✅ Complete** |
| **R-3:** Claude Code conversation format | CLI Phase 2 (Translation Layer) | TRN-002 | **✅ Complete** |
| **R-4:** Notor conversation format | CLI Phase 2 (Translation Layer) | TRN-003 | **✅ Complete** |
| **R-5:** Claude Code hook registration | CLI Phase 4 (Hook Injection) | HKI-001, HKI-007 | **✅ Complete** |
| **R-6:** Notor hook registration | CLI Phase 4 (Hook Injection) | HKI-002 | **✅ Complete** |
| **R-7:** Crockford base32 UID generation | CLI Phase 1 (Foundation) | FND-004 | **✅ Complete** |
| **R-8:** Cross-platform cron registration | CLI Phase 6 (Setup Wizard) | WIZ-004, WIZ-005 | **✅ Complete** |

**All research is now complete (R-1 through R-8).** All implementation phases can proceed without blocking on research.

**Research findings that reshaped tasks:**
- **R-1:** WIZ-001/WIZ-002 — use `charm.land/huh/v2` with declarative branching, 3-phase form architecture with bubbletea embedding for async inter-step logic.
- **R-2:** EXT-001 — InvokeModel (not Converse), `anthropic_version: "bedrock-2023-05-31"`, two-layer retry, 5-minute HTTP timeout, SSO error detection, `errors.go`/`retry.go` added.
- **R-3/R-4:** TRN-002/TRN-003 — per-message timestamps available on both harnesses.
- **R-5/R-6:** HKI-001/HKI-002/HKI-006/HKI-007 — structured JSON output for Claude Code, automation file for Notor.
- **R-7:** FND-004 — bit-buffer encoding algorithm, 5 shared test vectors, `EncodeCrockford` export.
- **R-8:** WIZ-004/WIZ-005 — crontab read-modify-write with inline marker, absolute binary path, `robfig/cron/v3` for next-run, Windows `schtasks.exe`, build tags.

---

## Phase 10: Quality & Integration Testing

_Cross-cutting validation phase. Tasks can start as soon as their dependencies are ready._

### QAT-001: Unit Test Coverage Pass
**Description:** Ensure comprehensive unit test coverage across all packages.
**Files:** All `*_test.go` files across `internal/`
**Dependencies:** All FND-*, TRN-*, EXT-*, HKI-*, DRM-*, WIZ-*, APR-*, SRV-* tasks
**Acceptance Criteria:**
- [x] `go test ./...` passes with no failures
- [x] `go test -race ./...` passes (no race conditions)
- [x] Every public function has at least one test
- [x] Edge cases from spec scenarios tested: oversized conversations, hook timeout, partial extraction, lock contention, re-processing modified conversations

### QAT-002 [P]: Integration Test Suite
**Description:** Create integration tests tagged with `//go:build integration` for real-service interactions.
**Files:**
- `internal/bedrock/client_integration_test.go`
- `internal/submit/remote_integration_test.go`
- `internal/recall/remote_integration_test.go`
- `internal/git/repo_integration_test.go`
- `internal/schedule/cron_integration_test.go`
**Dependencies:** EXT-001, EXT-007, HKI-003, FND-005, WIZ-004
**Acceptance Criteria:**
- [x] Bedrock InvokeModel: real call with extraction prompt → valid JSON array response
- [x] Remote submitKnowledge: real call → HTTP 202 with UID
- [x] Remote recallKnowledge: real call → JSON array of notes
- [x] Git operations: init, commit, grep against real git repo
- [x] Cron registration: register and verify on real crontab
- [x] All integration tests skip gracefully when credentials/services unavailable
- [x] Tagged with `//go:build integration` to exclude from `go test ./...`

### QAT-003 [P]: End-to-End Scenario Validation
**Description:** Validate the full user scenarios from spec.md against the running binary.
**Files:**
- `test/e2e/scenarios.md` — manual test checklist (reviewed and corrected against implementation; execution requires deployed stack)
**Dependencies:** All previous phases
**Acceptance Criteria:**
- [ ] **First-Time Setup:** Binary download → setup wizard → config written → hooks registered → cron registered (under 10 minutes)
- [ ] **Scheduled Capture:** Cron fires → conversations scanned → knowledge extracted → notes routed → run log written
- [ ] **Hook Injection:** New conversation → hook fires → recall queries → Markdown injected → conversation proceeds
- [ ] **Oversized Conversation:** >700K token conversation (the implementation threshold per FND-011) → chunked → all knowledge extracted
- [ ] **Extraction Failure:** Bedrock throttle → retry → partial acceptance → error logged
- [ ] **Hook Timeout:** Slow KB → timeout (8s default, configurable via `hook.timeout`) → partial results used
- [ ] **Re-Processing:** Modified old conversation → re-translated → new knowledge extracted
- [ ] **Approval Flow:** Pending notes → `multi-kb approve` → review → approve/reject → submitted/deleted
- [ ] **Dream Cycle:** Pending notes → singleton batches → related lookup (keyword-based git grep) → consolidation → active notes

### QAT-004 [P]: Security Review
**Description:** Verify security requirements per spec NFR-3. Findings documented in `test/security/review.md`.
**Dependencies:** All previous phases
**Acceptance Criteria:**
- [x] No credentials stored in config or state files (only profile names)
- [x] Exclusion rules properly appended to extraction prompt
- [x] Local KB content only leaves machine when explicitly routed to remote KB
- [x] Approval server binds to localhost only
- [x] No command injection vulnerabilities in git shell-outs or cron registration
- [x] Pending queue files not world-readable (appropriate file permissions)

---

## Dependency Graph (Critical Path)

```
ENV-001 → ENV-002
       → ENV-003
       → FND-001 → FND-002 ──────────────────────────────────→ EXT-008 → DRM-005
       → FND-003 ──────────────────────────────────────────────→ EXT-008
       → FND-004 → FND-006 → FND-007 ──────────────────────────→ HKI-007
       → FND-005 → FND-006 → FND-007
       → FND-008 ──────────────────────────────────────────────→ EXT-006 → EXT-008
       → FND-009 ──────────────────────────────────────────────→ EXT-008
       → FND-011
       → TRN-001 → TRN-002 ──────────────────────────────────→ EXT-008
                 → TRN-003 ──────────────────────────────────→ EXT-008
                 → TRN-004 ──────────────────────────────────→ EXT-008
       → EXT-001 → EXT-002 → EXT-003 → EXT-004 → EXT-005 ──→ EXT-008
                            → EXT-006 ──────────────────────→ EXT-008
                            → EXT-007 ──────────────────────→ EXT-008
       → HKI-001 ──────────→ WIZ-003
       → HKI-002 ──────────→ WIZ-003
       → HKI-003 → HKI-007
       → HKI-004 → HKI-007
       → HKI-005 → HKI-007
       → HKI-006 → HKI-007
       → PRM-002 ──────────────────────────────────────────→ DRM-004
       → DRM-001 → DRM-002 → DRM-003 → DRM-004 → DRM-005
       → PRM-005 ──────────────────────────────────────────→ EXT-004
       → APR-001 → APR-002 → APR-003 → APR-004
```

**Critical Path:** ENV-001 → FND-001 → TRN-001 → TRN-002 → EXT-001 → EXT-002 → EXT-003 → EXT-005 → EXT-008 → DRM-005

This is the longest dependency chain (~10 sequential tasks). To minimize total implementation time, prioritize this chain while parallelizing Foundation [P] tasks, Hook tasks, and Approval UI tasks.

## Parallel Execution Groups

| Group | Tasks | Can Start After |
|-------|-------|-----------------|
| Foundation Parallel | FND-003, FND-004, FND-008, FND-009, FND-011 | ENV-001 |
| Translation Parallel | TRN-002, TRN-003 | TRN-001 |
| Hook Registration Parallel | HKI-001, HKI-002 | ENV-001 |
| Hook Pipeline Parallel | HKI-003, HKI-004, HKI-005, HKI-006 | EXT-001 (HKI-003), ENV-001 (others) |
| Server Mode Parallel | SRV-003, SRV-004, SRV-005 | SRV-001 |
| Wizard Subcommands | WIZ-006 | FND-001 |
| Approval UI | APR-001 → APR-002 → APR-003 | Independent of extraction pipeline |
| Prompt Authoring | PRM-001, PRM-002, PRM-003, PRM-004, PRM-005 | PRM-002 needs consolidation contract; PRM-003 needs R-2; others can start immediately |
| Build Parallel | BLD-001, BLD-002 | ENV-003 |
| Quality Parallel | QAT-001, QAT-002, QAT-003, QAT-004 | All prior phases |
