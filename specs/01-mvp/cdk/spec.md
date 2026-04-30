# Multi-Team Knowledge Base — CDK Infrastructure (MVP)

**Created:** 2026-04-30
**Status:** Draft
**Branch:** feature/mvp-infrastructure

## Overview

This specification defines the back-end infrastructure for a multi-team knowledge base system, deployed via AWS CDK. The infrastructure enables teams to store, retrieve, and consolidate knowledge notes through a standardized API layer, backed by CodeCommit (git-based source of truth), S3 (replication layer), and OpenSearch Serverless (semantic search via Bedrock Knowledge Bases).

The CDK stack is designed to be deployed independently by any team — each deployment produces a self-contained knowledge base instance with its own storage, search index, and API surface. The CLI (separate repo) interacts with these instances through the standardized API layer.

## Clarifications

### Session 2026-04-30

- Q: VPC networking — VPC endpoints only vs NAT gateway vs hybrid? → A: VPC endpoints only (see "What is the exact set of VPC endpoints required?" below for full enumerated list). No NAT gateway.
- Q: How does `last-linked-to` get updated on the back end? → A: Deferred to post-MVP. The field remains in the frontmatter schema but is not populated by any process in the MVP.
- Q: How does the EC2 instance get the CLI binary? → A: S3 bucket. Deployer uploads CLI binary to a configurable S3 path; user data script downloads it on boot via the S3 VPC gateway endpoint.
- Q: Should the recall Lambda be VPC-attached to reach OpenSearch Serverless? → A: No. Lambda calls Bedrock Retrieve API only (public endpoint); Bedrock's service role accesses OpenSearch internally. No VPC attachment needed for Lambda.
- Q: Should the EC2 instance query OpenSearch directly or use Bedrock Retrieve API for dream cycle phases? → A: Direct OpenSearch Serverless queries via VPC endpoint. EC2 needs full query DSL control for metadata filtering (e.g., `status: pending`, excluding already-grouped UIDs) and iterative grouping logic in Phase 1. Bedrock Retrieve API is only used by the recall Lambda.
- Q: How does the EC2 instance trigger OpenSearch reindexing from S3? → A: Via Bedrock `StartIngestionJob` / `GetIngestionJob` APIs against the Knowledge Base data source. EC2 does not perform direct OpenSearch indexing operations — the Bedrock KB sync pipeline handles S3 → OpenSearch indexing.
- Q: What is the exact set of VPC endpoints required? → A: Six endpoints total. S3 gateway endpoint (`com.amazonaws.{region}.s3`); interface endpoints for SQS (`com.amazonaws.{region}.sqs`), CodeCommit git (`com.amazonaws.{region}.git-codecommit`), Bedrock Runtime (`com.amazonaws.{region}.bedrock-runtime`) for InvokeModel, Bedrock Agent (`com.amazonaws.{region}.bedrock-agent`) for StartIngestionJob/GetIngestionJob, and OpenSearch Serverless (`com.amazonaws.{region}.aoss`) for direct dream cycle queries. No generic `bedrock` control plane endpoint needed.
- Q: Should VPC interface endpoints span multiple AZs for resilience or single AZ for cost? → A: Single AZ. ASG is pinned to the same AZ as the interface endpoints. Trades AZ failover for ~50% cost reduction on endpoints (~$36.50/month vs ~$73/month). Acceptable for MVP since the system is single-instance with no HA requirement beyond instance recovery within the same AZ.
- Q: How are cron schedules (dream cycle, recall log processing) managed on the EC2 instance? → A: The CLI runs as a single long-running process with built-in timers. One process handles SQS polling, dream cycle scheduling, and recall log processing — managed as a single systemd unit. No system crontab entries.

## User Stories

