# Audit Tasks: Post-Wave 8 Defects, Gaps, and Remaining Work

**Created:** 2026-05-03
**Source:** Full-system audit of implementation-order.md, both task files, contracts, research findings, and codebase
**Status:** Open

## Overview

A comprehensive audit of the multi-kb system found 1 critical functional defect, 3 low-severity contract-vs-implementation gaps, 1 CDK warning worth addressing, and 16 incomplete CLI tasks from the original task file. This document organizes all findings into actionable task groups. The critical defect (Section 1) is resolved by adopting Bedrock KB's native metadata sidecar files rather than working around missing metadata with regex parsing or filesystem scans.

### Summary

| Section | Tasks | Severity | Blocked By |
|---------|-------|----------|------------|
| 1. Bedrock Metadata Sidecar + Dream Cycle Fix | 4 | **Critical** | None |
| 2. Server-Config Validation Gaps | 2 | Low | None |
| 3. CDK ASG desiredCapacity Warning | 1 | Low | None |
| 4. CLI Incomplete Tasks: Dream Cycle Tests | 2 | Medium | None |
| 5. CLI Incomplete Tasks: Setup Wizard | 6 | Medium | None |
| 6. CLI Incomplete Tasks: Approval UI Tests | 2 | Medium | None |
| 7. CLI Incomplete Tasks: Prompt Validation | 5 | Medium | Deployed stack (PRM-003) |
| 8. CLI Incomplete Tasks: E2E Scenarios | 1 | Medium | Deployed stack |

---

## Section 1: Bedrock Metadata Sidecar + Dream Cycle Fix

**Priority: Critical**
**Root Cause:** QAT-006 confirmed that Bedrock KB does NOT store YAML frontmatter fields (`uid`, `title`, `status`, `author`) in the `AMAZON_BEDROCK_METADATA` OpenSearch field. That field contains only Bedrock system metadata (`x-amz-bedrock-kb-source-uri`, `x-amz-bedrock-kb-data-source-id`, etc.). Additionally, the OpenSearch index maps `AMAZON_BEDROCK_METADATA` with `index: false` (`cdk/lambda/custom-resource/create-index.ts:33`), making the field completely unsearchable even if it contained custom fields.

**Impact:** `queryOpenSearchPending()` at `cli/internal/server/dreamcycle.go:147` queries `AMAZON_BEDROCK_METADATA.status == "pending"` which never matches any document. The server dream cycle silently no-ops every run — no pending notes are ever found, so phases 2-4 never execute. `queryOpenSearchRelated()` at line 165 has the same problem filtering on `AMAZON_BEDROCK_METADATA.status == "active"`.

**Solution Direction: Bedrock KB Metadata Sidecar Files**
Rather than working around the missing metadata with regex parsing of `AMAZON_BEDROCK_TEXT_CHUNK` or local filesystem scans, the proper fix is to use Bedrock KB's native metadata sidecar feature. For each note `{uid}.md` uploaded to S3, a companion `{uid}.md.metadata.json` file provides structured attributes that Bedrock indexes into `AMAZON_BEDROCK_METADATA` during data source ingestion. This enables native metadata filtering in both direct OpenSearch queries and the Bedrock Retrieve API `filter` parameter.

The recall Lambda (`cdk/lambda/recall/index.ts:33-76`) currently works around the missing metadata by extracting `uid` from the S3 URI and `title`/`status` via regex from `content.text`. Once sidecars are in place, this workaround can be removed in favor of reading metadata fields directly and using the Retrieve API's native `filter` parameter to exclude pending notes server-side.

### AUD-001: Research — Bedrock KB Metadata Sidecar Approach

