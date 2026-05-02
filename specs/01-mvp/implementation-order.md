# Implementation Order: CDK + CLI Cross-Component Execution Plan

**Created:** 2026-05-01
**CDK Tasks:** [cdk/tasks.md](cdk/tasks.md) (56 tasks)
**CLI Tasks:** [cli/tasks.md](cli/tasks.md) (87 tasks)
**Status:** In Progress — Wave 2 complete

## Overview

CDK and CLI are **~80% independent**. The integration surface is narrow, defined by shared contracts under `cdk/contracts/` and `cli/contracts/`. Most work can proceed in parallel across both components.

### Hard Sequencing Constraints

There are exactly **3 irreducible cross-component dependencies**:

1. **CLI binary must be built before CDK can be deployed.** CDK `CMP-003` (user data script) downloads the CLI binary from S3 at EC2 boot time. CDK *development* (`cdk synth`, unit tests, snapshot tests) is never blocked.
2. **CDK must be deployed before CLI server mode can be integration-tested.** CLI `SRV-001` through `SRV-007` require live SQS, CodeCommit, S3, OpenSearch, and Bedrock KB infrastructure.
3. **UID encoding must produce identical output** in both TypeScript (CDK `LMB-001`) and Go (CLI `FND-004`). The 5 shared test vectors enforce correctness independently.

The irreducible sequence:

```
CLI binary build (BLD-001) --> upload to S3 --> cdk deploy --> CLI server-mode integration tests
```

### What Does NOT Block

- CLI Phases 0-7 (75 tasks) have **zero runtime CDK dependency** (all client-mode, local operation)
- CDK Phases 0-9 can be developed and unit-tested without any CLI binary
- All prompt authoring tasks (`PRM-001` through `PRM-005`) are fully independent

---

## Wave 1: Scaffolding

Both components initialize their project structure. No cross-component dependency.

### CDK

| Task | Description |
|------|-------------|
| ENV-001 | CDK project initialization (`cdk init`, directory structure, `cdk synth` works) |
| ENV-002 [P] | Stack configuration and props (`MultiKbStackProps`, 12 parameters) |
| ENV-003 [P] | Development tooling (ESLint, Prettier, Jest, scripts) |

### CLI

| Task | Description |
|------|-------------|
| ENV-001 | Go module initialization (`go.mod`, directory structure, `go build` works) |
| ENV-002 [P] | Cobra root command + 10 subcommand stubs |
| ENV-003 [P] | Development tooling (Makefile, golangci-lint, build matrix) |
| PRM-001 [P] | Extraction system prompt (can start immediately) |
| PRM-002 [P] | Dream cycle consolidation prompt |
| PRM-003 [P] | Coverage assessment prompt |
| PRM-004 [P] | Keyword derivation prompt |
| PRM-005 [P] | Chunk summarization prompt |

**Exit criteria:** `cdk synth` produces a valid template. `go build ./cmd/multi-kb/` succeeds.

---

## Wave 2: Foundation

Both components build core infrastructure. The only cross-component touchpoint is the UID implementation.

### CDK

| Task | Description |
|------|-------------|
| NET-001 | VPC and subnet (single AZ) |
| NET-002 | S3 gateway endpoint |
| NET-004 | Security groups (EC2 SG + endpoint SG) |
| NET-003 | Interface VPC endpoints (9 endpoints, depends on NET-001 + NET-004) |
| STR-001 [P] | S3 bucket |
| STR-002 [P] | CodeCommit repository |
| STR-003 [P] | SQS queue with DLQ |
| STR-004 | Stack outputs for storage |
| SRC-002 | Encryption policy (prerequisite for collection) |
| LMB-001 | Shared Lambda utilities including UID generation |

### CLI

| Task | Description |
|------|-------------|
| FND-001 | Config YAML loading and validation |
| FND-002 | State YAML loading and atomic writing |
| FND-003 [P] | Lock file with heartbeat |
| FND-004 [P] | Crockford base32 UID generation |
| FND-005 | Local KB git repository creation |
| FND-006 | Local KB note file writing (depends on FND-004 + FND-005) |
| FND-007 | Git grep recall with match-count ranking (depends on FND-005 + FND-006) |
| FND-008 [P] | Pending queue file management |
| FND-009 [P] | Run log and error log appending |
| FND-010 | Status command (depends on FND-001 + FND-002 + FND-008 + FND-009) |
| FND-011 [P] | Token counting approximation |
| TRN-001 | Intermediate JSONL format types |
| TRN-002 | Claude Code translator (depends on TRN-001 + FND-002) |
| TRN-003 [P] | Notor translator (depends on TRN-001 + FND-002) |
| TRN-004 | Tool interaction summarization (depends on TRN-001 + FND-011) |