- As a team deploying a knowledge base, I want a single CDK deployment that provisions all required infrastructure so that I don't have to manually configure interconnected AWS services.
- As a CLI user submitting knowledge, I want the `submitKnowledge` API to validate my input synchronously and return a UID immediately so that I know my submission was accepted before the async processing completes.
- As a CLI user recalling knowledge, I want the `recallKnowledge` API to return semantically relevant notes from the knowledge base so that I can inject useful context into my AI conversations.
- As a KB operator, I want the dream cycle to consolidate and deduplicate knowledge automatically so that the knowledge base remains high-quality without manual curation.
- As a KB operator, I want configurable access control via IAM policies on API Gateway so that I can grant read-only access to external consumers while restricting writes to team members.
- As a KB operator, I want the system to recover gracefully from partial failures (dream cycle crashes, S3 sync failures) so that the knowledge base self-heals on the next cycle without manual intervention.

## Functional Requirements

### FR-1: API Gateway with Two Endpoints

**Description:** The stack deploys an API Gateway (REST API) with `AWS_IAM` authorization exposing two endpoints: `POST /recallKnowledge` and `POST /submitKnowledge`.

**Acceptance Criteria:**
- Both endpoints require valid SigV4-signed requests
- Unauthorized requests receive HTTP 401; insufficient permissions receive HTTP 403
- API Gateway is deployed with a `prod` stage
- CORS is not enabled (CLI is not a browser client)

### FR-2: submitKnowledge Request Validation (Lambda)

**Description:** A Lambda function sits behind the `submitKnowledge` endpoint and validates incoming requests before enqueuing them for processing.

**Acceptance Criteria:**
- Validates `title` is present, non-empty, and ≤ 255 characters
- Validates `content` is present, non-empty, and ≤ 100,000 characters
- Validates `author` is present, non-empty, and ≤ 100 characters
- On validation failure, returns HTTP 400 with a JSON body describing which fields failed
- On success, generates a 16-character Crockford base32 UID for the new note
- On success, enqueues a message to SQS containing the UID, title, content, and author
- On success, returns HTTP 202 with `{ "uid": "<UID>", "request_id": "<API Gateway request ID>" }`

### FR-3: SQS Queue for Ingestion

**Description:** An SQS queue buffers validated knowledge submissions for the EC2 instance to consume.

**Acceptance Criteria:**
- Standard queue (not FIFO — ordering is not required)
- Visibility timeout configured to 5 minutes (sufficient for EC2 batch processing)
- Message retention period of 14 days (maximum)
- Dead-letter queue (DLQ) configured after 3 failed receive attempts
- Messages contain: UID, title, content, author, and submission timestamp

### FR-4: EC2 Instance (Server Mode)

**Description:** A single long-lived EC2 instance runs the CLI binary in server mode. It performs SQS consumption, git operations against CodeCommit, S3 sync, and dream cycle processing.

**Acceptance Criteria:**
- Instance runs Amazon Linux 2023
- Instance has git and the CLI binary pre-installed via user data script (downloads CLI binary from a configurable S3 path on boot via the S3 VPC gateway endpoint)
- CLI binary runs in server mode as a single long-running process managed by a systemd unit, handling SQS polling, dream cycle scheduling, and recall log processing via built-in timers (no system crontab)
- Instance has IAM role with permissions for: SQS (receive/delete), CodeCommit (full repo access), S3 (read/write to KB bucket), OpenSearch Serverless (data plane access for direct queries during dream cycle), Bedrock (InvokeModel for dream cycle LLM calls, StartIngestionJob/GetIngestionJob for triggering KB data source sync)
- Instance polls SQS, batches ~5–10 messages, and commits them as Markdown files to CodeCommit in a single git commit per batch
- After each commit, instance syncs changed files to S3 (incremental, not full repo)
- Instance manages a lock file for concurrency control between ingestion batches and dream cycles
- Instance is deployed in a private subnet (no public IP); outbound access via VPC endpoints (no NAT gateway)
- VPC endpoints (6 total):
  - S3 gateway endpoint (`com.amazonaws.{region}.s3`) — CLI binary download, S3 sync (free)
  - SQS interface endpoint (`com.amazonaws.{region}.sqs`) — queue polling
  - CodeCommit git interface endpoint (`com.amazonaws.{region}.git-codecommit`) — git clone/push
  - Bedrock Runtime interface endpoint (`com.amazonaws.{region}.bedrock-runtime`) — InvokeModel for dream cycle LLM calls
  - Bedrock Agent interface endpoint (`com.amazonaws.{region}.bedrock-agent`) — StartIngestionJob/GetIngestionJob for KB data source sync
  - OpenSearch Serverless interface endpoint (`com.amazonaws.{region}.aoss`) — direct query DSL access for dream cycle phases
