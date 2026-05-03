# Audit Tasks: Post-Wave 8 Defects, Gaps, and Remaining Work

**Created:** 2026-05-03
**Source:** Full-system audit of implementation-order.md, both task files, contracts, research findings, and codebase
**Status:** Open

## Overview

A comprehensive audit of the multi-kb system found 1 critical functional defect, 3 low-severity contract-vs-implementation gaps, 1 CDK warning worth addressing, and 16 incomplete CLI tasks from the original task file. This document organizes all findings into actionable task groups.

### Summary

| Section | Tasks | Severity | Blocked By |
|---------|-------|----------|------------|
| 1. Server Dream Cycle Metadata Bug | 4 | **Critical** | None |
| 2. Server-Config Validation Gaps | 2 | Low | None |
| 3. CDK ASG desiredCapacity Warning | 1 | Low | None |
| 4. CLI Incomplete Tasks: Dream Cycle Tests | 2 | Medium | None |
| 5. CLI Incomplete Tasks: Setup Wizard | 6 | Medium | None |
| 6. CLI Incomplete Tasks: Approval UI Tests | 2 | Medium | None |
| 7. CLI Incomplete Tasks: Prompt Validation | 5 | Medium | Deployed stack (PRM-003) |
| 8. CLI Incomplete Tasks: E2E Scenarios | 1 | Medium | Deployed stack |

---

## Section 1: Server Dream Cycle Metadata Bug

**Priority: Critical**
**Root Cause:** QAT-006 confirmed that Bedrock KB does NOT store YAML frontmatter fields (`uid`, `title`, `status`, `author`) in the `AMAZON_BEDROCK_METADATA` OpenSearch field. That field contains only Bedrock system metadata (`x-amz-bedrock-kb-source-uri`, `x-amz-bedrock-kb-data-source-id`, etc.). The YAML frontmatter IS preserved as raw text in `AMAZON_BEDROCK_TEXT_CHUNK`.

**Impact:** `queryOpenSearchPending()` at `cli/internal/server/dreamcycle.go:147` queries `AMAZON_BEDROCK_METADATA.status == "pending"` which never matches any document. The server dream cycle silently no-ops every run — no pending notes are ever found, so phases 2-4 never execute. `queryOpenSearchRelated()` at line 165 has the same problem filtering on `AMAZON_BEDROCK_METADATA.status == "active"`.

**Note:** The CDK recall Lambda (`cdk/lambda/recall/index.ts:33-76`) was already fixed. It extracts `uid` from the S3 URI and `title`/`status` via regex from `content.text`. The server dream cycle needs the same approach.

### AUD-001: Research — OpenSearch Text-Based Status Queries

**Description:** Determine the best approach for querying notes by status when YAML frontmatter lives in `AMAZON_BEDROCK_TEXT_CHUNK` (a text field), not in a structured metadata field. The chosen approach must work with the existing OpenSearch Serverless VECTORSEARCH collection and its current index schema (`bedrock-kb-index`).
**Files to read:**
- `cli/internal/server/dreamcycle.go` — current broken queries
- `cdk/lambda/recall/index.ts:33-76` — the pattern already used by the recall Lambda
- `specs/01-mvp/cdk/research.md` (R-2, QAT-006 section) — documented metadata findings
- `cli/internal/dreamcycle/phase1.go` — local-mode approach (scans filesystem)
**Questions to resolve:**
- [ ] Can `AMAZON_BEDROCK_TEXT_CHUNK` be queried via OpenSearch `match_phrase` or `query_string` for `status: pending` embedded in frontmatter? Test viability: false positives when "status: pending" appears in note body content.
- [ ] Is the `AMAZON_BEDROCK_METADATA` field indexed as `text` (index: false in current schema) — if so, text queries against it are impossible and the only option is querying `AMAZON_BEDROCK_TEXT_CHUNK` or scanning the local repo.
- [ ] Evaluate the filesystem-scan approach: since the server has the full CodeCommit repo cloned at `/opt/multi-kb/repo`, scanning `.md` files and parsing frontmatter (reusing `dreamcycle.ParseNote`) is simple, reliable, and already proven in local mode. What are the tradeoffs vs. OpenSearch queries?
- [ ] For phase 2 (related notes): if pending/active filtering moves to local scan, should related-note retrieval use OpenSearch `more_like_this` on `AMAZON_BEDROCK_TEXT_CHUNK` (current approach minus the broken status filter), or use Bedrock KB Retrieve API (like the recall Lambda does)?

