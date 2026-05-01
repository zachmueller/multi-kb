# Task Breakdown: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Implementation Plan:** [plan.md](plan.md)
**Specification:** [spec.md](spec.md)
**Status:** Planning

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
- [ ] `go mod init` creates module (e.g., `github.com/<org>/multi-kb`)
- [ ] All directories from plan.md Module Architecture exist with `package` declaration files
- [ ] `go build ./cmd/multi-kb/` succeeds with a no-op `main.go`
- [ ] `go test ./...` passes (no tests yet, but no compilation errors)

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
- [ ] `multi-kb --help` lists all subcommands
- [ ] Each subcommand prints "not implemented" and exits 0
- [ ] `multi-kb --version` prints version string
- [ ] Global `--config` flag accepts a path (default `~/.multi-kb/config.yaml`)

### ENV-003 [P]: Development Tooling Configuration
**Description:** Set up linting, formatting, and test infrastructure per quickstart.md.
**Files:**
- `.golangci.yml` — linter configuration
- `Makefile` — build, test, lint targets
- `.gitignore` — Go binaries, editor files
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] `make build` produces `multi-kb` binary
- [ ] `make test` runs `go test ./...`
- [ ] `make lint` runs golangci-lint with no errors on empty project
- [ ] `make build-all` cross-compiles for all target platforms (linux/darwin amd64+arm64, windows amd64)
- [ ] `CGO_ENABLED=0` enforced in all build targets

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
- [ ] Parses all fields from data-model.md Entity 2 (mode, author, knowledge_bases, extraction, translation, dream_cycle, hook, exclusion_rules, sources with targets and overrides)
- [ ] Validates: mode ∈ {client, server}, author non-empty ≤100 chars, unique KB names, auth ∈ {iam, federate}, aws_profile required when auth=iam, routing ∈ {always, consider}, approval ∈ {auto-approve, require-manual-approval}, kb references resolve to knowledge_bases or local/<name>
- [ ] Returns structured errors listing all validation failures (not just first)
- [ ] Defaults: hook.timeout=8s, extraction/translation/dream_cycle model IDs have sensible defaults
- [ ] Test cases: valid config, missing required fields, invalid enum values, dangling KB references, empty sources

### FND-002: State YAML Loading and Writing
**Description:** Implement `state.yaml` schema, loader, and atomic writer per data-model.md Entity 3.
**Files:**
- `internal/config/state.go` — Go struct, YAML tags, loader, atomic writer (write temp → rename)
- `internal/config/state_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Parses per-directory `last_processed` timestamps and `last_dream_cycle` timestamp
- [ ] Atomic write: writes to temp file then renames (no partial writes on crash)
- [ ] Validates: directory paths are absolute, timestamps are valid ISO 8601
- [ ] Creates file with empty state if it doesn't exist
- [ ] Test cases: load existing, create new, atomic write verification, concurrent read safety

### FND-003 [P]: Lock File with Heartbeat
**Description:** Implement lock file acquisition, heartbeat goroutine, release, and stale detection per data-model.md Entity 10.
**Files:**
- `internal/lock/lock.go` — Acquire, Release, IsHeld, heartbeat goroutine
- `internal/lock/lock_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Lock file at `~/.multi-kb/lock` contains JSON: pid, started_at, heartbeat, activity
- [ ] Acquire succeeds if no lock file or stale heartbeat (>30 min old)
- [ ] Acquire fails with structured info (pid, activity, last heartbeat) if lock is active
- [ ] Heartbeat goroutine updates `heartbeat` field every 60 seconds
- [ ] Release deletes the lock file and stops the heartbeat goroutine
- [ ] Stale lock detection: heartbeat older than 30 minutes → force-acquire
- [ ] Test cases: acquire fresh, acquire stale, fail on active, release, heartbeat updates

### FND-004 [P]: Crockford Base32 UID Generation
**Description:** Implement 16-character Crockford base32 UID generation per research.md R-7.
**Files:**
- `internal/submit/uid.go` — GenerateUID function
- `internal/submit/uid_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Uses `crypto/rand` for 10 random bytes (80 bits of entropy)
- [ ] Encodes to exactly 16 characters using Crockford base32 alphabet (`0123456789ABCDEFGHJKMNPQRSTVWXYZ`)
- [ ] No I, L, O, U characters in output
- [ ] Uppercase output (Crockford canonical form)
- [ ] Test cases: length=16, valid alphabet, uniqueness over 10K generations, deterministic encoding of known bytes

### FND-005: Local KB Git Repository Creation
**Description:** Implement local KB directory and git repository initialization.
**Files:**
- `internal/git/repo.go` — InitRepo, CommitFiles, IsRepo functions
- `internal/git/repo_test.go`
**Dependencies:** ENV-001
**Implementation Note:** Use `os/exec` shell-out to the `git` binary for all git operations. Sanitize all user-derived inputs (directory names, file names) to prevent command injection. Requires `git` on PATH.
**Acceptance Criteria:**
- [ ] Creates `~/.multi-kb/local/<kb-name>/` directory
- [ ] Runs `git init` in the directory
- [ ] Creates initial commit (empty or with `.gitkeep`)
- [ ] IsRepo detects whether a path is an initialized git repo
- [ ] Default KB created at `~/.multi-kb/local/default/`
- [ ] All git arguments properly escaped/quoted to prevent command injection
- [ ] Test cases: init fresh repo, detect existing repo, commit files

### FND-006: Local KB Note File Writing
**Description:** Implement writing knowledge notes as Obsidian-flavor Markdown files with YAML frontmatter per data-model.md Entity 1.
**Files:**
- `internal/submit/local.go` — WriteNote function (generates UID, writes file, commits)
- `internal/submit/local_test.go`
**Dependencies:** FND-004, FND-005
**Implementation Note:** Uses git shell-out (FND-005) for commits. Sanitize title and author strings before use in file content or git commit messages.
**Acceptance Criteria:**
- [ ] Writes `<UID>.md` to the appropriate local KB directory
- [ ] YAML frontmatter includes all fields: uid, title, status (pending), author, last-updated, last-linked-to (empty), last-recalled (empty), consolidated-from-notes (empty)
- [ ] Body contains the note's Markdown content
- [ ] Generates UID via FND-004
- [ ] Commits the new file with a descriptive git commit message
- [ ] Validates note constraints: title ≤255 chars, content ≤100K chars, author ≤100 chars
- [ ] Test cases: write note, verify frontmatter, verify file naming, validation failures

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
- [ ] Runs `git grep -c <keyword>` per keyword against the working tree
- [ ] Filters results to `status: active` notes only (parses frontmatter to check status)
- [ ] Counts matches per note across all keywords
- [ ] Title matches weighted 3x (matches in YAML `title:` frontmatter line count triple)
- [ ] Returns results sorted by descending match count
- [ ] Returns structured results: uid, title, content, match_count
- [ ] Test cases: single keyword, multiple keywords, title weighting, status filtering, empty results

### FND-008 [P]: Pending Queue File Management
**Description:** Implement create, list, read, update, and delete for pending queue entries per data-model.md Entity 4.
**Files:**
- `internal/route/pending.go` — CreatePending, ListPending, ReadPending, UpdatePending (remove target), DeletePending
- `internal/route/pending_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Creates `~/.multi-kb/pending/` directory if not exists
- [ ] Writes JSON files named `<YYYYMMDDTHHMMSS>-<8-char-hex-hash>.json`
- [ ] Hash is first 8 hex chars of SHA-256 of `title + content`
- [ ] JSON schema matches data-model.md Entity 4 (title, content, author, target_kbs, source_conversation, extracted_at)
- [ ] ListPending returns all `.json` files in the directory
- [ ] UpdatePending removes a specific target from `target_kbs` array
- [ ] DeletePending removes the file when `target_kbs` is empty
- [ ] PendingCount returns count of pending files (for status display and hook notice)
- [ ] Test cases: create, list, read, remove single target, remove last target (auto-delete), concurrent access safety

