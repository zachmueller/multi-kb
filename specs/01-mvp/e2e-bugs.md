# E2E Bugs: Defects Found During AUD-023 Scenario Execution

**Created:** 2026-05-04
**Source:** AUD-023 E2E scenario execution against deployed stack (us-east-1, account 639628476385)
**Status:** 5 found, 4 fixed, 1 open (low)

## Summary

| Bug | Severity | Component | Status |
|-----|----------|-----------|--------|
| E2E-001: Bedrock model ID defaults use on-demand IDs that Bedrock rejects | High | CLI + CDK | **Fixed** |
| E2E-002: Dream cycle consolidation fails to parse LLM preamble text | Medium | CLI | **Fixed** |
| E2E-003: Local KB directory not auto-created on first note submission | Medium | CLI | **Fixed** |
| E2E-004: Unquoted YAML title values break frontmatter parsing | High | CLI | **Fixed** |
| E2E-005: Dream cycle cascading file-not-found on concurrent batch processing | Low | CLI | Open |

---

## E2E-001: Bedrock Model ID Defaults Use On-Demand IDs That Bedrock Rejects

**Severity:** High — all LLM invocations fail out of the box with default configuration
**Components:** CLI (`cli/internal/config/config.go`), CDK (`cdk/lib/multi-kb-stack.ts`, `cdk/lambda/recall/index.ts`)
**Status:** Fixed (2026-05-04)

### Problem

The default model IDs hardcoded in the CLI config defaults and CDK stack props are **on-demand model identifiers** (e.g. `anthropic.claude-sonnet-4-20250514`). Bedrock rejects these with:

```
Access denied. This Model is marked by provider as Legacy and you have not been actively
using the model in the last 30 days. Please upgrade to an active model on Amazon Bedrock
```

Or, when not yet available for on-demand:

```
Invocation of model ID anthropic.claude-sonnet-4-20250514-v1:0 with on-demand throughput
isn't supported. Retry your request with the ID or ARN of an inference profile that
contains this model.
```

This affects **every** LLM call in the system: extraction, dream cycle consolidation, chunk summarization, and recall coverage assessment.

### How to Replicate

1. Deploy the CDK stack with defaults (no `--context consolidationModelId=...` override)
2. Run `multi-kb run` with a config that omits `extraction.model_id` (relying on the default)
3. Every extraction attempt fails after 3 retries with the error above

### Root Cause

AWS Bedrock requires **inference profile IDs** (prefixed with `us.` or `global.`) for cross-region inference, or the full versioned model ID with `:0` suffix for on-demand — but even the versioned form is rejected for models the account hasn't used recently. Inference profiles are the reliable path.

### Affected Code Locations

**CLI defaults** — `cli/internal/config/config.go:150-157`:
```go
cfg.Extraction.ModelID = "anthropic.claude-sonnet-4-20250514"        // BROKEN
cfg.DreamCycle.ModelID = "anthropic.claude-sonnet-4-20250514"        // BROKEN
cfg.Translation.SummarizationModelID = "anthropic.claude-haiku-4-5-20251001" // BROKEN
```

**CDK stack defaults** — `cdk/lib/multi-kb-stack.ts:42-50`:
```typescript
embeddingModelId: "amazon.titan-embed-text-v2:0",                   // OK (embedding, not inference)
consolidationModelId: "anthropic.claude-sonnet-4-20250514",          // BROKEN
coverageModelId: "anthropic.claude-haiku-4-5-20251001",              // BROKEN
```

**Recall Lambda ARN construction** — `cdk/lambda/recall/index.ts:87`:
```typescript
const modelArn = `arn:aws:bedrock:${region}::foundation-model/${coverageModelId}`;
```
This constructs a foundation model ARN, but inference profile IDs need an inference profile ARN format, or should be passed directly as the `modelId` parameter (the SDK accepts inference profile IDs without wrapping them in an ARN).

### Fix

**Step 1: Update CLI defaults** in `cli/internal/config/config.go`:

