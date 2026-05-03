# End-to-End Scenario Validation Checklist

**CLI QAT-003 — Manual Validation**

Run these scenarios against a built binary (`make build`) with a deployed CDK stack.

## Prerequisites

- [ ] Binary built: `./multi-kb --version` prints version
- [ ] AWS credentials available (`aws sts get-caller-identity` works)
- [ ] CDK stack deployed and healthy (CloudWatch shows server tick loop)

---

## Scenario 1: First-Time Setup

**Target:** Complete setup in under 10 minutes.

- [ ] Run `multi-kb setup` — wizard launches with harness selection
- [ ] Select Claude Code harness → source directories discovered
- [ ] Configure at least one local KB and one routing rule
- [ ] Config written to `~/.multi-kb/config.yaml` — validate with `cat`
- [ ] Hook registered: verify `~/.claude/settings.json` contains `multi-kb hook` entry
- [ ] Cron registered: `crontab -l` shows `multi-kb run` entry
- [ ] `multi-kb status` shows config loaded, "No runs recorded yet", and next scheduled run time

## Scenario 2: Scheduled Capture

- [ ] Wait for cron to fire (or run `multi-kb run` manually)
- [ ] Conversations scanned: run log shows `directories_scanned > 0`
- [ ] Knowledge extracted: `notes_extracted > 0` in run log (requires conversation history)
- [ ] Notes routed: pending queue populated at `~/.multi-kb/pending/`
- [ ] Run log written to `~/.multi-kb/logs/runs.jsonl`

## Scenario 3: Hook Injection

- [ ] Start a new Claude Code conversation
- [ ] Hook fires on first message — verify `multi-kb hook` invoked
- [ ] Recall queries local KB — Markdown injected into conversation context
- [ ] Conversation proceeds normally after injection
- [ ] If no local KB content: injection is empty/minimal (does not error)

## Scenario 4: Oversized Conversation

**Target:** Process a conversation exceeding 700K tokens.

- [ ] Place a large conversation file (>700K tokens when translated) in source dir
- [ ] Run `multi-kb process` or `multi-kb run`
- [ ] Verify chunked extraction: multiple LLM calls in logs
- [ ] All knowledge extracted from all chunks
- [ ] No extraction errors or only partial warnings

## Scenario 5: Extraction Failure + Retry

- [ ] Simulate Bedrock throttling (rapid repeated runs, or invalid model ID)
- [ ] Verify retry behavior: up to 3 attempts per extract call
- [ ] On persistent failure: extraction error logged at `~/.multi-kb/logs/extraction-errors.jsonl`
- [ ] Partial results accepted: valid notes from successful chunks still routed

## Scenario 6: Hook Timeout

- [ ] Configure a remote KB endpoint that is slow (or temporarily unreachable)
- [ ] Start a conversation — hook fires
- [ ] After 8s timeout (default; configurable via `hook.timeout` in config): partial results returned (local KB results only)
- [ ] No crash or hang — conversation proceeds

## Scenario 7: Re-Processing Modified Conversation

- [ ] Process a conversation — notes extracted
- [ ] Modify the conversation file (add new messages)
- [ ] Run `multi-kb run` again
- [ ] Re-translated: new messages marked `previously_processed: false`
- [ ] New knowledge extracted from added messages only

## Scenario 8: Approval Flow

- [ ] Ensure pending notes exist at `~/.multi-kb/pending/`
- [ ] Run `multi-kb approve` — browser opens with approval UI
- [ ] Review a note — edit title/content if desired
- [ ] Approve to local KB — note appears in local KB git repo
- [ ] Reject a note target — target removed from pending entry
- [ ] When all targets resolved — pending file deleted
- [ ] Server shuts down on idle timeout or all-resolved

## Scenario 9: Dream Cycle

- [ ] Ensure local KB has pending notes (from Scenario 2 + 8 approve to local)
- [ ] Run `multi-kb dream-cycle`
- [ ] Phase 1: Pending notes found, singleton batches created
- [ ] Phase 2: Related active notes found via git grep (keyword-based, derived from pending note title)
- [ ] Phase 3: LLM consolidation — actions applied (keep/merge/split/consolidate)
- [ ] Pending notes transition to active status
- [ ] Git commit created in local KB repo (author: `multi-kb <multi-kb@local>`)
- [ ] Run log shows `dream_cycle` entry with action counts and duration

---

## Result Summary

| Scenario | Status | Notes |
|----------|--------|-------|
| 1. First-Time Setup | | |
| 2. Scheduled Capture | | |
| 3. Hook Injection | | |
| 4. Oversized Conversation | | |
| 5. Extraction Failure + Retry | | |
| 6. Hook Timeout | | |
| 7. Re-Processing | | |
| 8. Approval Flow | | |
| 9. Dream Cycle | | |

**Tester:** _______________  **Date:** _______________