### FND-009 [P]: Run Log and Error Log Appending
**Description:** Implement structured JSONL logging for runs, extraction errors, and hook errors per data-model.md Entities 7, 8, 9.
**Files:**
- `internal/logging/runs.go` — AppendRunLog (capture and dream cycle variants)
- `internal/logging/errors.go` — AppendExtractionError, AppendHookError
- `internal/logging/runs_test.go`
- `internal/logging/errors_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Creates `~/.multi-kb/logs/` directory if not exists
- [ ] `runs.jsonl`: appends one JSON line per run with all fields from data-model.md Entity 7 (timestamp, type, trigger, directories_scanned, conversations_processed, notes_extracted, notes_routed map, errors, duration_ms)
- [ ] `runs.jsonl` dream cycle variant: timestamp, type, trigger, batches_processed, actions map, errors, duration_ms
- [ ] `extraction-errors.jsonl`: timestamp, conversation_id, source_path, error, retries
- [ ] `hook-errors.jsonl`: timestamp, harness, directory, error, partial_results
- [ ] All writes are append-only (open file with O_APPEND)
- [ ] Test cases: append single entry, append multiple entries, file creation, valid JSON per line

### FND-010: Status Command Implementation
**Description:** Implement `multi-kb status` displaying config summary, run history, pending count, and next scheduled run per spec FR-11.
**Files:**
- `internal/cmd/status.go` — full implementation replacing stub
**Dependencies:** FND-001, FND-002, FND-008, FND-009
**Acceptance Criteria:**
- [ ] Displays current configuration summary: tracked directories, configured KBs (names, auth types), author
- [ ] Displays last N runs (default 10) from `runs.jsonl` with success/failure status and key counts
- [ ] Displays pending approval queue count when non-empty (e.g., "3 notes awaiting approval")
- [ ] Displays next scheduled run time (placeholder until cron parsing in WIZ-005)
- [ ] Handles missing config gracefully (suggests running `multi-kb setup`)
- [ ] Handles empty run log gracefully ("No runs recorded yet")
- [ ] Test cases: with full data, with empty logs, with no config

### FND-011 [P]: Token Counting Approximation
**Description:** Implement fast token estimation for determining when conversations exceed the chunking threshold per spec FR-5.
**Files:**
- `internal/token/count.go` — EstimateTokens function, ChunkingThreshold constant
- `internal/token/count_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Estimates tokens from a string using a fast heuristic (e.g., ~4 chars per token for English text, ~3.5 for code-heavy content)
- [ ] Does not use an external tokenizer library (speed priority)
- [ ] Accuracy within ±20% for typical conversation content
- [ ] Exports `ChunkingThreshold = 700_000` constant (conservative value to leave headroom for estimation error — actual model context windows are larger)
- [ ] Handles empty strings, very long strings, and mixed content
- [ ] Test cases: known calibration strings, edge cases (empty, single char, 1M chars)

---

## Phase 2: Translation Layer

_Corresponds to plan.md Phase B. Builds harness-specific conversation translators._

### TRN-001: Intermediate JSONL Format Types
**Description:** Define Go types for the intermediate conversation format per contracts/intermediate-format.md.
**Files:**
- `internal/translate/intermediate.go` — ConversationHeader, Message, ToolUse structs; Writer that produces JSONL buffer
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] ConversationHeader: type, id, source_harness, source_path, started_at, metadata (persona, workflow, project_dir)
- [ ] Message: type, role, content, timestamp, previously_processed, tool_uses
- [ ] ToolUse: tool_name, summary
- [ ] Writer serializes header + messages as JSONL (one JSON object per line)
- [ ] Role enum: user, assistant, system
- [ ] Test cases: serialize header, serialize messages with tool uses, round-trip parse

### TRN-002: Claude Code Translator
**Description:** Implement translator that reads Claude Code conversation files from `~/.claude/projects/` and produces intermediate format per spec FR-4.
**Files:**
- `internal/translate/claudecode.go` — ClaudeCodeTranslator: discover sessions, translate messages, flag previously_processed
- `internal/translate/claudecode_test.go`
**Dependencies:** TRN-001, FND-002
**Acceptance Criteria:**
- [ ] Reads from `~/.claude/projects/<project>/` where `<project>` is derived from the user-configured directory path
- [ ] Discovers all `.jsonl` session files in the project directory
- [ ] Parses Claude Code JSONL format: handles message roles, content block arrays, tool call/result pairs
- [ ] Flattens content block arrays to plain text strings (images → `[Image]` placeholder)
- [ ] Collapses tool call/result pairs into `tool_uses` entries on assistant messages
- [ ] Sets `previously_processed` flag at file level: if conversation was previously processed (file mtime ≤ `last_processed`), all prior messages flagged true
- [ ] Uses file last-modified time as timestamp for all messages
- [ ] Populates conversation header with source_harness="claude-code", source_path, project_dir
- [ ] Test cases: single conversation, re-processed conversation, tool calls, content blocks, project path mapping

### TRN-003 [P]: Notor Translator
**Description:** Implement translator that reads Notor chat history from `{vault}/notor/history/` and produces intermediate format per spec FR-4.
**Files:**
- `internal/translate/notor.go` — NotorTranslator: discover conversations, translate messages, per-message previously_processed
- `internal/translate/notor_test.go`
**Dependencies:** TRN-001, FND-002
**Acceptance Criteria:**
- [ ] Reads from `{vault}/notor/history/` where vault is the user-configured directory
- [ ] Parses Notor native format (specific format TBD by R-4 research)
- [ ] Per-message timestamps: compares each message timestamp to `last_processed` for previously_processed flag
- [ ] Extracts persona and workflow metadata into conversation header
- [ ] Handles role normalization, content flattening, tool call collapsing
- [ ] Test cases: single conversation, re-processed with per-message timestamps, persona extraction

### TRN-004: Tool Interaction Summarization
**Description:** Implement summarization of tool call/result pairs — mechanical templates for small interactions, LLM for large ones per contracts/intermediate-format.md.
**Files:**
- `internal/translate/summarize.go` — SummarizeTool function (dispatches to template or LLM)
- `internal/translate/summarize_test.go`
**Dependencies:** TRN-001, FND-011
**Acceptance Criteria:**
- [ ] Small interactions (<~1K tokens by estimate): uses mechanical template `"{tool_name}: {brief action} — {brief result}"`
- [ ] Large interactions (≥~1K tokens): delegates to Bedrock LLM via `translation.summarization_model_id` (interface dependency; actual Bedrock call wired in EXT-001)
- [ ] Template patterns: Read → "Read file X (N lines)", Bash → "Ran 'cmd' — brief result", Write → "Wrote file X", Edit → "Edited file X"
- [ ] LLM summarization produces a 1-2 sentence summary of the tool interaction
- [ ] Test cases: small Read interaction, small Bash interaction, large interaction (mocked LLM), threshold boundary

