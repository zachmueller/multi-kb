# Multi-KB CLI — MVP

**Created:** 2026-04-30
**Status:** Draft
**Branch:** feature/01-mvp

## Overview

A unified CLI binary written in **Go** that enables individuals to automatically extract knowledge from their AI conversations and share it across team and personal knowledge bases. The CLI operates in two modes — client mode (local, on a user's machine) and server mode (deployed to back-end infrastructure) — using a single codebase with shared logic and swappable storage/search backends.

The MVP focuses on the client-mode experience: scanning AI conversations, extracting knowledge via LLM, routing extracted notes to configured knowledge bases (local or remote), and injecting relevant knowledge into new AI conversations via harness hooks.

## Clarifications

### Session 2026-04-30
- Q: What implementation language should be used for the CLI binary? → A: Go — chosen for trivial cross-compilation to all target platforms, single static binary output, strong AWS SDK support, and fast compile times.
- Q: Are coverage assessment (two-pass recall) and recall logging (S3 logs + daily last-recalled updates) in scope for MVP? → A: Both deferred — they add latency and infrastructure complexity without being essential for the core capture-and-inject loop.
- Q: How should the CLI handle scheduled processing on the user's machine? → A: OS-native cron — CLI registers with crontab (macOS/Linux) or Task Scheduler (Windows) during setup; each run is a short-lived process, not a daemon.
- Q: What level of observability should MVP provide beyond error logs? → A: Structured run log + status command — each run appends a summary to `runs.jsonl`; `multi-kb status` displays last N runs and current config summary.
- Q: Should `federate` auth be deferred from MVP given the open-source focus? → A: Keep in MVP — from the CLI's perspective, `federate` is simply "no auth config, call endpoint directly" since the backend handles auth transparently. Minimal additional implementation cost.
- Q: Where does the pending approval queue live for notes awaiting manual review before submission to target KBs? → A: Dedicated `~/.multi-kb/pending/` directory with one JSON file per staged note (containing title, content, target KB, source conversation reference, timestamp). Simple, inspectable, no extra dependencies.
- Q: How are harness hooks (for knowledge injection) installed — automatically during setup or manually by the user? → A: CLI auto-registers hooks during setup. For each selected harness, the CLI writes the necessary hook configuration entries automatically. This keeps the "under 10 minutes" success criterion achievable and avoids error-prone manual configuration.
- Q: Where does the CLI get the `author` identity for `submitKnowledge` API calls? → A: Single `author` field configured in `config.yaml` during initial setup (one identity across all KBs). For `federate` auth this would typically be the Amazon alias; for `iam`/open-source it's a free-form string the user provides.
- Q: What happens if a scheduled capture processing run overlaps with a still-running previous run? → A: Lock file with heartbeat TTL (same pattern as dream cycles) — if a previous run still holds the lock, the new run skips. Consistent with the existing dream cycle concurrency control.
- Q: What do local dream cycle Phases 0 and 4 look like concretely, given there's no S3/OpenSearch locally? → A: Phase 0 is a no-op (local git repo is always current). Phase 4 is just git commit + update timestamps + clear manifest + release lock. No sync or reindex steps.
- Q: Should per-directory `last-processed` timestamps and other mutable runtime state live in `config.yaml` or a separate file? → A: Separate state file (`~/.multi-kb/state.yaml`) — config is user-editable intent, state is CLI-managed runtime data. Separating them avoids read-modify-write conflicts if the user edits config while the CLI is running, and keeps config validation simple.
- Q: What interaction model should the setup wizard (FR-2) use? → A: Interactive terminal wizard using guided prompts and menus (e.g., Go libraries like `bubbletea` or `survey`). Keeps the single-binary promise, requires no web UI dependency for setup, and is idiomatic for CLI tools.
- Q: What specific full text search mechanism should local KB recall use? → A: `git grep` directly against the working tree — no separate search index. Zero-maintenance, no additional dependencies, and fast enough for expected MVP local KB sizes (hundreds to low thousands of notes).
- Q: How should local KB recall results (from `git grep`) be ranked for interleaving with scored remote KB results? → A: Match count ranking — rank local results by number of query term matches per note, with title matches weighted higher than body matches. Provides a coarse relevance signal sufficient for rank-based interleaving.
- Q: Should the approval web server run persistently or on-demand? → A: On-demand via `multi-kb approve` — server starts, opens browser, shuts down when done. Consistent with the "short-lived process, not a daemon" design principle. Approval is a deliberate user action, not a background task.
- Q: How should local dream cycle Phase 1 (group pending notes by similarity) work without OpenSearch similarity search? → A: Singleton batches — each pending note is processed individually (no similarity grouping). Phase 2 uses keyword-based `git grep` queries derived from the note's title and key terms to find related existing notes. The Phase 3 consolidation LLM still catches duplicates within its batch. This avoids faking similarity search with `git grep` and keeps local dream cycles simple.
- Q: Where does the CLI find Claude Code conversation history — in the user-pointed directory or a fixed known location? → A: Fixed location with project mapping — CLI always reads from `~/.claude/projects/`, using the user-pointed directory to identify which project subdirectory to scan (by matching the project path). The user doesn't need to know where Claude Code stores its files.
- Q: How does the user learn about pending notes awaiting approval? → A: Passive notification — `multi-kb status` output and hook injection output both include a pending note count (e.g., "3 notes awaiting approval") when the queue is non-empty. No OS-native notifications or active alerting for MVP.
- Q: Does the CLI retain a local record of notes submitted to remote KBs (e.g., the returned UID)? → A: Fire-and-forget — CLI submits to remote KB, logs success/failure in `runs.jsonl`, but retains no per-note submission record. Duplicates handled by dream cycles. Local UID generation only occurs for locally-targeted notes (i.e., UIDs in the local KB have no connection to UIDs generated server-side by `submitKnowledge`, even for the same note content routed to both local and remote KBs).
- Q: How should the CLI handle pre-existing harness hooks at the same trigger points during setup? → A: Append — add the multi-kb hook alongside any existing hooks. Both Notor and Claude Code support multiple hooks per trigger point, so appending is safe and avoids breaking existing user workflows.
- Q: Should scheduled capture processing and local dream cycles run as separate cron entries or a single combined command? → A: Single combined command (`multi-kb run`) that performs capture processing then dream cycle sequentially under one lock acquisition. Avoids skip-contention between two independent cron entries competing for the shared lock. Manual triggers (`multi-kb process`, `multi-kb dream-cycle`) remain available as standalone subcommands.
- Q: Which model handles chunk summarization when oversized conversations (>800K tokens) are split across multiple extraction passes? → A: The extraction model (`extraction.model_id`) with a summarization-specific prompt. High-quality context preservation is essential for later chunks to extract knowledge correctly, and this is a rare edge case so cost is negligible.
- Q: How does `multi-kb approve` detect that the user is done and shut down the web server? → A: Idle timeout — server shuts down after a configurable period (default: 5 minutes) with no browser activity. Also shuts down immediately when all pending notes are resolved. Ctrl+C always works as an explicit kill.
- Q: Should the CLI validate the configured `author` field against the current AWS caller identity before submitting to remote KBs? → A: No — author verification is purely a server-side concern. The CLI sends whatever is configured; the backend rejects mismatches for `federate` auth. For `iam` auth there's no reliable local mapping from AWS profile to human identity string.
- Q: How should the Claude Code translator derive per-message timestamps for the `previously_processed` flag, given Claude Code's native format lacks reliable per-message timestamps? → A: File-level ordering — use the conversation file's last-modified time as the effective timestamp for all messages. If a conversation was previously processed and is modified, all prior messages are flagged `previously_processed: true` and the full conversation is re-translated. The extraction prompt's focus on `previously_processed: false` messages handles the rest. Avoids fragile per-message timestamp inference.
- Q: How should the CLI decompose a natural language first-message query into effective `git grep` searches for local KB recall? → A: LLM-derived keywords — use the translation summarization model (`translation.summarization_model_id`, e.g., Claude Haiku) to generate 3–5 search keywords from the user's natural language query, then `git grep` each keyword. Results are ranked by match count as already specified. This adds one fast, cheap LLM call to the hook injection path but produces much better keyword selection than mechanical stop-word removal.
- Q: How should the approval web UI (FR-9) assets be packaged and served, given the single-binary constraint? → A: Embed static HTML/CSS/JS assets in the Go binary via `embed.FS`. The UI is a simple approve/reject interface, so a single-page app bundled at compile time keeps the single-binary promise intact with zero runtime file dependencies. Assets are served from memory at runtime by the built-in HTTP server.
- Q: Should standalone manual subcommands (`multi-kb process`, `multi-kb dream-cycle`) respect the same lock file as the combined `multi-kb run`, or bypass it since they're user-initiated? → A: Respect the same lock — manual subcommands use the identical lock acquisition logic. If the lock is already held, the CLI prints a user-friendly message identifying what holds the lock and when the heartbeat was last updated, then exits immediately. This prevents concurrent writes to shared state/KB files regardless of trigger source.
- Q: What specific Claude Code hook mechanism should the CLI use for knowledge injection? → A: User-prompt-submit hook with first-message guard — the CLI registers a Claude Code `user_prompt_submit` hook during setup. The hook fires on every user message, but the CLI checks whether this is the first message in the conversation (e.g., by checking for the absence of prior assistant messages in the session context provided by the hook). If it is not the first message, the CLI exits immediately with no output. If it is the first message, the CLI performs knowledge recall and outputs the injected context block. This reuses a well-defined, stable Claude Code hook event while preserving the "conversation-start only" injection constraint.
- Q: What weight multiplier should title matches receive relative to body matches in local KB recall ranking? → A: 3x — a title match counts as 3 body matches when computing match-count rank scores. Simple, meaningful signal boost without over-dominating results.

## User Stories

- As a developer using AI coding assistants, I want knowledge from my AI conversations to be automatically captured and shared with my team so that tribal knowledge is reduced to zero.
- As a team member, I want relevant knowledge from my team's KB to be injected into my AI conversations automatically so that I benefit from collective team knowledge without manual lookup.
- As an individual, I want a local knowledge base for personal/private knowledge so that I can capture insights without sharing them externally.
- As a user of multiple AI harnesses, I want a single tool that works across Notor and Claude Code so that I don't need separate knowledge capture solutions for each tool.
- As a user joining a new team, I want simple setup that walks me through connecting to team KBs so that I can start benefiting from shared knowledge quickly.
- As a team lead, I want to control what gets auto-published vs. what requires manual approval so that quality is maintained without creating bottlenecks.

## Functional Requirements

### FR-1: Unified Binary with Client/Server Modes

**Description:** The CLI is distributed as a single standalone binary supporting two operational modes determined by configuration.

**Acceptance Criteria:**
- Binary operates in client mode by default (local operation on user's machine)
- Binary operates in server mode when configured via `mode: server` in `config.yaml`
- Standalone binaries are produced for Linux, macOS, and Windows with no external runtime dependencies
- Same dream cycle logic, consolidation prompts, action application code, and error handling are shared across modes
- Only storage/search backends and ingestion sources differ between modes

### FR-2: Initial Setup Experience

**Description:** First-run setup walks the user through configuring chat sources, target knowledge bases, and routing rules via an interactive terminal wizard.

**Acceptance Criteria:**
- Setup uses an interactive terminal wizard with guided prompts and selection menus (e.g., `bubbletea` or `survey` Go library)
- Individual setup steps are also available as standalone subcommands (e.g., `multi-kb add-source`, `multi-kb add-kb`) for post-setup modifications
- User can select which AI harnesses they use (Notor, Claude Code for MVP)
- User can point to directories where they use those harnesses
- CLI auto-discovers chat history locations within specified directories and presents a summary for confirmation. For Claude Code, the CLI reads from the fixed location `~/.claude/projects/` and uses the user-pointed directory to identify which project subdirectory to scan (by matching the project path).
- A local KB is created automatically (no additional setup required)
- User can add remote KBs by providing an API endpoint URL and selecting an auth type (`iam` or `federate`)
- For `iam` auth, user specifies an AWS CLI profile name; no credentials are stored by the CLI itself
- For `federate` auth, no additional auth configuration is needed from the user — the CLI calls the endpoint directly and the backend handles authentication transparently
- CLI fetches and displays the remote KB's self-description to confirm connectivity
- User can configure routing rules per directory, per directory+harness, and per directory+harness+persona/workflow
- Each routing pairing supports two settings: routing mode (`always` or `consider`) and approval mode (`auto-approve` or `require-manual-approval`)
- User can define global exclusion rules describing content that should never be shared with non-local KBs
- Simplified onboarding presets are available for approval mode: auto-approve always, always require manual approval, or select per group
- CLI auto-registers harness hooks during setup for each selected harness (e.g., Notor conversation-start hook, Claude Code `user_prompt_submit` hook with first-message guard), appending the multi-kb hook alongside any pre-existing hooks at the same trigger points (never overwriting existing hooks)
- User provides their author identity during setup (stored as top-level `author` in `config.yaml`), used for all `submitKnowledge` API calls. The CLI performs no local validation of this value against the AWS caller identity — author verification is purely a server-side concern (backend rejects mismatches for `federate` auth).

### FR-3: Conversation Scanning and Capture Processing

**Description:** The CLI scans tracked directories on a schedule (or manual trigger) for modified conversations, extracts knowledge via LLM, and routes to target KBs.

**Acceptance Criteria:**
- Scanning runs on a user-configurable schedule or via manual trigger (e.g., `multi-kb process`)
- Scheduled runs use the OS-native scheduler: crontab on macOS/Linux, Task Scheduler on Windows
- During initial setup, the CLI registers a single combined command (`multi-kb run`) with the OS-native scheduler at the user-configured interval (e.g., every 30 minutes). This command performs capture processing followed by the local dream cycle sequentially under one lock acquisition.
- Manual triggers remain available as standalone subcommands (`multi-kb process` for capture only, `multi-kb dream-cycle` for dream cycle only)
- All subcommands (`multi-kb run`, `multi-kb process`, `multi-kb dream-cycle`) share the same lock file. If the lock is already held, scheduled runs skip silently; manual subcommands print a user-friendly message identifying the lock holder and last heartbeat timestamp, then exit immediately.
- Each scheduled run is a short-lived process (not a long-running daemon)
- A lock file with heartbeat TTL prevents concurrent runs; if a previous run still holds the lock, the new run skips (same pattern as dream cycle concurrency control)
- Only conversations modified since the per-directory `last-processed` timestamp are scanned
- `last-processed` is based on conversation file's last-modified time, not wall clock time
- Each conversation is translated from native harness format into an intermediate format before extraction
- Extraction is performed via a single Bedrock API call per conversation (or per chunk for oversized conversations)
- Extracted notes are routed to target KBs per the user's routing configuration
- For `always`-mode KBs, notes are unconditionally routed regardless of LLM suggestions
- For `consider`-mode KBs, the LLM's routing recommendations are respected
- Auto-approved notes are submitted directly via the target KB's `submitKnowledge` API (or written to the local KB)
- Manual-approval notes are staged as individual JSON files in `~/.multi-kb/pending/` (one file per note, containing title, content, target KB name, source conversation path, and extraction timestamp)
- Remote submissions are self-throttled to a maximum of 10 requests per second per target KB
- After processing all conversations in a directory, the `last-processed` timestamp updates to the last-modified time of the final conversation file

### FR-4: Conversation Translation Layer

**Description:** Before extraction, conversations are translated from harness-native format into a standardized intermediate representation (JSONL).

**Acceptance Criteria:**
- Intermediate format uses JSONL with a conversation header line followed by message lines
- Roles are normalized to `user`, `assistant`, `system`
- Tool call/result pairs are collapsed into `tool_uses` entries on the assistant message
- Small tool interactions (<~1K tokens) use mechanical summary templates without an LLM call
- Large tool interactions (≥~1K tokens) are summarized via a fast, cheap LLM (configurable `translation.summarization_model_id`)
- Content block arrays are flattened to plain text strings
- Previously processed messages are flagged with `previously_processed: true` based on the directory's `last-processed` timestamp. For harnesses with per-message timestamps (Notor), each message's timestamp is compared individually. For harnesses without reliable per-message timestamps (Claude Code), the flag is applied at the file level: if the conversation file was previously processed, all messages from the prior processing are flagged `previously_processed: true` (the entire conversation is re-translated, with the extraction prompt relying on the flag to focus on new content).
- Per-harness translator modules exist for Notor and Claude Code
- Claude Code translator reads from the fixed location `~/.claude/projects/<project>/<session>.jsonl`, where `<project>` is derived from the user-configured directory path

### FR-5: Extraction Sub-Agent

**Description:** The core LLM-powered component that reads translated conversations and produces candidate knowledge notes.

**Acceptance Criteria:**
- Operates as a single Bedrock API call (not multi-turn)
- Model is globally configurable in the CLI config (`extraction.model_id`)
- System prompt is hardcoded in the binary (versioned per release)
- Users can optionally extend the system prompt via `~/.multi-kb/prompts/extraction-append.md`
- The append file is read fresh on each extraction run (no restart required)
- Output is a JSON array of objects with `title`, `content`, and `suggested_target_kbs` fields
- For re-processed conversations, extraction focuses on `previously_processed: false` messages while using the full conversation for context
- Respects the user's global exclusion rules
- Conversations exceeding 800K tokens are split at message boundaries and processed iteratively with summarized context carried forward. Chunk summarization uses the extraction model (`extraction.model_id`) with a summarization-specific prompt — not the cheaper translation model — to ensure high-quality context preservation across chunks.

### FR-6: Extraction Error Handling

**Description:** Extraction failures are handled with retry logic and partial acceptance.

**Acceptance Criteria:**
- Bedrock API failures (throttling, timeout, network error) retry up to 3 times with exponential backoff
- Malformed JSON output retries up to 3 times (fresh API call each retry)
- Partially valid JSON arrays (some entries parse, some don't) are accepted for the valid entries; conversation marked `partially-processed`
- After 3 retries with no usable output, conversation is marked `failed` and skipped
- Both `partially-processed` and `failed` conversations are logged to `~/.multi-kb/logs/extraction-errors.jsonl` with conversation ID, source path, error details, and timestamp
- Failed conversations are not automatically retried on subsequent runs (log provides data for future re-processing capability)

### FR-7: Hook-Based Knowledge Injection

**Description:** The CLI integrates with AI harness hook systems to inject relevant KB knowledge into new conversations at conversation start.

**Acceptance Criteria:**
- Hooks are auto-registered during initial setup (see FR-2); no manual hook configuration required from the user
- Hook fires only at conversation start (per-message injection is out of scope for MVP)
- User's first message is used verbatim as the query for remote KB `recallKnowledge` API calls. For local KBs, the CLI first derives 3–5 search keywords from the message via the translation summarization model (see FR-8) before executing `git grep` queries.
- All target KBs matching the current routing configuration are queried concurrently
- Results from multiple KBs are merged via rank-based interleaving (top-ranked from each KB first, then second-ranked, etc.) until 10 notes are selected
- Injected content is formatted as a Markdown list with note title, source KB name, and full content
- No token budget cap on injected content for MVP
- Hook is blocking with a configurable timeout (default: 5 seconds)
- Partial results from responsive KBs are used if other KBs time out
- If no KBs respond within timeout, conversation proceeds with no injection and a warning is logged
- Notor integration: injected block prepended to conversation system context via conversation-start hook
- Claude Code integration: CLI registers a `user_prompt_submit` hook. The hook fires on every user message but includes a first-message guard — if the conversation already has prior assistant messages, the CLI exits immediately with no output. On the first message, the CLI performs knowledge recall and outputs the injected context block, which Claude Code prepends to the conversation's system context.
- When the pending approval queue (`~/.multi-kb/pending/`) is non-empty, the injected block includes a notice with the pending note count (e.g., "3 notes awaiting approval — run `multi-kb approve` to review")

### FR-8: Local Knowledge Base Storage

**Description:** The CLI maintains a local KB that mirrors remote KB format and lifecycle.

**Acceptance Criteria:**
- Local KBs are stored under `~/.multi-kb/local/`
- A `default` local KB is created automatically during CLI setup
- Users can create additional named local KBs
- Each local KB is its own git repository
- Notes use Obsidian-flavor Markdown with the same frontmatter schema as remote KBs (uid, title, status, last-updated, etc.)
- UIDs are 16-character Crockford base32 strings generated locally (local KB UIDs are completely independent of remote KB UIDs — even when the same note content is routed to both local and remote KBs, each KB generates its own UID with no correlation between them)
- Newly captured notes start with `status: pending`
- Knowledge recall against local KBs uses `git grep` against the working tree — no separate search index, no vector embeddings
- For hook-based recall (FR-7), the CLI first calls the translation summarization model (`translation.summarization_model_id`) to derive 3–5 search keywords from the user's natural language query, then runs `git grep` per keyword. For dream cycle Phase 2 recall, keywords are derived mechanically from the note's title and key terms (no LLM call).
- Local recall results are ranked by match count (number of query term matches per note, with title matches weighted at 3x body matches) to produce a coarse relevance ordering for interleaving with remote KB results
- Local dream cycles run as part of the combined `multi-kb run` command (capture processing then dream cycle sequentially) on the OS-native cron schedule, or via manual trigger with `multi-kb dream-cycle`
- Local dream cycles use the same Phase 1–4 logic as server mode with the following local adaptations: Phase 0 is a no-op (local git repo is always current); Phase 1 skips similarity grouping entirely — each pending note is processed as a singleton batch; Phase 2 uses keyword-based `git grep` queries (derived from the note's title and key terms) to find related existing notes; Phase 4 is git commit + update dream cycle timestamp + clear manifest + release lock (no S3 sync or OpenSearch reindex)

### FR-9: Local Web UI for Approvals

**Description:** The CLI hosts an on-demand local web server (via `multi-kb approve`) providing a UI for reviewing and approving staged knowledge notes.

**Acceptance Criteria:**
- Web server launches on-demand via `multi-kb approve` command, automatically opens the user's default browser, and shuts down after a configurable idle timeout (default: 5 minutes) with no browser activity, or when all pending notes are resolved, whichever comes first. Ctrl+C in the terminal always terminates the server immediately.
- Web UI assets (HTML, CSS, JS) are embedded in the Go binary via `embed.FS` and served from memory at runtime — no external asset files or runtime dependencies required
- Web server is accessible locally for reviewing pending notes from `~/.multi-kb/pending/`
- Users can approve or reject individual staged notes
- Approved notes are submitted to their target KB (local or remote) and the corresponding JSON file is deleted from `~/.multi-kb/pending/`
- Rejected notes have their JSON file deleted from `~/.multi-kb/pending/`
- UI displays note title, content, target KB, and source conversation context (all read from the pending JSON file)

### FR-10: Configuration and State File Structure

**Description:** CLI configuration is stored in `~/.multi-kb/config.yaml` (user-editable intent). Mutable runtime state is stored separately in `~/.multi-kb/state.yaml` (CLI-managed, not user-edited).

**Acceptance Criteria:**
- **Config file (`config.yaml`):**
  - Top-level `mode` setting determines client vs. server operation
  - Top-level `author` field stores the user's identity string (used for all `submitKnowledge` API calls)
  - `knowledge_bases` array defines remote KB connections (name, endpoint, auth, description)
  - `extraction` section defines model ID, AWS profile, and region for Bedrock calls
  - `translation` section optionally overrides summarization model
  - `dream_cycle` section optionally overrides consolidation model
  - `hook` section defines injection timeout
  - Per-directory routing configuration tracks harness pairings, target KBs, routing modes, and approval modes
- **State file (`state.yaml`):**
  - Per-directory `last-processed` timestamps
  - Per-conversation processing status (`partially-processed`, `failed`)
  - Last dream cycle timestamp
  - CLI never writes to `config.yaml` after initial setup (except via explicit user-triggered commands like adding a new KB)

### FR-11: Observability and Status Reporting

**Description:** The CLI maintains a structured run log and provides a status command so users can verify the tool is working correctly.

**Acceptance Criteria:**
- Each capture processing run appends a summary entry to `~/.multi-kb/logs/runs.jsonl` containing: timestamp, run trigger (cron or manual), directories scanned, conversations processed, notes extracted, notes routed (by target KB), errors encountered, and run duration
- Each dream cycle run appends a similar summary entry (timestamp, trigger, batches processed, actions taken by type, errors, duration)
- `multi-kb status` displays a summary of the last N runs (default 10), including success/failure status and key counts
- `multi-kb status` also displays current configuration summary: tracked directories, configured KBs, and next scheduled run time
- `multi-kb status` displays the pending approval queue count when non-empty (e.g., "3 notes awaiting approval")
- Run log entries use structured JSONL format for machine parseability

## Non-Functional Requirements

### NFR-1: Cross-Platform Distribution

**Description:** The CLI is implemented in Go and must be easily installable across all major platforms without dependency management.

**Acceptance Criteria:**
- Implemented in Go, compiled via `go build` with `CGO_ENABLED=0` for fully static binaries
- Standalone binaries produced for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64)
- No external runtime dependencies (no Python, Node.js, Java, etc.)
- Install experience is: download binary, place on PATH, run
- Same binary is deployable to EC2 for server mode

### NFR-2: Performance

**Description:** The CLI must handle conversation processing and hook injection within acceptable time bounds.

**Acceptance Criteria:**
- Hook-based injection completes within the configurable timeout (default 5 seconds) including all network round-trips
- Conversation scanning and discovery completes within seconds for typical directory sizes (hundreds of conversation files)
- Token counting approximation is fast enough to not meaningfully add to processing time
- Remote API submissions are throttled to 10 req/s per target KB to avoid overwhelming back-end infrastructure

### NFR-3: Security and Privacy

**Description:** The CLI must handle credentials safely and respect user privacy boundaries.

**Acceptance Criteria:**
- CLI stores no credentials itself — delegates entirely to the AWS SDK credential chain
- Global exclusion rules prevent specified content categories from being shared with non-local KBs
- Local KB content never leaves the user's machine unless explicitly routed to a remote KB
- Clear error messages surface when credentials are expired or insufficient (guiding users to standard AWS CLI auth flows)
- Manual approval mode is available for any target KB to prevent unreviewed publication

### NFR-4: Reliability

**Description:** The CLI must handle failures gracefully without data loss.

**Acceptance Criteria:**
- Crash between processing and timestamp update does not cause missed conversations (worst case: re-processing, handled gracefully by dream cycle deduplication)
- Extraction failures are logged with sufficient detail for future re-processing
- Partial extraction results are accepted rather than discarding entire conversations
- Lock file with heartbeat TTL prevents concurrent capture processing runs and stuck dream cycles from blocking future runs indefinitely
- Network failures during hook injection degrade gracefully (conversation proceeds without injection)

### NFR-5: Extensibility

**Description:** The CLI must support customization without requiring users to fork the project.

**Acceptance Criteria:**
- User-extensible extraction prompt via `~/.multi-kb/prompts/extraction-append.md`
- Configurable model IDs for extraction, translation summarization, and dream cycle consolidation
- Per-directory/per-harness/per-persona routing granularity supports diverse team workflows
- Global exclusion rules are user-defined natural language descriptions

## User Scenarios & Testing

### Primary Flow: First-Time Setup

1. User downloads and runs the CLI binary for their platform
2. CLI detects no existing configuration and enters setup wizard
3. User selects "Notor" as their AI harness
4. User points to their Obsidian vault directory
5. CLI discovers Notor chat history at `{vault}/notor/history/` and presents summary
6. User confirms the discovered chat source
7. CLI creates the default local KB at `~/.multi-kb/local/default/`
8. User opts to add a remote team KB, provides endpoint URL, selects `iam` auth, specifies AWS profile
9. CLI fetches the KB's self-description and displays it for confirmation
10. User configures routing: all conversations from this directory route to both local (always, auto-approve) and team KB (consider, require-manual-approval)
11. CLI writes `~/.multi-kb/config.yaml` and reports setup complete

### Primary Flow: Scheduled Capture Processing

1. Cron triggers the CLI's capture processing
2. CLI reads config, identifies tracked directories
3. For each directory, CLI checks `last-processed` timestamp against conversation files' last-modified times
4. New/modified conversations are found and queued for processing
5. Each conversation is translated to intermediate format (tool interactions summarized)
6. Translated conversation is sent to Bedrock extraction model
7. LLM returns JSON array of candidate knowledge notes
8. CLI routes each note per configuration: local KB notes are written directly; team KB notes with `require-manual-approval` are staged in pending queue
9. `last-processed` timestamp updates to the last-modified time of the final processed file
10. User later opens web UI, reviews pending notes, approves relevant ones which are submitted to the remote KB

### Primary Flow: Hook-Based Injection

1. User starts a new conversation in Notor
2. Notor's conversation-start hook invokes the CLI
3. CLI takes the user's first message verbatim
4. CLI identifies target KBs for this directory (local default + team KB)
5. CLI sends `recallKnowledge` requests to both KBs concurrently
6. Local KB returns results via full text search; remote KB returns results via API
7. CLI merges results via rank-based interleaving, selects top 10
8. CLI formats results as Markdown and returns to Notor
9. Notor prepends the knowledge block to the conversation's system context
10. User's AI conversation benefits from injected team knowledge

### Alternative Flow: Oversized Conversation Processing

1. A conversation exceeds 800K tokens after translation
2. CLI splits at the 800K boundary on a message boundary
3. First chunk is processed through extraction, yielding knowledge notes
4. First chunk is summarized to ~10-20K tokens
5. Summary is prepended to the next chunk as context
6. Next chunk is processed, yielding additional notes
7. Process repeats until all chunks are processed
8. All extracted notes from all chunks are combined and routed normally

### Alternative Flow: Extraction Failure

1. CLI sends translated conversation to Bedrock
2. Bedrock returns a throttling error
3. CLI retries with exponential backoff (attempt 2)
4. Bedrock returns malformed JSON
5. CLI retries (attempt 3)
6. Bedrock returns valid JSON with 5 entries, 1 malformed
7. CLI accepts 4 valid entries, marks conversation as `partially-processed`
8. Valid notes are routed normally
9. Error details logged to `~/.multi-kb/logs/extraction-errors.jsonl`

### Alternative Flow: Hook Timeout

1. User starts a new conversation
2. Hook invokes CLI, which dispatches `recallKnowledge` to 3 configured KBs
3. Local KB responds in 200ms with results
4. Remote KB #1 responds in 1.5s with results
5. Remote KB #2 does not respond within 5s timeout
6. CLI merges results from local KB and remote KB #1 (ignoring KB #2)
7. Top 10 notes selected from available results and injected
8. Warning logged to `~/.multi-kb/logs/hook-errors.jsonl`

### Edge Case: Re-Processing a Modified Conversation

1. User returns to an old conversation (already processed) and adds new messages
2. Conversation file's last-modified time now exceeds the directory's `last-processed` timestamp
3. On next scan, CLI picks up this conversation
4. During translation, messages with timestamps ≤ `last-processed` are flagged `previously_processed: true`
5. New messages are flagged `previously_processed: false`
6. Full conversation is sent to extraction sub-agent for context
7. Sub-agent extracts knowledge only from the new portion
8. Any resulting duplicates with previously extracted knowledge are handled by dream cycle consolidation

## Success Criteria

- Users can go from binary download to first successful knowledge capture in under 10 minutes of setup time
- Knowledge from AI conversations is captured without any manual user action after initial setup (zero ongoing effort for auto-approved flows)
- Relevant team knowledge surfaces in new AI conversations within the hook timeout window (default 5 seconds)
- The system handles conversations of any length (including those exceeding 800K tokens) without silent data loss
- Manual approval workflow enables users to review and filter knowledge before it reaches team KBs, with a clear queue and simple approve/reject interaction
- A single CLI binary serves both local development use and server-mode deployment without code divergence

## Key Entities

### Knowledge Note
- **UID:** 16-character Crockford base32 string (generated once, never changed)
- **Title:** Succinct title (≤255 characters)
- **Content:** Markdown body, self-contained
- **Status:** `pending` → `active` (lifecycle managed by dream cycles)
- **Frontmatter:** uid, title, status, last-updated, last-linked-to, last-recalled, consolidated-from-notes

### Configuration (config.yaml)
- **Mode:** `client` | `server`
- **Knowledge bases:** Array of remote KB definitions (name, endpoint, auth, description)
- **Chat sources:** Per-directory harness pairings with routing rules
- **Extraction settings:** Model ID, AWS profile, region
- **Translation settings:** Summarization model override
- **Dream cycle settings:** Model override
- **Hook settings:** Injection timeout

### Intermediate Conversation Format
- **Conversation header:** id, source harness, source path, timestamps, metadata (persona, workflow, project dir)
- **Messages:** role (user/assistant/system), content (plain string), timestamp, previously_processed flag, tool_uses array

### Pending Queue Entry
- **Note:** Title + content of extracted knowledge note
- **Target KB:** Which KB(s) the note is destined for
- **Source:** Conversation ID and path for context
- **Timestamp:** When the note was extracted

## Assumptions

- Users have working AWS CLI credentials for any remote KBs using `iam` auth (the CLI does not manage credential lifecycle)
- Bedrock API access is available in the configured region with the configured model IDs
- AI harness hook mechanisms (Notor conversation-start, Claude Code session init) are stable and available
- Conversation history files are accessible on the local filesystem in known locations per harness
- Users have git installed locally (required for local KB git operations)
- Network connectivity to remote KBs is generally available (graceful degradation on failure)
- The `~/.multi-kb/` directory is writable and has sufficient disk space for local KB storage

## Out of Scope

- **Server mode implementation details:** This spec covers the CLI binary's client-mode behavior. Server-mode specifics (SQS consumption, S3 sync, OpenSearch operations, CodeCommit) are covered by the back-end infrastructure spec.
- **Per-message hook injection:** Only conversation-start injection is in scope for MVP.
- **Vector embeddings for local KBs:** Local recall uses full text search only. Vector support deferred to a future iteration.
- **Automatic retry of failed conversations:** Failed conversations are logged but not retried on subsequent runs.
- **Submission deduplication:** No idempotency key or content-hash dedup at the API layer; dream cycle handles duplicates.
- **Cross-KB score normalization:** Raw scores from different KBs are not compared; rank-based interleaving is used instead.
- **Token budget cap on injected content:** No maximum enforced for MVP.
- **Harnesses beyond Notor and Claude Code:** Other harnesses (Kiro IDE, Kiro CLI, Cline) are deferred.
- **Web UI design details:** The approval web UI's visual design and interaction specifics are deferred to a separate spec.
- **Back-end CDK infrastructure:** Covered by the `multi-kb-cdk` repository spec.
- **Coverage assessment for knowledge recall:** The two-pass retrieval quality check (secondary LLM call to detect gaps in recall results) described in the design doc is deferred to a future iteration.
- **Recall logging and last-recalled updates:** S3-based recall logging and daily batch processing to update `last-recalled` frontmatter timestamps are deferred to a future iteration.