**Description:** Validate the Bedrock KB metadata sidecar file approach and determine the exact implementation requirements for this codebase.
**Files to read:**
- `cli/internal/server/s3sync.go` — current S3 upload logic (`s3Upload` at line 92, `s3Delete` at line 114)
- `cdk/lambda/custom-resource/create-index.ts` — current OpenSearch index schema (`AMAZON_BEDROCK_METADATA` mapped as `text`, `index: false` at line 33)
- `cdk/lib/constructs/knowledge-base.ts` — data source and field mapping configuration (lines 86-129)
- `cdk/lambda/recall/index.ts:33-76` — current regex workaround to be replaced
- `cli/internal/server/dreamcycle.go` — broken metadata queries (lines 147-196)
- `specs/01-mvp/cdk/research.md` (R-2, QAT-006 section) — documented metadata findings
**Questions to resolve:**
- [ ] Confirm the `.metadata.json` sidecar file format: is it `{"metadataAttributes": {"key": {"value": "...", "type": "STRING"}}}` or a flat `{"key": "value"}` structure? (AWS documentation has shown both in different contexts — confirm which Bedrock KB S3 data sources expect)
- [ ] Which frontmatter fields to include as metadata attributes? Minimum: `status` (for filtering) and `uid` (for identity). Candidates for inclusion: `title`, `author`, `last-updated`, `last-recalled`. Evaluate: does including all fields add value for future queries, or does it add unnecessary ingestion overhead?
- [ ] Does the OpenSearch index schema need to change? When Bedrock ingests sidecar metadata, does it populate `AMAZON_BEDROCK_METADATA` as a structured object (requiring the field type to change from `text` to something else), or does Bedrock handle the mapping internally? Does `index: false` need to change to `index: true`?
- [ ] Can the existing `bedrock-kb-index` be updated in-place via the OpenSearch Update Mapping API, or must it be deleted and recreated? (OpenSearch Serverless VECTORSEARCH collections may have restrictions on mapping changes)
- [ ] Does the Bedrock Retrieve API `filter` parameter work with sidecar-provided attributes? e.g., `filter: { equals: { key: "status", value: "active" } }` — confirm this works for the recall Lambda's use case
- [ ] Is there a size or count limit on metadata attributes per document?
- [ ] Are there any IAM permission changes needed? The Bedrock KB service role currently has `s3:GetObject` and `s3:ListBucket` — confirm these are sufficient to read `.metadata.json` files alongside the source documents

**Decision criteria:** Confirm viability before proceeding with AUD-002 through AUD-004. If the sidecar approach has a blocking limitation (e.g., VECTORSEARCH collections cannot index metadata), fall back to local repo scan for pending queries and regex parsing for related-note queries.

### AUD-002: Generate `.metadata.json` Sidecars in S3 Sync

**Description:** Modify the S3 sync path to write a `.metadata.json` sidecar file alongside each `.md` note uploaded to S3. When a note is deleted, also delete its sidecar. After the next Bedrock data source ingestion job, the metadata attributes will be indexed and queryable.
**Files:**
- `cli/internal/server/s3sync.go` — extend `s3Upload()` (line 92) to also upload `{filename}.metadata.json`; extend `s3Delete()` (line 114) to also delete the sidecar
- `cli/internal/server/s3sync_test.go` — add/update tests
**Dependencies:** AUD-001 (confirm sidecar format)
**Acceptance Criteria:**
- [ ] For each `.md` file upload, `s3Upload()` parses YAML frontmatter (reusing `dreamcycle.ParseNote()` from `cli/internal/dreamcycle/phase1.go`) and uploads `{filename}.metadata.json` with the Bedrock sidecar schema alongside the note
- [ ] Sidecar contains at minimum: `status` (STRING), `uid` (STRING), `title` (STRING), `author` (STRING) — exact schema per AUD-001 research findings
- [ ] `s3Delete()` deletes both `{filename}` and `{filename}.metadata.json`
- [ ] Non-`.md` files are uploaded without sidecars (no frontmatter parse attempt)
- [ ] Malformed or missing frontmatter: sidecar is still uploaded with whatever fields could be extracted; missing fields omitted rather than erroring the upload
- [ ] Existing retry logic (3 attempts, exponential backoff) applies to sidecar uploads and deletes
- [ ] Test: upload a well-formed note — both `.md` and `.md.metadata.json` PutObject calls made with correct S3 keys and content
- [ ] Test: delete a note — both `.md` and `.md.metadata.json` DeleteObject calls made
- [ ] Test: upload a non-`.md` file — no sidecar generated
- [ ] Test: note with missing frontmatter fields — sidecar contains only the fields that were present

### AUD-003: Update CDK OpenSearch Index Schema for Metadata Indexing

**Description:** Update the OpenSearch index schema so that `AMAZON_BEDROCK_METADATA` is indexed and queryable. With sidecar files providing structured metadata during ingestion, this field must be searchable to support term queries on `status`, `uid`, etc.
**Files:**
- `cdk/lambda/custom-resource/create-index.ts` — change `AMAZON_BEDROCK_METADATA` mapping (line 33, currently `type: "text"`, `index: false`)
- `cdk/test/__snapshots__/multi-kb-stack.test.ts.snap` — snapshot update
**Dependencies:** AUD-001 (confirm required schema changes — field type and index settings)
**Acceptance Criteria:**
- [ ] `AMAZON_BEDROCK_METADATA` field mapping updated to support structured metadata queries (type and index settings per AUD-001 research findings)
- [ ] Custom resource handler logic accounts for index migration: if an existing index with the old schema exists, handle recreation (delete + create) or document that manual intervention is needed
- [ ] `cdk synth` succeeds without new warnings
- [ ] CDK tests pass (update snapshot with `npx jest -u`)
- [ ] Document any manual migration steps required for existing deployments (e.g., "must delete and recreate index, which triggers full re-ingestion")