Replace lines 150-157:
```go
if cfg.Extraction.ModelID == "" {
    cfg.Extraction.ModelID = "us.anthropic.claude-sonnet-4-6"
}
if cfg.DreamCycle.ModelID == "" {
    cfg.DreamCycle.ModelID = "us.anthropic.claude-sonnet-4-6"
}
if cfg.Translation.SummarizationModelID == "" {
    cfg.Translation.SummarizationModelID = "us.anthropic.claude-haiku-4-5-20251001-v1:0"
}
```

**Step 2: Update CDK stack defaults** in `cdk/lib/multi-kb-stack.ts`:

Replace lines 46-50:
```typescript
consolidationModelId:
    app.node.tryGetContext("consolidationModelId") ??
    "us.anthropic.claude-sonnet-4-6",
coverageModelId:
    app.node.tryGetContext("coverageModelId") ??
    "us.anthropic.claude-haiku-4-5-20251001-v1:0",
```

Leave `embeddingModelId` unchanged — `amazon.titan-embed-text-v2:0` is an embedding model invoked via the Bedrock KB service, not directly.

**Step 3: Fix recall Lambda ARN construction** in `cdk/lambda/recall/index.ts`:

Replace line 87:
```typescript
// Old: const modelArn = `arn:aws:bedrock:${region}::foundation-model/${coverageModelId}`;
// New: pass the inference profile ID directly — the SDK accepts it as modelId
const modelArn = coverageModelId;
```

The `InvokeModelCommand` accepts both full ARNs and inference profile IDs as the `modelId` parameter. Inference profile IDs (e.g. `us.anthropic.claude-haiku-4-5-20251001-v1:0`) do not need to be wrapped in an ARN. The old foundation-model ARN format is only valid for on-demand model IDs.

**Step 4: Update tests:**
- `cli/internal/config/config_test.go` — update assertions on default model ID values
- `cdk/test/` — any tests that assert on default model IDs or the recall Lambda environment
- `cli/internal/bedrock/client_integration_test.go` — update hardcoded model ID if present

**Step 5: Redeploy CDK stack** to pick up the new server-config model IDs and recall Lambda environment variable.

**Acceptance Criteria:**
- [x] `multi-kb run` with no explicit `model_id` in config succeeds (extraction + dream cycle) — verified 2026-05-04; full dream cycle completes (phases 0–4) with inference profile model IDs after adding bedrock-agent-runtime VPC endpoint, bedrock:Retrieve permission, and cross-region foundation-model ARN in IAM
- [x] CDK `cdk synth` produces server config with inference profile model IDs
- [x] Recall Lambda coverage assessment succeeds with default model ID — verified 2026-05-04; Lambda calls Retrieve API and InvokeModel with inference profile IDs; IAM policies include both inference-profile and foundation-model ARNs for cross-region routing
- [x] All existing tests pass (Go config tests: 24/24, CDK tests: 194/194)

---

## E2E-002: Dream Cycle Consolidation Fails to Parse LLM Preamble Text

**Severity:** Medium — 49 of 109 dream cycle batches failed (45% failure rate)
**Component:** CLI (`cli/internal/dreamcycle/phase3.go`)
**Status:** Fixed (2026-05-04)

### Problem

The LLM (Claude Sonnet 4.6 via Bedrock) frequently returns preamble text before the JSON response in the consolidation step. The parser only strips markdown code fences but does not extract JSON from surrounding prose. This causes `json.Unmarshal` to fail on the first non-JSON character.

Observed error patterns (49 failures across 109 batches):
```
dream-cycle: phase 3 error for batch: phase3: parse response: invalid JSON: invalid character '#' looking for beginning of value
dream-cycle: phase 3 error for batch: phase3: parse response: invalid JSON: invalid character 'I' looking for beginning of value
dream-cycle: phase 3 error for batch: phase3: parse response: invalid JSON: invalid character 'L' looking for beginning of value
```

The `#` is a markdown heading (e.g. `## Analysis`), `I` is from "I'll analyze...", and `L` is from "Let me evaluate...".

### How to Replicate

1. Run `multi-kb run` or `multi-kb dream-cycle` with pending notes in a local KB
2. The LLM returns a response like:
   ```
   I'll analyze this pending note against the active notes.

   ```json
   {"actions": [{"type": "keep", "source_uid": "ABC123", "reason": "novel topic"}]}
   ```

   This note covers a new topic not found in the existing knowledge base.
   ```
