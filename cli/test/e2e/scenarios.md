# End-to-End Scenario Validation Checklist

**CLI QAT-003 — Manual Validation**

Run these scenarios against a built binary (`go build -o bin/multi-kb ./cmd/multi-kb/`) with a deployed CDK stack.

## Prerequisites

- [ ] Binary built: `./bin/multi-kb --version` prints version
- [ ] AWS credentials available (`aws sts get-caller-identity` works)
- [ ] CDK stack deployed and healthy (CloudWatch shows server tick loop)

---

## Scenario 1: First-Time Setup

**Target:** Complete setup in under 10 minutes.

- [ ] Run `./bin/multi-kb setup` — wizard launches with harness selection (Claude Code, Notor)
- [ ] Select Claude Code harness → enter the Claude Code project directory (must exist on disk)
- [ ] Confirm discovered sources, create a default local KB
- [ ] Optionally configure a remote KB (name, endpoint, auth type, AWS profile/region)
- [ ] Set author identity, exclusion rules (optional), approval preset (auto-approve / require-manual / mixed)
- [ ] Set cron schedule for automatic capture
- [ ] Wizard completes — verify: `cat ~/.multi-kb/config.yaml` shows `mode: client`, your source directory, and `targets: [{kb: local/default}]`
- [ ] Hook registered: `cat ~/.claude/settings.json` contains a `hooks.UserPromptSubmit` entry invoking `multi-kb hook --harness claude-code` with a 10s timeout
- [ ] Cron registered: `crontab -l` shows a `multi-kb run` entry
- [ ] State file created: `cat ~/.multi-kb/state.yaml` exists (may be empty/minimal)
- [ ] `./bin/multi-kb status` prints config summary (mode, author, KBs, tracked directories), "No runs recorded yet", and next scheduled run time
- [ ] Total elapsed time from starting `setup` to passing `status` check: under 10 minutes

## Scenario 2: Scheduled Capture

Requires at least one real Claude Code conversation file in the configured source directory.

- [ ] Confirm conversation files exist: `ls ~/.claude/projects/` — at least one `.jsonl` file
- [ ] Run manually (don't wait for cron): `./bin/multi-kb run`
- [ ] Run completes with exit 0 — runs capture processing first, then dream cycle
- [ ] Verify run log: `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` — shows `type: "capture"`, `directories_scanned > 0`, `conversations_processed > 0`, `notes_extracted > 0`
- [ ] Verify notes routed: `notes_routed` map in run log shows KB name with count > 0
- [ ] If approval mode is `require-manual-approval`: pending queue populated at `~/.multi-kb/pending/` (at least one `.json` file)
- [ ] If approval mode is `auto-approve` with local KB target: notes written directly to `~/.multi-kb/local/default/` with `status: pending`
- [ ] State updated: `cat ~/.multi-kb/state.yaml` shows `last_processed` timestamp for the source directory
- [ ] If `notes_extracted == 0`: the source directory has no conversations or they've all been processed — point the config at a directory with fresh conversations and re-run

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

- [ ] Create or locate a large conversation file. A synthetic one can be generated:
  ```bash
  # Generate a ~3MB synthetic JSONL conversation
  python3 -c "
  import json
  for i in range(500):
      print(json.dumps({'type': 'user', 'content': 'Message ' + str(i) + ': ' + ('x ' * 1000)}))
      print(json.dumps({'type': 'assistant', 'content': 'Response ' + str(i) + ': ' + ('y ' * 1000)}))
  " > /tmp/large-conversation.jsonl
  ```
- [ ] Place the file in the configured source directory (must match expected file format for the harness)
- [ ] Run: `./bin/multi-kb run`
- [ ] Verify chunked extraction: `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` — `notes_extracted > 0`; stderr shows processing activity for the large conversation
- [ ] Chunking splits at message boundaries; each chunk processed independently with rolling summary context carried forward
- [ ] No hard crash or OOM — process completes with exit 0

## Scenario 5: Extraction Failure + Retry

- [ ] Temporarily set an invalid model ID in `~/.multi-kb/config.yaml` under `extraction.model_id` (e.g. `anthropic.claude-invalid-model`)
- [ ] Run: `./bin/multi-kb run`
- [ ] Observe: each conversation extraction retries up to 3 times on API failure or malformed output (visible via stderr)
- [ ] On persistent failure (all 3 attempts): verify `cat ~/.multi-kb/logs/extraction-errors.jsonl | tail -1 | jq .` has an entry with non-empty `error` field and `retries: 3`
- [ ] Conversations that fail extraction are skipped — other conversations in the same run still process successfully
- [ ] Restore the correct `model_id` in config

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

- [ ] Identify a conversation already processed in Scenario 2 (check `~/.multi-kb/state.yaml` — the `last_processed` timestamp for your source directory marks the cutoff)
- [ ] Modify the conversation file: add several new user+assistant message pairs to the end (the file's modification time must be newer than `last_processed`)
- [ ] Run: `./bin/multi-kb run`
- [ ] Verify: run log shows `notes_extracted > 0` from this re-run
- [ ] The translator marks old messages as `previously_processed: true` and new messages as `previously_processed: false`; the extraction prompt only extracts knowledge from `previously_processed: false` messages (using old messages for context only)
- [ ] Verify: the newly extracted notes appear in `~/.multi-kb/pending/` or `~/.multi-kb/local/` depending on approval mode

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

- [ ] Confirm a pending note exists in the local KB: `grep -r "status: pending" ~/.multi-kb/local/`
- [ ] Run: `./bin/multi-kb dream-cycle`
- [ ] Phase 1 — find pending notes, create singleton batches: errors (if any) appear on stderr as `dream-cycle: phase 1 error for KB "...": ...`
- [ ] Phase 2 — find related active notes via git grep (keyword-based, derived from pending note title): related notes found for each batch (may be 0 for a fresh KB — that's valid)
- [ ] Phase 3 — LLM consolidation + action application: actions are keep/merge/split/consolidate; errors appear on stderr as `dream-cycle: phase 3 error for batch: ...`
- [ ] Verify: the pending note is now `status: active` — `grep -r "status: active" ~/.multi-kb/local/`
- [ ] Verify: git commit created in the local KB repo — `git -C ~/.multi-kb/local/default log --oneline -1` shows a commit by `multi-kb <multi-kb@local>`
- [ ] Verify: run log has a `dream_cycle` entry — `cat ~/.multi-kb/logs/runs.jsonl | tail -1 | jq .` shows `type: "dream_cycle"`, `batches_processed`, and `actions` map with keep/merge/split/consolidate counts

---

## Result Summary

| Scenario | Status | Notes |
|----------|--------|-------|
| 1. First-Time Setup | PARTIAL | Config created manually (wizard requires TTY); status command verified |
| 2. Scheduled Capture | PASS | 46 conversations, 178 notes extracted, 0 errors, routed to local/default |
| 3. Hook Injection | SKIP | Requires interactive Claude Code session (TTY) |
| 4. Oversized Conversation | SKIP | Requires generating large synthetic file + significant Bedrock spend |
| 5. Extraction Failure + Retry | PASS | Invalid model → 3 retries → error logged with retries=3 |
| 6. Hook Timeout | SKIP | Requires interactive Claude Code session (TTY) |
| 7. Re-Processing | PASS | Modified conversation picked up, re-processed with 0 errors |
| 8. Approval Flow | SKIP | Requires browser UI interaction |
| 9. Dream Cycle | PASS | 109 batches, 60 keep actions, git commits by multi-kb <multi-kb@local> |

**Tester:** Claude (automated)  **Date:** 2026-05-04