**Decision criteria:** Reliability > query performance > code simplicity. The server processes at most hundreds of notes; query latency is not critical.

### AUD-002: Fix `queryOpenSearchPending` — Use Local Repo Scan

**Description:** Replace the broken OpenSearch metadata query in `queryOpenSearchPending()` with a local filesystem scan of the CodeCommit repo, reusing the existing `dreamcycle.ParseNote` and frontmatter parsing.
**Files:**
- `cli/internal/server/dreamcycle.go` — rewrite `queryOpenSearchPending()` (lines 147-163)
- `cli/internal/server/dreamcycle_test.go` — replace `TestParseOpenSearchNotes_*` tests with filesystem-based tests
**Dependencies:** AUD-001 (research decision)
**Acceptance Criteria:**
- [ ] `queryOpenSearchPending()` scans all `.md` files in `repoDir`, parses frontmatter, returns notes where `status == "pending"`
- [ ] Reuses `dreamcycle.ParseNote()` from `cli/internal/dreamcycle/phase1.go` for frontmatter parsing (no duplicate parsing logic)
- [ ] Skips non-`.md` files and directories
- [ ] Returns `([]dreamcycle.Note, error)` with same signature as before (no caller changes needed)
- [ ] Handles empty repo (no files) gracefully — returns empty slice, nil error
- [ ] Test: directory with 3 files (1 pending, 1 active, 1 no-frontmatter) returns only the pending note
- [ ] Test: empty directory returns empty slice
- [ ] Test: file with malformed frontmatter is skipped without error

### AUD-003: Fix `queryOpenSearchRelated` — Remove Broken Status Filter

**Description:** Fix `queryOpenSearchRelated()` so the `more_like_this` query works without relying on the non-functional `AMAZON_BEDROCK_METADATA.status` filter. Apply post-query client-side filtering to exclude pending notes instead.
**Files:**
- `cli/internal/server/dreamcycle.go` — rewrite `queryOpenSearchRelated()` (lines 165-196)
- `cli/internal/server/dreamcycle.go` — update `parseOpenSearchNotes()` (lines 270-314) to extract uid/title/status from `AMAZON_BEDROCK_TEXT_CHUNK` content text instead of from `AMAZON_BEDROCK_METADATA` map
- `cli/internal/server/dreamcycle_test.go` — update tests
**Dependencies:** AUD-001 (research decision), AUD-002
**Acceptance Criteria:**
- [ ] `queryOpenSearchRelated()` sends `more_like_this` query on `AMAZON_BEDROCK_TEXT_CHUNK` WITHOUT a `term` filter on `AMAZON_BEDROCK_METADATA.status`
- [ ] `parseOpenSearchNotes()` extracts `uid` from the S3 source URI in `AMAZON_BEDROCK_METADATA["x-amz-bedrock-kb-source-uri"]` (strip path, remove `.md` suffix) — matches the pattern in `cdk/lambda/recall/index.ts:33-35`
- [ ] `parseOpenSearchNotes()` extracts `title` and `status` by regex-parsing `AMAZON_BEDROCK_TEXT_CHUNK` content text (frontmatter lines) — matches the pattern in `cdk/lambda/recall/index.ts:38-41`
- [ ] Post-query: filters out notes where parsed `status != "active"` (excludes pending notes from related results)
- [ ] Post-query: filters out the batch's own pending note UID from results
- [ ] Test: `parseOpenSearchNotes` with realistic OpenSearch response containing `x-amz-bedrock-kb-source-uri` and frontmatter in text chunk
- [ ] Test: notes with `status: pending` in content text are filtered out of related results

### AUD-004: Update Server Dream Cycle Tests