- All interface endpoints and the EC2 ASG are pinned to a single AZ to minimize endpoint costs (~$36.50/month for 5 interface endpoints in 1 AZ)

### FR-5: CodeCommit Repository

**Description:** A CodeCommit repository serves as the git-backed source of truth for all knowledge notes.

**Acceptance Criteria:**
- Repository is created by the CDK stack with a configurable name
- Notes are stored as `<UID>.md` files in the repository root (flat structure, no subdirectories for MVP)
- Each note file contains YAML frontmatter with: `uid`, `title`, `status`, `last-updated`, `last-linked-to`, `last-recalled`, `consolidated-from-notes`, and `author`
- `last-linked-to` is present in the schema but not populated by any MVP process (deferred to post-MVP)
- The EC2 instance is the sole writer to this repository

### FR-6: S3 Bucket (Replication Layer)

**Description:** An S3 bucket holds a one-way replicated copy of knowledge notes from CodeCommit, serving as the data source for OpenSearch Serverless indexing.

**Acceptance Criteria:**
- Bucket is created by the CDK stack with a configurable name prefix
- Only changed files are synced from CodeCommit (not full repo clone on each commit)
- Files deleted from CodeCommit are also deleted from S3
- Bucket also stores recall logs under `recall-logs/<YYYY-MM-DD>/<request-id>.json`
- Bucket has versioning disabled (CodeCommit provides version history)
- Bucket has server-side encryption enabled (SSE-S3)

### FR-7: OpenSearch Serverless Collection

**Description:** An OpenSearch Serverless collection indexes the S3 bucket contents and provides semantic search capabilities via Bedrock Knowledge Bases.

**Acceptance Criteria:**
- Collection type is "VECTORSEARCH"
- Data access policy grants the EC2 instance role and the Bedrock Knowledge Base service role read/write access to the collection's indexes
- Network policy allows access from VPC (EC2 instance) and from the Bedrock service (for Knowledge Base Retrieve API queries)
- Lambda functions do not access OpenSearch directly — they call the Bedrock Retrieve API, which accesses OpenSearch via its own service role
- Encryption policy uses AWS-owned key for MVP
- Reindexing is triggered indirectly by the EC2 instance via Bedrock `StartIngestionJob` API (Bedrock KB sync pipeline handles S3 → OpenSearch indexing)

### FR-8: Bedrock Knowledge Base

**Description:** A Bedrock Knowledge Base is configured on top of the OpenSearch Serverless collection to provide the Retrieve API for semantic search.

**Acceptance Criteria:**
- Knowledge Base data source points to the S3 bucket
- Embedding model is configurable (default: Amazon Titan Embeddings V2)
- Chunking strategy is "no chunking" (each knowledge note is its own chunk — notes are atomic by design)
- Knowledge Base ID is exported as a CloudFormation output for reference
- The recall Lambda uses the Bedrock Retrieve API to query this Knowledge Base

### FR-9: recallKnowledge Lambda

**Description:** A Lambda function behind the `recallKnowledge` endpoint queries the Bedrock Knowledge Base and optionally performs a coverage assessment.

**Acceptance Criteria:**
- Accepts `query` (string, required) and `limit` (integer, optional, default 10) parameters
- Calls Bedrock Knowledge Base Retrieve API with the query
- Optionally filters results to `status: active` notes only (configurable via environment variable `EXCLUDE_PENDING`, default `true`)
- Returns JSON array of notes with `uid`, `title`, `content`, and `score` fields, ordered by descending score
- Performs coverage assessment when top score < 0.3 (threshold configurable via environment variable):
  - Sends query + result summaries to a fast LLM (configurable model ID via environment variable)
  - If a gap is detected, executes a follow-up Bedrock Retrieve query
  - Deduplicates combined results by UID
  - Falls back to original results silently on any failure in the coverage step
- After returning results, asynchronously writes a recall log to S3 (`recall-logs/<YYYY-MM-DD>/<request-id>.json`)

### FR-10: Dream Cycle Execution