---

## Phase 3: Extraction Pipeline

_Corresponds to plan.md Phase C. Builds LLM-powered extraction and routing._

### EXT-001: Bedrock Client Wrapper
**Description:** Implement reusable Bedrock InvokeModel client with retry, backoff, and credential profile resolution per research.md R-2.
**Files:**
- `internal/bedrock/client.go` — Client struct: NewClient (profile, region, model), InvokeModel (system prompt, user message → string response)
- `internal/bedrock/models.go` — Request/response types for Claude Messages API format
- `internal/bedrock/client_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Creates AWS config with named SSO profile support (`aws_profile` from config)
- [ ] Configures BedrockRuntime client for specified region
- [ ] InvokeModel sends system prompt + user message in Claude Messages API format
- [ ] Parses response to extract text content from response body
- [ ] Retry logic: 3 retries with exponential backoff for throttling, timeout, network errors
- [ ] Supports configurable model ID
- [ ] Test cases: successful invocation (mocked), retry on throttle, retry exhaustion, profile resolution

### EXT-002: Extraction System Prompt Construction
**Description:** Build the extraction system prompt from hardcoded base + exclusion rules + optional append file per contracts/extraction-output.md.
**Files:**
- `internal/extract/prompt.go` — BuildExtractionPrompt function
- `internal/extract/prompt_test.go`
**Dependencies:** FND-001
**Acceptance Criteria:**
- [ ] Hardcoded base prompt defines: extraction task, output format (JSON array of {title, content, suggested_target_kbs}), quality guidelines, previously_processed handling
- [ ] Appends exclusion rules section when `exclusion_rules` is non-empty: header "Content exclusion rules — never include in notes destined for non-local KBs" + bulleted list of strings
- [ ] Reads and appends `~/.multi-kb/prompts/extraction-append.md` if it exists (fresh read each call)
- [ ] Returns combined prompt string
- [ ] Test cases: base only, with exclusion rules, with append file, with both, append file missing (no error)

### EXT-003: Single-Pass Extraction
**Description:** Implement single-pass extraction: send translated conversation to Bedrock, parse JSON array output per contracts/extraction-output.md.
**Files:**
- `internal/extract/extract.go` — Extract function (conversation → []Note)
- `internal/extract/parse.go` — ParseExtractionOutput (JSON parsing with partial acceptance)
- `internal/extract/extract_test.go`
- `internal/extract/parse_test.go`
**Dependencies:** EXT-001, EXT-002, TRN-001
**Acceptance Criteria:**
- [ ] Sends full intermediate JSONL conversation as user message with constructed system prompt
- [ ] Parses JSON array response into []Note structs (title, content, suggested_target_kbs)
- [ ] Partial acceptance: valid entries accepted, invalid entries logged and dropped
- [ ] Empty array is valid (no knowledge extracted)
- [ ] Validates extracted notes: title non-empty ≤255 chars, content non-empty, suggested_target_kbs is string array
- [ ] Test cases: successful extraction, empty result, partial valid JSON, completely invalid JSON, field validation

### EXT-004: Chunked Extraction for Oversized Conversations
**Description:** Implement conversation chunking for >700K token conversations per spec FR-5.
**Files:**
- `internal/extract/extract.go` — ExtractChunked (splits, summarizes, iterates)
**Dependencies:** EXT-003, FND-011, PRM-005
**Acceptance Criteria:**
- [ ] Detects when translated conversation exceeds 700K tokens (conservative threshold from FND-011's `ChunkingThreshold` constant, leaves headroom for token estimation error)
- [ ] Splits at message boundaries (never mid-message) near the 700K mark
- [ ] First chunk processed normally via Extract
- [ ] Each processed chunk summarized to ~10-20K tokens using extraction model with summarization-specific prompt (PRM-005)
- [ ] **Latest summary only** carried forward: each chunk's summary replaces the previous chunk's summary as the preamble for the next chunk (not accumulated). Keeps preamble bounded at ~10-20K regardless of conversation length.
- [ ] Context preamble formatted as a leading section in the user message (before the JSONL conversation content)
- [ ] All extracted notes from all chunks combined into single result
- [ ] Test cases: conversation under threshold (no chunking), over threshold (2 chunks), very large (3+ chunks), split at message boundary, verify preamble replacement (not accumulation)

### EXT-005: Extraction Error Handling
**Description:** Implement retry logic and error logging for extraction failures per spec FR-6 and contracts/extraction-output.md.
**Files:**
- `internal/extract/extract.go` — retry wrapper around extraction calls
**Dependencies:** EXT-003, FND-009
**Acceptance Criteria:**
- [ ] Bedrock API failures (throttle, timeout, network): retry up to 3 times with exponential backoff
- [ ] Malformed JSON output: retry up to 3 times (fresh API call each retry)
- [ ] Partially valid JSON arrays: accept valid entries, log invalid ones
- [ ] After 3 retries with no usable output: skip conversation, log to `extraction-errors.jsonl`
- [ ] Error log entry includes: conversation_id, source_path, error details, retry count
- [ ] Test cases: successful after retry, exhausted retries, partial acceptance

### EXT-006: Routing Engine
**Description:** Implement routing rules that determine which KBs receive each extracted note per spec FR-3 and contracts/extraction-output.md Routing Integration.
**Files:**
- `internal/route/route.go` — RouteNotes function: applies routing rules (always/consider, overrides, fallback)
- `internal/route/route_test.go`
**Dependencies:** FND-001, FND-008
**Acceptance Criteria:**
- [ ] For each note, collects target KBs:
  - All `always`-mode KBs for the directory → unconditionally added
  - `consider`-mode KBs whose names appear in `suggested_target_kbs` → added
- [ ] Applies per-harness and per-harness+persona overrides (overrides replace, not merge)
- [ ] Suggested KB names not matching any configured KB → silently dropped
- [ ] If no targets after resolution + no always-mode KBs → fallback to `local/default`
- [ ] For each target: checks approval mode → dispatches to auto-approve or pending queue
- [ ] Test cases: always-mode routing, consider-mode routing, override resolution, empty suggestions (fallback), mixed approval modes, unknown KB names dropped

### EXT-007: Remote KB submitKnowledge Client
**Description:** Implement the remote KB submission client per contracts/submit-knowledge.md.
**Files:**
- `internal/submit/remote.go` — SubmitToRemoteKB function (SigV4-signed POST, throttle, retry, error handling)
- `internal/submit/remote_test.go`
**Dependencies:** EXT-001, FND-001
**Acceptance Criteria:**
- [ ] Sends SigV4-signed POST to `{endpoint}/submitKnowledge` with {title, content, author}
- [ ] For `iam` auth: signs with configured AWS profile via SigV4; for `federate` auth: sends plain HTTP POST with no auth headers (the network layer handles identity transparently)
- [ ] Pre-flight validation: title ≤255 chars, content ≤100K chars, author ≤100 chars
- [ ] Self-throttle: max 10 requests/second per target KB
- [ ] HTTP 202: success (UID logged but not stored)
- [ ] HTTP 400: pass error + original note to extraction LLM for correction, retry up to 2 times; on persistent failure, stage in pending queue. **Note:** This LLM correction only applies in the capture processing pipeline (EXT-008 path). The approval web server (APR-003) does NOT use LLM correction — it returns 502 immediately.
- [ ] HTTP 401/403: log error, skip remaining submissions to this KB for the run, surface credential refresh guidance
- [ ] HTTP 5xx / network: retry 3 times with exponential backoff; on persistent failure, log and continue
- [ ] Test cases: successful submission, throttling, 400 correction flow, 401 skip behavior, 5xx retry

### EXT-008: Capture Processing Orchestrator (`multi-kb process`)
**Description:** Wire up the full capture pipeline: scan directories → translate → extract → route → submit/stage per spec FR-3.
**Files:**
- `internal/cmd/process.go` — full implementation replacing stub
**Dependencies:** FND-001, FND-002, FND-003, FND-006, FND-009, TRN-002, TRN-003, TRN-004, EXT-003, EXT-004, EXT-005, EXT-006, EXT-007
**Acceptance Criteria:**
- [ ] Acquires lock (exits with message if held)
- [ ] Reads config and state
- [ ] For each tracked directory: identifies conversations modified since `last_processed`
- [ ] For each modified conversation: translates → extracts → routes → submits/stages
- [ ] Updates `last_processed` to the last-modified time of the final processed file per directory
- [ ] Writes atomic state update
- [ ] Appends run log entry with all counts
- [ ] Releases lock
- [ ] Handles partial failures: continues processing remaining conversations/directories after errors
- [ ] Test cases: end-to-end with mocked Bedrock (happy path), no new conversations (no-op), lock contention, extraction failure + continue

---

## Phase 4: Hook Injection

_Corresponds to plan.md Phase D. Builds harness hook system._

### HKI-001: Claude Code Hook Registration
**Description:** Implement programmatic registration of a `user_prompt_submit` hook in Claude Code per research.md R-5 and spec FR-7.
**Files:**
- `internal/hook/claudecode.go` — RegisterClaudeCodeHook, UnregisterClaudeCodeHook
- `internal/hook/claudecode_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Locates Claude Code hook configuration file
- [ ] Registers `user_prompt_submit` hook that invokes `multi-kb hook --harness claude-code`
- [ ] Appends alongside any pre-existing hooks (never overwrites)
- [ ] Registration is idempotent (re-running doesn't duplicate)
- [ ] UnregisterClaudeCodeHook removes only the multi-kb hook entry
- [ ] Test cases: register fresh, register alongside existing, idempotent re-register, unregister

### HKI-002 [P]: Notor Hook Registration
**Description:** Implement programmatic registration of a conversation-start hook in Notor per research.md R-6 and spec FR-7.
**Files:**
- `internal/hook/notor.go` — RegisterNotorHook, UnregisterNotorHook
- `internal/hook/notor_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Locates Notor hook configuration mechanism
- [ ] Registers conversation-start hook that invokes `multi-kb hook --harness notor`
- [ ] Appends alongside pre-existing hooks
- [ ] Idempotent registration
- [ ] Test cases: register, re-register, unregister

### HKI-003: Remote KB recallKnowledge Client
**Description:** Implement the remote KB recall client per contracts/recall-knowledge.md.
**Files:**
- `internal/recall/remote.go` — RecallFromRemoteKB function (SigV4-signed POST, timeout)
- `internal/recall/remote_test.go`
**Dependencies:** EXT-001, FND-001
**Acceptance Criteria:**
- [ ] Sends SigV4-signed POST to `{endpoint}/recallKnowledge` with {query, limit}
- [ ] Parses response: JSON array of {uid, title, content, score}
- [ ] Handles `iam` vs `federate` auth: `iam` uses SigV4 signing; `federate` sends plain HTTP POST with no auth headers
- [ ] Respects configurable timeout (from hook.timeout)
- [ ] Returns partial results on timeout (context cancellation)
- [ ] Test cases: successful recall, timeout, auth error, empty results

### HKI-004: LLM-Derived Keyword Generation
**Description:** Implement keyword derivation from natural language queries for local KB recall per spec FR-8.
**Files:**
- `internal/recall/keywords.go` — DeriveKeywords function (LLM call → 3-5 keywords)
- `internal/recall/keywords_test.go`
**Dependencies:** EXT-001
**Acceptance Criteria:**
- [ ] Calls `translation.summarization_model_id` (e.g., Claude Haiku) to derive 3-5 search keywords from user's first message
- [ ] System prompt instructs: extract 3-5 key search terms, return as JSON array of strings
- [ ] Parses response as string array
- [ ] Fallback: if LLM call fails, use mechanical keyword extraction (split on whitespace, remove stop words)
- [ ] Test cases: successful derivation (mocked), fallback on failure, various query types

### HKI-005: Rank-Based Result Interleaving
**Description:** Implement merging of results from multiple KBs via rank-based interleaving per spec FR-7.
**Files:**
- `internal/recall/merge.go` — InterleaveResults function
- `internal/recall/merge_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] Accepts ranked result lists from multiple KBs (remote sorted by score, local by match count)
- [ ] Interleaves by rank: top-ranked from each KB first, then second-ranked, etc.
- [ ] If one KB has fewer results, remaining slots filled from KBs with results remaining
- [ ] Truncates to 10 total notes
- [ ] Deduplicates by UID (if same note appears from multiple KBs)
- [ ] Test cases: equal-length lists, unequal lengths, single KB, no results, duplicate UIDs, more than 10 total