**Description:** Replace the test fixtures in `dreamcycle_test.go` that fabricate `AMAZON_BEDROCK_METADATA` maps with uid/title/status fields. The current tests pass but validate against a data shape that Bedrock never produces.
**Files:**
- `cli/internal/server/dreamcycle_test.go` — rewrite `TestParseOpenSearchNotes_*` tests
**Dependencies:** AUD-002, AUD-003
**Acceptance Criteria:**
- [ ] `TestParseOpenSearchNotes_ValidResults`: uses a realistic OpenSearch response where `AMAZON_BEDROCK_METADATA` contains only `x-amz-bedrock-kb-source-uri` and `AMAZON_BEDROCK_TEXT_CHUNK` contains the full Markdown with YAML frontmatter
- [ ] `TestParseOpenSearchNotes_EmptyResults`: unchanged (still valid)
- [ ] `TestParseOpenSearchNotes_MissingUID`: updated — UID extraction now fails when S3 URI is missing or malformed, not when metadata map lacks `uid` key
- [ ] New test: `TestQueryPendingNotes_LocalScan` — creates temp dir with mixed-status note files, verifies only pending notes returned
- [ ] New test: `TestQueryPendingNotes_EmptyDir` — empty temp dir returns zero notes
- [ ] All `go test -race ./internal/server/...` pass

---

## Section 2: Server-Config Validation Gaps

**Priority: Low**
**Root Cause:** The server-config contract (`specs/01-mvp/cdk/contracts/server-config.md`) marks several fields as required, but `cli/internal/config/validate.go:validateServerMode()` only validates a subset. In practice, CDK always provides all fields, so this only affects hand-crafted server configs.

### AUD-005: Add Missing Server-Mode Validation Rules

**Description:** Add validation for required fields that the contract specifies but `validateServerMode()` currently skips: `dream_cycle.model_id`, `sqs.batch_size`, `codecommit.region`, `s3.region`, and `opensearch.region`.
**Files:**
- `cli/internal/config/validate.go` — extend `validateServerMode()` (lines 105-148)
- `cli/internal/config/config_test.go` — add test cases to `TestLoad_ServerModeMissingRequiredFields`
**Dependencies:** None
**Acceptance Criteria:**
- [ ] `dream_cycle.model_id` validated as non-empty when `mode == "server"`
- [ ] `sqs.batch_size` validated as a positive integer (1-10) when `mode == "server"`
- [ ] `codecommit.region` validated as non-empty when `mode == "server"`
- [ ] `s3.region` validated as non-empty when `mode == "server"`
- [ ] `opensearch.region` validated as non-empty when `mode == "server"`
- [ ] Test: config missing each field individually produces the expected error message
- [ ] Test: valid config with all fields still passes
- [ ] Existing tests unaffected (the existing `minimalValidServerConfig()` helper already includes all fields)

### AUD-006: Validate `sqs.batch_size` Range in Server Mode

**Description:** `sqs.batch_size` defaults to Go's zero value (0) when omitted from YAML, which would cause `ReceiveMessage` to request 0 messages. Add a range check and a sensible default.
**Files:**
- `cli/internal/config/validate.go` — add batch_size range check
- `cli/internal/config/config.go` — add default for `batch_size` in `applyDefaults()` if needed
- `cli/internal/config/config_test.go` — test batch_size edge cases
**Dependencies:** AUD-005
**Acceptance Criteria:**
- [ ] `batch_size` of 0 or negative in server mode produces a validation error OR is defaulted to 10
- [ ] `batch_size` > 10 produces a validation error (SQS max is 10)
- [ ] Test: `batch_size: 0` triggers error or gets defaulted
- [ ] Test: `batch_size: 11` triggers error

---

## Section 3: CDK ASG desiredCapacity Warning

**Priority: Low**
**Root Cause:** `cdk/lib/constructs/compute.ts:346` sets `desiredCapacity: 1` on the ASG. CDK warns that this resets the ASG's desired count on every deployment, overriding any manual scaling adjustments. Since `minCapacity` and `maxCapacity` are both 1, the `desiredCapacity` is redundant.

