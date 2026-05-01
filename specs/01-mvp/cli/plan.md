# Implementation Plan: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Specification:** [spec.md](spec.md)
**Cross-Reference:** [CDK spec](../cdk/spec.md)
**Status:** Planning

## Technical Context

### Architecture Decisions

| Area | Decision | Rationale |
|------|----------|-----------|
| **Language** | Go | Specified in spec — single static binary, cross-compilation, strong AWS SDK, fast compile times |
| **Binary Distribution** | `CGO_ENABLED=0` static binaries for Linux/macOS/Windows (amd64 + arm64 where applicable) | NFR-1: no external runtime dependencies |
| **Config Format** | YAML (`config.yaml` + `state.yaml`) | Spec-defined; human-editable config separate from machine-managed state |
| **Local KB Storage** | Git repositories under `~/.multi-kb/local/` with Obsidian-flavor Markdown | Spec-defined; mirrors remote KB note format |
| **Local Search** | `git grep` against working tree | Spec-defined; zero additional dependencies |
| **LLM Backend** | AWS Bedrock (configurable model IDs) | Spec-defined; single AWS dependency for all LLM calls |
| **Scheduling** | OS-native cron (crontab on macOS/Linux, Task Scheduler on Windows) | Spec-defined; short-lived processes, not daemons |
| **Interactive Setup** | `bubbletea` or `survey` Go library | Spec-defined; terminal-native wizard |
| **Approval Web UI** | Embedded static assets via `embed.FS`, served by `net/http` | Spec-defined; single-binary constraint |
| **Concurrency Control** | Lock file with heartbeat (30-min TTL, 60-sec update interval) | Spec-defined; consistent with server-side pattern |
| **UID Generation** | 16-character Crockford base32, locally generated | Spec-defined; independent of remote KB UIDs |

### Technology Stack

| Component | Choice | Notes |
|-----------|--------|-------|
| Language | Go 1.22+ | Minimum version for `embed.FS` improvements and recent stdlib features |
| AWS SDK | `aws-sdk-go-v2` | Bedrock, SQS client (for understanding server mode), S3 |
| CLI Framework | `cobra` | Standard Go CLI framework; subcommand routing |
| Terminal UI | `bubbletea` + `lipgloss` (charmbracelet) | Interactive setup wizard |
| YAML | `gopkg.in/yaml.v3` | Config/state parsing |
| JSON | `encoding/json` (stdlib) | JSONL translation, pending notes, API responses |
| HTTP Client | `net/http` (stdlib) | Remote KB API calls, SigV4-signed |
| HTTP Server | `net/http` (stdlib) | Approval web server |
| Git Operations | `os/exec` shelling to `git` | Local KB commits, `git grep` for recall |
| Logging | `log/slog` (stdlib) | Structured JSON logging |
| Testing | `testing` (stdlib) + `testify` | Unit and integration tests |
| Build | `goreleaser` or Makefile with `GOOS`/`GOARCH` matrix | Cross-platform binary production |

### Technology Stack Rationale

**Go + Cobra for CLI Framework**
- **Decision:** Use Go with `cobra` for command routing
- **Rationale:** `cobra` is the de facto standard for Go CLIs (used by kubectl, gh, docker). Provides subcommand routing, help generation, shell completion, and flag parsing out of the box. Aligns with the Go language requirement in the spec.
- **Alternatives Considered:** `urfave/cli` (lighter but less ecosystem support), `kong` (annotation-based, less community adoption)
- **Trade-offs:** Cobra adds ~2MB to binary size but eliminates significant boilerplate

**Charmbracelet (bubbletea/lipgloss) for Terminal UI**
- **Decision:** Use the Charmbracelet ecosystem for the interactive setup wizard
- **Rationale:** `bubbletea` provides an Elm-architecture TUI framework ideal for multi-step wizards. The spec explicitly names it as a candidate. Supports styled selection menus, text inputs, and progressive disclosure.
- **Alternatives Considered:** `survey` (simpler but less flexible for complex flows), `tview` (overkill for a setup wizard)
- **Trade-offs:** Slightly more complex model vs `survey`, but handles the multi-step wizard flow more naturally

**`aws-sdk-go-v2` for AWS Integration**
- **Decision:** Use the v2 AWS SDK for all AWS service interactions
- **Rationale:** v2 is the current supported SDK. Required for Bedrock API calls (InvokeModel), SigV4 request signing for remote KB APIs, and credential chain delegation (including SSO profile support).
- **Alternatives Considered:** v1 SDK (legacy, no Bedrock support without extensions)
- **Trade-offs:** v2 has a module-per-service design (more imports) but better performance and smaller binary per-service