3. The parser sees `I` as the first character and fails

### Root Cause

`parseConsolidationOutput()` in `cli/internal/dreamcycle/phase3.go:124-147` only handles one non-JSON wrapper: markdown code fences starting with `` ``` `` at the beginning of the response. It does not handle:
- Preamble text before a code fence
- Preamble text before raw JSON (no code fence)
- Trailing commentary after the JSON

### Fix

**File:** `cli/internal/dreamcycle/phase3.go`, function `parseConsolidationOutput()` (line 124)

Replace the current implementation with a more robust JSON extraction strategy:

```go
func parseConsolidationOutput(response string) (*consolidationOutput, error) {
    response = strings.TrimSpace(response)

    // Strategy 1: Try to parse the entire response as JSON
    var output consolidationOutput
    if err := json.Unmarshal([]byte(response), &output); err == nil {
        if len(output.Actions) > 0 {
            return &output, nil
        }
    }

    // Strategy 2: Extract JSON from markdown code fence anywhere in the response
    if idx := strings.Index(response, "```json"); idx >= 0 {
        inner := response[idx+len("```json"):]
        if end := strings.Index(inner, "```"); end >= 0 {
            inner = strings.TrimSpace(inner[:end])
            if err := json.Unmarshal([]byte(inner), &output); err == nil && len(output.Actions) > 0 {
                return &output, nil
            }
        }
    }
    // Also try bare ``` (no language tag)
    if idx := strings.Index(response, "```\n"); idx >= 0 {
        inner := response[idx+len("```\n"):]
        if end := strings.Index(inner, "```"); end >= 0 {
            inner = strings.TrimSpace(inner[:end])
            if err := json.Unmarshal([]byte(inner), &output); err == nil && len(output.Actions) > 0 {
                return &output, nil
            }
        }
    }

    // Strategy 3: Find the first '{' and last '}' — extract the outermost JSON object
    start := strings.Index(response, "{")
    end := strings.LastIndex(response, "}")
    if start >= 0 && end > start {
        candidate := response[start : end+1]
        if err := json.Unmarshal([]byte(candidate), &output); err == nil && len(output.Actions) > 0 {
            return &output, nil
        }
    }

    return nil, fmt.Errorf("invalid JSON: could not extract actions object from response (len=%d, first 200 chars: %s)",
        len(response), truncate(response, 200))
}

func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n] + "..."
}
```

**Test updates** in `cli/internal/dreamcycle/dreamcycle_test.go`:

Add test cases for:
- Preamble text before code fence: `"I'll analyze this.\n\n```json\n{\"actions\":[...]}\n```"`
- Preamble text before raw JSON: `"Here is my analysis:\n\n{\"actions\":[...]}"`
- Trailing commentary after JSON: `"{\"actions\":[...]}\n\nThis note is novel."`
- Mixed preamble + code fence + trailing: `"Analysis:\n\n```json\n{\"actions\":[...]}\n```\n\nDone."`

**Acceptance Criteria:**
- [x] Existing tests (`TestParseConsolidationOutput_Valid`, `_MarkdownFenced`, `_InvalidJSON`, `_EmptyActions`) still pass
- [x] New tests for preamble/trailing text pass (6 new test cases: preamble+code fence, preamble+raw JSON, trailing commentary, preamble+code fence+trailing, bare code fence+preamble, no parsable JSON)
- [x] Re-running dream cycle on the same 109 batches produces significantly fewer parse failures (target: <5%) — verified 2026-05-04; dream cycle phase 3 completed with 0 parse errors across 2 batches (2 keep actions, 0 failures)

---

## E2E-003: Local KB Directory Not Auto-Created on First Note Submission

**Severity:** Medium — first `multi-kb run` with auto-approve to a local KB fails for every note
**Component:** CLI (`cli/internal/cmd/process.go`)
**Status:** Fixed (2026-05-04)

### Problem

When `multi-kb process` (or `multi-kb run`) routes notes to a local KB target with `approval: auto-approve`, it calls `submit.WriteNote(kbDir, ...)` which attempts to write a file directly into `~/.multi-kb/local/<name>/`. If this directory does not exist (first run, or the user has never run `multi-kb approve`), every note submission fails with:

```
process: submit note to "local/default": submit: cannot write note file: open
/Users/zmueller/.multi-kb/local/default/5JAN44VDNKEV2BAB.md: no such file or directory
```

This error is logged but not fatal — the process continues, but **all extracted notes are lost** because every submission fails. The state file is still updated with `last_processed`, so re-running does not re-process the same conversations.

### How to Replicate

1. Start with a fresh `~/.multi-kb/` (no `local/` subdirectory)
2. Configure a source with `approval: auto-approve` targeting `local/default`
3. Run `multi-kb run`
4. All notes fail to write; `notes_routed` in the run log shows 0 even though `notes_extracted` is > 0
5. State is updated — the conversations are marked as processed and won't be re-extracted

### Root Cause

The `submitNote()` function in `cli/internal/cmd/process.go:305-313` calls `submit.WriteNote()` without ensuring the local KB directory exists or that it's initialized as a git repo. By contrast, the approval handler at `cli/internal/approve/handlers.go:271` correctly calls `git.InitRepo(kbDir)` before writing, which creates the directory and initializes git.

### Fix

**File:** `cli/internal/cmd/process.go`

In the `submitNote()` function, before calling `submit.WriteNote()`, add the same initialization the approval handler uses. Around line 305 (the `// Auto-approve — submit directly` local KB branch):

