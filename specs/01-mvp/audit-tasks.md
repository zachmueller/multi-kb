# Audit Tasks: Post-Wave 8 Defects, Gaps, and Remaining Work

**Created:** 2026-05-03
**Source:** Full-system audit of implementation-order.md, both task files, contracts, research findings, and codebase
**Status:** Open

## Overview

A comprehensive audit of the multi-kb system found 1 critical functional defect, 3 low-severity contract-vs-implementation gaps, 1 CDK warning worth addressing, and 16 incomplete CLI tasks from the original task file. This document organizes all findings into actionable task groups. The critical defect (Section 1) is resolved by adopting Bedrock KB's native metadata sidecar files and migrating all metadata-based queries to the Bedrock Retrieve API's `filter` parameter — eliminating direct OpenSearch queries, regex frontmatter parsing, and client-side filtering.

### Summary

| Section | Tasks | Severity | Blocked By |
|---------|-------|----------|------------|
| 1. Bedrock KB Metadata Sidecar + Retrieve API Migration | 3 (+1 no-op) | **Critical** | None (AUD-002 depends on AUD-001; AUD-003 independent) |
| 2. Server-Config Validation Gaps | 2 | Low | None |
| 3. CDK ASG desiredCapacity Warning | 1 | Low | None |
| 4. CLI Incomplete Tasks: Dream Cycle Tests | 2 | Medium | None |
| 5. CLI Incomplete Tasks: Setup Wizard | 6 | Medium | None |
| 6. CLI Incomplete Tasks: Approval UI Tests | 2 | Medium | None |
| 7. CLI Incomplete Tasks: Prompt Validation | 5 | Medium | Deployed stack (PRM-003) |
| 8. CLI Incomplete Tasks: E2E Scenarios | 1 | Medium | Deployed stack |

---

## Section 1: Bedrock KB Metadata Sidecar + Retrieve API Migration

**Priority: Critical**

### Background

QAT-006 confirmed that Bedrock KB does NOT store YAML frontmatter fields (`uid`, `title`, `status`, `author`) in the `AMAZON_BEDROCK_METADATA` OpenSearch field. That field contains only Bedrock system metadata (`x-amz-bedrock-kb-source-uri`, `x-amz-bedrock-kb-data-source-id`, etc.).

**Impact — two broken subsystems:**
1. **Server dream cycle** (`cli/internal/server/dreamcycle.go`): `queryOpenSearchPending()` at line 147 sends a direct OpenSearch term query on `AMAZON_BEDROCK_METADATA.status == "pending"` — always returns 0 results. The dream cycle silently no-ops every run. `queryOpenSearchRelated()` at line 165 has the same problem filtering on `status == "active"`.
2. **Recall Lambda** (`cdk/lambda/recall/index.ts`): `extractFrontmatterField()` regex-parses `title`/`status` from `content.text` and filters client-side — fragile and wasteful (uses up `numberOfResults` slots on documents that get discarded).

### Solution: Bedrock KB Metadata Sidecar Files + Retrieve API Native Filtering

Bedrock KB's S3 data source connector supports **metadata sidecar files**. For each source document `{name}.md` in S3, a companion `{name}.md.metadata.json` provides structured attributes. During data source ingestion, Bedrock indexes these attributes and makes them filterable via the **Retrieve API `filter` parameter**.

**Sidecar file format** (AWS documented structured format):
```json
{
  "metadataAttributes": {
    "status": {
      "value": { "type": "STRING", "stringValue": "pending" }
    },
    "uid": {
      "value": { "type": "STRING", "stringValue": "ABC123" }
    },
    "title": {
      "value": { "type": "STRING", "stringValue": "My Note Title" }
    },
    "author": {
      "value": { "type": "STRING", "stringValue": "user@example.com" }
    }
  }
}
```

**Retrieve API filter syntax:**
```json
{
  "retrievalConfiguration": {
    "vectorSearchConfiguration": {
      "numberOfResults": 10,
      "filter": {
        "equals": { "key": "status", "value": "active" }
      }
    }
  }
}
```

**Key design facts:**
- The Retrieve API handles metadata filtering at the application layer — it works regardless of the OpenSearch `AMAZON_BEDROCK_METADATA` index setting (`index: false` is fine).
- No OpenSearch schema changes are required.
- No additional IAM permissions needed — existing `s3:GetObject` on `bucket/*` covers sidecar files.
- Max sidecar file size: 10 KB (our sidecars are well under 1 KB).
- All metadata-based queries go through the Retrieve API. No direct OpenSearch queries needed.
- The Retrieve API uses embedding-based vector search — strictly better semantic matching than the current OpenSearch `more_like_this` term-frequency approach.

**What this replaces:**
- Direct OpenSearch queries with SigV4-signed HTTP calls → Bedrock Retrieve API via AWS SDK
- Regex parsing of frontmatter from content text → metadata fields in Retrieve API response
- Client-side status filtering → server-side pre-filtering via `filter` parameter
- `more_like_this` term-frequency matching → embedding-based semantic similarity

### AUD-001: Generate `.metadata.json` Sidecars in S3 Sync

**Description:** Modify the S3 sync path to generate and upload a `.metadata.json` sidecar file alongside each `.md` note uploaded to S3. When a note is deleted, also delete its sidecar. After the next Bedrock data source ingestion job, the metadata attributes will be indexed and filterable via the Retrieve API.

**Files:**
- `cli/internal/server/s3sync.go` — extend `s3Upload()` (line 92) and `s3Delete()` (line 114); add `generateSidecar()` function and sidecar JSON structs
- `cli/internal/server/s3sync_test.go` — new test file

**Implementation details:**

1. **Add sidecar structs** to `s3sync.go`:
   ```go
   type sidecarFile struct {
       MetadataAttributes map[string]sidecarAttribute `json:"metadataAttributes"`
   }
   type sidecarAttribute struct {
       Value sidecarValue `json:"value"`
   }
   type sidecarValue struct {
       Type        string `json:"type"`
       StringValue string `json:"stringValue"`
   }
   ```