### AUD-007: Remove Redundant `desiredCapacity` from ASG

**Description:** Remove the explicit `desiredCapacity: 1` from the ASG construct. With `minCapacity: 1` and `maxCapacity: 1`, the desired count is implicitly 1 and CloudFormation won't reset it on redeployment.
**Files:**
- `cdk/lib/constructs/compute.ts` — remove line 346
- `cdk/test/compute.test.ts` — update if any assertion checks desiredCapacity
- `cdk/test/__snapshots__/multi-kb-stack.test.ts.snap` — snapshot will change (needs `npx jest -u`)
**Dependencies:** None
**Acceptance Criteria:**
- [ ] `desiredCapacity` property removed from ASG construct
- [ ] `cdk synth` no longer emits the `@aws-cdk/aws-autoscaling:desiredCapacitySet` warning
- [ ] All CDK tests pass (update snapshot if needed)

---

## Section 4: CLI Incomplete Tasks — Dream Cycle Tests

**Priority: Medium**
**Context:** DRM-001 (dream cycle orchestrator) and DRM-005 (dream cycle commands) are functionally implemented but missing test coverage for their final acceptance criteria.

### AUD-008: DRM-001 Test Coverage — Dream Cycle Orchestrator

**Description:** Add unit tests for `dreamcycle.RunDreamCycle()` in `cli/internal/dreamcycle/cycle.go`. The function is implemented but the task's final acceptance criterion (test cases) is not met.
**Files:**
- `cli/internal/dreamcycle/dreamcycle_test.go` — add tests
**Dependencies:** None
**Acceptance Criteria:**
- [ ] Test: successful full cycle with mocked Bedrock client and mocked NoteStore — phases 1-3 execute, run log is written
- [ ] Test: failure mid-cycle (e.g., phase 2 git grep error) — cycle continues for remaining batches, error count incremented in run log
- [ ] Test: lock acquisition failure — returns error immediately without executing any phases
- [ ] All tests use `t.TempDir()` for isolation

### AUD-009: DRM-005 Test Coverage — Dream Cycle Commands

**Description:** Add tests for the `multi-kb dream-cycle` and `multi-kb run` command wiring in `cli/internal/cmd/dreamcycle.go` and `cli/internal/cmd/run.go`.
**Files:**
- `cli/internal/cmd/dreamcycle.go`
- `cli/internal/cmd/run.go`
- New or existing test file in `cli/internal/cmd/`
**Dependencies:** AUD-008
**Acceptance Criteria:**
- [ ] Test: standalone `dream-cycle` command invocation with a valid config (mocked deps) succeeds
- [ ] Test: `run` command with `--dream-cycle` flag executes both process and dream cycle
- [ ] Test: lock contention — dream cycle returns error when lock is already held

---

## Section 5: CLI Incomplete Tasks — Setup Wizard

**Priority: Medium**
**Context:** All 6 wizard tasks (WIZ-001 through WIZ-006) have their implementation complete but are missing their final test coverage acceptance criterion. These are interactive terminal flows, so tests must mock terminal I/O.

### AUD-010: WIZ-001 Tests — Harness Selection and Source Discovery

**Files:** `cli/internal/cmd/setup.go` or relevant wizard package
**Acceptance Criteria:**
- [ ] Test: single harness selection (claude-code only) — config contains only claude-code harness
- [ ] Test: both harnesses selected — config contains both
- [ ] Test: directory validation — rejects non-existent directory, accepts existing directory
- [ ] Test: source discovery — finds Claude Code projects in given directory
- [ ] Test: conditional group hiding — Notor questions hidden when only claude-code selected

### AUD-011: WIZ-002 Tests — KB Configuration and Routing

**Files:** `cli/internal/cmd/setup.go` or relevant wizard package
**Acceptance Criteria:**
- [ ] Test: minimal setup (local-only) — no remote KB configured, config has only local KB targets
- [ ] Test: with remote KB — config includes knowledge_bases entry with API endpoint and auth
- [ ] Test: with overrides — directory-specific routing overrides written to config
- [ ] Test: with exclusion rules — regex exclusion patterns written correctly
- [ ] Test: accessible mode — wizard works without color/cursor control codes

