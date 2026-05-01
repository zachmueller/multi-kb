# Implementation Plan: Multi-KB CDK Infrastructure — MVP

**Created:** 2026-05-01
**Specification:** [spec.md](spec.md)
**Cross-Reference:** [CLI spec](../cli/spec.md) | [CLI plan](../cli/plan.md) | [CLI contracts](../cli/contracts/)
**Status:** Planning

## Technical Context

### Architecture Decisions

| Area | Decision | Rationale |
|------|----------|-----------|
| **IaC Framework** | AWS CDK v2 (TypeScript) | Spec-defined; native AWS construct library, type-safe infrastructure |
| **Lambda Runtime** | Node.js 22 (`nodejs22.x`) on ARM64 (Graviton) | Spec-defined; fastest cold starts, natural CDK pairing, cost savings |
| **API Layer** | API Gateway REST API with `AWS_IAM` auth | Spec-defined; SigV4 auth, `prod` stage |
| **Message Queue** | SQS Standard Queue + DLQ | Spec-defined; async ingestion buffer, 14-day retention |
| **Source of Truth** | CodeCommit git repository | Spec-defined; git-backed note storage |
| **Replication Layer** | S3 bucket (SSE-S3, no versioning) | Spec-defined; one-way replica for OpenSearch indexing |
| **Search** | OpenSearch Serverless (VECTORSEARCH) + Bedrock Knowledge Base | Spec-defined; semantic search via Bedrock Retrieve API |
| **Compute** | Single EC2 instance (Amazon Linux 2023) via ASG (1/1/1) | Spec-defined; server-mode CLI binary |
| **Networking** | Private subnet, VPC endpoints only (no NAT) | Spec-defined; 10 endpoints, single AZ |
| **Operator Access** | AWS Systems Manager Session Manager | Spec-defined; no SSH, no bastion |
| **Observability** | CloudWatch Logs + CloudWatch Alarms (no actions) | Spec-defined; metrics-only for MVP |

### Technology Stack

| Component | Choice | Notes |
|-----------|--------|-------|
| CDK Version | AWS CDK v2.x (latest) | TypeScript constructs |
| Language | TypeScript 5.x | CDK stack + Lambda handlers |
| Lambda Bundling | `NodejsFunction` (esbuild) | Tree-shaking, minification, source maps |
| Testing | Jest + CDK assertions (`aws-cdk-lib/assertions`) | Snapshot tests + fine-grained assertions |
| Linting | ESLint + Prettier | Consistent code style |
| Package Manager | npm | Standard for CDK TypeScript projects |

### Technology Stack Rationale

**CDK v2 TypeScript**
- **Decision:** Use AWS CDK v2 with TypeScript
- **Rationale:** CDK v2 unifies all AWS constructs in a single package. TypeScript provides type safety for infrastructure definitions and shares the language with Lambda handlers. The spec targets CDK deployment.
- **Alternatives Considered:** CDK Python (less IDE support for CDK constructs), CloudFormation YAML (no abstractions), Terraform (different ecosystem)
- **Trade-offs:** Requires Node.js for synthesis; TypeScript compilation adds a build step but catches errors at compile time

**NodejsFunction for Lambda Bundling**
- **Decision:** Use the CDK `NodejsFunction` construct with esbuild
- **Rationale:** Handles TypeScript compilation, tree-shaking, and bundling automatically. Produces minimal deployment packages. Source maps enable readable CloudWatch stack traces.
- **Alternatives Considered:** Manual `lambda.Function` with pre-built zip (more control, more CI complexity), Docker-based bundling (slower)
- **Trade-offs:** esbuild doesn't support all TypeScript features (decorators), but Lambda handlers don't need them

**Jest + CDK Assertions**
- **Decision:** Use Jest with CDK's `assertions` module
- **Rationale:** CDK's built-in assertion library enables testing that synthesized templates contain expected resources, properties, and relationships. Jest is the standard CDK testing framework.
- **Alternatives Considered:** None — this is the canonical approach for CDK testing
- **Trade-offs:** Snapshot tests can be brittle on minor CDK version upgrades; use fine-grained assertions for critical properties

### Integration Points