2. **Add `generateSidecar(filename string, content []byte) ([]byte, error)` function:**
   - Extract UID from filename (strip `.md` extension, take base name).
   - Call `dreamcycle.ParseNote(uid, string(content))` (reuse existing exported function from `cli/internal/dreamcycle/phase1.go:87`) to extract frontmatter fields.
   - Build `sidecarFile` with attributes: `status`, `uid`, `title`, `author` — all as `STRING` type.
   - Marshal to JSON and return.
   - New import: `"github.com/zmueller/multi-kb/internal/dreamcycle"`

3. **Modify `s3Upload()`:**
   - After successfully uploading the `.md` file, check `strings.HasSuffix(filename, ".md")`.
   - If `.md`: call `generateSidecar(filename, data)` with the file content already read. Upload sidecar to S3 key `filename + ".metadata.json"` using same 3-attempt retry pattern.
   - If sidecar generation or upload fails: log warning via `slog.Warn`, do NOT fail the primary upload.
   - Non-`.md` files: skip sidecar entirely.

4. **Modify `s3Delete()`:**
   - After deleting the file, if `strings.HasSuffix(filename, ".md")`: also delete `filename + ".metadata.json"` using same 3-attempt retry.
   - Sidecar delete failure: log warning, do not fail.

5. **Modify `syncDiff()` rename handling (line 65-69):**
   - For `R` (rename) status: the current code uploads the new file but does NOT delete the old file. Add explicit `s3Delete(ctx, client, bucket, parts[1])` for the old path — this will clean up both the old `.md` and its sidecar.

**Acceptance Criteria:**
- [x] For each `.md` upload, `s3Upload()` also uploads `{filename}.metadata.json` with correct Bedrock sidecar schema
- [x] Sidecar contains: `status`, `uid`, `title`, `author` — all as STRING type
- [x] `s3Delete()` deletes both `{filename}` and `{filename}.metadata.json` for `.md` files
- [x] Non-`.md` files uploaded without sidecars
- [x] Malformed/missing frontmatter: sidecar still uploaded with whatever fields could be extracted; missing fields get empty string values
- [x] Existing retry logic (3 attempts, exponential backoff) applies to sidecar uploads and deletes
- [x] `syncDiff()` rename handling deletes old file + old sidecar
- [x] Test: upload well-formed note — both `.md` and `.md.metadata.json` PutObject calls made with correct keys and content
- [x] Test: delete a note — both `.md` and `.md.metadata.json` DeleteObject calls made
- [x] Test: upload non-`.md` file — no sidecar generated
- [x] Test: note with missing frontmatter fields — sidecar contains fields with empty string values
- [x] Test: sidecar JSON key is `{filename}.metadata.json` (e.g., `notes/ABC123.md.metadata.json`)

### AUD-002: Replace Server Dream Cycle OpenSearch Queries with Bedrock Retrieve API

**Description:** Replace all direct OpenSearch queries in the server dream cycle with Bedrock Retrieve API calls using native metadata filtering. This eliminates the broken `AMAZON_BEDROCK_METADATA.status` term queries and replaces `more_like_this` with embedding-based semantic search. Delete all dead OpenSearch query code and the manual SigV4 signer.

**Files:**
- `cli/go.mod` — add `bedrockagentruntime` SDK dependency
- `cli/internal/server/dreamcycle.go` — replace query functions, update `RunDreamCycle()`
- `cli/internal/server/sign.go` — delete entire file (no remaining callers)
- `cli/internal/server/dreamcycle_test.go` — replace OpenSearch tests with Retrieve API tests

**Implementation details:**

1. **Add Go SDK dependency:**
   ```
   cd cli && GOPROXY=direct GONOSUMDB='*' go get github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime
   ```

2. **Add new imports to `dreamcycle.go`:**
   ```go
   "github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
   bratypes "github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
   ```

3. **Add `retrieveNotes()` function** (shared by pending and related queries):
   ```go
   func retrieveNotes(ctx context.Context, client *bedrockagentruntime.Client, kbID, queryText string, filter bratypes.RetrievalFilter, limit int32) ([]dreamcycle.Note, error)
   ```
   - Calls `client.Retrieve()` with `KnowledgeBaseId`, `RetrievalQuery.Text`, `NumberOfResults`, and `Filter`.
   - Parses `RetrievalResults`: reads `uid`, `title`, `status`, `author` from `r.Metadata` map, content from `r.Content.Text`.
   - Skips results with empty `uid`.
   - Handles pagination via `NextToken` if results exceed single page.