**Description:** The EC2 instance runs dream cycle consolidation on a configurable cron schedule, processing pending notes through grouping, context retrieval, and LLM-driven consolidation.

**Acceptance Criteria:**
- Schedule is configurable (default: every 6 hours), managed by the CLI process's built-in timer (not system crontab)
- Acquires lock file before processing; skips if lock is held with active heartbeat
- Stale lock (no heartbeat update within 30 minutes) can be force-acquired
- Phase 0: Syncs CodeCommit → S3, triggers Bedrock KB data source sync (`StartIngestionJob`), polls `GetIngestionJob` for completion, waits up to 10 minutes
- Phase 1: Queries OpenSearch directly (via VPC endpoint, using OpenSearch query DSL) for `status: pending` notes, groups by similarity (max 10 per batch)
- Phase 2: For each batch, queries OpenSearch directly for related `status: active` notes (max 10 per batch)
- Phase 3: Sends each work item to LLM, applies returned actions (keep/merge/split/consolidate), commits per batch
- Phase 4: Final S3 sync, triggers Bedrock KB data source sync (`StartIngestionJob`), clears manifest, releases lock
- Tracks per-batch completion in a manifest file for partial failure recovery
- Uses configurable Bedrock model for consolidation LLM calls

### FR-11: Recall Log Processing (Daily Batch)

**Description:** The EC2 instance processes recall logs daily to update `last-recalled` timestamps on knowledge notes.

**Acceptance Criteria:**
- Runs once per day, managed by the CLI process's built-in timer (not system crontab)
- Scans S3 objects under the previous day's `recall-logs/` prefix
- Collects all recalled UIDs
- Updates `last-recalled` frontmatter on each referenced note in CodeCommit
- Silently skips UIDs for notes that no longer exist (deleted during consolidation)
- Commits all `last-recalled` updates as a single git commit

## Non-Functional Requirements

### NFR-1: Performance

**Description:** The system should respond to API calls within acceptable latency bounds and handle expected throughput.

**Acceptance Criteria:**
- `submitKnowledge` returns HTTP 202 within 2 seconds (p99) under normal conditions
- `recallKnowledge` returns results within 5 seconds (p99) including coverage assessment
- `recallKnowledge` without coverage assessment returns within 3 seconds (p99)
- System handles up to 100 `submitKnowledge` calls per minute sustained without message loss
- SQS-to-commit latency (time from enqueue to git commit) is under 60 seconds for non-locked periods

### NFR-2: Reliability

**Description:** The system should be resilient to component failures and self-healing.

**Acceptance Criteria:**
- SQS DLQ captures messages that fail processing after 3 attempts
- Dream cycle partial failures are recoverable on next run via manifest
- Lock file heartbeat TTL prevents permanent deadlocks from EC2 crashes
- EC2 instance auto-recovers via Auto Scaling Group (min: 1, max: 1, desired: 1), pinned to a single AZ (same AZ as VPC interface endpoints)
- S3 sync failures on one cycle are caught by the next cycle's Phase 0

### NFR-3: Security

**Description:** The system should enforce authentication and least-privilege access.

**Acceptance Criteria:**
- All API endpoints require AWS_IAM authorization (SigV4)
- EC2 instance runs in a private subnet with no public IP
- EC2 instance role follows least-privilege (scoped to specific SQS queue ARN, CodeCommit repo ARN, S3 bucket ARN, OpenSearch collection ARN, Bedrock model ARNs)
- S3 bucket blocks public access
- OpenSearch Serverless collection is not publicly accessible
- No secrets stored in code or environment variables (IAM roles handle all auth)

### NFR-4: Observability

**Description:** Operators can monitor system health and debug issues.

**Acceptance Criteria:**
- API Gateway access logs enabled (CloudWatch Logs)
- Lambda functions log to CloudWatch with structured JSON output
- EC2 instance logs dream cycle progress, errors, and batch outcomes to CloudWatch Logs (via CloudWatch agent)
- DLQ depth is surfaced as a CloudWatch metric
- CloudWatch alarms configured for: DLQ messages > 0, EC2 instance unhealthy, dream cycle lock held > 60 minutes