### AUD-004: Fix Server Dream Cycle and Recall Lambda — Use Native Metadata Filtering

**Description:** With sidecar metadata now indexed, update the server dream cycle queries to use proper structured metadata fields, and simplify the recall Lambda by removing the regex workaround in favor of native Bedrock metadata filtering.
**Files:**
- `cli/internal/server/dreamcycle.go` — fix `queryOpenSearchPending()` (line 147) and `queryOpenSearchRelated()` (line 165) to query the now-populated `AMAZON_BEDROCK_METADATA.status` field
- `cli/internal/server/dreamcycle.go` — simplify `parseOpenSearchNotes()` (line 270) to read `uid`, `title`, `status` directly from the `AMAZON_BEDROCK_METADATA` map instead of regex-parsing `AMAZON_BEDROCK_TEXT_CHUNK`
- `cdk/lambda/recall/index.ts` — replace regex workaround (lines 33-76): use Bedrock Retrieve API `filter` parameter for status filtering, read `uid`/`title` from `metadata` response fields instead of regex-parsing content text
- `cli/internal/server/dreamcycle_test.go` — rewrite tests with realistic sidecar-populated metadata fixtures
- `cdk/lambda/recall/index.test.ts` — update recall Lambda tests if they exist
**Dependencies:** AUD-002 (sidecars generated), AUD-003 (metadata indexed)
**Acceptance Criteria:**
- [ ] `queryOpenSearchPending()` queries `AMAZON_BEDROCK_METADATA.status == "pending"` — now succeeds because Bedrock populates this field from sidecars during ingestion
- [ ] `queryOpenSearchRelated()` uses `AMAZON_BEDROCK_METADATA.status` term filter for `"active"` notes — no more client-side post-filtering needed
- [ ] `parseOpenSearchNotes()` reads `uid`, `title`, `status` directly from `AMAZON_BEDROCK_METADATA` map — no regex parsing of `AMAZON_BEDROCK_TEXT_CHUNK` content
- [ ] Recall Lambda uses Bedrock Retrieve API `filter` parameter (e.g., `{ equals: { key: "status", value: "active" } }`) to exclude pending notes server-side
- [ ] Recall Lambda reads `uid`, `title` from `metadata` response fields instead of regex-parsing content text
- [ ] `extractFrontmatterField()` and `extractUidFromS3Uri()` helper functions removed from recall Lambda (no longer needed)
- [ ] Test: `parseOpenSearchNotes` with realistic OpenSearch response containing sidecar-populated metadata fields
- [ ] Test: `queryOpenSearchPending` returns only notes with `status: "pending"` in metadata
- [ ] Test: `queryOpenSearchRelated` excludes pending notes via metadata term filter
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
Phase 1 (Critical fix):    AUD-001 -> AUD-002 + AUD-003 (parallel) -> AUD-004
  AUD-001: Research sidecar format, schema requirements
  AUD-002: S3 sync generates .metadata.json sidecars (CLI change)
  AUD-003: OpenSearch index schema updated for metadata indexing (CDK change)
  AUD-004: Server dream cycle + recall Lambda use native metadata (depends on AUD-002 + AUD-003)
Phase 2 (Low, parallel):   AUD-005 + AUD-006 + AUD-007 (all independent)
Phase 3 (Medium, parallel): AUD-008 + AUD-009 + AUD-010..015 + AUD-016 + AUD-017 (all independent)
Phase 4 (Medium, parallel): AUD-018..022 (prompt validation, needs Bedrock access)
Phase 5 (Deployed stack):  AUD-023 (E2E scenarios)
```

Phase 1 note: After AUD-001 research, AUD-002 (CLI/S3 sync) and AUD-003 (CDK/OpenSearch schema) are independent and can proceed in parallel. AUD-004 (query fixes) depends on both being complete and a data source re-ingestion having run with sidecars present. Existing deployments will need an index recreation and full re-ingestion to populate metadata for existing notes.

Phases 2 and 3 can run in parallel with each other and with Phase 1. Phase 4 requires Bedrock model access but not a deployed stack. Phase 5 requires a fully deployed stack with the Phase 1 sidecar changes deployed.