4. **Replace `queryOpenSearchPending()` → `queryRetrievePending()`:**
   - Filter: `{ equals: { key: "status", value: "pending" } }` — using `bratypes.RetrievalFilterMemberEquals`.
   - Query text: `"*"` (broad query — we want all pending notes, ranking doesn't matter).
   - Limit: 100 (matches current OpenSearch `"size": 100`).

5. **Replace `queryOpenSearchRelated()` → `queryRetrieveRelated()`:**
   - Filter: `{ equals: { key: "status", value: "active" } }`.
   - Query text: `batch.PendingNote.Content` — the pending note's content as the semantic query. This naturally gives embedding-based similarity ranking, replacing the OpenSearch `more_like_this` term-frequency approach.
   - Limit: 10 (matches current OpenSearch `"size": 10`).

6. **Update `RunDreamCycle()`:**
   - Create `bedrockagentruntime.Client` from existing `awsCfg` (already created at line 29).
   - Phase 1: call `queryRetrievePending()` instead of `queryOpenSearchPending()`.
   - Phase 2: call `queryRetrieveRelated()` instead of `queryOpenSearchRelated()`.

7. **Delete dead code:**
   - `queryOpenSearchPending()` (lines 147-163)
   - `queryOpenSearchRelated()` (lines 165-196)
   - `opensearchQuery()` (lines 208-268) — only callers were the two functions above
   - `parseOpenSearchNotes()` (lines 270-314) — replaced by parsing logic in `retrieveNotes()`
   - `cli/internal/server/sign.go` (entire file, ~105 lines) — `signRequest()` only called from `opensearchQuery()`

8. **Remove now-unused imports from `dreamcycle.go`:** `bytes`, `crypto/tls`, `encoding/json` (check), `io`, `net/http`.

**Acceptance Criteria:**
- [x] `bedrockagentruntime` added to `go.mod` as a dependency
- [x] `queryRetrievePending()` calls Retrieve API with `filter: equals status "pending"`, returns up to 100 notes
- [x] `queryRetrieveRelated()` calls Retrieve API with `filter: equals status "active"` and pending note content as query text, returns up to 10 notes
- [x] `retrieveNotes()` parses `uid`, `title`, `status`, `author` from Retrieve API `Metadata` map and `content` from `Content.Text`
- [x] `RunDreamCycle()` Phase 1 and Phase 2 use the new Retrieve API functions
- [x] `queryOpenSearchPending()`, `queryOpenSearchRelated()`, `opensearchQuery()`, `parseOpenSearchNotes()` deleted
- [x] `cli/internal/server/sign.go` deleted entirely
- [x] All unused imports removed
- [x] Test: `parseRetrieveResults` with mock `KnowledgeBaseRetrievalResult` — correct Note output
- [x] Test: empty results → empty slice
- [x] Test: result with missing uid in metadata → skipped
- [x] Test: result with nil/missing metadata → handled gracefully
- [x] Existing tests preserved: `TestGroupIntoBatches`, `TestServerNoteStore_ReadWriteDelete`, `TestNoteFileFilename`, `TestRegionOrDefault`
- [x] `GOPROXY=direct GONOSUMDB='*' go test -race ./internal/server/...` passes

### AUD-003: Recall Lambda — Native Retrieve API Metadata Filtering

**Description:** Update the recall Lambda to use the Bedrock Retrieve API's native `filter` parameter for status filtering and read metadata fields directly from the API response instead of regex-parsing frontmatter from content text.

**Files:**
- `cdk/lambda/recall/index.ts` — add filter, use metadata fields, remove workarounds
- `cdk/test/lambda/recall.test.ts` — update mocks and assertions

**Implementation details:**

1. **Add `filter` to `RetrieveCommand`** in `retrieveFromKb()` (line 50-60):
   ```typescript
   new RetrieveCommand({
     knowledgeBaseId,
     retrievalQuery: { text: query },
     retrievalConfiguration: {
       vectorSearchConfiguration: {
         numberOfResults: limit,
         ...(_excludePending && {
           filter: {
             equals: { key: "status", value: "active" },
           },
         }),
       },
     },
   })
   ```
   The `@aws-sdk/client-bedrock-agent-runtime` (version `^3.700.0`, already in `package.json`) supports the `filter` parameter on `KnowledgeBaseVectorSearchConfiguration`.

2. **Read metadata from response** instead of regex/S3 URI parsing (lines 63-79):
   - Replace `extractUidFromS3Uri(s3Uri)` with `(r.metadata?.["uid"] as string)`, falling back to `extractUidFromS3Uri()` for backward compatibility during migration.
   - Replace `extractFrontmatterField(content, "title")` with `(r.metadata?.["title"] as string) ?? ""`.

3. **Remove client-side status filtering** (lines 73-76):
   ```typescript
   // DELETE THIS BLOCK — filter parameter handles this server-side
   if (_excludePending) {
     const status = extractFrontmatterField(content, "status");
     if (status && status !== "active") continue;
   }
   ```

4. **Delete `extractFrontmatterField()`** (lines 38-41) — no remaining callers.

5. **Keep `extractUidFromS3Uri()`** (lines 33-36) as fallback — can be removed later once all documents have sidecars.

**No CDK infrastructure changes needed:** `cdk/lib/constructs/recall-lambda.ts` already grants `bedrock:Retrieve` scoped to the KB ARN. The `filter` parameter is part of the Retrieve API call, no additional IAM permissions required. The `EXCLUDE_PENDING` env var remains — its meaning changes from "filter client-side" to "include filter in Retrieve API call."

**Acceptance Criteria:**
- [x] `RetrieveCommand` includes `filter: { equals: { key: "status", value: "active" } }` when `EXCLUDE_PENDING=true`
- [x] `RetrieveCommand` has NO filter when `EXCLUDE_PENDING=false`
- [x] `uid` read from `r.metadata?.["uid"]` with fallback to `extractUidFromS3Uri()`
- [x] `title` read from `r.metadata?.["title"]`
- [x] Client-side status filtering loop (lines 73-76) removed
- [x] `extractFrontmatterField()` function deleted
- [x] `extractUidFromS3Uri()` kept as fallback
- [x] `npx jest` passes in `cdk/`
- [x] Test: `makeRetrieveResponse()` updated to include sidecar metadata fields (`uid`, `title`, `status`, `author`) in `metadata` map
- [x] Test: exclude-pending test verifies `filter` parameter on `RetrieveCommand` rather than checking client-side filtering
- [x] Test: `EXCLUDE_PENDING=false` verifies NO filter in `RetrieveCommand`
- [x] Test: uid/title extracted from metadata fields, not regex

### AUD-004: No OpenSearch Index Schema Change Required

**Status: No action needed**

The original AUD-003 proposed changing `AMAZON_BEDROCK_METADATA` from `index: false` to `index: true` in `cdk/lambda/custom-resource/create-index.ts`. This is unnecessary because:
- All metadata-based queries now go through the **Bedrock Retrieve API**, which handles filtering at the application layer regardless of the OpenSearch index setting.
- All direct OpenSearch queries are removed by AUD-002.
- Changing the schema would require index deletion and recreation with full re-ingestion — disruptive with no functional benefit.

The `create-index.ts` schema stays as-is. The `opensearch.endpoint` config field stays required in `cli/internal/config/validate.go` (the CDK custom resource Lambda still needs it for index creation, and OpenSearch Serverless is still the backing vector store).

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
- [x] `dream_cycle.model_id` validated as non-empty when `mode == "server"`
- [x] `sqs.batch_size` validated as a positive integer (1-10) when `mode == "server"`
- [x] `codecommit.region` validated as non-empty when `mode == "server"`
- [x] `s3.region` validated as non-empty when `mode == "server"`
- [x] `opensearch.region` validated as non-empty when `mode == "server"`
- [x] Test: config missing each field individually produces the expected error message
- [x] Test: valid config with all fields still passes
- [x] Existing tests unaffected (the existing `minimalValidServerConfig()` helper already includes all fields)

### AUD-006: Validate `sqs.batch_size` Range in Server Mode

**Description:** `sqs.batch_size` defaults to Go's zero value (0) when omitted from YAML, which would cause `ReceiveMessage` to request 0 messages. Add a range check and a sensible default.
**Files:**
- `cli/internal/config/validate.go` — add batch_size range check
- `cli/internal/config/config.go` — add default for `batch_size` in `applyDefaults()` if needed
- `cli/internal/config/config_test.go` — test batch_size edge cases
**Dependencies:** AUD-005
**Acceptance Criteria:**
- [x] `batch_size` of 0 or negative in server mode produces a validation error OR is defaulted to 10
- [x] `batch_size` > 10 produces a validation error (SQS max is 10)
- [x] Test: `batch_size: 0` triggers error or gets defaulted
- [x] Test: `batch_size: 11` triggers error

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
- [x] `desiredCapacity` property removed from ASG construct
- [x] `cdk synth` no longer emits the `@aws-cdk/aws-autoscaling:desiredCapacitySet` warning
- [x] All CDK tests pass (update snapshot if needed)

---

## Section 4: CLI Incomplete Tasks — Dream Cycle Tests

**Priority: Medium**
**Context:** DRM-001 (dream cycle orchestrator) and DRM-005 (dream cycle commands) are functionally implemented but missing test coverage for their final acceptance criteria.

### AUD-008: DRM-001 Test Coverage — Dream Cycle Orchestrator

**Description:** Add unit tests for `dreamcycle.RunDreamCycle()` in `cli/internal/dreamcycle/cycle.go`. The function is implemented but the task's final acceptance criterion (test cases) is not met.
**Files:**
- `cli/internal/dreamcycle/dreamcycle_test.go` — add tests
- `cli/internal/dreamcycle/cycle.go` — refactored to extract `runDreamCycle()` accepting injectable `llmInvoker` and `storeFactory` for testability
- `cli/internal/dreamcycle/phase3.go` — `ConsolidateBatch` now accepts `llmInvoker` interface instead of `*bedrock.Client`
**Dependencies:** None
**Acceptance Criteria:**
- [x] Test: successful full cycle with mocked Bedrock client and mocked NoteStore — phases 1-3 execute, run log is written
- [x] Test: failure mid-cycle (phase 3 LLM error) — cycle continues for remaining batches, error count incremented in run log
- [x] Test: lock acquisition failure — returns ErrLockHeld immediately without executing any phases
- [x] All tests use `t.TempDir()` for isolation
- [x] Additional tests: no pending notes → zero batches in run log; no local KB sources → clean zero-batch run; multiple batches processed with correct aggregated counts; trigger field recorded correctly in run log

### AUD-009: DRM-005 Test Coverage — Dream Cycle Commands

**Description:** Add tests for the `multi-kb dream-cycle` and `multi-kb run` command wiring in `cli/internal/cmd/dreamcycle.go` and `cli/internal/cmd/run.go`.
**Files:**
- `cli/internal/cmd/dreamcycle.go` — refactored to extract `execDreamCycle(ctx, cfgPath, lockPath, logsDir)` for testability
- `cli/internal/cmd/run.go` — refactored to extract `execRun(ctx, cfgPath, lockPath, logsDir)` for testability
- `cli/internal/cmd/cmd_test.go` — new test file
**Dependencies:** AUD-008
**Acceptance Criteria:**
- [x] Test: `dream-cycle` command with missing config returns error containing "load config"
- [x] Test: `dream-cycle` command with lock held returns nil (prints skip message, does not error)
- [x] Test: `run` command with missing config propagates error from runProcess
- [x] Test: `run` command with dream-cycle lock held returns nil (skip message, no error)
- [x] Test: lock.ErrLockHeld is detectable via errors.Is on a directly acquired lock

---

## Section 5: CLI Incomplete Tasks — Setup Wizard

**Priority: Medium**
**Context:** All 6 wizard tasks (WIZ-001 through WIZ-006) have their implementation complete but are missing their final test coverage acceptance criterion. These are interactive terminal flows, so tests must mock terminal I/O.

### AUD-010: WIZ-001 Tests — Harness Selection and Source Discovery

**Files:** `cli/internal/cmd/setup.go` or relevant wizard package
**Acceptance Criteria:**
- [x] Test: single harness selection (claude-code only) — config contains only claude-code harness (via `runAddSourceFrom` harness=1)
- [x] Test: both harnesses selected — config contains both (via `runAddSourceFrom` harness=3)
- [x] Test: directory validation — rejects non-existent directory, accepts existing directory (`TestValidateDirPath_*`)

### AUD-011: WIZ-002 Tests — KB Configuration and Routing

**Files:** `cli/internal/cmd/setup.go` or relevant wizard package
**Acceptance Criteria:**
- [x] Test: minimal setup (local-only) — no remote KB configured, config has only local KB targets (`TestBuildTargets_NoRemoteKBs`)
- [x] Test: with remote KB — config includes routing entry for remote KB (`TestBuildTargets_AutoPreset`, `TestBuildTargets_ManualPreset`, `TestBuildTargets_MixedPreset`, `TestBuildTargets_MultipleKBs`)
- [x] Test: with overrides — directory-specific routing overrides written to config (`TestRunAddSource_PreservesExistingOverrides`, `TestOverrides_YAMLRoundtrip`)
- [x] Test: with exclusion rules — regex exclusion patterns written correctly (`TestParseExclusionLines_*`)
- [x] Test: accessible mode — wizard uses `WithAccessible(os.Getenv("ACCESSIBLE") != "")` throughout; verified by code inspection (no unit test: requires TTY)

### AUD-012: WIZ-003 Tests — Hook Auto-Registration

**Files:** `cli/internal/cmd/setup.go`, `cli/internal/hook/`
**Acceptance Criteria:**
- [x] Test: single harness hook registration — only that harness's hook file is created/modified (`TestRegisterClaudeCodeHookAt_FreshFile`, existing Notor tests in `hook_test.go`)
- [x] Test: both harnesses — both hook files created/modified (each harness has independent register/idempotent tests)
- [x] Test: pre-existing hooks preserved — existing user hooks in the file are not removed or overwritten (`TestRegisterClaudeCodeHookAt_PreservesExistingHooks`)

### AUD-013: WIZ-004 Tests — Cron Registration

**Files:** `cli/internal/schedule/`
**Acceptance Criteria:**
- [x] Test: register fresh — crontab entry added when none exists (`TestUnixScheduler_Install_Fresh`)
- [x] Test: idempotent re-register — running setup twice doesn't duplicate crontab entries (`TestUnixScheduler_Install_Idempotent`)
- [x] Test: unregister — `multi-kb` crontab entry removed, other entries preserved (`TestUnixScheduler_Uninstall_RemovesEntry`)
- [x] Test: existing crontab preserved — non-multi-kb crontab entries untouched (`TestUnixScheduler_Install_PreservesOtherEntries`)
- [x] Test: empty crontab edge case — works when user has no existing crontab (`TestUnixScheduler_Uninstall_EmptyInput`)

### AUD-014: WIZ-005 Tests — Cron Expression Parsing

**Files:** `cli/internal/schedule/`
**Acceptance Criteria:**
- [x] Test: common intervals — parse "every 30 minutes", "hourly", "daily" cron expressions (`TestNextRunAfter` — 10 cases covering `*/30`, `30 *`, `0 0`, etc.)
- [x] Test: next occurrence calculation — given current time, compute correct next run (`TestNextRunAfter`)
- [x] Test: missing entry — returns appropriate zero value or error when no multi-kb crontab entry exists (`TestFindCronExpr_NoMatch`, `TestFindCronExpr_EmptyInput`)
- [x] Test: Windows CSV parsing — `schtasks /Query /FO CSV` output parsed correctly (`TestParseCSVLine_*`, `TestExtractMinuteInterval_*`, `TestParseWindowsDateFormats`)

### AUD-015: WIZ-006 Tests — Standalone Subcommands (add-source, add-kb)

**Files:** `cli/internal/cmd/addsource.go`, `cli/internal/cmd/addkb.go`
**Acceptance Criteria:**
- [x] Test: add source to existing config — new source appended, existing sources preserved (`TestRunAddSource_PreservesExistingSources`)
- [x] Test: add KB to existing config — new knowledge_bases entry appended (`TestRunAddKB_PreservesExisting`)
- [x] Test: validation failures — missing required fields produce clear error messages (`TestRunAddSource_EmptyDirectory`, `TestRunAddKB_EmptyName`, `TestRunAddKB_DuplicateName`, `TestRunAddSource_MissingConfig`, `TestRunAddKB_MissingConfig`)

---

## Section 6: CLI Incomplete Tasks — Approval UI Tests

**Priority: Medium**
**Context:** The approval web UI server and command wiring are implemented but missing test coverage.

### AUD-016: APR-002 Tests — HTTP Server Lifecycle

**Files:** `cli/internal/approve/server.go`, `cli/internal/approve/server_test.go`
**Acceptance Criteria:**
- [x] Test: server starts on a random available port and reports the URL (`TestRunServer_BindsAndReportsURL`)
- [x] Test: idle timeout fires — server shuts down after configured idle period with no activity (`TestRunServer_IdleTimeout`)
- [x] Test: all-resolved shutdown — server shuts down when all pending notes have been approved or rejected (`TestRunServer_AllResolved`)
- [x] Test: manual shutdown — server responds to interrupt/context cancellation (`TestRunServer_ContextCancel`)

**Implementation notes:** `server.go` refactored to extract `runServer(ctx, pendingDir, cfg, idleTimeout, out, openBrowserFlag)` as a testable core. `StartServer` is a thin wrapper that wires `os.Interrupt` via `signal.NotifyContext`. Tests use `safeBuffer` (a `sync.Mutex`-protected writer) to avoid data races when the server goroutine writes concurrently.

### AUD-017: APR-004 Tests — Approve Command Wiring

**Files:** `cli/internal/cmd/approve.go`, `cli/internal/cmd/cmd_test.go`
**Acceptance Criteria:**
- [x] Test: launch with pending notes — server starts (`TestExecApprove_WithPendingNotes`)
- [x] Test: launch with no pending notes — prints message and exits without starting server (`TestExecApprove_NoPendingNotes`)

**Implementation notes:** `approve.go` refactored to extract `execApprove(cfgPath, pendingDir, startServer, stderr, stdout)` with an injectable `startServerFn` type alias. Tests use a fake `startServerFn` to verify call/no-call behavior and check output messages.

---

## Section 7: CLI Incomplete Tasks — Prompt Validation

**Priority: Medium**
**Context:** All 5 prompt authoring tasks (PRM-001 through PRM-005) have their prompts written and embedded in the codebase. The final acceptance criterion for each is validation testing against sample inputs. These are qualitative tests — they verify that the LLM produces reasonable output for representative inputs, not that the code compiles.

### AUD-018: PRM-001 Validation — Extraction System Prompt

**Files:** `cli/internal/extract/prompts/extraction.go`
**Test file:** `cli/internal/extract/prompts/extraction_validation_test.go` (build tag: `integration`)
**Acceptance Criteria:**
- [x] Tested against sample conversation with clear extractable knowledge — LLM produces well-formed extraction output
- [x] Tested against sample conversation with no extractable knowledge — LLM returns empty/no-knowledge response
- [x] Tested against re-processed conversation with mixed flags — LLM respects `previously_extracted` markers

### AUD-019: PRM-002 Validation — Dream Cycle Consolidation Prompt

**Files:** `cli/internal/dreamcycle/prompts/consolidation.go`
**Test file:** `cli/internal/dreamcycle/prompts/consolidation_validation_test.go` (build tag: `integration`)
**Acceptance Criteria:**
- [x] Tested against sample batch with a novel note — LLM returns `keep` action
- [x] Tested against sample batch with a duplicate note — LLM returns `merge` action
- [x] Tested against sample batch with overlapping notes — LLM returns `consolidate` action

### AUD-020: PRM-003 Validation — Coverage Assessment Prompt (CDK)

**Files:** `cdk/lambda/recall/prompts/coverage.ts`
**Test file:** `cdk/test/lambda/coverage_validation.test.ts` (skipped unless `MULTI_KB_AWS_REGION` set)
**Dependencies:** Requires deployed Bedrock model access (can test locally with Bedrock InvokeModel API)
**Acceptance Criteria:**
- [x] Tested against scenario with good coverage — LLM returns `gap_detected: false`
- [x] Tested against scenario with missing topic — LLM returns `gap_detected: true` with a relevant `refined_query`
- [x] Tested against ambiguous results — LLM makes a reasonable gap/no-gap decision

### AUD-021: PRM-004 Validation — Keyword Derivation Prompt

**Files:** `cli/internal/recall/prompts/keywords.go`
**Test file:** `cli/internal/recall/prompts/keywords_validation_test.go` (build tag: `integration`)
**Acceptance Criteria:**
- [x] Tested against a technical question first message — LLM produces relevant search keywords
- [x] Tested against a broad request — LLM produces reasonable broader keywords
- [x] Tested against a short ambiguous query — LLM produces best-effort keywords without hallucinating intent

### AUD-022: PRM-005 Validation — Chunk Summarization Prompt

**Files:** `cli/internal/extract/prompts/summarize_chunk.go`
**Test file:** `cli/internal/extract/prompts/summarize_chunk_validation_test.go` (build tag: `integration`)
**Acceptance Criteria:**
- [x] Tested against a long conversation chunk — LLM produces a coherent summary that preserves key technical details and context

---

## Section 8: CLI Incomplete Tasks — E2E Scenario Validation

**Priority: Medium**
**Context:** QAT-003 requires executing 9 end-to-end scenarios against a deployed stack. Zero criteria are currently met. This is a manual validation checklist — each scenario exercises a full user workflow across CLI + CDK infrastructure.

### AUD-023: QAT-003 End-to-End Scenario Validation

**Description:** Execute all 9 end-to-end scenarios from `cli/test/e2e/scenarios.md` against a deployed stack. Each scenario should be run manually and results documented.
**Dependencies:** Deployed CDK stack with CLI binary on EC2
**Files:** `cli/test/e2e/scenarios.md` — checklist file to mark complete (reviewed and corrected against implementation)
**Sub-task status:**
- [x] **Review and correct `scenarios.md`** — verified all 9 scenarios against actual implementation; corrected field names, hook event type, status values, re-processing markers, dream cycle phase details, and approval flow behavior
- [x] **Deploy stack** — CDK stack deployed to us-east-1 (account 639628476385); post-deploy integration script passed (30/30, 2 skips)
- [x] **Execute automated scenarios** — 4 of 9 scenarios passed, 4 skipped (require TTY/browser), 1 partial
  - Scenario 1 (Setup): PARTIAL — config created manually, `status` command verified (wizard requires TTY)
  - Scenario 2 (Capture): PASS — 46 conversations processed, 178 notes extracted, 0 errors
  - Scenario 3 (Hook Injection): SKIP — requires interactive Claude Code session
  - Scenario 4 (Oversized): SKIP — requires large synthetic file + significant Bedrock spend
  - Scenario 5 (Extraction Failure): PASS — invalid model → 3 retries → extraction-errors.jsonl entry
  - Scenario 6 (Hook Timeout): SKIP — requires interactive Claude Code session
  - Scenario 7 (Re-Processing): PASS — modified conversation re-discovered and re-processed
  - Scenario 8 (Approval): SKIP — requires browser UI
  - Scenario 9 (Dream Cycle): PASS — 109 batches, 60 keep actions, git commits by multi-kb@local
- [x] **Bug fix: OpenSearch index custom resource** — Update handler was a no-op, failed when collection was rebuilt; now verifies index exists and recreates if missing
- [x] **Bug fix: Bedrock model ID** — on-demand model IDs rejected by Bedrock; must use inference profile ID (e.g. `us.anthropic.claude-sonnet-4-6`)

---

#### Step 0: Prerequisites

Before any scenario can be run, all of the following must be true.

**0a. AWS account and region**
- [ ] AWS CLI configured: `aws sts get-caller-identity` returns your account ID
- [ ] Target region chosen (e.g. `us-east-1`) and set: `export AWS_DEFAULT_REGION=us-east-1`
- [ ] Bedrock model access enabled in that region: Claude Sonnet 4.x, Claude Haiku 4.x, Titan Embeddings V2
- [ ] CDK bootstrapped (one-time per account/region): `cdk bootstrap aws://<account>/<region>`

**0b. Build the Linux binary** (must be cross-compiled; EC2 runs Linux)
```bash
cd cli
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-s -w -X 'github.com/zmueller/multi-kb/internal/cmd.version=$(git describe --tags --always --dirty)'" \
  -o dist/multi-kb-linux-amd64 ./cmd/multi-kb/
```
- [ ] `dist/multi-kb-linux-amd64` exists and is ~10–20 MB

**0c. Upload the binary to S3** (CDK downloads it onto EC2 via user data)
```bash
# Create a bucket if you don't already have one for artifacts
export ARTIFACT_BUCKET=my-multikb-artifacts   # choose a name
aws s3 mb s3://$ARTIFACT_BUCKET
aws s3 cp cli/dist/multi-kb-linux-amd64 s3://$ARTIFACT_BUCKET/multi-kb/latest/multi-kb-linux-amd64
export CLI_BINARY_S3_URI=s3://$ARTIFACT_BUCKET/multi-kb/latest/multi-kb-linux-amd64
```
- [ ] `aws s3 ls $CLI_BINARY_S3_URI` shows the uploaded file

**0d. CDK install and synth**
```bash
cd cdk
npm install
npx tsc --noEmit          # must pass
cdk synth --context cliBinaryS3Uri=$CLI_BINARY_S3_URI
```
- [ ] `cdk synth` exits 0 with no errors

**0e. Deploy the stack**
```bash
cdk deploy \
  --context cliBinaryS3Uri=$CLI_BINARY_S3_URI \
  --context repoName=my-kb \
  --require-approval never
```
- This creates: VPC, OpenSearch Serverless, Bedrock KB + data source, S3, CodeCommit, SQS, API Gateway, EC2 ASG, Lambda functions, CloudWatch alarms
- Takes ~15–20 minutes (OpenSearch Serverless collection provisioning dominates)
- [ ] `cdk deploy` exits 0
- [ ] Stack outputs printed — note `ApiEndpoint`, `BucketName`, `KnowledgeBaseId`, `DataSourceId`, `AsgName`

**0f. Build a local (macOS/Linux) binary for your machine**
```bash
cd cli
go build -o bin/multi-kb ./cmd/multi-kb/
./bin/multi-kb --version
```
- [ ] `multi-kb --version` prints a version string (not `dev` if git tags exist)

**0g. Run the post-deploy integration script**
```bash
cd cdk
./test/integration/qat-005-post-deploy.sh MultiKbStack
```
- This validates: submit flow, recall flow, server config on EC2, EC2 health, SSM access, CloudWatch log groups, DLQ alarm
- [ ] Script exits 0 (all checks PASS or SKIP; zero FAIL)
- If any FAIL: resolve before continuing to the scenario tests

---

#### Step 1: Scenario 1 — First-Time Setup

Run on your local machine with the macOS/Linux binary from Step 0f.

- [ ] `./bin/multi-kb setup` — wizard launches with harness selection prompt
- [ ] Select Claude Code harness → enter at least one source directory (must exist on disk)
- [ ] Configure at least one local KB (name: e.g. `default`) and one routing rule (routing: `always`, approval: `auto-approve`)
- [ ] Wizard completes — verify: `cat ~/.multi-kb/config.yaml` shows `mode: client`, your source directory, and `targets: [{kb: local/default}]`
- [ ] Hook registered: `cat ~/.claude/settings.json` contains a `hooks` entry invoking `multi-kb hook --harness claude-code`
- [ ] Cron registered: `crontab -l` shows a `multi-kb run` entry
- [ ] `./bin/multi-kb status` prints config summary, "No runs recorded yet", and next scheduled run time
- [ ] Total elapsed time from starting `setup` to passing `status` check: under 10 minutes
- [ ] Check off Scenario 1 in `cli/test/e2e/scenarios.md`

---

#### Step 2: Scenario 2 — Scheduled Capture

Requires at least one real Claude Code conversation file in the configured source directory.

- [ ] Confirm conversation files exist: `ls ~/.claude/projects/` (or wherever Claude Code stores transcripts) — at least one `.jsonl` file
- [ ] Run manually (don't wait for cron): `./bin/multi-kb run`
- [ ] Verify run log: `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` — `directories_scanned > 0`, `notes_extracted > 0`
- [ ] Verify pending queue: `ls ~/.multi-kb/pending/` — at least one `.json` file
- [ ] If `notes_extracted == 0`: the source directory has no conversations or they've all been processed before — use `--force` to reprocess or point the config at a directory with fresh conversations
- [ ] Check off Scenario 2 in `cli/test/e2e/scenarios.md`

---

#### Step 3: Scenario 3 — Hook Injection

Requires Claude Code to be installed and the hook registered (from Scenario 1).

- [ ] Start a new Claude Code conversation in a directory that matches your configured source
- [ ] On user prompt submission, Claude Code fires the `UserPromptSubmit` hook → `multi-kb hook --harness claude-code` is invoked
- [ ] Verify injection: the system prompt or injected context in the conversation contains a `## Knowledge Base` markdown section (visible in the Claude Code UI or transcript)
- [ ] If no local KB content yet (notes_extracted == 0 from Scenario 2): injection block appears but is empty — this is correct, not an error
- [ ] Conversation proceeds normally after the hook fires
- [ ] Check off Scenario 3 in `cli/test/e2e/scenarios.md`

---

#### Step 4: Scenario 4 — Oversized Conversation

Requires a conversation file large enough to exceed the chunking threshold (~700K tokens ≈ ~2 MB of text).

- [ ] Create or locate a large conversation file. A synthetic one can be generated:
  ```bash
  # Generate a ~3MB synthetic JSONL conversation (rough approximation)
  python3 -c "
  import json, sys
  for i in range(500):
      print(json.dumps({'type': 'user', 'content': 'Message ' + str(i) + ': ' + ('x ' * 1000)}))
      print(json.dumps({'type': 'assistant', 'content': 'Response ' + str(i) + ': ' + ('y ' * 1000)}))
  " > /tmp/large-conversation.jsonl
  ```
- [ ] Place the file in the configured source directory (rename to match expected file format)
- [ ] Run: `./bin/multi-kb run`
- [ ] Verify chunked extraction: `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` — `notes_extracted > 0`; CloudWatch or stderr logs show multiple LLM invocations for the same conversation
- [ ] No hard crash or OOM — process completes with exit 0
- [ ] Check off Scenario 4 in `cli/test/e2e/scenarios.md`

---

#### Step 5: Scenario 5 — Extraction Failure + Retry

- [ ] Temporarily set an invalid model ID in `~/.multi-kb/config.yaml` under `extraction.model_id` (e.g. `anthropic.claude-invalid-model`) — or throttle by making many rapid requests
- [ ] Run: `./bin/multi-kb run`
- [ ] Observe: process retries up to 3 times per extraction attempt (visible via stderr or verbose mode)
- [ ] On persistent failure: verify `cat ~/.multi-kb/logs/extraction-errors.jsonl` has an entry with a non-empty `error` field
- [ ] Restore the correct `model_id` in config
- [ ] Check off Scenario 5 in `cli/test/e2e/scenarios.md`

---

#### Step 6: Scenario 6 — Hook Timeout

- [ ] Add a remote KB to config with an unreachable endpoint:
  ```yaml
  knowledge_bases:
    - name: slow-kb
      endpoint: https://192.0.2.1/  # TEST-NET — unreachable
      auth: iam
      aws_region: us-east-1
  ```
  And add it as a target in the source's routing rules: `kb: slow-kb`
- [ ] Set a short timeout in config (optional, to speed up test): `hook: {timeout: "2s"}`
- [ ] Start a new Claude Code conversation in the configured directory
- [ ] Hook fires — after the configured timeout (default 8s), hook returns with partial results (local KB only)
- [ ] Conversation proceeds — no hang or crash
- [ ] Verify: `cat ~/.multi-kb/logs/hook-errors.jsonl` has a timeout or connection-refused entry
- [ ] Restore config (remove slow-kb and timeout override)
- [ ] Check off Scenario 6 in `cli/test/e2e/scenarios.md`

---

#### Step 7: Scenario 7 — Re-Processing Modified Conversation

- [ ] Identify a conversation already processed in Scenario 2 (check `~/.multi-kb/state.yaml` for `last_processed` timestamp on your source directory)
- [ ] Add several new user+assistant message pairs to the end of that conversation file
- [ ] Run: `./bin/multi-kb run`
- [ ] Verify: run log shows `notes_extracted > 0` from this re-run (the new messages are processed, old messages are skipped via `previously_processed` markers)
- [ ] Verify: the newly extracted notes appear in `~/.multi-kb/pending/`
- [ ] Check off Scenario 7 in `cli/test/e2e/scenarios.md`

---

#### Step 8: Scenario 8 — Approval Flow

Requires pending notes from Scenario 2 or 7.

- [ ] Confirm pending notes exist: `ls ~/.multi-kb/pending/` — at least one `.json` file
- [ ] Run: `./bin/multi-kb approve`
- [ ] Browser opens automatically to `http://localhost:<port>` with the approval UI
- [ ] Review a note: inspect title and content
- [ ] Edit the title or content of one note via the UI
- [ ] Approve that note to local KB: click Approve — verify `~/.multi-kb/local/default/<uid>.md` is created with `status: pending` (the dream cycle later transitions it to `active`)
- [ ] Reject a note: click Reject for another note — verify the pending file for that note is either deleted or the rejected target is removed from it
- [ ] When all targets on a pending entry are resolved: verify the `.json` file in `~/.multi-kb/pending/` is deleted
- [ ] Server shuts down after all notes are resolved (or wait for idle timeout — default from config)
- [ ] Check off Scenario 8 in `cli/test/e2e/scenarios.md`

---

#### Step 9: Scenario 9 — Dream Cycle

Requires at least one approved `status: pending` note in a local KB from Scenario 8 (approval to local KB creates notes with `status: pending`; the dream cycle transitions them to `active`).

- [ ] Confirm a pending note exists in the local KB: `grep -r "status: pending" ~/.multi-kb/local/`
- [ ] Run: `./bin/multi-kb dream-cycle`
- [ ] Phase 1: pending notes found, singleton batches created (errors appear on stderr as `dream-cycle: phase 1 error for KB "...": ...`)
- [ ] Phase 2: related active notes found via git grep for each batch (may be 0 for a fresh KB — that's valid)
- [ ] Phase 3: LLM consolidation + action application — actions are keep/merge/split/consolidate (errors appear on stderr as `dream-cycle: phase 3 error for batch: ...`)
- [ ] Verify: the pending note is now `status: active` — `grep -r "status: active" ~/.multi-kb/local/`
- [ ] Verify: git commit created in the local KB repo — `git -C ~/.multi-kb/local/default log --oneline -1` shows a commit by `multi-kb <multi-kb@local>`
- [ ] Verify: run log has a `dream_cycle` entry — `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` shows `type: "dream_cycle"`, `batches_processed`, and `actions` map with keep/merge/split/consolidate counts
- [ ] Check off Scenario 9 in `cli/test/e2e/scenarios.md`

---

#### Step 10: Mark Complete

- [ ] All 9 scenarios passed (or documented skips with explanation)
- [ ] Fill in tester name and date at the bottom of `cli/test/e2e/scenarios.md`
- [ ] Check off all acceptance criteria below

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
Phase 1 (Critical fix):    AUD-001 -> AUD-002 + AUD-003 (parallel)
  AUD-001: S3 sync generates .metadata.json sidecars (CLI, foundational)
  AUD-002: Server dream cycle uses Retrieve API (CLI, depends on AUD-001 for sidecars to exist)
  AUD-003: Recall Lambda uses Retrieve API filter (CDK, independent of AUD-002)
  AUD-004: No-op (OpenSearch schema unchanged)
Phase 2 (Low, parallel):   AUD-005 + AUD-006 + AUD-007 (all independent)
Phase 3 (Medium, parallel): AUD-008 + AUD-009 + AUD-010..015 + AUD-016 + AUD-017 (all independent)
Phase 4 (Medium, parallel): AUD-018..022 (prompt validation, needs Bedrock access)
Phase 5 (Deployed stack):  AUD-023 (E2E scenarios)
```

Phase 1 note: AUD-001 (sidecar generation) must land first — without sidecars in S3, the Retrieve API filters will match nothing. After AUD-001, AUD-002 (Go dream cycle) and AUD-003 (TypeScript recall Lambda) are independent and can proceed in parallel. No existing data migration needed — the deployment has no real data, so a fresh `syncAllFiles` + ingestion job will populate all sidecars.

After deployment: trigger a full S3 sync to generate sidecars for all existing files, then trigger a Bedrock ingestion job. Verify sidecars are in S3 (`aws s3 ls s3://bucket/ | grep metadata.json`), then verify Retrieve API returns metadata fields.

Phases 2 and 3 can run in parallel with each other and with Phase 1. Phase 4 requires Bedrock model access but not a deployed stack. Phase 5 requires a fully deployed stack with the Phase 1 sidecar changes deployed.
