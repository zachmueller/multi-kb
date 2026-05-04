# End-to-End Scenario Validation Checklist

**CLI QAT-003 — Manual Validation**

Run these scenarios against a built binary (`go build -o bin/multi-kb ./cmd/multi-kb/`) with a deployed CDK stack.

## Prerequisites

- [x] Binary built: `./bin/multi-kb --version` prints version
- [x] AWS credentials available (`aws sts get-caller-identity` works)
- [x] CDK stack deployed and healthy (CloudWatch shows server tick loop)

---

## Scenario 1: First-Time Setup

**Target:** Complete setup in under 10 minutes.

- [ ] Run `./bin/multi-kb setup` — wizard launches with harness selection (Claude Code, Notor)
- [ ] Select Claude Code harness → enter the Claude Code project directory (must exist on disk)
- [ ] Confirm discovered sources, create a default local KB
- [ ] Optionally configure a remote KB (name, endpoint, auth type, AWS profile/region)
- [ ] Set author identity, exclusion rules (optional), approval preset (auto-approve / require-manual / mixed)
- [ ] Set cron schedule for automatic capture
- [x] Wizard completes — verify: `cat ~/.multi-kb/config.yaml` shows `mode: client`, your source directory, and `targets: [{kb: local/default}]`
- [ ] Hook registered: `cat ~/.claude/settings.json` contains a `hooks.UserPromptSubmit` entry invoking `multi-kb hook --harness claude-code` with a 10s timeout
- [ ] Cron registered: `crontab -l` shows a `multi-kb run` entry
- [x] State file created: `cat ~/.multi-kb/state.yaml` exists (may be empty/minimal)
- [x] `./bin/multi-kb status` prints config summary (mode, author, KBs, tracked directories), "No runs recorded yet", and next scheduled run time
- [ ] Total elapsed time from starting `setup` to passing `status` check: under 10 minutes

## Scenario 2: Scheduled Capture

Requires at least one real Claude Code conversation file in the configured source directory.