### Checkpoint: UID Parity

After CDK `LMB-001` and CLI `FND-004` are both complete, verify both implementations produce identical output for all 5 shared test vectors:

| Input | Expected Output |
|-------|----------------|
| `[0x00 x 10]` | `0000000000000000` |
| `[0xFF x 10]` | `ZZZZZZZZZZZZZZZZ` |
| `[0x00..0x09]` | `000G40R40M30E209` |
| `[0xDE,0xAD,0xBE,0xEF,0xCA,0xFE,0xBA,0xBE,0x00,0x42]` | `VTPVXVYAZTXBW022` |
| `"HelloWorld"` bytes | `91JPRV3FAXQQ4V34` |

---

## Wave 3: Core Logic

CDK builds search infrastructure and Lambda handlers. CLI builds extraction pipeline and hook injection. Both sides implement their halves of the shared API contracts.

### CDK

| Task | Description |
|------|-------------|
| SRC-001 | OpenSearch Serverless collection (depends on SRC-002) |
| SRC-003 | Network policy — dual access (depends on NET-003) |
| KBS-002 | Bedrock KB service role (depends on STR-001 + SRC-001) |
| SRC-006 | Custom resource — vector index creation (depends on SRC-001 + SRC-004 + NET-003) |
| SRC-004 | Data access policy (depends on SRC-001 + CMP-001 + KBS-002 + SRC-006 Lambda role) |
| KBS-001 | Bedrock Knowledge Base (depends on SRC-001 + KBS-002 + SRC-006) |
| KBS-003 | Bedrock KB data source (depends on KBS-001 + STR-001) |
| LMB-002 | submitKnowledge Lambda handler (depends on LMB-001) |
| LMB-003 | submitKnowledge CDK construct (depends on LMB-002 + STR-003) |
| LMB-004 | recallKnowledge Lambda handler (depends on LMB-001 + PRM-003) |
| LMB-005 | recallKnowledge CDK construct (depends on LMB-004 + KBS-001 + STR-001) |

### CLI

| Task | Description |
|------|-------------|
| EXT-001 | Bedrock client wrapper (retry, backoff, SSO) |
| EXT-002 | Extraction system prompt construction (depends on FND-001) |
| EXT-003 | Single-pass extraction (depends on EXT-001 + EXT-002 + TRN-001) |
| EXT-004 | Chunked extraction for oversized conversations (depends on EXT-003 + FND-011 + PRM-005) |
| EXT-005 | Extraction error handling (depends on EXT-003 + FND-009) |
| EXT-006 | Routing engine (depends on FND-001 + FND-008) |
| EXT-007 | Remote KB submitKnowledge client (depends on EXT-001 + FND-001) |
| EXT-008 | Capture processing orchestrator `multi-kb process` (depends on all Phase 1-3 tasks) |
| HKI-001 | Claude Code hook registration |
| HKI-002 [P] | Notor hook registration |
| HKI-003 | Remote KB recallKnowledge client (depends on EXT-001 + FND-001) |
| HKI-004 | LLM-derived keyword generation (depends on EXT-001) |
| HKI-005 | Rank-based result interleaving |
| HKI-006 | Markdown injection formatting (depends on FND-008) |
| HKI-007 | Hook entry point `multi-kb hook` (depends on FND-001 + FND-007 + FND-009 + HKI-003/004/005/006) |

### Checkpoint: Contract Compliance

Code-review both sides of the shared contracts for schema alignment. No deployment needed.

| Contract | CDK Side | CLI Side | Verify |
|----------|----------|----------|--------|
| `submit-knowledge` | LMB-002 (Lambda handler) | EXT-007 (remote submit client) | Request/response shapes, validation rules, error format |
| `recall-knowledge` | LMB-004 (Lambda handler) | HKI-003 (remote recall client) | Response array shape, field names, score type |
| SQS message | LMB-002 (produces) | SRV-003 (consumes, Wave 6) | `{uid, title, content, author, submitted_at}` schema |

---

## Wave 4: API + Dream Cycle

CDK wires API Gateway. CLI builds local dream cycle and setup wizard.

### CDK