```go
// Auto-approve — submit directly
if strings.HasPrefix(target.KB, "local/") {
    kbName := target.KB[6:]
    kbDir := filepath.Join(homeDir(), ".multi-kb", "local", kbName)

    // Ensure local KB directory exists and is a git repo
    if err := git.InitRepo(kbDir); err != nil {
        return fmt.Errorf("submit: cannot init local KB %q: %w", kbName, err)
    }

    _, err := submit.WriteNote(kbDir, submit.NoteFields{
        // ... (unchanged)
    })
    return err
}
```

**File:** `cli/internal/git/repo.go` — verify `InitRepo()` is idempotent (it should be — `git init` on an existing repo is a no-op).

**Secondary fix — prevent state update on total submission failure:**

The state file should not advance `last_processed` when all note submissions for a directory failed. This is a separate but related issue in `cli/internal/cmd/process.go` around line 138-146. Currently, `last_processed` is updated based on the latest file modification time, regardless of whether submissions succeeded. If all submissions failed, the conversations are effectively lost.

Consider tracking per-directory submission success and only advancing state when at least one note was successfully submitted, or when the error count is 0 for that directory. This is a more involved change and could be addressed separately — the primary fix (auto-creating the directory) prevents the scenario entirely.

**Test updates:**

Add to `cli/internal/cmd/` test file (or create a new integration-style test):
- Test: `submitNote()` with a non-existent local KB directory — should succeed (creates dir + repo)
- Test: `submitNote()` with an existing local KB directory — should succeed (idempotent)
- Test: `submitNote()` with local KB and then read back the note file — frontmatter correct

**Acceptance Criteria:**
- [x] `multi-kb run` with auto-approve to a fresh local KB (no pre-existing directory) writes notes successfully
- [x] `git.InitRepo()` is called before `submit.WriteNote()` on the local KB path
- [x] The local KB directory is created as a git repo on first use
- [x] Subsequent runs with the same local KB target are unaffected (idempotent)
- [x] Existing tests pass (34/34 in cmd package, including 3 new tests)

---

---

## E2E-004: Unquoted YAML Title Values Break Frontmatter Parsing

**Severity:** High — dream cycle silently skips ~54% of notes (100 of 184 with colons in titles)
**Components:** CLI (`cli/internal/submit/local.go`, `cli/internal/dreamcycle/local_store.go`)
**Status:** Fixed (2026-05-04)

### Problem

Note files are written with unquoted YAML title values. When a title contains a colon followed by a space (e.g., `title: CDK Stack Integration Tests: Template.fromStack()`), the Go YAML parser interprets the second colon as a nested mapping, producing a `yaml: line 2: mapping values are not allowed in this context` error. The `parseFrontmatter()` function silently returns empty fields on parse error, causing `CreateBatches()` to see `status: ""` instead of `status: "pending"` and skip the note entirely.