**Shelling to `git` for Local KB Operations**
- **Decision:** Use `os/exec` to invoke the `git` CLI rather than a pure-Go git library
- **Rationale:** The spec assumes git is installed locally (listed under Assumptions). `git grep` is the specified search mechanism. A pure-Go library (go-git) would eliminate the git dependency but doesn't support `git grep` and has performance gaps for working-tree operations.
- **Alternatives Considered:** `go-git` (no `git grep`, slower for large repos), `libgit2` via cgo (breaks `CGO_ENABLED=0` requirement)
- **Trade-offs:** Requires git installed on user's machine (stated assumption in spec); slightly less testable than a library but fully compatible with all spec requirements

### Integration Points

| Integration | Protocol | Auth | Notes |
|-------------|----------|------|-------|
| Remote KB `submitKnowledge` | HTTPS POST, SigV4 | IAM or Federate (transparent) | CDK spec FR-2: HTTP 202 with UID |
| Remote KB `recallKnowledge` | HTTPS POST, SigV4 | IAM or Federate (transparent) | CDK spec FR-9: JSON array of notes |
| AWS Bedrock `InvokeModel` | AWS SDK | IAM (configured profile) | Extraction, translation summarization, dream cycle consolidation |
| Claude Code hooks | `user_prompt_submit` shell hook | Local process | First-message guard; stdout injection |
| Notor hooks | Conversation-start hook | Local process | Stdout injection |
| Claude Code history | File read from `~/.claude/projects/` | Local filesystem | JSONL conversation files |
| Notor history | File read from `{vault}/notor/history/` | Local filesystem | Native Notor format |
| OS cron | crontab (macOS/Linux) / Task Scheduler (Windows) | Local OS | `multi-kb run` entry |

### Cross-Spec Compatibility Notes

The following points were verified against the CDK spec to ensure CLI/CDK alignment:

1. **submitKnowledge contract:** CLI sends `{ title, content, author }` → CDK validates and returns `{ uid, request_id }` with HTTP 202. Field constraints: title ≤255 chars, content ≤100K chars, author ≤100 chars (CDK FR-2). CLI must match these limits in pre-flight validation to surface friendly errors before network round-trips.
2. **recallKnowledge contract:** CLI sends `{ query, limit? }` → CDK returns `[{ uid, title, content, score }]` (CDK FR-9). CLI interleaves these with local results by rank.
3. **Error format:** HTTP 400 returns `{ "errors": { "<field>": "<reason>" } }` — a flat object, not an array (CDK clarification).
4. **Note frontmatter schema:** Both specs agree on: `uid`, `title`, `status`, `author`, `last-updated`, `last-linked-to`, `last-recalled`, `consolidated-from-notes`. `last-linked-to` is not populated in MVP by either side.
5. **UID format:** 16-character Crockford base32, generated by submitKnowledge Lambda for remote KBs and by CLI for local KBs. No correlation between them.
6. **Dream cycle phases:** Server mode (CDK FR-10) uses OpenSearch + S3 sync. Client mode (CLI FR-8) uses `git grep` + local git. Same Phase 0–4 structure, same LLM prompts and action types, different storage/search backends.
7. **Lock file pattern:** Both use 30-min TTL, 60-sec heartbeat. Shared logic.
8. **SQS message schema:** `{ uid, title, content, author, submitted_at }` — CLI doesn't interact with SQS directly (that's server mode), but the server mode CLI code must produce/consume this format.
9. **Recall logs:** Written by CDK recall Lambda to S3. Processed by server-mode CLI. Client-mode CLI does not write or process recall logs (accepted MVP limitation).

## Phase 0: Research & Architecture

### Technology Research Tasks

All major technology choices are resolved by the spec (Go, Bedrock, git grep, YAML, etc.). The following items require targeted investigation before implementation begins:

#### R-1: Bubbletea Wizard Pattern Research
- **Research Task:** Evaluate `bubbletea` vs `survey` for the multi-step setup wizard (FR-2)
- **Questions to Answer:** Which library better handles the branching flow (select harness → discover sources → add KBs → configure routing → set approval mode)? How do they handle terminal compatibility across macOS/Linux/Windows?
- **Success Criteria:** Working prototype of a 3-step wizard flow with selection menus and text inputs
- **Recommendation:** Start with `bubbletea` + `huh` (Charmbracelet's form library built on bubbletea) — designed specifically for multi-step form flows

#### R-2: Bedrock InvokeModel Go SDK Pattern
- **Research Task:** Establish the Go SDK pattern for calling Bedrock `InvokeModel` with Claude models
- **Questions to Answer:** How to configure model ID, pass system prompt + user content, parse JSON response? How does credential chain resolution work with named SSO profiles?
- **Success Criteria:** Working extraction call that returns parsed JSON array of notes
- **Deliverable:** Reusable `bedrock.Client` wrapper with retry + backoff

#### R-3: Claude Code Conversation Format
- **Research Task:** Document the exact JSONL schema of Claude Code conversation files in `~/.claude/projects/<project>/<session>.jsonl`
- **Questions to Answer:** What are the message roles? How are tool calls/results structured? How to identify project subdirectory from a user-provided path? How to detect first-message vs. continued conversation from the hook context?
- **Success Criteria:** Working translator that reads a real Claude Code conversation and produces intermediate format

#### R-4: Notor Conversation Format
- **Research Task:** Document the Notor chat history format at `{vault}/notor/history/`
- **Questions to Answer:** File format (JSON, JSONL, other)? Message schema? Per-message timestamps available? Persona/workflow metadata available?
- **Success Criteria:** Working translator that reads a real Notor conversation and produces intermediate format

#### R-5: Claude Code Hook Registration
- **Research Task:** Document how to programmatically register a `user_prompt_submit` hook in Claude Code
- **Questions to Answer:** Where is the hook config file? What is the registration format? Can multiple hooks coexist at the same trigger point? What context (env vars, stdin, args) does the hook receive? How to detect first-message from within the hook?
- **Success Criteria:** CLI can register a hook that fires on conversation start and injects a test string into the system context

#### R-6: Notor Hook Registration
- **Research Task:** Document how to programmatically register a conversation-start hook in Notor
- **Questions to Answer:** Where is the hook config? What format? How does Notor pass conversation context to the hook? What does the hook return (stdout? file?) for injection?
- **Success Criteria:** CLI can register a hook that injects a test string at conversation start

#### R-7: Crockford Base32 UID Generation
- **Research Task:** Identify or implement Crockford base32 encoding for 16-character UIDs
- **Questions to Answer:** Existing Go library? If not, what's the input entropy source (crypto/rand)? How to ensure exactly 16 characters?
- **Success Criteria:** Function that generates collision-resistant 16-char Crockford base32 UIDs
- **Note:** 16 chars of Crockford base32 = 80 bits of entropy (5 bits per char). Use `crypto/rand` for 10 random bytes, encode to Crockford base32.

#### R-8: Cross-Platform Cron Registration
- **Research Task:** Implement crontab registration on macOS/Linux and Task Scheduler on Windows
- **Questions to Answer:** How to safely append a crontab entry without clobbering existing entries? How to use `schtasks.exe` on Windows? How to make the entry idempotent (re-running setup doesn't duplicate)?
- **Success Criteria:** `multi-kb setup` can register and `multi-kb status` can read the scheduled entry on all platforms

### Research Deliverables
- `research.md` — Consolidated findings for all R-1 through R-8 items (generated below)

## Phase 1: Design & Contracts

**Prerequisites:** Research complete (R-1 through R-8 resolved)

### Module Architecture

The CLI is structured as a single Go module with the following internal packages:

```
cmd/multi-kb/
├── main.go                     # Entry point, cobra root command
internal/
├── cmd/                        # Cobra subcommand definitions
│   ├── root.go                 # Root command, global flags
│   ├── setup.go                # `multi-kb setup` — interactive wizard
│   ├── run.go                  # `multi-kb run` — combined capture + dream cycle
│   ├── process.go              # `multi-kb process` — capture only
│   ├── dreamcycle.go           # `multi-kb dream-cycle` — dream cycle only
│   ├── approve.go              # `multi-kb approve` — launch approval web UI
│   ├── status.go               # `multi-kb status` — display run history + config
│   ├── addsource.go            # `multi-kb add-source` — post-setup source addition
│   ├── addkb.go                # `multi-kb add-kb` — post-setup KB addition
│   └── hook.go                 # `multi-kb hook` — hook entry point (called by harness)
├── config/                     # Config + state file loading, validation, writing
│   ├── config.go               # config.yaml schema and loader
│   ├── state.go                # state.yaml schema and loader
│   └── validate.go             # Config validation rules
├── translate/                  # Conversation translation layer (FR-4)
│   ├── intermediate.go         # Intermediate JSONL format types
│   ├── claudecode.go           # Claude Code translator
│   ├── notor.go                # Notor translator
│   └── summarize.go            # Tool interaction summarization (small: template, large: LLM)
├── extract/                    # Extraction sub-agent (FR-5, FR-6)
│   ├── extract.go              # Extraction orchestrator (single-pass and chunked)
│   ├── prompt.go               # System prompt construction (hardcoded + append + exclusions)
│   └── parse.go                # Output JSON parsing with partial acceptance
├── route/                      # Routing engine (FR-3 routing logic)
│   ├── route.go                # Applies routing rules (always/consider, overrides)
│   └── pending.go              # Pending queue file management (~/.multi-kb/pending/)
├── recall/                     # Knowledge recall (FR-7, FR-8)
│   ├── local.go                # Local KB git grep recall with keyword derivation
│   ├── remote.go               # Remote KB recallKnowledge API client
│   ├── merge.go                # Rank-based interleaving of results
│   └── format.go               # Markdown output formatting for injection
├── submit/                     # Knowledge submission
│   ├── local.go                # Write note to local KB git repo
│   ├── remote.go               # submitKnowledge API client with retry/throttle
│   └── uid.go                  # Crockford base32 UID generation
├── dreamcycle/                 # Local dream cycle (FR-8 phases)
│   ├── cycle.go                # Orchestrator: Phase 0–4 sequencing
│   ├── phase1.go               # Singleton batch creation
│   ├── phase2.go               # git grep related note retrieval
│   ├── phase3.go               # LLM consolidation + action application
│   └── actions.go              # Action types: keep, merge, split, consolidate
├── hook/                       # Harness hook logic (FR-7)
│   ├── inject.go               # Core injection logic (recall + format + pending notice)
│   ├── claudecode.go           # Claude Code hook specifics (first-message guard)
│   └── notor.go                # Notor hook specifics
├── bedrock/                    # AWS Bedrock client wrapper
│   ├── client.go               # InvokeModel with retry, backoff, profile resolution
│   └── models.go               # Model ID constants and request/response types
├── lock/                       # Lock file with heartbeat (shared client/server)
│   ├── lock.go                 # Acquire, release, heartbeat goroutine
│   └── lock_test.go
├── schedule/                   # OS-native scheduler integration
│   ├── cron_unix.go            # crontab registration (macOS/Linux)
│   ├── cron_windows.go         # Task Scheduler registration (Windows)
│   └── parse.go                # Cron expression parsing for next-run display
├── server/                     # Server mode (FR-12)
│   ├── server.go               # Tick loop, activity dispatch
│   ├── ingest.go               # SQS polling, batching, CodeCommit commit
│   ├── s3sync.go               # Incremental S3 sync from git diff
│   ├── recalllog.go            # Daily recall log processing
│   └── dreamcycle.go           # Server dream cycle (OpenSearch-backed phases)
├── approve/                    # Approval web UI (FR-9)
│   ├── server.go               # HTTP server lifecycle (auto-port, idle timeout, browser open)
│   ├── handlers.go             # API handlers: GET /api/notes, POST approve/reject
│   └── assets/                 # Embedded static assets (HTML, CSS, JS)
│       └── embed.go            # embed.FS declaration
├── git/                        # Git operations wrapper
│   ├── repo.go                 # Clone, commit, grep, diff
│   └── grep.go                 # git grep with match counting and title weighting
├── logging/                    # Structured logging
│   ├── runs.go                 # runs.jsonl append
│   └── errors.go               # extraction-errors.jsonl, hook-errors.jsonl
└── token/                      # Token counting approximation
    └── count.go                # Fast token estimation for chunking threshold
```

### Data Model

See [data-model.md](data-model.md) for full entity definitions, relationships, validation rules, and state transitions.

### API Contracts

See [contracts/](contracts/) directory:
- [contracts/submit-knowledge.md](contracts/submit-knowledge.md) — `submitKnowledge` client contract
- [contracts/recall-knowledge.md](contracts/recall-knowledge.md) — `recallKnowledge` client contract
- [contracts/approval-api.md](contracts/approval-api.md) — Local approval web server API
- [contracts/intermediate-format.md](contracts/intermediate-format.md) — Translated conversation intermediate format
- [contracts/extraction-output.md](contracts/extraction-output.md) — Extraction LLM output contract

### Development Environment Setup

See [quickstart.md](quickstart.md) for developer onboarding and build instructions.

## Implementation Phases

### Phase A: Foundation (No LLM, No Network)

Build the project skeleton and local-only infrastructure:

1. Go module initialization, Cobra command tree, config/state YAML loading
2. Lock file with heartbeat
3. Crockford base32 UID generation
4. Local KB git repository creation and note file writing
5. `git grep` recall with match-count ranking and title weighting
6. Pending queue file management (create, list, delete)
7. Run log and error log appending
8. `multi-kb status` command (config summary, run history, pending count)

### Phase B: Translation Layer

Build harness-specific conversation translators:

1. Intermediate JSONL format types and writer
2. Claude Code translator (read `~/.claude/projects/`, derive project path, translate messages, flag `previously_processed`)
3. Notor translator (read vault history, translate messages, per-message timestamps)
4. Tool interaction summarization (small: mechanical template, large: LLM via Bedrock)
5. Token counting approximation for chunking threshold

### Phase C: Extraction Pipeline

Build the LLM-powered extraction and routing:

1. Bedrock client wrapper (InvokeModel with retry, backoff, profile)
2. Extraction system prompt construction (hardcoded + append file + exclusion rules)
3. Single-pass extraction (conversation → JSON array of notes)
4. Chunked extraction for oversized conversations (>800K tokens)
5. Extraction error handling (retry, partial acceptance, error logging)
6. Routing engine (always/consider modes, per-directory/harness/persona overrides, fallback to local default)
7. Remote KB `submitKnowledge` client (SigV4, throttle, retry, error-type handling)
8. Capture processing orchestrator (`multi-kb process`)

### Phase D: Hook Injection

Build the harness hook system:

1. Claude Code hook registration and first-message guard
2. Notor hook registration
3. Remote KB `recallKnowledge` client (SigV4, timeout)
4. LLM-derived keyword generation for local recall queries
5. Rank-based result interleaving (local + remote)
6. Markdown injection formatting (with pending notice)
7. Hook entry point command (`multi-kb hook`)

### Phase E: Dream Cycle (Local)

Build client-mode dream cycle:

1. Phase 0 (no-op for local)
2. Phase 1 — singleton batches from `status: pending` notes
3. Phase 2 — `git grep` for related `status: active` notes
4. Phase 3 — LLM consolidation prompt, action parsing, action application (keep/merge/split/consolidate), per-batch git commit
5. Phase 4 — update timestamp, release lock
6. `multi-kb dream-cycle` command
7. Combined `multi-kb run` command (capture → dream cycle under single lock)

### Phase F: Setup Wizard & Scheduling

Build the interactive setup and cron integration:

1. Terminal wizard flow (bubbletea): harness selection → directory pointing → source discovery → local KB creation → remote KB addition → routing configuration → approval mode → author identity → exclusion rules
2. Hook auto-registration during setup
3. Cron registration (crontab on macOS/Linux, Task Scheduler on Windows)
4. Standalone subcommands: `multi-kb add-source`, `multi-kb add-kb`
5. Cron expression parsing for `multi-kb status` next-run display

### Phase G: Approval Web UI

Build the on-demand approval web server:

1. Embedded HTML/CSS/JS assets (single-page app)
2. HTTP server lifecycle (auto-port selection, browser launch, idle timeout, ctrl+c)
3. `GET /api/notes` — list pending notes
4. `POST /api/notes/:filename/approve` — approve with optional edit, submit to KB
5. `POST /api/notes/:filename/reject` — reject, remove target
6. Pending file management (remove target from `target_kbs`, delete when empty)

### Phase H: Server Mode

Build server-mode operation (FR-12):

1. Tick loop and activity dispatch (systemd-friendly)
2. SQS polling and batching
3. CodeCommit git operations (commit batch of note files)
4. Incremental S3 sync (git diff → upload/delete)
5. Server dream cycle phases (OpenSearch queries for Phase 1/2, Bedrock StartIngestionJob for Phase 0/4)
6. Daily recall log processing
7. Server-mode config loading and validation

### Phase I: Cross-Platform Build & Distribution

1. Build matrix: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
2. `CGO_ENABLED=0` static compilation verification
3. Goreleaser or Makefile configuration
4. Binary size optimization (if needed)

## Implementation Readiness Validation

### Technical Completeness Check
- [x] All technology choices made and documented (Go, Cobra, bubbletea, aws-sdk-go-v2, git CLI)
- [x] Data model covers all functional requirements (see data-model.md)
- [x] API contracts support all user scenarios (see contracts/)
- [x] Security requirements addressed (IAM delegation, no stored credentials, exclusion rules)
- [x] Performance considerations documented (8s hook timeout, 10 req/s throttle, token counting)
- [x] Integration points defined (Bedrock, remote KB APIs, harness hooks, git, cron)
- [x] Development environment specified (see quickstart.md)

### Quality Validation
- [x] Architecture supports scalability requirements (single binary, configurable models, modular packages)
- [x] Security model matches spec (IAM-delegated auth, local-only approval server, exclusion rules)
- [x] Data model supports all business rules (routing, approval, status lifecycle)
- [x] API design follows established patterns (REST for approval, SigV4 for remote KBs)
- [x] Documentation covers all major decisions

## Risk Assessment

### Technical Risks

**High Risk:**
- **Bedrock API Latency in Hook Path:** The hook has an 8-second timeout and must complete: LLM keyword derivation (local recall) + git grep + remote recallKnowledge + result merging + formatting. If Bedrock keyword derivation takes >2s, the remaining budget for remote KB calls is tight.
  - **Mitigation:** Keyword derivation uses the cheapest/fastest model (Haiku). Consider caching keyword derivation results. Measure end-to-end latency in R-2 research.
  - **Contingency:** Fall back to mechanical keyword extraction (stop-word removal) if LLM call is too slow.

**Medium Risk:**
- **Claude Code Hook Context:** The first-message guard relies on detecting absence of prior assistant messages from within the hook. If the hook context doesn't expose conversation history, the guard needs an alternative signal.
  - **Mitigation:** R-5 research will document available hook context. Alternatives: check session file size, use a CLI-side state file tracking active sessions.
- **Oversized Conversation Chunking:** Splitting at message boundaries and carrying forward summarized context is complex. Off-by-one errors or poor summaries could cause knowledge loss.
  - **Mitigation:** Conservative chunk sizes (well under 800K). Integration tests with real large conversations. Summary quality validation in testing.
- **Cross-Platform Cron Registration:** Windows Task Scheduler has a very different API from Unix crontab. Edge cases around existing entries, permissions, and idempotency.
  - **Mitigation:** R-8 research. Consider initially shipping macOS/Linux cron support only, with Windows support as a fast-follow.

**Low Risk:**
- **Git grep performance on large KBs:** For KBs with thousands of notes, `git grep` may become slow.
  - **Mitigation:** Spec says "fast enough for expected MVP local KB sizes (hundreds to low thousands of notes)." Monitor and optimize if needed post-MVP.
- **Bubbletea terminal compatibility:** Some terminals (especially Windows) may have rendering issues.
  - **Mitigation:** Test on major terminals (iTerm2, Terminal.app, Windows Terminal, WSL). Fallback to simpler prompts if needed.

### Dependencies and Assumptions

**External Dependencies:**
- AWS Bedrock access (model IDs configured and granted in target region)
- Claude Code hook system stability and documentation
- Notor hook system stability and documentation
- `git` installed on user's machine
- Network connectivity to remote KBs (graceful degradation on failure)

**Technical Assumptions:**
- Conversation files are accessible and not locked by the harness during processing
- `~/.multi-kb/` directory is writable with sufficient disk space
- AWS credential chain (including SSO profiles) works correctly with `aws-sdk-go-v2`
- `git grep` performance is adequate for KBs up to ~5,000 notes

**Business Assumptions:**
- Users will run setup once and rely on cron for ongoing operation
- Most conversations produce 0–5 knowledge notes (extraction output is modest)
- Manual approval is the minority case (most users will auto-approve for trusted KBs)

## Next Phase Preparation

### Task Breakdown Readiness
- [x] Clear technology choices and architecture
- [x] Complete data model and API specifications
- [x] Development environment and tooling defined
- [x] Quality standards and testing approach specified
- [x] Integration requirements and dependencies clear
- [x] Implementation phases ordered by dependency

### Recommended Next Step
Run `/speckit-05-tasks` to break down each implementation phase into specific, estimable tasks with acceptance criteria.