### AUD-012: WIZ-003 Tests — Hook Auto-Registration

**Files:** `cli/internal/cmd/setup.go`, `cli/internal/hook/`
**Acceptance Criteria:**
- [ ] Test: single harness hook registration — only that harness's hook file is created/modified
- [ ] Test: both harnesses — both hook files created/modified
- [ ] Test: pre-existing hooks preserved — existing user hooks in the file are not removed or overwritten

### AUD-013: WIZ-004 Tests — Cron Registration

**Files:** `cli/internal/schedule/`
**Acceptance Criteria:**
- [ ] Test: register fresh — crontab entry added when none exists
- [ ] Test: idempotent re-register — running setup twice doesn't duplicate crontab entries
- [ ] Test: unregister — `multi-kb` crontab entry removed, other entries preserved
- [ ] Test: existing crontab preserved — non-multi-kb crontab entries untouched
- [ ] Test: empty crontab edge case — works when user has no existing crontab

### AUD-014: WIZ-005 Tests — Cron Expression Parsing

**Files:** `cli/internal/schedule/`
**Acceptance Criteria:**
- [ ] Test: common intervals — parse "every 30 minutes", "hourly", "daily" cron expressions
- [ ] Test: next occurrence calculation — given current time, compute correct next run
- [ ] Test: missing entry — returns appropriate zero value or error when no multi-kb crontab entry exists
- [ ] Test: Windows CSV parsing — `schtasks /Query /FO CSV` output parsed correctly (if platform-supported)

### AUD-015: WIZ-006 Tests — Standalone Subcommands (add-source, add-kb)

**Files:** `cli/internal/cmd/addsource.go`, `cli/internal/cmd/addkb.go`
**Acceptance Criteria:**
- [ ] Test: add source to existing config — new source appended, existing sources preserved
- [ ] Test: add KB to existing config — new knowledge_bases entry appended
- [ ] Test: validation failures — missing required fields produce clear error messages

---

## Section 6: CLI Incomplete Tasks — Approval UI Tests

**Priority: Medium**
**Context:** The approval web UI server and command wiring are implemented but missing test coverage.

### AUD-016: APR-002 Tests — HTTP Server Lifecycle

**Files:** `cli/internal/approve/server.go`, `cli/internal/approve/handlers_test.go`
**Acceptance Criteria:**
- [ ] Test: server starts on a random available port and reports the URL
- [ ] Test: idle timeout fires — server shuts down after configured idle period with no activity
- [ ] Test: all-resolved shutdown — server shuts down when all pending notes have been approved or rejected
- [ ] Test: manual shutdown — server responds to interrupt/context cancellation

### AUD-017: APR-004 Tests — Approve Command Wiring

**Files:** `cli/internal/cmd/approve.go`
**Acceptance Criteria:**
- [ ] Test: launch with pending notes — server starts, browser open attempted
- [ ] Test: launch with no pending notes — prints message and exits without starting server

---

## Section 7: CLI Incomplete Tasks — Prompt Validation

**Priority: Medium**
**Context:** All 5 prompt authoring tasks (PRM-001 through PRM-005) have their prompts written and embedded in the codebase. The final acceptance criterion for each is validation testing against sample inputs. These are qualitative tests — they verify that the LLM produces reasonable output for representative inputs, not that the code compiles.

### AUD-018: PRM-001 Validation — Extraction System Prompt

**Files:** `cli/internal/extract/prompts/extraction.go`
**Acceptance Criteria:**
- [ ] Tested against sample conversation with clear extractable knowledge — LLM produces well-formed extraction output
- [ ] Tested against sample conversation with no extractable knowledge — LLM returns empty/no-knowledge response
- [ ] Tested against re-processed conversation with mixed flags — LLM respects `previously_extracted` markers

### AUD-019: PRM-002 Validation — Dream Cycle Consolidation Prompt