| Task | Description |
|------|-------------|
| API-001 | REST API and `prod` stage |
| API-002 | submitKnowledge endpoint (depends on API-001 + LMB-003) |
| API-003 [P] | recallKnowledge endpoint (depends on API-001 + LMB-005) |
| API-004 | Stack output for API endpoint |
| SRC-005 | Stack outputs for search infrastructure |

### CLI

| Task | Description |
|------|-------------|
| DRM-001 | Dream cycle orchestrator (depends on FND-003 + FND-002) |
| DRM-002 | Phase 1 — singleton batch creation (depends on FND-005 + FND-006) |
| DRM-003 | Phase 2 — git grep related note retrieval (depends on FND-007) |
| DRM-004 | Phase 3 — LLM consolidation + action application (depends on EXT-001 + FND-006 + FND-005 + PRM-002) |
| DRM-005 | Dream cycle commands (depends on DRM-001 + EXT-008) |
| WIZ-001 | Terminal wizard — harness selection + source discovery (depends on ENV-002 + FND-001) |
| WIZ-002 | Terminal wizard — KB configuration + routing (depends on WIZ-001 + FND-005) |
| WIZ-003 | Hook auto-registration during setup (depends on WIZ-001 + HKI-001 + HKI-002) |
| WIZ-004 | Cron registration |
| WIZ-005 | Cron expression parsing for status display (depends on WIZ-004) |
| WIZ-006 [P] | Standalone subcommands: add-source, add-kb (depends on FND-001 + WIZ-001) |
| APR-001 | Embedded HTML/CSS/JS assets |
| APR-002 | HTTP server lifecycle (depends on APR-001) |
| APR-003 | API handlers (depends on APR-002 + FND-008 + FND-006 + EXT-007) |
| APR-004 | Approve command wiring (depends on APR-002 + APR-003) |

---

## Wave 5: Compute + Build

CDK builds EC2 infrastructure. CLI builds cross-platform binaries. The CLI binary produced here is what CDK will deploy.

### CDK

| Task | Description |
|------|-------------|
| CMP-001 | EC2 IAM role (depends on STR-001 + STR-002 + STR-003 + SRC-001 + KBS-001 + KBS-003) |
| CMP-002 | Launch template (depends on CMP-001 + NET-004) |
| CMP-003 | User data script (depends on CMP-001 + CMP-004 + all storage/search/KB outputs + OBS-001) |
| CMP-004 | Auto Scaling Group (depends on CMP-002 + NET-001) |
| CMP-005 | Stack output for compute |
| OBS-001 | CloudWatch log groups (depends on API-001 + LMB-003 + LMB-005) |
| OBS-002 [P] | CloudWatch alarms (depends on STR-003 + CMP-004) |

### CLI

| Task | Description |
|------|-------------|
| BLD-001 | Cross-platform build matrix (depends on ENV-003) |
| BLD-002 [P] | Binary size optimization (depends on BLD-001) |

### Checkpoint: Config Schema Alignment

Verify that CDK `CMP-003`'s `config.yaml` template produces output matching the schema validated by CLI `SRV-001`. Compare against `cdk/contracts/server-config.md`. No deployment needed.

---

## Wave 6: Stack Wiring + Server Mode

CDK assembles and tests the full stack. CLI builds server-mode logic.

### CDK

| Task | Description |
|------|-------------|
| WIR-001 | Main stack assembly (depends on all Phase 1-7 constructs) |
| WIR-002 | Stack snapshot test (depends on WIR-001) |
| QAT-001 | CDK assertion test coverage (depends on all constructs) |
| QAT-002 [P] | Lambda handler unit tests (depends on LMB-001 + LMB-002 + LMB-004) |
| QAT-003 [P] | Security review (depends on WIR-001) |
| QAT-004 [P] | Multi-tenancy validation (depends on WIR-001) |

### CLI

| Task | Description |
|------|-------------|
| SRV-001 | Server config loading and validation (depends on FND-001) |
| SRV-002 | Tick loop and activity dispatch (depends on SRV-001 + FND-003) |
| SRV-003 [P] | SQS polling and batching (depends on SRV-001) |
| SRV-004 [P] | CodeCommit git operations (depends on FND-005) |
| SRV-005 [P] | Incremental S3 sync (depends on SRV-004) |
| SRV-006 | Server dream cycle — OpenSearch-backed (depends on DRM-004 + SRV-004 + SRV-005) |
| SRV-007 | Daily recall log processing (depends on SRV-004 + SRV-005) |

### GATE: CLI Binary Upload to S3

After `BLD-001` produces a Linux arm64 binary, upload it to S3. Update the CDK context `cliBinaryS3Uri`. **This unblocks CDK deployment.**

