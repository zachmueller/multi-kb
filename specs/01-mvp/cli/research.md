# Research: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)
**Status:** Open (findings to be populated during Phase 0)

## R-1: Bubbletea Wizard Pattern

**Question:** Which terminal UI library best handles the multi-step setup wizard flow?

**Options:**
- `bubbletea` + `huh` (Charmbracelet form library)
- `survey` (AlecAivazis/survey)
- Raw `bubbletea` with custom components

**Evaluation Criteria:**
- Multi-step form flow support (branching based on answers)
- Terminal compatibility (macOS Terminal, iTerm2, Windows Terminal, WSL)
- Accessibility (screen readers, keyboard-only navigation)
- Maintained and well-documented

**Prototype Task:** Build a 3-step wizard: (1) select from list, (2) text input, (3) confirmation screen. Evaluate all three options.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-2: Bedrock InvokeModel Go SDK Pattern

**Question:** What is the correct Go SDK v2 pattern for calling Bedrock InvokeModel with Claude models?

**Areas to Investigate:**
- `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` — InvokeModel API
- Request body format for Claude models (Messages API vs. legacy)
- How to specify model ID, system prompt, user content
- Response parsing (JSON body extraction from InvokeModelOutput)
- Credential chain with named SSO profiles (`aws_profile` in config)
- Retry and backoff configuration (SDK-level vs. application-level)

**Prototype Task:** Make a working InvokeModel call that sends a system prompt + user message and parses a JSON array response.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-3: Claude Code Conversation Format

**Question:** What is the exact schema of Claude Code conversation files?

**Location:** `~/.claude/projects/<project>/<session>.jsonl`

**Areas to Investigate:**
- How `<project>` directory name maps to the user's project path
- JSONL line schema: message roles, content block structure, tool call/result format
- How to identify conversation boundaries (one file = one conversation?)
- Presence/absence of per-message timestamps
- How to detect file modifications for re-processing

**Prototype Task:** Read a real Claude Code conversation file, document the schema, build a parser.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-4: Notor Conversation Format

**Question:** What is the exact schema of Notor chat history files?

**Location:** `{vault}/notor/history/`

**Areas to Investigate:**
- File format (JSON, JSONL, Markdown, other?)
- Message schema (roles, content, timestamps, tool calls)
- Persona/workflow metadata (where stored, how to extract)
- Per-message timestamps (confirmed available per spec — verify format)
- Conversation boundaries

**Prototype Task:** Read a real Notor history directory, document the schema, build a parser.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-5: Claude Code Hook Registration

**Question:** How to programmatically register a `user_prompt_submit` hook in Claude Code?

**Areas to Investigate:**
- Hook configuration file location and format
- Registration API (file edit? CLI command? JSON schema?)
- How multiple hooks at the same trigger point coexist
- Context provided to the hook at runtime (env vars, stdin, args)
- How the hook's stdout is consumed (prepended to system context? shown to user?)
- First-message detection: what signals are available to determine if this is the first message in a conversation?

**Prototype Task:** Register a test hook that prints "Hello from multi-kb" on first message only.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-6: Notor Hook Registration

**Question:** How to programmatically register a conversation-start hook in Notor?

**Areas to Investigate:**
- Hook configuration mechanism (Obsidian plugin settings? Config file?)
- Registration format
- How multiple hooks at the same trigger point coexist
- Context provided to the hook at runtime
- How hook output is injected into the conversation system context

**Prototype Task:** Register a test hook that injects a test string at conversation start.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-7: Crockford Base32 UID Generation

**Question:** Best approach for generating 16-character Crockford base32 UIDs in Go?

**Areas to Investigate:**
- Existing Go libraries for Crockford base32 (vs. standard base32)
- Crockford base32 alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ` (excludes I, L, O, U)
- Input: 10 bytes from `crypto/rand` = 80 bits → 16 Crockford base32 characters (5 bits each)
- Collision probability at scale (80 bits ≈ 1.2 × 10²⁴ possible values — more than sufficient)

**Prototype Task:** Implement and test the function. Verify output is always exactly 16 chars, valid alphabet, and passes round-trip decode.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-8: Cross-Platform Cron Registration

**Question:** How to safely register scheduled tasks on macOS/Linux (crontab) and Windows (Task Scheduler)?

**macOS/Linux — crontab:**
- Read existing crontab: `crontab -l`
- Append entry with a marker comment (e.g., `# multi-kb scheduled run`)
- Write back: `crontab -` (pipe new content)
- Idempotency: check for marker comment before appending
- Removal: filter out lines with marker comment

**Windows — Task Scheduler:**
- Use `schtasks.exe /Create` with appropriate flags
- XML task definition for more control
- Idempotency: check for existing task by name before creating
- Removal: `schtasks.exe /Delete /TN "multi-kb-run" /F`

**Areas to Investigate:**
- Does `crontab -l` fail on empty crontab? (Yes on some systems — handle gracefully)
- Windows permissions requirements (does it need admin?)
- How to parse the cron expression back for `multi-kb status` next-run display

**Prototype Task:** Implement register/unregister/check on macOS. Implement register/unregister/check on Windows (if available).

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_