**Files:** `cli/internal/dreamcycle/prompts/consolidation.go`
**Acceptance Criteria:**
- [ ] Tested against sample batch with a novel note — LLM returns `keep` action
- [ ] Tested against sample batch with a duplicate note — LLM returns `merge` action
- [ ] Tested against sample batch with overlapping notes — LLM returns `consolidate` action

### AUD-020: PRM-003 Validation — Coverage Assessment Prompt (CDK)

**Files:** `cdk/lambda/recall/prompts/coverage.ts`
**Dependencies:** Requires deployed Bedrock model access (can test locally with Bedrock InvokeModel API)
**Acceptance Criteria:**
- [ ] Tested against scenario with good coverage — LLM returns `gap_detected: false`
- [ ] Tested against scenario with missing topic — LLM returns `gap_detected: true` with a relevant `refined_query`
- [ ] Tested against ambiguous results — LLM makes a reasonable gap/no-gap decision

### AUD-021: PRM-004 Validation — Keyword Derivation Prompt

**Files:** `cli/internal/recall/prompts/keywords.go`
**Acceptance Criteria:**
- [ ] Tested against a technical question first message — LLM produces relevant search keywords
- [ ] Tested against a broad request — LLM produces reasonable broader keywords
- [ ] Tested against a short ambiguous query — LLM produces best-effort keywords without hallucinating intent

### AUD-022: PRM-005 Validation — Chunk Summarization Prompt

**Files:** `cli/internal/extract/prompts/summarize_chunk.go`
**Acceptance Criteria:**
- [ ] Tested against a long conversation chunk — LLM produces a coherent summary that preserves key technical details and context

---

## Section 8: CLI Incomplete Tasks — E2E Scenario Validation

**Priority: Medium**
**Context:** QAT-003 requires executing 9 end-to-end scenarios against a deployed stack. Zero criteria are currently met. This is a manual validation checklist — each scenario exercises a full user workflow across CLI + CDK infrastructure.

### AUD-023: QAT-003 End-to-End Scenario Validation

**Description:** Execute all 9 end-to-end scenarios from `cli/test/e2e/scenarios.md` against a deployed stack. Each scenario should be run manually and results documented.
**Dependencies:** Deployed CDK stack with CLI binary on EC2
**Files:** `cli/test/e2e/scenarios.md` — checklist file to mark complete
**Acceptance Criteria:**
- [ ] **First-Time Setup:** Binary download -> setup wizard -> config written -> hooks registered -> cron registered (under 10 minutes)
- [ ] **Scheduled Capture:** Cron fires -> conversations scanned -> knowledge extracted -> notes routed -> run log written
- [ ] **Hook Injection:** New conversation -> hook fires -> recall queries -> Markdown injected -> conversation proceeds
- [ ] **Oversized Conversation:** >700K token conversation -> chunked -> all knowledge extracted
- [ ] **Extraction Failure:** Bedrock throttle -> retry -> partial acceptance -> error logged
- [ ] **Hook Timeout:** Slow KB -> timeout -> partial results used
- [ ] **Re-Processing:** Modified old conversation -> re-translated -> new knowledge extracted
- [ ] **Approval Flow:** Pending notes -> `multi-kb approve` -> review -> approve/reject -> submitted/deleted
- [ ] **Dream Cycle:** Pending notes -> singleton batches -> related lookup -> consolidation -> active notes

---

## Execution Order

The recommended execution order respects dependencies and prioritizes by severity:

```
Phase 1 (Critical fix):    AUD-001 -> AUD-002 -> AUD-003 -> AUD-004
Phase 2 (Low, parallel):   AUD-005 + AUD-006 + AUD-007 (all independent)
Phase 3 (Medium, parallel): AUD-008 + AUD-009 + AUD-010..015 + AUD-016 + AUD-017 (all independent)
Phase 4 (Medium, parallel): AUD-018..022 (prompt validation, needs Bedrock access)
Phase 5 (Deployed stack):  AUD-023 (E2E scenarios)
```

Phases 2 and 3 can run in parallel with each other. Phase 4 requires Bedrock model access but not a deployed stack. Phase 5 requires a fully deployed stack.