- [x] Confirm conversation files exist: `ls ~/.claude/projects/` — at least one `.jsonl` file
- [x] Run manually (don't wait for cron): `./bin/multi-kb run`
- [x] Run completes with exit 0 — runs capture processing first, then dream cycle
- [x] Verify run log: `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` — shows `type: "capture"`, `directories_scanned > 0`, `conversations_processed > 0`, `notes_extracted > 0`
- [x] Verify notes routed: `notes_routed` map in run log shows KB name with count > 0
- [x] If approval mode is `auto-approve` with local KB target: notes written directly to `~/.multi-kb/local/default/` with `status: pending`
- [x] State updated: `cat ~/.multi-kb/state.yaml` shows `last_processed` timestamp for the source directory
- [x] If `notes_extracted == 0`: the source directory has no conversations or they've all been processed — point the config at a directory with fresh conversations and re-run

## Scenario 3: Hook Injection

Requires Claude Code to be installed and the hook registered (from Scenario 1).

- [ ] Start a new Claude Code conversation in a directory that matches your configured source
- [ ] On user prompt submission, Claude Code fires the `UserPromptSubmit` hook → `multi-kb hook --harness claude-code` is invoked
- [ ] Verify injection: the hook reads stdin (JSON with conversation context), queries local KB, and returns context to be injected
- [ ] If local KB has content (from Scenario 2): injected context contains relevant knowledge base content
- [ ] If no local KB content yet: hook returns minimal/empty output — no error
- [ ] Conversation proceeds normally after the hook fires (within 10s hook timeout)

## Scenario 4: Oversized Conversation

**Target:** Process a conversation exceeding 700K tokens (~2 MB of text).

- [x] Create or locate a large conversation file. A synthetic one can be generated:
  ```bash
  # Generate a ~3MB synthetic JSONL conversation
  python3 -c "
  import json
  for i in range(500):
      print(json.dumps({'type': 'user', 'content': 'Message ' + str(i) + ': ' + ('x ' * 1000)}))
      print(json.dumps({'type': 'assistant', 'content': 'Response ' + str(i) + ': ' + ('y ' * 1000)}))
  " > /tmp/large-conversation.jsonl
  ```
- [x] Place the file in the configured source directory (must match expected file format for the harness)
- [x] Run: `./bin/multi-kb run`
- [x] Verify chunked extraction: `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` — `notes_extracted > 0`; stderr shows processing activity for the large conversation
- [x] Chunking splits at message boundaries; each chunk processed independently with rolling summary context carried forward
- [x] No hard crash or OOM — process completes with exit 0

## Scenario 5: Extraction Failure + Retry

- [x] Temporarily set an invalid model ID in `~/.multi-kb/config.yaml` under `extraction.model_id` (e.g. `anthropic.claude-invalid-model`)
- [x] Run: `./bin/multi-kb run`
- [x] Observe: each conversation extraction retries up to 3 times on API failure or malformed output (visible via stderr)
- [x] On persistent failure (all 3 attempts): verify `cat ~/.multi-kb/logs/extraction-errors.jsonl | tail -1 | jq .` has an entry with non-empty `error` field and `retries: 3`
- [x] Conversations that fail extraction are skipped — other conversations in the same run still process successfully
- [x] Restore the correct `model_id` in config

## Scenario 6: Hook Timeout

- [ ] Add a remote KB to config with an unreachable endpoint:
  ```yaml
  knowledge_bases:
    - name: slow-kb
      endpoint: https://192.0.2.1/  # TEST-NET — unreachable
      auth: iam
      aws_region: us-east-1
  ```
  And add it as a target in the source's routing rules: `kb: slow-kb`
- [ ] Set a short timeout in config to speed up test: `hook: {timeout: "2s"}`
- [ ] Start a new Claude Code conversation in the configured directory
- [ ] Hook fires — after the configured timeout (default 8s, overridden to 2s above), hook returns with partial results (local KB results only)
- [ ] Conversation proceeds — no hang or crash
- [ ] Verify: `cat ~/.multi-kb/logs/hook-errors.jsonl | tail -1 | jq .` has a timeout or connection-refused entry
- [ ] Restore config (remove slow-kb entry and timeout override)

## Scenario 7: Re-Processing Modified Conversation

- [x] Identify a conversation already processed in Scenario 2 (check `~/.multi-kb/state.yaml` — the `last_processed` timestamp for your source directory marks the cutoff)
- [x] Modify the conversation file: add several new user+assistant message pairs to the end (the file's modification time must be newer than `last_processed`)
- [x] Run: `./bin/multi-kb run`
- [x] Verify: run log shows `notes_extracted > 0` from this re-run
- [x] The translator marks old messages as `previously_processed: true` and new messages as `previously_processed: false`; the extraction prompt only extracts knowledge from `previously_processed: false` messages (using old messages for context only)
- [x] Verify: the newly extracted notes appear in `~/.multi-kb/pending/` or `~/.multi-kb/local/` depending on approval mode

## Scenario 8: Approval Flow

Requires pending notes — either from Scenario 2 (if approval mode is `require-manual-approval`) or manually staged.

- [ ] Confirm pending notes exist: `ls ~/.multi-kb/pending/` — at least one `.json` file
- [ ] Run: `./bin/multi-kb approve`
- [ ] Browser opens automatically to `http://localhost:<port>` with the approval UI
- [ ] Review a note — inspect title and content
- [ ] Edit the title or content of one note via the UI
- [ ] Approve that note to local KB: click Approve → verify `~/.multi-kb/local/default/<uid>.md` is created with `status: pending` in frontmatter (the dream cycle later transitions to `active`)
- [ ] Reject a note: click Reject for a target → the target is removed from the pending entry's target list
- [ ] When all targets on a pending entry are resolved (approved or rejected): verify the `.json` file in `~/.multi-kb/pending/` is deleted
- [ ] Server shuts down after all notes are resolved (or after idle timeout if configured)

## Scenario 9: Dream Cycle

Requires at least one `status: pending` note in a local KB (from Scenario 8 approval to local KB, or from Scenario 2 with auto-approve to local KB).

- [x] Confirm a pending note exists in the local KB: `grep -r "status: pending" ~/.multi-kb/local/`
- [x] Run: `./bin/multi-kb dream-cycle`
- [x] Phase 1 — find pending notes, create singleton batches: errors (if any) appear on stderr as `dream-cycle: phase 1 error for KB "...": ...`
- [x] Phase 2 — find related active notes via git grep (keyword-based, derived from pending note title): related notes found for each batch (may be 0 for a fresh KB — that's valid)
- [x] Phase 3 — LLM consolidation + action application: actions are keep/merge/split/consolidate; errors appear on stderr as `dream-cycle: phase 3 error for batch: ...`
- [x] Verify: the pending note is now `status: active` — `grep -r "status: active" ~/.multi-kb/local/`
- [x] Verify: git commit created in the local KB repo — `git -C ~/.multi-kb/local/default log --oneline -1` shows a commit by `multi-kb <multi-kb@local>`
- [x] Verify: run log has a `dream_cycle` entry — `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` shows `type: "dream_cycle"`, `batches_processed`, and `actions` map with keep/merge/split/consolidate counts

---

## Result Summary

| Scenario | Status | Notes |
|----------|--------|-------|
| 1. First-Time Setup | PARTIAL | Config, state, status verified; wizard/hooks/cron require TTY |
| 2. Scheduled Capture | PASS | 34 conversations, 43 notes extracted, 0 errors; routed to local/default |
| 3. Hook Injection | SKIP | Requires interactive Claude Code session (TTY) |
| 4. Oversized Conversation | PASS | 3.5MB synthetic conversation processed, exit 0, 0 errors, no OOM |
| 5. Extraction Failure + Retry | PASS | Invalid model → 3 retries → error logged with retries=3 |
| 6. Hook Timeout | SKIP | Requires interactive Claude Code session (TTY) |
| 7. Re-Processing | PASS | Modified conversation re-processed; 2 convs, 3 notes, 0 errors |
| 8. Approval Flow | SKIP | Requires browser UI interaction |
| 9. Dream Cycle | PASS | 41 batches, 60 keep/22 merge/16 consolidate; git commits by multi-kb@local |

**New defects found during execution:**

| Bug | Severity | Description |
|-----|----------|-------------|
| E2E-004 | High | Unquoted YAML title values with colons break frontmatter parsing; dream cycle silently skips all affected notes |
| E2E-005 | Low | Dream cycle cascading file-not-found errors when consolidate/merge deletes notes referenced by later batches |

**Tester:** Claude (automated + manual verification)  **Date:** 2026-05-04