### HKI-006: Markdown Injection Formatting
**Description:** Format recalled notes as Markdown for stdout injection, including pending notice per spec FR-7.
**Files:**
- `internal/recall/format.go` — FormatInjection function
- `internal/recall/format_test.go`
**Dependencies:** FND-008
**Acceptance Criteria:**
- [ ] Outputs raw Markdown (no JSON wrapper)
- [ ] Each note includes: title (as heading), source KB name, full content
- [ ] When pending queue is non-empty: includes notice at the end (e.g., "3 notes awaiting approval — run `multi-kb approve` to review")
- [ ] Empty results produce no output (silent — no "no results found" message)
- [ ] Test cases: multiple notes from multiple KBs, single note, pending notice, no pending, empty results

### HKI-007: Hook Entry Point Command (`multi-kb hook`)
**Description:** Wire up the hook subcommand that orchestrates the full injection flow per spec FR-7.
**Files:**
- `internal/cmd/hook.go` — full implementation replacing stub
- `internal/hook/inject.go` — core injection orchestrator
**Dependencies:** FND-001, FND-007, FND-009, HKI-003, HKI-004, HKI-005, HKI-006
**Acceptance Criteria:**
- [ ] Accepts `--harness` flag (claude-code or notor)
- [ ] Claude Code: implements first-message guard — checks for absence of prior assistant messages; if not first message, exits immediately with no output. **Note:** The exact detection mechanism depends on R-5 research findings (what data does the hook receive?). Defer implementation approach until R-5 completes.
- [ ] Reads user's first message (from stdin or hook context)
- [ ] Identifies target KBs for the current directory from config
- [ ] Queries all target KBs concurrently (local via git grep with LLM keywords, remote via recallKnowledge)
- [ ] Merges results via rank-based interleaving → top 10
- [ ] Formats as Markdown and writes to stdout
- [ ] Enforces configurable timeout (default 8s) — partial results from responsive KBs used if others time out
- [ ] On complete timeout (no KBs respond): no output, warning logged to `hook-errors.jsonl`
- [ ] Test cases: full injection path (mocked), first-message guard (pass/block), timeout handling, partial results

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
- [ ] Acquires lock with activity="dream_cycle"
- [ ] Executes phases 0-4 in sequence
- [ ] Phase 0: no-op for local (returns immediately)
- [ ] Releases lock on completion or error
- [ ] Records run log entry (dream_cycle type) on completion
- [ ] If any phase fails: already-committed batches preserved, remaining pending notes left for next run
- [ ] Test cases: successful full cycle (mocked phases), failure mid-cycle, lock acquisition failure