| Integration | Direction | Protocol | Notes |
|-------------|-----------|----------|-------|
| CLI → API Gateway | Inbound | HTTPS + SigV4 | submitKnowledge + recallKnowledge |
| API GW → submitKnowledge Lambda | Internal | Lambda proxy integration | Validation + SQS enqueue |
| API GW → recallKnowledge Lambda | Internal | Lambda proxy integration | Bedrock Retrieve + coverage assessment |
| submitKnowledge Lambda → SQS | Internal | AWS SDK | Validated message enqueue |
| recallKnowledge Lambda → Bedrock KB | Internal | Bedrock Retrieve API | Semantic search |
| recallKnowledge Lambda → Bedrock Runtime | Internal | InvokeModel | Coverage assessment LLM call |
| recallKnowledge Lambda → S3 | Internal | PutObject | Recall log writing |
| EC2 (CLI) → SQS | Internal (VPC endpoint) | AWS SDK | Message polling + deletion |
| EC2 (CLI) → CodeCommit | Internal (VPC endpoint) | git over HTTPS | Clone, commit, push |
| EC2 (CLI) → S3 | Internal (VPC endpoint) | AWS SDK | Incremental sync, recall log reads |
| EC2 (CLI) → OpenSearch Serverless | Internal (VPC endpoint) | OpenSearch query DSL | Dream cycle Phase 1/2 |
| EC2 (CLI) → Bedrock Runtime | Internal (VPC endpoint) | InvokeModel | Dream cycle Phase 3 LLM |
| EC2 (CLI) → Bedrock Agent | Internal (VPC endpoint) | StartIngestionJob/GetIngestionJob | KB data source sync |
| Bedrock KB → S3 | Internal (AWS service) | S3 read | Data source ingestion |
| Bedrock KB → OpenSearch Serverless | Internal (AWS service) | Index writes | Vector embeddings |

### Cross-Spec Compatibility Notes

Verified against CLI spec and CLI plan contracts to ensure CDK/CLI alignment:

1. **submitKnowledge request contract:** CLI sends `{ title, content, author }`. Lambda validates: title ≤255 chars non-empty, content ≤100K chars non-empty, author ≤100 chars non-empty. Returns HTTP 202 `{ uid, request_id }`. Error format: `{ "errors": { "<field>": "<reason>" } }` (flat object, not array). Aligned with [CLI contract](../cli/contracts/submit-knowledge.md).
2. **recallKnowledge request contract:** CLI sends `{ query, limit? }`. Lambda returns `[{ uid, title, content, score }]` sorted by descending score. Default limit 10. Aligned with [CLI contract](../cli/contracts/recall-knowledge.md).
3. **SQS message schema:** `{ uid, title, content, author, submitted_at }` — consumed by server-mode CLI on EC2. No additional Lambda-specific fields.
4. **Note frontmatter:** Both specs agree on the schema: `uid`, `title`, `status`, `author`, `last-updated`, `last-linked-to`, `last-recalled`, `consolidated-from-notes`. `last-linked-to` not populated in MVP.
5. **UID format:** 16-character Crockford base32, generated by submitKnowledge Lambda. CLI generates independent UIDs for local KBs. No correlation.
6. **EC2 server-mode behavior:** The CDK stack provisions infrastructure; the CLI binary (downloaded from S3 on boot) handles all server-mode logic (SQS consumption, dream cycles, recall log processing). CDK owns the IAM role, VPC, and systemd unit setup — not the application logic.
7. **Recall logs:** Written by recallKnowledge Lambda to `recall-logs/<YYYY-MM-DD>/<request-id>.json`. Read by server-mode CLI on EC2 during daily batch processing. Lambda needs `s3:PutObject` scoped to `recall-logs/*`; EC2 needs `s3:GetObject` + `s3:ListBucket` on the same prefix.
8. **OpenSearch access pattern:** Lambda does NOT access OpenSearch directly — it calls Bedrock Retrieve API. EC2 accesses OpenSearch directly via VPC endpoint for dream cycle Phase 1/2. This drives the data access policy: both the EC2 role and Bedrock KB service role need OpenSearch access, but the Lambda roles do not.
9. **Dream cycle lock:** The CLI binary on EC2 manages its own lock file locally. CDK doesn't need to provision any locking infrastructure — the single-tick model and single-instance ASG handle concurrency.

## Phase 0: Research & Architecture

### Technology Research Tasks

Most infrastructure choices are specified directly. The following require targeted investigation:

#### R-1: OpenSearch Serverless Collection Setup via CDK ✅
- **Research Task:** Determine the correct CDK L1/L2 constructs for creating an OpenSearch Serverless collection with VECTORSEARCH type
- **Questions to Answer:** Is there an L2 construct, or must we use `CfnCollection`? How to create encryption, network, and data access policies? How do policies reference the Bedrock KB service role before it exists (circular dependency)?
- **Success Criteria:** Working CDK code that creates a collection accessible by both the EC2 role and Bedrock KB service
- **Key Concern:** OpenSearch Serverless policies use JSON policy documents separate from IAM — three policy types (encryption, network, data access) must all be configured correctly
- **Resolution:** No L2 constructs exist — use L1 constructs (`CfnCollection`, `CfnSecurityPolicy`, `CfnAccessPolicy`). Encryption policy must be created before collection via `addDependency()`. No circular dependency: CDK tokens resolve all forward references between roles, collection, and policies. Policy names must match `^[a-z][a-z0-9-]{2,31}$`. CDK tokens in `JSON.stringify()` synthesize to `Fn::Join`/`Fn::Sub` intrinsics. See [research.md R-1](research.md#r-1-opensearch-serverless-collection-setup-via-cdk).

#### R-2: Bedrock Knowledge Base CDK Construct ✅
- **Research Task:** Determine how to create a Bedrock Knowledge Base with S3 data source via CDK
- **Questions to Answer:** Is there an L2 construct (`bedrock.CfnKnowledgeBase`, `bedrock.CfnDataSource`)? How to configure "no chunking" strategy? How to wire the S3 data source? How does the KB service role get created and permissioned for both S3 and OpenSearch?
- **Success Criteria:** Working CDK code that creates a KB pointed at the S3 bucket with "no chunking", using the OpenSearch collection as the vector store
- **Key Concern:** The service role for Bedrock KB needs permissions to read S3 AND write to OpenSearch — this is a separate role from the Lambda or EC2 roles
- **Resolution:** No L2 constructs — use `CfnKnowledgeBase` and `CfnDataSource` (L1). Three new findings that reshape Phase C:
  1. **Missing permission:** Bedrock KB service role needs `bedrock:InvokeModel` on the foundation model ARN (for embeddings). This was not in the original plan.
  2. **Index pre-creation required:** Bedrock does NOT auto-create the OpenSearch vector index via CDK/CloudFormation (console-only feature). A custom resource Lambda must create the index before the KB. Index needs: `knn_vector` field (1024 dims for Titan V2, `faiss`/`hnsw` engine), plus `text` and `metadata` fields matching the KB `fieldMapping`.
  3. **Field mappings required:** All three field mappings (`vectorField`, `textField`, `metadataField`) are required and must match between index schema and KB configuration.
  See [research.md R-2](research.md#r-2-bedrock-knowledge-base-cdk-construct).

#### R-3: EC2 User Data Script Best Practices
- **Research Task:** Design the user data script for Amazon Linux 2023 that installs the CLI binary, CloudWatch agent, clones CodeCommit, and starts the systemd unit
- **Questions to Answer:** Correct ordering of operations? How to configure the CloudWatch agent for structured JSON log shipping? How to template the config.yaml for server mode (inject CDK outputs)? How to handle user data script failures (signal ASG)?
- **Success Criteria:** Working user data script that boots a fresh instance to running state with the CLI process active
- **Key Concern:** The systemd unit, config.yaml, and CloudWatch agent config must all be templated with CDK-resolved values (SQS URL, repo name, bucket name, etc.)

#### R-4: VPC Endpoint Security Group Configuration ✅
- **Research Task:** Determine the security group rules for 9 interface VPC endpoints + 1 gateway endpoint
- **Questions to Answer:** Can all interface endpoints share one security group? What inbound rules are needed (HTTPS/443 from EC2 SG)? Does the gateway endpoint need special route table configuration?
- **Success Criteria:** Working VPC with all 10 endpoints, EC2 instance can reach all services
- **Key Concern:** The OpenSearch Serverless VPC endpoint has its own network policy requirement (separate from the security group)
- **Resolution:** Two SGs (EC2 + endpoint). EC2 SG: outbound 443 to endpoint SG. Endpoint SG: inbound 443 from EC2 SG. All 9 interface endpoints share the endpoint SG. Key findings: (1) AOSS uses `opensearchserverless.CfnVpcEndpoint` (L1), not `ec2.InterfaceVpcEndpoint` — takes `securityGroupIds` as `string[]`. (2) Must set `open: false` on `InterfaceVpcEndpoint` to prevent CDK from auto-adding permissive `0.0.0.0/0` ingress. (3) S3 gateway endpoint needs route table association only, no SG. See [research.md R-4](research.md#r-4-vpc-endpoint-security-group-configuration).

#### R-5: Crockford Base32 UID Generation in Node.js ✅
- **Research Task:** Implement or find a library for 16-character Crockford base32 UID generation in the submitKnowledge Lambda
- **Questions to Answer:** Existing npm package? If not, how to encode 10 random bytes to 16 Crockford base32 characters? Use `crypto.randomBytes(10)` as entropy source.
- **Success Criteria:** Function that generates collision-resistant 16-char UIDs with the correct alphabet (`0-9A-HJKMNP-TV-Z`, excluding I, L, O, U)
- **Note:** Must match the CLI's format exactly (CLI plan R-7). Both produce 80-bit entropy encoded as 16 Crockford base32 chars.
- **Resolution:** Zero-dependency implementation using bit-buffer encoding. `crypto.randomBytes(10)` → bit-buffer extraction (5 bits at a time, MSB first, using `>>>` unsigned right shift) → 16 uppercase Crockford chars. No npm package needed — the encoding is ~12 lines, which is better for Lambda cold starts. `encodeCrockford()` exported separately from `generateUid()` for deterministic testing against 5 shared test vectors (verified identical output with CLI R-7 Go implementation). JavaScript integer safety is not a concern — the bit buffer never exceeds 2^15 (12 bits max accumulation). See [research.md R-5](research.md#r-5-crockford-base32-uid-generation-in-nodejs).

#### R-6: OpenSearch Serverless Network Policy for Dual Access ✅
- **Research Task:** Configure OpenSearch Serverless network policy to allow access from both VPC (EC2) and from the Bedrock service (for Retrieve API queries)
- **Questions to Answer:** Can a single network policy have both VPC and public/service access rules? Does Bedrock access OpenSearch via a service-linked role or a customer-managed role? How does the `AllowFromPublic` rule interact with VPC-only access for EC2?
- **Success Criteria:** EC2 can query OpenSearch via VPC endpoint AND Bedrock Retrieve API can query the same collection
- **Key Concern:** If the network policy is VPC-only, Bedrock Retrieve API (called by Lambda, which is NOT in the VPC) may not be able to reach OpenSearch. Need dual-access network policy.
- **Resolution:** **IMPORTANT CHANGE:** Do NOT use `AllowFromPublic: true` — it silently overrides `SourceVPCEs` and `SourceServices`, making the collection fully public. Instead use `AllowFromPublic: false` with `SourceVPCEs: [<vpce-id>]` + `SourceServices: ["bedrock.amazonaws.com"]`. Bedrock accesses OpenSearch via AWS internal service-to-service networking (not the customer's VPC). The correct field name is `SourceVPCEs` (not `SourceVPCEndpoints`). This provides three-layer security: network origin + data access policy + IAM. See [research.md R-6](research.md#r-6-opensearch-serverless-network-policy-for-dual-access).

#### R-7: Lambda Proxy Integration Response Format
- **Research Task:** Confirm the exact response format required by API Gateway Lambda proxy integration for each status code
- **Questions to Answer:** What is the required response shape (`statusCode`, `headers`, `body`)? How to return structured JSON errors? How does API Gateway handle Lambda errors vs. Lambda-returned error status codes?
- **Success Criteria:** Both Lambda handlers return correctly formatted responses for all status codes (200, 202, 400, 401, 403, 500)

### Research Deliverables
- `research.md` — Consolidated findings for R-1 through R-7

## Phase 1: Design & Contracts

**Prerequisites:** Research complete (R-1 through R-7 resolved)

### CDK Stack Architecture

The stack is a single CDK stack (`MultiKbStack`) with logically grouped constructs:

```
lib/
├── multi-kb-stack.ts              # Main stack: wires all constructs together
├── constructs/
│   ├── networking.ts              # VPC, subnets, VPC endpoints, security groups
│   ├── storage.ts                 # S3 bucket, CodeCommit repository
│   ├── search.ts                  # OpenSearch Serverless collection + policies
│   ├── knowledge-base.ts          # Bedrock Knowledge Base + S3 data source
│   ├── api.ts                     # API Gateway REST API + resource/method definitions
│   ├── submit-lambda.ts           # submitKnowledge Lambda + IAM role
│   ├── recall-lambda.ts           # recallKnowledge Lambda + IAM role
│   ├── compute.ts                 # EC2 ASG, launch template, instance role, user data
│   └── observability.ts           # CloudWatch log groups, alarms
├── lambda/
│   ├── submit/
│   │   └── index.ts               # submitKnowledge handler
│   ├── recall/
│   │   └── index.ts               # recallKnowledge handler
│   └── shared/
│       ├── uid.ts                  # Crockford base32 UID generation
│       ├── response.ts            # API Gateway response helpers
│       └── validation.ts          # Field validation utilities
bin/
└── multi-kb.ts                    # CDK app entry point
test/
├── multi-kb-stack.test.ts         # Stack-level snapshot + assertions
├── constructs/
│   ├── networking.test.ts
│   ├── storage.test.ts
│   ├── search.test.ts
│   ├── api.test.ts
│   └── compute.test.ts
└── lambda/
    ├── submit.test.ts             # Handler unit tests
    └── recall.test.ts             # Handler unit tests
```

### Construct Dependency Graph

```
MultiKbStack
  │
  ├── Networking (VPC, subnets, endpoints, security groups)
  │     │
  ├── Storage (S3 bucket, CodeCommit repo)
  │     │         │
  ├── Search (OpenSearch Serverless collection + policies)
  │     │         │                uses: Networking (VPC endpoint ID)
  │     │         │
  ├── KnowledgeBase (Bedrock KB + data source)
  │     │         │  uses: Storage (S3 bucket), Search (collection ARN)
  │     │         │
  ├── SubmitLambda
  │     │         uses: Storage (SQS queue)
  │     │
  ├── RecallLambda
  │     │         uses: KnowledgeBase (KB ID), Storage (S3 bucket for recall logs)
  │     │
  ├── Api (API Gateway)
  │     │         uses: SubmitLambda, RecallLambda
  │     │
  ├── Compute (EC2 ASG + launch template)
  │     │         uses: Networking (VPC, subnet, SGs), Storage (SQS, S3, CodeCommit),
  │     │               Search (OpenSearch endpoint), KnowledgeBase (KB ID, data source ID)
  │     │
  └── Observability (log groups, alarms)
                    uses: Storage (SQS DLQ), Compute (ASG)
```

### Data Model

See [data-model.md](data-model.md) for full entity definitions including knowledge note Markdown format, SQS messages, recall logs, and all configuration parameters.

### API Contracts

See [contracts/](contracts/) directory:
- [contracts/submit-knowledge.md](contracts/submit-knowledge.md) — Server-side submitKnowledge endpoint contract
- [contracts/recall-knowledge.md](contracts/recall-knowledge.md) — Server-side recallKnowledge endpoint contract

### Development Environment Setup

See [quickstart.md](quickstart.md) for developer onboarding, CDK bootstrapping, and deployment instructions.

## Implementation Phases

### Phase A: Project Skeleton & Networking

Build the CDK project and VPC infrastructure:

1. CDK app initialization (`cdk init app --language typescript`)
2. Stack configuration with all configurable parameters (CDK context)
3. VPC construct: VPC, private subnet, single-AZ placement
4. VPC endpoints: S3 gateway + 9 interface endpoints
5. Security groups: EC2 SG, VPC endpoint SG (inbound 443 from EC2 SG)
6. Stack outputs: VPC ID, subnet ID

### Phase B: Storage Layer

Build the persistent storage infrastructure:

1. S3 bucket (SSE-S3, public access blocked, no versioning)
2. CodeCommit repository (configurable name)
3. SQS standard queue (5-min visibility, 14-day retention)
4. SQS dead-letter queue (3 receive attempts)
5. Stack outputs: bucket name, repo clone URL, queue URL, DLQ URL

### Phase C: Search Infrastructure

Build OpenSearch Serverless + Bedrock Knowledge Base:

1. OpenSearch Serverless collection (VECTORSEARCH type) — L1 `CfnCollection`
2. Encryption policy (AWS-owned key) — must be created before collection via `addDependency()`
3. Network policy — `AllowFromPublic: false` with `SourceVPCEs` + `SourceServices: ["bedrock.amazonaws.com"]` (R-6)
4. Data access policy (EC2 role + Bedrock KB service role) — CDK tokens resolve role ARNs
5. Bedrock KB service role (S3 read + OpenSearch write + **`bedrock:InvokeModel` for embeddings**) (R-2)
6. **Custom resource Lambda to pre-create OpenSearch vector index** — Bedrock does not auto-create via CDK (R-2)
7. Bedrock Knowledge Base with S3 data source — L1 `CfnKnowledgeBase`, `CfnDataSource`
8. "No chunking" chunking strategy (`chunkingStrategy: 'NONE'`)
9. Stack outputs: collection endpoint, KB ID, data source ID

### Phase D: Lambda Functions

Build the API handler functions:

1. Shared utilities: Crockford base32 UID, response helpers, validation
2. submitKnowledge Lambda: validation, UID generation, SQS enqueue, HTTP 202/400 responses
3. submitKnowledge IAM role: SQS SendMessage
4. recallKnowledge Lambda: Bedrock Retrieve, coverage assessment, recall log writing
5. recallKnowledge IAM role: Bedrock Retrieve, Bedrock InvokeModel, S3 PutObject (recall-logs/*)
6. Lambda unit tests (handler logic in isolation)

### Phase E: API Gateway

Wire up the REST API:

1. API Gateway REST API with `prod` stage
2. `POST /submitKnowledge` resource + method + Lambda proxy integration
3. `POST /recallKnowledge` resource + method + Lambda proxy integration
4. `AWS_IAM` authorization on both methods
5. Access logging to CloudWatch Logs
6. Stack output: API endpoint URL

### Phase F: EC2 Compute

Build the server-mode compute infrastructure:

1. EC2 IAM role with all required permissions (SQS, CodeCommit, S3, OpenSearch, Bedrock, SSM)
2. Launch template: Amazon Linux 2023, configurable instance type, instance profile
3. User data script: install CLI binary from S3, install CloudWatch agent, clone CodeCommit repo, template config.yaml, create systemd unit, start service
4. Auto Scaling Group: min 1, max 1, desired 1, single-AZ (same AZ as endpoints)
5. CloudWatch agent configuration: ship structured JSON logs from CLI process
6. Stack output: instance ID

### Phase G: Observability

Build monitoring infrastructure:

1. CloudWatch Log Groups: API Gateway access logs, Lambda function logs, EC2 CLI process logs
2. CloudWatch Alarms:
   - DLQ messages > 0
   - EC2 instance unhealthy (ASG health check)
   - Dream cycle lock held > 60 minutes (custom metric from CLI logs)
3. No alarm actions (metrics only for MVP — operators poll console)

### Phase H: Integration Testing & Hardening

End-to-end validation:

1. CDK snapshot tests for all constructs
2. Fine-grained CDK assertions for critical IAM policies, security groups, VPC endpoint configs
3. Deploy to test account and validate:
   - submitKnowledge → SQS → EC2 commit → S3 sync → OpenSearch index
   - recallKnowledge → Bedrock Retrieve → results + recall log
   - Dream cycle: pending → active via consolidation
   - EC2 replacement: ASG launches new instance, clones repo, resumes processing
4. Validate stack outputs are correct and usable by CLI
5. Test second deployment with different stack name (multi-tenancy)

## Implementation Readiness Validation

### Technical Completeness Check
- [x] All technology choices made and documented (CDK v2 TypeScript, Node.js 22 Lambda, API GW REST)
- [x] Data model covers all functional requirements (see data-model.md)
- [x] API contracts support all user scenarios (see contracts/)
- [x] Security requirements addressed (IAM auth, private subnet, least-privilege roles, no public access)
- [x] Performance considerations documented (Lambda sizing, SQS batching, OpenSearch OCUs)
- [x] Integration points defined (10 VPC endpoints, Lambda→SQS, Lambda→Bedrock, EC2→all services)
- [x] Development environment specified (see quickstart.md)

### Quality Validation
- [x] Architecture supports single-`cdk deploy` requirement
- [x] Security model enforces least-privilege per component
- [x] Data model supports all frontmatter fields and lifecycle states
- [x] API design matches CLI contract expectations exactly
- [x] Documentation covers all configurable parameters and stack outputs

## Risk Assessment

### Technical Risks

**High Risk:**
- ~~**OpenSearch Serverless Dual-Access Network Policy:**~~ **RESOLVED (R-6).** Use `AllowFromPublic: false` with `SourceVPCEs` + `SourceServices: ["bedrock.amazonaws.com"]`. Bedrock uses AWS internal service networking, not the customer's VPC. Single-rule policy handles both access paths. Critical gotcha documented: `AllowFromPublic: true` silently overrides VPC/service restrictions.

- ~~**Bedrock KB + OpenSearch Serverless Circular Dependencies:**~~ **RESOLVED (R-1, R-2).** No circular dependency exists. CDK tokens resolve all forward references. Collection depends only on encryption policy. Roles use collection ARN tokens. Data access policy uses role ARN tokens. All resolve to CloudFormation intrinsics at synthesis time.

- **OpenSearch Vector Index Pre-Creation (NEW — from R-2):** Bedrock KB does NOT auto-create the OpenSearch vector index via CDK/CloudFormation. A custom resource Lambda must create the index (1024-dim `knn_vector` field, `faiss`/`hnsw` engine) after the collection is available but before the KB is created.
  - **Mitigation:** New task SRC-006 adds a CDK custom resource Lambda that creates the index via the OpenSearch REST API. The Lambda needs VPC access (to reach the collection via AOSS endpoint) and data access policy permissions.
  - **Contingency:** If custom resource approach is unreliable, the index could be created manually as a one-time post-deploy step (documented in quickstart.md). However, this breaks the single-`cdk deploy` success criterion.

**Medium Risk:**
- **User Data Script Reliability:** The EC2 user data script must install the CLI binary, CloudWatch agent, clone CodeCommit, template config, and start systemd — all on first boot. Any failure leaves the instance in a bad state.
  - **Mitigation:** R-3 research. Use `set -euxo pipefail` at script top. Signal ASG on failure. Test on a fresh Amazon Linux 2023 AMI.
  - **Contingency:** Add a health check that validates the CLI process is running; ASG will replace unhealthy instances.

- **OpenSearch Serverless Minimum OCU Cost:** 2 OCUs for indexing + 2 for search = 4 OCUs minimum. At ~$0.24/OCU-hr, this is ~$700/month. This is a significant fixed cost for small teams.
  - **Mitigation:** Document the cost in the deployment guide. This is an inherent OpenSearch Serverless constraint, not a design choice. Post-MVP, investigate time-based OCU scaling or alternative search backends.

**Low Risk:**
- **CodeCommit Deprecation:** AWS announced CodeCommit is no longer accepting new customers (as of mid-2024). Existing customers can continue using it.
  - **Mitigation:** The spec explicitly uses CodeCommit. Document this risk. Post-MVP, evaluate migration to S3-backed git (gitiles) or another git hosting solution.
  - **Contingency:** The CLI's git operations are standard git — switching the remote backend requires only config changes, not code changes.

- **Lambda Cold Start Latency:** recallKnowledge Lambda at 1024 MB has ~200-500ms cold starts on ARM64 Node.js 22. Within the 5-second p99 target but could stack with Bedrock Retrieve latency.
  - **Mitigation:** 1024 MB allocation provides proportional CPU. Provisioned concurrency is available post-MVP if needed. Node.js 22 has the fastest managed runtime cold starts.

### Dependencies and Assumptions

**External Dependencies:**
- AWS CDK v2 bootstrapped in target account/region
- Bedrock model access granted (embedding model + LLM models)
- OpenSearch Serverless available in target region
- CLI binary built and published to S3 (separate build pipeline)
- Deployer has sufficient IAM permissions for all resource types

**Technical Assumptions:**
- Single EC2 instance is sufficient for MVP throughput (spec states single-writer)
- OpenSearch Serverless minimum OCU configuration handles expected note volumes (hundreds to low thousands)
- CodeCommit git operations over VPC endpoint are reliable and performant
- S3 gateway endpoint handles all S3 traffic without bandwidth constraints

**Business Assumptions:**
- Teams deploy one KB instance each (no multi-tenancy within a single stack)
- Operators monitor CloudWatch console manually (no automated alerting)
- The system handles up to ~100 pending notes per dream cycle (spec success criterion)

## Next Phase Preparation

### Task Breakdown Readiness
- [x] Clear technology choices and architecture
- [x] Complete data model and API specifications
- [x] Development environment and tooling defined
- [x] Quality standards and testing approach specified
- [x] Integration requirements and dependencies clear
- [x] Implementation phases ordered by dependency
- [x] Construct dependency graph documented

### Recommended Next Step
Run `/speckit-05-tasks` to break down each implementation phase into specific, estimable tasks with acceptance criteria.