### How to Replicate

1. Run `multi-kb run` to extract notes (many titles naturally contain colons)
2. Run `multi-kb dream-cycle` — reports 0 batches despite having pending notes
3. Check: `grep -rl "status: pending" ~/.multi-kb/local/default/ | wc -l` shows notes exist

### Root Cause

Three locations write `title: %s` without quoting:
- `cli/internal/submit/local.go:54` — `renderNote()`
- `cli/internal/dreamcycle/local_store.go:84` — `renderNoteFile()`
- `cli/internal/dreamcycle/local_store.go:101` — `renderNoteFileWithConsolidated()`

### Fix

Changed all three to use `%q` (Go's quoted-string format): `title: %q`. This produces `title: "CDK Stack Integration Tests: Template.fromStack()"` which YAML parses correctly. The YAML parser naturally strips the outer quotes when deserializing.

Additionally, repaired 100 existing notes in `~/.multi-kb/local/default/` by quoting their title values in-place.

**Acceptance Criteria:**
- [x] `renderNote()` and `renderNoteFile()` use `%q` for title values
- [x] `parseFrontmatter()` correctly parses titles with colons
- [x] `CreateBatches()` returns correct count of pending notes
- [x] All existing tests updated and pass (submit: 11/11, dreamcycle: 22/22, cmd: 34/34)
- [x] Dream cycle processes all pending notes after fix (41 batches from 46 remaining pending notes)

---

## E2E-005: Dream Cycle Cascading File-Not-Found on Concurrent Batch Processing

**Severity:** Low — non-fatal, affects ~8-17% of batches per run
**Component:** CLI (`cli/internal/dreamcycle/cycle.go`, `cli/internal/dreamcycle/phase3.go`)
**Status:** Open

### Problem

When the dream cycle processes multiple batches sequentially, a consolidate or merge action in an earlier batch may delete an active note (e.g., `FJAGQHHBP4QNN12S`) that a later batch references as a related note. The later batch's keep/merge action then fails with:

```
dream-cycle: phase 3 error for batch: phase3: apply actions: keep: read "FJAGQHHBP4QNN12S":
open /Users/zmueller/.multi-kb/local/default/FJAGQHHBP4QNN12S.md: no such file or directory
```

Observed error rates across runs:
- Run 1: 16 errors out of 76 batches (21%)
- Run 2: 7 errors out of 41 batches (17%)

The dream cycle continues processing remaining batches — these errors are non-fatal but result in some pending notes not being fully processed.

### Root Cause

Phase 2 (related note discovery via git grep) runs before Phase 3 (LLM consolidation + action application) for all batches. The related notes are resolved at Phase 2 time. By the time Phase 3 processes a batch, earlier batches may have deleted some of the related notes through consolidate or merge actions.

### Suggested Fix

In Phase 3, when a keep/merge action references a note that no longer exists, skip the stale reference gracefully instead of reporting an error. This is a best-effort strategy — the note was already handled by a prior batch.

**Acceptance Criteria:**
- [ ] Phase 3 action application handles missing related notes gracefully (skip + warn instead of error)
- [ ] Error count for cascading file-not-found drops to 0 or near-0
- [ ] Run log `errors` field reflects only genuine failures

---

## Execution Order

These bugs are independent and can be fixed in parallel:

```
E2E-001 (High):   CLI defaults + CDK defaults + recall Lambda ARN → redeploy
E2E-002 (Medium): phase3.go parser robustness → unit tests only
E2E-003 (Medium): process.go auto-create local KB dir → unit tests only
E2E-004 (High):   submit/local.go + dreamcycle/local_store.go YAML quoting → unit tests + data repair
E2E-005 (Low):    dreamcycle/phase3.go graceful handling of deleted notes → unit tests only
```

E2E-001 should be fixed first as it blocks all LLM functionality with default configuration. E2E-004 should be fixed second as it silently breaks the entire dream cycle. E2E-002, E2E-003, and E2E-005 can be fixed in any order.