### DRM-002: Phase 1 — Singleton Batch Creation
**Description:** Implement Phase 1: find all `status: pending` notes and create singleton batches per spec FR-8.
**Files:**
- `internal/dreamcycle/phase1.go` — CreateBatches function
- `internal/dreamcycle/phase1_test.go`
**Dependencies:** FND-005, FND-006
**Acceptance Criteria:**
- [ ] Scans local KB for all notes with `status: pending` in frontmatter
- [ ] Each pending note becomes its own batch (singleton — no similarity grouping for local)
- [ ] Returns list of batches, each containing one note
- [ ] Empty list if no pending notes (dream cycle exits early)
- [ ] Test cases: multiple pending notes, no pending notes, mixed pending/active notes

### DRM-003: Phase 2 — Git Grep Related Note Retrieval
**Description:** Implement Phase 2: for each batch, find related `status: active` notes via keyword-based `git grep` per spec FR-8.
**Files:**
- `internal/dreamcycle/phase2.go` — FindRelatedNotes function
- `internal/dreamcycle/phase2_test.go`
**Dependencies:** FND-007
**Acceptance Criteria:**
- [ ] Derives keywords mechanically from the note's title and key terms (no LLM call — unlike hook recall)
- [ ] Runs `git grep` per keyword against the local KB
- [ ] Filters to `status: active` notes only
- [ ] Returns up to 10 related active notes per batch
- [ ] Ranks by match count (same as recall ranking)
- [ ] Test cases: note with matching active notes, no related notes, keyword derivation from title

### DRM-004: Phase 3 — LLM Consolidation and Action Application
**Description:** Implement Phase 3: send each batch (pending note + related notes) to LLM for consolidation, parse actions, apply them per spec FR-8 and contracts/consolidation-output.md.
**Files:**
- `internal/dreamcycle/phase3.go` — ConsolidateBatch function (LLM call, action parsing)
- `internal/dreamcycle/actions.go` — ApplyActions function (keep, merge, split, consolidate)
- `internal/dreamcycle/phase3_test.go`
- `internal/dreamcycle/actions_test.go`
**Dependencies:** EXT-001, FND-006, FND-005, PRM-002
**Contract:** [consolidation-output.md](contracts/consolidation-output.md)
**Acceptance Criteria:**
- [ ] Constructs consolidation prompt with: the pending note (singleton batch) + related active notes
- [ ] Calls `dream_cycle.model_id` via Bedrock
- [ ] Parses LLM response into action types: keep, merge, split, consolidate
- [ ] **keep:** flip pending note to `status: active`, update `last-updated`
- [ ] **merge:** merge content into target note, delete source note, update `consolidated-from-notes` on target
- [ ] **split:** create multiple new active notes from one pending note, delete original
- [ ] **consolidate:** create one new active note from multiple notes, delete originals, update `consolidated-from-notes`
- [ ] Per-batch git commit after applying all actions
- [ ] Test cases: keep action, merge action, split action, consolidate action, LLM failure (skip batch)

### DRM-005: Dream Cycle Commands
**Description:** Wire up `multi-kb dream-cycle` (standalone) and integrate into `multi-kb run` (combined capture + dream cycle).
**Files:**
- `internal/cmd/dreamcycle.go` — full implementation replacing stub
- `internal/cmd/run.go` — full implementation replacing stub
**Dependencies:** DRM-001, EXT-008
**Acceptance Criteria:**
- [ ] `multi-kb dream-cycle`: acquires lock, runs dream cycle, releases lock
- [ ] `multi-kb run`: acquires lock once, runs capture processing then dream cycle sequentially, releases lock
- [ ] Both respect lock file: if held, manual commands print message with lock holder info and exit; scheduled runs skip silently
- [ ] Both append appropriate run log entries
- [ ] Test cases: standalone dream cycle, combined run, lock contention

---

## Phase 6: Setup Wizard & Scheduling

_Corresponds to plan.md Phase F. Builds interactive setup and cron integration._

### WIZ-001: Terminal Wizard — Harness Selection and Source Discovery
**Description:** Implement the first portion of the setup wizard: select harnesses, point to directories, discover sources per spec FR-2.
**Files:**
- `internal/cmd/setup.go` — partial implementation (wizard flow part 1)
**Dependencies:** ENV-002, FND-001
**Acceptance Criteria:**
- [ ] Uses bubbletea + huh for interactive terminal forms
- [ ] Step 1: Multi-select harnesses (Notor, Claude Code)
- [ ] Step 2: For each harness, prompt for directory path(s)
- [ ] Step 3: Auto-discovers chat history locations:
  - Claude Code: reads from `~/.claude/projects/`, matches user-pointed directory to project subdirectory, presents summary
  - Notor: checks `{vault}/notor/history/` exists, presents summary
- [ ] User confirms discovered sources
- [ ] Test cases: single harness selection, both harnesses, directory validation, source discovery

### WIZ-002: Terminal Wizard — KB Configuration and Routing
**Description:** Implement the second portion: local KB creation, remote KB addition, routing configuration, approval mode, author, exclusion rules per spec FR-2.
**Files:**
- `internal/cmd/setup.go` — partial implementation (wizard flow part 2)
**Dependencies:** WIZ-001, FND-005
**Acceptance Criteria:**
- [ ] Creates default local KB automatically (`~/.multi-kb/local/default/`)
- [ ] Prompts to add remote KBs: endpoint URL, auth type (iam/federate), AWS profile (if iam), region, description
- [ ] Configures routing rules per directory: which KBs, routing mode (always/consider), approval mode
- [ ] Simplified presets: auto-approve always, always require manual approval, select per group
- [ ] Prompts for author identity string
- [ ] Prompts for exclusion rules (optional, can skip)
- [ ] Writes complete `config.yaml` and initial empty `state.yaml`
- [ ] Test cases: minimal setup (local only), with remote KB, with overrides, with exclusion rules

### WIZ-003: Hook Auto-Registration During Setup
**Description:** During setup, automatically register hooks for each selected harness per spec FR-2.
**Files:**
- `internal/cmd/setup.go` — hook registration step
**Dependencies:** WIZ-001, HKI-001, HKI-002
**Acceptance Criteria:**
- [ ] For each selected harness, calls the appropriate hook registration function
- [ ] Claude Code: registers `user_prompt_submit` hook
- [ ] Notor: registers conversation-start hook
- [ ] Appends alongside existing hooks (never overwrites)
- [ ] Reports registration status to user
- [ ] Test cases: single harness hook registration, both harnesses, pre-existing hooks preserved