### NFR-5: Cost Efficiency

**Description:** The infrastructure should be cost-effective for small-to-medium team deployments.

**Acceptance Criteria:**
- EC2 instance size is configurable (default: t3.small — sufficient for the single-writer workload)
- OpenSearch Serverless uses minimum OCU configuration (2 OCUs for indexing, 2 for search)
- Lambda functions use ARM64 architecture (Graviton) for cost savings
- S3 uses standard storage class (no lifecycle rules for MVP)
- No NAT gateway — all outbound traffic routed via VPC endpoints (S3 gateway endpoint is free; 5 interface endpoints in a single AZ for SQS, CodeCommit git, Bedrock Runtime, Bedrock Agent, and OpenSearch Serverless — ~$36.50/month)

## User Scenarios & Testing

### Primary Flow: Submit Knowledge

1. CLI sends `POST /submitKnowledge` with title, content, author (SigV4-signed)
2. API Gateway validates IAM auth → routes to validation Lambda
3. Lambda validates fields, generates UID, enqueues to SQS, returns HTTP 202 with UID
4. EC2 instance polls SQS, collects batch of messages
5. EC2 writes `<UID>.md` files with frontmatter, commits to CodeCommit
6. EC2 syncs committed files to S3
7. Note is available in OpenSearch after next index refresh

### Primary Flow: Recall Knowledge

1. CLI sends `POST /recallKnowledge` with query and optional limit (SigV4-signed)
2. API Gateway validates IAM auth → routes to recall Lambda
3. Lambda calls Bedrock Knowledge Base Retrieve API
4. Lambda evaluates coverage (if top score < threshold)
5. Lambda returns JSON array of matching notes
6. Lambda asynchronously writes recall log to S3

### Alternative Flow: Dream Cycle Consolidation

1. Cron triggers dream cycle on EC2
2. EC2 acquires lock (or skips if held)
3. Phase 0: Sync and reindex
4. Phase 1: Group pending notes into batches
5. Phase 2: Find related active notes per batch
6. Phase 3: LLM evaluates each work item, EC2 applies actions, commits per batch
7. Phase 4: Final sync, reindex, release lock

### Error Conditions

- **submitKnowledge with invalid fields:** Lambda returns HTTP 400 with field-level error details; nothing enqueued
- **SQS message processing failure:** Message returns to queue after visibility timeout; after 3 failures, moved to DLQ
- **Dream cycle crash mid-batch:** Lock heartbeat expires after 30 min; next run force-acquires lock; manifest indicates which batches completed; unfinished batches are retried
- **OpenSearch reindex timeout (>10 min):** Dream cycle proceeds best-effort with current index state
- **Bedrock API throttling during dream cycle:** Retries up to 3 times with exponential backoff per batch; persistent failure marks batch as failed in manifest
- **EC2 instance termination:** ASG launches replacement; on boot, instance resumes SQS polling and dream cycle scheduling; any in-flight batch is retried (SQS visibility timeout returns message to queue)

## Success Criteria

- A team can deploy the full stack with a single `cdk deploy` command and have a working knowledge base API within 30 minutes
- Knowledge submitted via `submitKnowledge` is searchable via `recallKnowledge` within 10 minutes under normal conditions
- The dream cycle processes 100 pending notes through consolidation within a single 6-hour cycle
- The system operates continuously for 7 days without manual intervention or data loss
- A second team can deploy their own independent instance using the same CDK code with only configuration changes (stack name, repo name, etc.)
- Cross-team recall works: a CLI configured with multiple KB endpoints can query all of them and receive merged results

## Key Entities

### Knowledge Note (Markdown File)

- **Filename:** `<UID>.md`
- **Frontmatter properties:**
  - `uid` (string, required) — 16-character Crockford base32 identifier
  - `title` (string, required) — Succinct note title
  - `status` (string, required) — `pending` | `active`
  - `author` (string, required) — Submitter identity
  - `last-updated` (ISO 8601 timestamp) — Last body content modification
  - `last-linked-to` (ISO 8601 timestamp) — Last time another note linked to this one
  - `last-recalled` (ISO 8601 timestamp) — Last time recalled via `recallKnowledge`
  - `consolidated-from-notes` (array of strings) — Format: `<filename> (<commit-hash>)`
