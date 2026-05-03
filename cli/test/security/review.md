# Security Review — CLI QAT-004

**Date:** 2026-05-03
**Scope:** All packages under `cli/internal/`

## Findings

### 1. No Credentials in Config or State Files — PASS

Config and state files store only AWS profile names (`aws_profile`), never actual credentials. AWS credentials are loaded at runtime via the standard AWS SDK credential chain (SSO, instance role, etc.).

### 2. Exclusion Rules Properly Appended — PASS

`internal/extract/prompt.go:BuildExtractionPrompt()` correctly appends exclusion rules from config to the extraction system prompt under the section header "Content exclusion rules — never include in notes destined for non-local KBs".

### 3. Local KB Content Only Leaves Machine When Routed — PASS

`internal/route/route.go:RouteNotes()` applies explicit routing rules from config:
- `routing: "always"` — sends to configured KB
- `routing: "consider"` — only sends if LLM suggested this KB
- Fallback goes to `local/default`

Content only reaches remote KBs when explicitly routed via config + approval flow.

### 4. Approval Server Binds to Localhost Only — PASS

`internal/approve/server.go:69` binds to `127.0.0.1:0` — not accessible from the network.

### 5. No Command Injection in Git Shell-Outs — PASS (fixed)

- All `exec.Command` calls pass arguments as separate strings, not concatenated
- `internal/git/grep.go:validateKeyword()` rejects shell metacharacters
- **Fixed:** Keywords are now escaped with `regexp.QuoteMeta()` before use in `-E` regex patterns to prevent regex injection

### 6. Pending Queue Files Not World-Readable — PASS

- Pending queue files: `0o600`
- Config/state files: `0o600`
- Lock files: `0o600`
- Log files: `0o600`
- **Fixed:** Server-mode note files changed from `0o644` to `0o600`

### 7. Path Traversal Protection — PASS (fixed)

**Fixed:** `internal/approve/handlers.go` now rejects filenames containing `/`, `\`, or `..` in the API route handler, preventing directory traversal via crafted POST requests.

## Fixes Applied

| File | Change | Risk |
|------|--------|------|
| `internal/git/grep.go:82` | `regexp.QuoteMeta(kw)` in title grep pattern | Regex injection via keywords |
| `internal/server/codecommit.go:23` | `0o644` → `0o600` for note files | World-readable server notes |
| `internal/server/dreamcycle.go:332` | `0o644` → `0o600` for note files | World-readable server notes |
| `internal/server/recalllog.go:104` | `0o644` → `0o600` for note files | World-readable server notes |
| `internal/approve/handlers.go:130` | Path traversal check on filename | Directory traversal |

## Accepted Risks

- **Cron expression injection (LOW):** `cronExpr` is user-provided during interactive setup wizard only. The `robfig/cron/v3` parser validates syntax before use, but injection into raw crontab is theoretically possible. Risk is low because the wizard is interactive and user-controlled.
- **LLM exclusion rule compliance (LOW):** Exclusion rules are enforced via LLM prompt instruction, not cryptographic guarantee. This is a documented design trade-off — the LLM may occasionally fail to follow exclusion rules perfectly.