### WIZ-004: Cron Registration
**Description:** Implement OS-native cron registration for `multi-kb run` per research.md R-8 and spec FR-3.
**Files:**
- `internal/schedule/cron_unix.go` — RegisterCron, UnregisterCron, ReadCronEntry (macOS/Linux)
- `internal/schedule/cron_windows.go` — RegisterCron, UnregisterCron, ReadScheduledTask (Windows)
- `internal/schedule/cron_unix_test.go`
- `internal/schedule/cron_windows_test.go`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [ ] macOS/Linux: reads existing crontab, appends entry with marker comment `# multi-kb scheduled run`, writes back
- [ ] Windows: uses `schtasks.exe /Create` to register task
- [ ] Idempotent: re-running doesn't duplicate entries (checks for marker/task name)
- [ ] Configurable interval (e.g., every 30 minutes)
- [ ] Entry runs `multi-kb run` with `--config` pointing to the config file
- [ ] UnregisterCron removes only the multi-kb entry
- [ ] Test cases: register fresh, idempotent re-register, unregister, existing crontab preserved

### WIZ-005: Cron Expression Parsing for Status Display
**Description:** Parse the crontab entry to compute and display the next scheduled run time per spec FR-11.
**Files:**
- `internal/schedule/parse.go` — ParseNextRun function (reads crontab, finds multi-kb entry, computes next occurrence)
- `internal/schedule/parse_test.go`
**Dependencies:** WIZ-004
**Acceptance Criteria:**
- [ ] Reads user's crontab, finds the `multi-kb run` entry by marker comment
- [ ] Parses the cron expression (e.g., `*/30 * * * *`)
- [ ] Computes next occurrence from current time
- [ ] Returns absolute timestamp (e.g., "2026-05-01 14:30:00")
- [ ] Returns nil/error if no cron entry found
- [ ] Test cases: common intervals (every 30 min, hourly, daily), next occurrence calculation, missing entry

### WIZ-006 [P]: Standalone Subcommands — add-source and add-kb
**Description:** Implement post-setup subcommands for adding sources and KBs per spec FR-2.
**Files:**
- `internal/cmd/addsource.go` — full implementation (interactive prompts for new source directory)
- `internal/cmd/addkb.go` — full implementation (interactive prompts for new remote KB)
**Dependencies:** FND-001, WIZ-001
**Acceptance Criteria:**
- [ ] `multi-kb add-source`: prompts for directory, harness(es), routing targets, writes to existing config.yaml
- [ ] `multi-kb add-kb`: prompts for name, endpoint, auth type, profile, region, description, writes to existing config.yaml
- [ ] Both validate input against config validation rules
- [ ] Both handle the case where config doesn't exist (suggests running setup first)
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
- [ ] UI lists all pending notes with: title, content preview, target KBs, source conversation, extraction timestamp
- [ ] Each target KB shown as individual approval target (approve/reject per KB)
- [ ] Inline edit: title and content fields are editable before approving
- [ ] Approve button sends POST to `/api/notes/:filename/approve`
- [ ] Reject button sends POST to `/api/notes/:filename/reject`
- [ ] UI updates dynamically after each action (removes resolved targets, removes fully resolved notes)
- [ ] `embed.FS` embeds all assets at compile time
- [ ] Responsive, functional design (not polished — visual design deferred per spec Out of Scope)

### APR-002: HTTP Server Lifecycle
**Description:** Implement the approval web server lifecycle: auto-port, browser launch, idle timeout, shutdown per spec FR-9.
**Files:**
- `internal/approve/server.go` — StartServer, activity tracking, idle timeout, shutdown
- `internal/approve/server_test.go`
**Dependencies:** APR-001
**Acceptance Criteria:**
- [ ] Binds to `localhost` on auto-selected available port
- [ ] Prints URL to terminal (e.g., "Approval UI running at http://localhost:52431")
- [ ] Opens user's default browser to the URL
- [ ] Tracks activity: any HTTP request resets the idle timer
- [ ] Shuts down after configurable idle timeout (default 5 minutes)
- [ ] Shuts down immediately when all pending notes are resolved
- [ ] Ctrl+C terminates immediately (graceful shutdown)
- [ ] Test cases: server starts, idle timeout fires, all-resolved shutdown, manual shutdown

### APR-003: API Handlers
**Description:** Implement the REST API endpoints per contracts/approval-api.md.
**Files:**
- `internal/approve/handlers.go` — GET /api/notes, POST approve, POST reject
- `internal/approve/handlers_test.go`
**Dependencies:** APR-002, FND-008, FND-006, EXT-007
**Acceptance Criteria:**
- [ ] `GET /` — serves embedded HTML
- [ ] `GET /api/notes` — returns JSON array of all pending notes from `~/.multi-kb/pending/`
- [ ] `POST /api/notes/:filename/approve` — body: {target_kb, title, content}
  - Submits to target KB:
    - **Local KB:** generates UID at approval time via GenerateUID() (FND-004), writes `<UID>.md` file with frontmatter + content
    - **Remote KB:** calls submitKnowledge API (server generates UID)
  - On successful submission: removes target from pending file's target_kbs; deletes file if no targets remain
  - Edits to title/content persist in pending file for remaining targets
  - Returns {remaining_targets}
- [ ] `POST /api/notes/:filename/reject` — body: {target_kb}
  - Removes target from target_kbs
  - Deletes file if no targets remain
  - Returns {remaining_targets}
- [ ] Error responses: 404 (file not found), 400 (target not in array), 502 (submission failed — pending file left unchanged, target NOT removed, user can retry)
- [ ] **Approval error handling:** On remote KB submission failure (400/401/5xx), return HTTP 502 with error details to the UI. Leave the pending file unchanged (target not removed). No retry logic, no LLM correction — keep the approval flow simple. User can retry from the UI.
- [ ] Test cases: list notes, approve for one target (local KB — verify UID generated), approve for remote KB, approve last target (file deleted), reject, edit before approve, 404, 400, remote submission failure (502 returned, pending unchanged)

### APR-004: Approve Command Wiring
**Description:** Wire up `multi-kb approve` to launch the web server per spec FR-9.
**Files:**
- `internal/cmd/approve.go` — full implementation replacing stub
**Dependencies:** APR-002, APR-003
**Acceptance Criteria:**
- [ ] Starts the approval web server
- [ ] Checks for pending notes first; if none, prints "No notes awaiting approval" and exits
- [ ] Prints server URL to terminal
- [ ] Blocks until server shuts down (idle timeout, all resolved, or Ctrl+C)
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
- [ ] Parses all server-mode fields: sqs.queue_url, sqs.batch_size, codecommit.repo_name/region, s3.bucket/region, opensearch.endpoint/region, bedrock_kb.knowledge_base_id/data_source_id, tick_interval, dream_cycle.interval/model_id, recall_log.schedule
- [ ] Server-mode fields only validated when `mode: server`
- [ ] Required fields for server mode: sqs.queue_url, codecommit.repo_name, s3.bucket, opensearch.endpoint, bedrock_kb.knowledge_base_id/data_source_id
- [ ] Test cases: valid server config, missing required server fields, client mode ignores server fields