- **Body:** Markdown content with inline `[[UID|Title]]` wikilinks

### SQS Message (Ingestion)

- `uid` (string) — Pre-generated by validation Lambda
- `title` (string)
- `content` (string)
- `author` (string)
- `submitted_at` (ISO 8601 timestamp)

### Recall Log (S3 Object)

- **Path:** `recall-logs/<YYYY-MM-DD>/<request-id>.json`
- **Body:** `{ "timestamp": "...", "query": "...", "recalled_uids": [...] }`

### Dream Cycle Manifest

- **Location:** Local file on EC2 instance
- **Content:** Tracks batch IDs, completion status, and timestamps for partial failure recovery

## CDK Stack Structure

### Configurable Parameters (CDK Context / Props)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `repoName` | `"multi-kb"` | CodeCommit repository name |
| `bucketPrefix` | `"multi-kb"` | S3 bucket name prefix |
| `ec2InstanceType` | `"t3.small"` | EC2 instance type |
| `embeddingModelId` | `"amazon.titan-embed-text-v2:0"` | Bedrock embedding model |
| `consolidationModelId` | `"anthropic.claude-sonnet-4-20250514"` | Dream cycle LLM model |
| `coverageModelId` | `"anthropic.claude-haiku-3-20240307"` | Coverage assessment LLM model |
| `dreamCycleSchedule` | `"0 */6 * * *"` | Cron expression for dream cycle |
| `excludePendingFromRecall` | `true` | Whether to filter pending notes from recall |
| `coverageScoreThreshold` | `0.3` | Score below which coverage assessment fires |
| `cliBinaryS3Uri` | _(required)_ | S3 URI of the CLI binary to install on the EC2 instance (e.g., `s3://my-artifacts/multi-kb-cli/latest/multi-kb-cli-linux-amd64`) |
| `vpcId` | _(none — creates new VPC)_ | Optional existing VPC to deploy into |

### Stack Outputs

| Output | Description |
|--------|-------------|
| `ApiEndpoint` | API Gateway endpoint URL |
| `ApiId` | API Gateway REST API ID |
| `RepoCloneUrl` | CodeCommit repository HTTPS clone URL |
| `KnowledgeBaseId` | Bedrock Knowledge Base ID |
| `BucketName` | S3 bucket name |
| `CollectionEndpoint` | OpenSearch Serverless collection endpoint |
| `Ec2InstanceId` | EC2 instance ID |

## Assumptions

- Deployers have AWS CDK v2 installed and bootstrapped in their target account/region
- Deployers have sufficient IAM permissions to create all resource types in the stack
- Bedrock model access has been requested and granted in the target region for both the embedding model and LLM models
- OpenSearch Serverless is available in the target region
- A single EC2 instance is sufficient for MVP throughput (single-writer, no horizontal scaling)
- The CLI binary is built and published separately; this CDK stack handles downloading/installing it on the EC2 instance via user data
- Teams using internal Amazon deployments will layer Federate auth on top of this base stack (out of scope for this CDK spec — handled via a separate internal overlay)

## Out of Scope

- **Federate auth integration:** Internal Amazon deployments layer this separately; this CDK stack only supports `AWS_IAM` auth
- **Multi-region deployment:** Single-region only for MVP
- **Horizontal scaling of EC2:** Single instance; no auto-scaling beyond health recovery
- **Custom domain names / Route53:** API Gateway default domain only
- **WAF / rate limiting at API Gateway:** Rely on IAM auth as the access control layer
- **Backup/restore automation:** CodeCommit provides git history; no additional backup infrastructure
- **CI/CD pipeline for the CDK stack itself:** Deployers run `cdk deploy` manually
- **Monitoring dashboards:** Alarms are configured but no pre-built CloudWatch dashboards
- **Cost allocation tags beyond stack-level:** Basic stack tags only
- **VPC peering or cross-account networking:** Each KB instance is self-contained
- **CLI binary build/release pipeline:** Handled in the `multi-kb-cli` repo
- **`last-linked-to` timestamp updates:** Field is in the frontmatter schema but not populated by any MVP process