> **Tip:** To unblock CDK deployment testing earlier, upload a minimal stub binary (even just the Cobra skeleton from CLI `ENV-002`). The EC2 instance will boot, download, and attempt to start it — validating the download/install/systemd pipeline even if the server functionality isn't complete yet.

---

## Wave 7: First Deployment + Integration Testing

This is the first wave where both components must interact at runtime. Sequential execution required.

### Step 1: Deploy

```
cdk deploy --context cliBinaryS3Uri=s3://... 
```

Stack creates all infrastructure (15-20 minutes). EC2 instance boots, downloads CLI binary, starts server mode.

### Step 2: Validate Highest-Risk Assumption First

| Test | CDK Task | What to Verify |
|------|----------|----------------|
| **QAT-006** | Bedrock KB metadata extraction | Upload test note with YAML frontmatter to S3, trigger ingestion, call Retrieve, confirm `metadata.uid` and `metadata.title` are present |

**This is the single highest-risk integration point.** If Bedrock does not extract YAML frontmatter as queryable metadata, both CDK `LMB-004` and CLI recall flows need rework. Run this test first.

### Step 3: Smoke Tests

| Test | What to Verify |
|------|----------------|
| Submit flow | `POST /submitKnowledge` returns 202, SQS message appears, EC2 processes it, CodeCommit commit created, S3 sync completes |
| Recall flow | `POST /recallKnowledge` returns 200 with results (after KB sync), recall log appears in S3 |
| Server config | CDK-generated `config.yaml` passes CLI `SRV-001` validation |
| EC2 health | CloudWatch logs show server tick loop running |

### Step 4: Full End-to-End (CDK QAT-005)

| Scenario | What to Verify |
|----------|----------------|
| Dream cycle | Pending notes processed, status changed to active, S3 sync + reindex |
| EC2 recovery | Terminate instance, ASG launches replacement, server resumes |
| SSM access | `aws ssm start-session` connects |
| Alarms | DLQ alarm fires on test message |
| Recall logs | Lambda writes recall log, CLI SRV-007 reads and updates `last-recalled` |

---

## Wave 8: Quality + Hardening

### CDK

| Task | Description |
|------|-------------|
| QAT-005 | Full post-deploy integration checklist completion |

### CLI

| Task | Description |
|------|-------------|
| QAT-001 | Unit test coverage pass (`go test -race ./...`) |
| QAT-002 [P] | Integration test suite (real-service tests with `//go:build integration`) |
| QAT-003 [P] | End-to-end scenario validation (manual checklist against deployed stack) |
| QAT-004 [P] | Security review (command injection, localhost-only binding, file permissions) |

---

## Risk Areas

### HIGH: Bedrock KB Metadata Extraction (CDK QAT-006)

The `recallKnowledge` response mapping assumes Bedrock extracts YAML frontmatter fields (`uid`, `title`) as queryable metadata in Retrieve API results. If this assumption is wrong, both CDK `LMB-004` and CLI recall flows (`HKI-003`, `HKI-005`) need rework.

**Mitigation:** Deploy a minimal standalone Bedrock KB with a single test markdown note before Wave 7 to validate this assumption early. The earlier this is tested, the less rework risk.

### MEDIUM: server-config.md Schema Divergence

CDK `CMP-003` templates `config.yaml` using CDK construct outputs. CLI `SRV-001` validates and loads this file. If either side drifts from `cdk/contracts/server-config.md`, the EC2 instance fails to start.

**Mitigation:** Treat `server-config.md` as immutable during implementation. Any change requires agreement from both sides. Run the Wave 5 checkpoint (generate sample config from CDK template, validate with CLI).

### MEDIUM: Recall Log Format (No Formal Contract)

CDK Lambda writes `recall-logs/<date>/<request-id>.json`; CLI `SRV-007` reads them. The schema is specified inline in `cdk/data-model.md` and `cdk/contracts/recall-knowledge.md`, not in a dedicated contract file.

**Mitigation:** Consider extracting a formal `contracts/recall-log.md` before implementation begins.

### LOW: UID Format Divergence

Well-mitigated by 5 shared test vectors covering edge cases (all zeros, all ones, mixed bytes). Both implementations are simple (~15 lines) and independently testable.

### LOW: CLI Binary Timing for CDK Deploy

Process dependency only — CDK development is never blocked. Only the first real deployment needs the binary.

**Mitigation:** Upload a stub binary early (Wave 5 tip above).