### SRV-002: Tick Loop and Activity Dispatch
**Description:** Implement the server-mode main loop: periodic tick, dream cycle vs ingestion dispatch per spec FR-12.
**Files:**
- `internal/server/server.go` — RunServer function (tick loop, activity selection, systemd-friendly)
- `internal/server/server_test.go`
**Dependencies:** SRV-001, FND-003
**Acceptance Criteria:**
- [ ] Runs as long-lived process (not short-lived)
- [ ] Wakes every `tick_interval` (default 5 minutes)
- [ ] On each tick: checks if dream cycle is due (elapsed > dream_cycle.interval, default 3h)
  - If due: runs dream cycle
  - Otherwise: runs SQS ingestion + recall log processing (if daily threshold crossed)
- [ ] If previous tick still running, skips current tick (no concurrent processing)
- [ ] Maintains lock file with heartbeat
- [ ] Handles SIGTERM/SIGINT for graceful shutdown (systemd integration)
- [ ] Test cases: tick dispatching logic, dream cycle scheduling, skip-on-busy, graceful shutdown

### SRV-003 [P]: SQS Polling and Batching
**Description:** Implement SQS message polling, batching, and acknowledgement per spec FR-12 SQS Ingestion.
**Files:**
- `internal/server/ingest.go` — PollSQS, ProcessBatch, DeleteMessages
- `internal/server/ingest_test.go`
**Dependencies:** SRV-001
**Acceptance Criteria:**
- [ ] Polls configured SQS queue via AWS SDK
- [ ] Batches ~5-10 messages (configurable batch_size)
- [ ] Parses each message: {uid, title, content, author, submitted_at}
- [ ] Hands batch to CodeCommit operations for commit
- [ ] Deletes successfully processed messages from queue
- [ ] Failed messages left in queue for SQS retry (up to 3 attempts before DLQ)
- [ ] Test cases: successful batch, partial failure, empty queue, malformed message

### SRV-004 [P]: CodeCommit Git Operations
**Description:** Implement CodeCommit git operations for server mode: clone, commit batches of note files per spec FR-12.
**Files:**
- `internal/server/codecommit.go` — CloneRepo, CommitBatch
**Dependencies:** FND-005
**Implementation Note:** Uses `os/exec` shell-out to `git` binary. CodeCommit credential helper configured via `git config` (set up by CDK user data script). Sanitize SQS message fields (UID, title, author) before use in file names or git commit messages.
**Acceptance Criteria:**
- [ ] Clones CodeCommit repository via HTTPS (git-codecommit VPC endpoint)
- [ ] Creates `<UID>.md` Markdown files with full frontmatter per data-model.md Entity 1: uid, title, status: pending, author, last-updated (from submitted_at), empty last-linked-to/last-recalled/consolidated-from-notes
- [ ] Commits entire batch as a single git commit
- [ ] Handles git push to CodeCommit
- [ ] Test cases: clone, commit single note, commit batch, push

### SRV-005 [P]: Incremental S3 Sync
**Description:** Implement incremental S3 sync after each git commit per spec FR-12 S3 Incremental Sync.
**Files:**
- `internal/server/s3sync.go` — SyncToS3 function (git diff → upload/delete)
- `internal/server/s3sync_test.go`
**Dependencies:** SRV-004
**Acceptance Criteria:**
- [ ] Uses `git diff` between previous and current commit to determine changeset
- [ ] Files added/modified → S3 PutObject
- [ ] Files deleted → S3 DeleteObject
- [ ] Not a full repo comparison — incremental only
- [ ] Retry up to 3 times with exponential backoff on sync failure
- [ ] On persistent failure: log error and continue (next sync or Phase 0 catches missed files)
- [ ] Test cases: upload new files, delete files, retry on failure, empty diff (no-op)

### SRV-006: Server Dream Cycle (OpenSearch-Backed Phases)
**Description:** Implement server-mode dream cycle with OpenSearch for Phase 1/2 and Bedrock KB sync for Phase 0/4 per spec FR-12.
**Files:**
- `internal/server/dreamcycle.go` — ServerDreamCycle: Phase 0 (sync+reindex), Phase 1 (OpenSearch pending query + similarity grouping), Phase 2 (OpenSearch related query), Phase 4 (final sync + reindex)
- `internal/server/dreamcycle_test.go`
**Dependencies:** DRM-004, SRV-004, SRV-005
**Contract:** [consolidation-output.md](contracts/consolidation-output.md) — Phase 3 shares logic with DRM-004
**Acceptance Criteria:**
- [ ] Phase 0: syncs CodeCommit→S3, triggers `StartIngestionJob`, polls `GetIngestionJob` with 10-min hard cutoff (proceeds best-effort if timeout)
- [ ] Phase 1: queries OpenSearch (via VPC endpoint) for `status: pending` notes; groups into batches by similarity (max 10 per batch): pick ungrouped seed, query for similar pending notes, form batch, repeat
- [ ] Phase 2: for each batch, queries OpenSearch for related `status: active` notes (max 10 per batch)
- [ ] Phase 3: identical to local mode (DRM-004 — shared logic)
- [ ] Phase 4: final S3 sync, triggers StartIngestionJob, polls for completion, updates dream cycle timestamp, releases lock
- [ ] Test cases: full cycle with mocked OpenSearch/Bedrock, Phase 0 timeout, similarity grouping

### SRV-007: Daily Recall Log Processing
**Description:** Implement daily recall log processing per spec FR-12 Recall Log Processing.
**Files:**
- `internal/server/recalllog.go` — ProcessRecallLogs function
- `internal/server/recalllog_test.go`
**Dependencies:** SRV-004, SRV-005
**Acceptance Criteria:**
- [ ] Runs once per day (tracked by last-run timestamp; first eligible non-dream-cycle tick after daily threshold)
- [ ] Scans S3 objects under `recall-logs/<previous-day-YYYY-MM-DD>/` prefix
- [ ] Parses each recall log JSON: {timestamp, query, recalled_uids}
- [ ] For each unique UID: updates `last-recalled` frontmatter in the CodeCommit note
- [ ] Silently skips UIDs for notes that no longer exist
- [ ] Commits all `last-recalled` updates as a single git commit
- [ ] Test cases: process logs with multiple UIDs, missing notes, empty log day, already processed

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
- [ ] Builds for: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
- [ ] All builds use `CGO_ENABLED=0` for fully static binaries
- [ ] Binary naming convention: `multi-kb-<os>-<arch>[.exe]`
- [ ] `make build-all` produces all 5 binaries
- [ ] Each binary runs `--version` successfully on its target platform (or via cross-testing where available)

### BLD-002 [P]: Binary Size Optimization
**Description:** Audit and optimize binary size using ldflags and build tags.
**Files:**
- `Makefile` — add optimization flags
**Dependencies:** BLD-001
**Acceptance Criteria:**
- [ ] Uses `-ldflags="-s -w"` to strip debug info and DWARF symbols
- [ ] Evaluates `upx` compression (optional, document trade-offs)
- [ ] Documents final binary sizes per platform
- [ ] Binary size is reasonable for a Go CLI with embedded web assets (<30MB target)

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
- [ ] Defines the extraction task, output JSON format (title/content/suggested_target_kbs), and quality guidelines
- [ ] Instructs LLM to focus on `previously_processed: false` messages while using full conversation for context
- [ ] Instructs LLM to avoid extracting trivial or obvious information
- [ ] Specifies that empty array `[]` is valid when no knowledge is extractable
- [ ] Tested against ≥3 sample conversations: one with clear knowledge, one with no extractable knowledge, one re-processed conversation with mixed flags
- [ ] Prompt length reasonable (under ~2K tokens)

### PRM-002 [P]: Dream Cycle Consolidation Prompt
**Description:** Author the consolidation system prompt for dream cycle Phase 3 per contracts/consolidation-output.md.
**Files:**
- `internal/dreamcycle/prompts/consolidation.go` — prompt text
- Sample test batches for validation
**Dependencies:** contracts/consolidation-output.md
**Acceptance Criteria:**
- [ ] Instructs LLM to evaluate each pending note against related active notes
- [ ] Defines all four action types (keep, merge, split, consolidate) with clear criteria for when to use each
- [ ] Specifies the JSON output schema from consolidation-output.md
- [ ] Instructs that every pending note UID must appear in exactly one action
- [ ] Instructs to preserve information — never silently discard content
- [ ] Distinguishes merge (absorb into existing) from consolidate (create new from multiple)
- [ ] Tested against ≥3 sample batches: one with a novel note (keep), one with a duplicate (merge), one with overlapping notes (consolidate)
- [ ] Prompt length reasonable (under ~3K tokens)

### PRM-003 [P]: Coverage Assessment Prompt (CDK)
**Description:** Author the coverage gap detection prompt for the recallKnowledge Lambda's coverage assessment flow.
**Files:**
- `lambda/recall/prompts/coverage.ts` — prompt text (or inline in handler)
- Sample test inputs for validation
**Dependencies:** R-2 research (Bedrock KB metadata extraction)
**Acceptance Criteria:**
- [ ] Input format: user's original query + summaries of top Retrieve results (title + content snippet)
- [ ] Output format: JSON `{ "gap_detected": boolean, "refined_query": string | null }`
- [ ] When gap detected, `refined_query` contains a reformulated query targeting the gap
- [ ] When no gap, `refined_query` is null
- [ ] Tested against ≥3 sample scenarios: good coverage (no gap), missing topic (gap + refined query), ambiguous results
- [ ] Prompt is concise — designed for a fast model (Haiku-class)

### PRM-004 [P]: Keyword Derivation Prompt
**Description:** Author the prompt for deriving 3-5 search keywords from a user's first message for local KB recall per spec FR-7.
**Files:**
- `internal/recall/prompts/keywords.go` — prompt text
**Dependencies:** None
**Acceptance Criteria:**
- [ ] System prompt instructs: extract 3-5 key search terms from the user's message, return as JSON array of strings
- [ ] Keywords should be specific technical terms, not generic words
- [ ] Output format: `["keyword1", "keyword2", "keyword3"]`
- [ ] Tested against ≥3 sample first messages: a technical question, a broad request, a short ambiguous query
- [ ] Prompt is concise — designed for a fast model (Haiku-class)

### PRM-005 [P]: Chunk Summarization Prompt
**Description:** Author the prompt for summarizing processed conversation chunks to ~10-20K tokens for context carry-forward per spec FR-5.
**Files:**
- `internal/extract/prompts/summarize_chunk.go` — prompt text
**Dependencies:** None
**Acceptance Criteria:**
- [ ] Instructs LLM to summarize the conversation chunk preserving: key topics discussed, decisions made, technical details, and context needed for understanding subsequent messages
- [ ] Target output length: ~10-20K tokens
- [ ] Specifies that only the latest summary is carried forward (not accumulated from all prior chunks)
- [ ] Tested against ≥1 sample long conversation chunk
- [ ] Uses extraction model (`extraction.model_id`), not the cheaper translation model

---

## Research Ordering

_Research items must complete before their dependent implementation phases._

| Research | Must Complete Before | Blocks |
|----------|---------------------|--------|
| **R-2:** Bedrock KB metadata extraction | CDK Phase 4 (Lambda Functions) | LMB-004, PRM-003 |
| **R-4:** Notor conversation format | CLI Phase 2 (Translation Layer) | TRN-003 |
| **R-5:** Claude Code hook registration | CLI Phase 4 (Hook Injection) | HKI-001, HKI-007 |
| **R-6:** Notor hook registration | CLI Phase 4 (Hook Injection) | HKI-002 |

**Recommended start order:** R-5 and R-2 are highest priority (block the most downstream tasks). R-4 and R-6 can follow.

**Tasks that can proceed without research:** All of CLI Phase 0, Phase 1 (Foundation), Phase 3 (Extraction — uses Claude Code only for initial testing), Phase 5 (Dream Cycle), Phase 7 (Approval UI). All of CDK Phases 0-3 and Phase 5-8.

---

## Phase 10: Quality & Integration Testing

_Cross-cutting validation phase. Tasks can start as soon as their dependencies are ready._

### QAT-001: Unit Test Coverage Pass
**Description:** Ensure comprehensive unit test coverage across all packages.
**Files:** All `*_test.go` files across `internal/`
**Dependencies:** All FND-*, TRN-*, EXT-*, HKI-*, DRM-*, WIZ-*, APR-*, SRV-* tasks
**Acceptance Criteria:**
- [ ] `go test ./...` passes with no failures
- [ ] `go test -race ./...` passes (no race conditions)
- [ ] Every public function has at least one test
- [ ] Edge cases from spec scenarios tested: oversized conversations, hook timeout, partial extraction, lock contention, re-processing modified conversations

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
- [ ] Bedrock InvokeModel: real call with extraction prompt → valid JSON array response
- [ ] Remote submitKnowledge: real call → HTTP 202 with UID
- [ ] Remote recallKnowledge: real call → JSON array of notes
- [ ] Git operations: init, commit, grep against real git repo
- [ ] Cron registration: register and verify on real crontab
- [ ] All integration tests skip gracefully when credentials/services unavailable
- [ ] Tagged with `//go:build integration` to exclude from `go test ./...`

### QAT-003 [P]: End-to-End Scenario Validation
**Description:** Validate the full user scenarios from spec.md against the running binary.
**Files:**
- Manual test checklist (not automated)
**Dependencies:** All previous phases
**Acceptance Criteria:**
- [ ] **First-Time Setup:** Binary download → setup wizard → config written → hooks registered → cron registered (under 10 minutes)
- [ ] **Scheduled Capture:** Cron fires → conversations scanned → knowledge extracted → notes routed → run log written
- [ ] **Hook Injection:** New conversation → hook fires → recall queries → Markdown injected → conversation proceeds
- [ ] **Oversized Conversation:** >800K token conversation → chunked → all knowledge extracted
- [ ] **Extraction Failure:** Bedrock throttle → retry → partial acceptance → error logged
- [ ] **Hook Timeout:** Slow KB → timeout → partial results used
- [ ] **Re-Processing:** Modified old conversation → re-translated → new knowledge extracted
- [ ] **Approval Flow:** Pending notes → `multi-kb approve` → review → approve/reject → submitted/deleted
- [ ] **Dream Cycle:** Pending notes → singleton batches → related lookup → consolidation → active notes

### QAT-004 [P]: Security Review
**Description:** Verify security requirements per spec NFR-3.
**Dependencies:** All previous phases
**Acceptance Criteria:**
- [ ] No credentials stored in config or state files (only profile names)
- [ ] Exclusion rules properly appended to extraction prompt
- [ ] Local KB content only leaves machine when explicitly routed to remote KB
- [ ] Approval server binds to localhost only
- [ ] No command injection vulnerabilities in git shell-outs or cron registration
- [ ] Pending queue files not world-readable (appropriate file permissions)

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
