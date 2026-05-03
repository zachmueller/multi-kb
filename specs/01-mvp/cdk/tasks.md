# Task Breakdown: Multi-KB CDK Infrastructure — MVP

**Created:** 2026-05-01
**Implementation Plan:** [plan.md](plan.md)
**Specification:** [spec.md](spec.md)
**Status:** In Progress

## Task Summary

**Total Tasks:** 56
**Phases:** 9 (Setup → Networking → Storage → Search → Lambda → API → Compute → Observability → Quality)
**Estimated Complexity:** High
**Parallel Execution Opportunities:** 12 task groups
**Research Status:** R-1 through R-7 all resolved

## Dependency Legend

- **[P]** — Can execute in parallel with other [P] tasks in the same phase section
- **Dependencies** — Tasks that must complete before this task can start
- **FR-N** — References to spec functional requirements
- **Contract** — References to files in `contracts/`

---

## Phase 0: Setup & Environment

### ENV-001: CDK Project Initialization
**Description:** Initialize the CDK TypeScript project, install dependencies, and establish the directory structure from plan.md.
**Files:**
- `bin/multi-kb.ts` — CDK app entry point
- `lib/multi-kb-stack.ts` — main stack (skeleton)
- `lib/constructs/` — empty construct files with exports
- `lambda/` — directory structure for handler code
- `test/` — test directory structure
- `package.json`, `tsconfig.json`, `cdk.json`
**Dependencies:** None
**Acceptance Criteria:**
- [x] `cdk init app --language typescript` or equivalent manual setup complete
- [x] All directories from plan.md CDK Stack Architecture exist
- [x] `npx tsc --noEmit` succeeds
- [x] `cdk synth` produces a valid (empty) CloudFormation template
- [x] `npm test` runs Jest with no failures

**Commands:**
```bash
npm install aws-cdk-lib constructs @aws-sdk/client-sqs @aws-sdk/client-bedrock-agent-runtime @aws-sdk/client-bedrock-runtime @aws-sdk/client-s3
npm install -D jest ts-jest @types/jest esbuild
```

### ENV-002 [P]: Stack Configuration and Props
**Description:** Define all configurable CDK context parameters and stack props per spec CDK Stack Structure and data-model.md Entity 5.
**Files:**
- `lib/multi-kb-stack.ts` — `MultiKbStackProps` interface, context resolution
- `bin/multi-kb.ts` — context extraction and props passing
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] `MultiKbStackProps` interface defines all 12 parameters from spec: `repoName`, `bucketPrefix`, `ec2InstanceType`, `embeddingModelId`, `consolidationModelId`, `coverageModelId`, `tickInterval`, `dreamCycleInterval`, `excludePendingFromRecall`, `coverageScoreThreshold`, `cliBinaryS3Uri`, `vpcId`
- [x] Defaults applied for all optional parameters per spec table
- [x] `cliBinaryS3Uri` is required — synthesis fails with clear error if missing
- [x] Context values resolved from `cdk.json` and CLI `--context` flags
- [x] Test: synth succeeds with only `cliBinaryS3Uri` provided; synth fails without it

### ENV-003 [P]: Development Tooling Configuration
**Description:** Set up ESLint, Prettier, Jest config, and development scripts.
**Files:**
- `.eslintrc.json` — ESLint configuration
- `.prettierrc` — Prettier configuration
- `jest.config.ts` — Jest + ts-jest config with CDK assertions
- `package.json` — scripts: build, test, lint, synth, deploy
- `.gitignore`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] `npm run build` compiles TypeScript
- [x] `npm test` runs Jest with CDK assertions module available
- [x] `npm run lint` runs ESLint with no errors on scaffold
- [x] Snapshot test infrastructure ready (`jest --updateSnapshot` works)
- [x] `cdk.json` configures app entry point and default context

---

## Phase 1: Networking

_Corresponds to plan.md Phase A. Builds VPC infrastructure._

### NET-001: VPC and Subnet
**Description:** Create VPC with a private subnet in a single AZ, or import an existing VPC via `vpcId` prop per spec FR-4.
**Files:**
- `lib/constructs/networking.ts` — `NetworkingConstruct`: VPC, private subnet, AZ pinning
- `test/constructs/networking.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [x] When `vpcId` is not provided: creates new VPC with a single private subnet in one AZ
- [x] When `vpcId` is provided: imports existing VPC via `Vpc.fromLookup()`
- [x] No public subnets, no NAT gateway
- [x] Single AZ for cost optimization (all endpoints and ASG pinned to same AZ)
- [x] Exports: VPC, subnet, availability zone
- [x] CDK assertion test: VPC resource exists with expected CIDR; no NAT gateway resource; single subnet

### NET-002: S3 Gateway Endpoint
**Description:** Create the S3 gateway VPC endpoint per spec FR-4.
**Files:**
- `lib/constructs/networking.ts` — S3 gateway endpoint addition
- `test/constructs/networking.test.ts` — additional assertions
**Dependencies:** NET-001
**Acceptance Criteria:**
- [x] Gateway endpoint for `com.amazonaws.{region}.s3`
- [x] Associated with the private subnet's route table
- [x] Free resource (no hourly cost)
- [x] CDK assertion test: VPC endpoint resource with `s3` service name and `Gateway` type

### NET-003: Interface VPC Endpoints (9 endpoints)
**Description:** Create all 9 interface VPC endpoints per spec FR-4 VPC endpoint list and research.md R-4.
**Files:**
- `lib/constructs/networking.ts` — interface endpoint creation
- `test/constructs/networking.test.ts` — endpoint assertions
**Dependencies:** NET-001, NET-004
**Acceptance Criteria:**
- [x] Creates **8 standard interface endpoints** via `ec2.InterfaceVpcEndpoint`:
  1. `com.amazonaws.{region}.sqs`
  2. `com.amazonaws.{region}.git-codecommit`
  3. `com.amazonaws.{region}.bedrock-runtime`
  4. `com.amazonaws.{region}.bedrock-agent`
  5. `com.amazonaws.{region}.ssm`
  6. `com.amazonaws.{region}.ssmmessages`
  7. `com.amazonaws.{region}.ec2messages`
  8. `com.amazonaws.{region}.logs` (CloudWatch Logs)
- [x] Creates **1 AOSS VPC endpoint** via `opensearchserverless.CfnVpcEndpoint` (L1 construct) — NOT `ec2.InterfaceVpcEndpoint`. This uses `AWS::OpenSearchServerless::VpcEndpoint`, not `AWS::EC2::VPCEndpoint`.
  - AOSS endpoint props: `name` (must match `^[a-z][a-z0-9-]{2,31}$`), `vpcId`, `subnetIds`, `securityGroupIds`
  - AOSS endpoint returns `attrId` (the VPC endpoint ID used in the network policy's `SourceVPCEs` field)
- [x] All endpoints placed in the single private subnet (same AZ)
- [x] All endpoints (8 standard + 1 AOSS) share the VPC endpoint security group (NET-004)
- [x] **CRITICAL: `open: false`** on all `InterfaceVpcEndpoint` constructs to prevent CDK from auto-adding permissive `0.0.0.0/0` ingress rule
- [x] Private DNS enabled on all 8 standard interface endpoints
- [x] Exports: AOSS VPC endpoint ID (needed by SRC-003 network policy `SourceVPCEs`)
- [x] CDK assertion test: 8 `AWS::EC2::VPCEndpoint` resources + 1 `AWS::OpenSearchServerless::VpcEndpoint` resource; all reference the endpoint SG

### NET-004: Security Groups
**Description:** Create security groups for EC2 instance and VPC endpoints per research.md R-4.
**Files:**
- `lib/constructs/networking.ts` — EC2 SG, endpoint SG, rules
- `test/constructs/networking.test.ts`
**Dependencies:** NET-001
**Acceptance Criteria:**
- [x] EC2 security group: `allowAllOutbound: false`, explicit egress TCP 443 to endpoint SG only
- [x] VPC endpoint security group: `allowAllOutbound: false`, explicit ingress TCP 443 from EC2 SG only
- [x] No public inbound rules on either SG
- [x] Both SGs shared by all 9 interface endpoints (8 standard + 1 AOSS)
- [x] AOSS `CfnVpcEndpoint` receives `[endpointSg.securityGroupId]` as `securityGroupIds` (string, not construct reference)
- [x] Exports: EC2 SG (for compute construct), endpoint SG (for endpoints)
- [x] CDK assertion test: two SG resources with correct ingress/egress rules; no `0.0.0.0/0` rules

---

## Phase 2: Storage Layer

_Corresponds to plan.md Phase B._

### STR-001: S3 Bucket
**Description:** Create the S3 bucket for note replication and recall logs per spec FR-6.
**Files:**
- `lib/constructs/storage.ts` — `StorageConstruct`: S3 bucket
- `test/constructs/storage.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [x] Bucket name derived from `bucketPrefix` prop + account/region suffix for uniqueness
- [x] Server-side encryption: SSE-S3 (AES-256)
- [x] Public access blocked (all four block settings enabled)
- [x] Versioning disabled
- [x] No lifecycle rules (MVP)
- [x] `removalPolicy: RemovalPolicy.RETAIN` (protect against accidental `cdk destroy`)
- [x] Exports: bucket object, bucket name, bucket ARN
- [x] CDK assertion test: bucket has encryption, public access blocked, versioning off

### STR-002 [P]: CodeCommit Repository
**Description:** Create the CodeCommit git repository per spec FR-5.
**Files:**
- `lib/constructs/storage.ts` — CodeCommit repository addition
- `test/constructs/storage.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [x] Repository name from `repoName` prop (default: `"multi-kb"`)
- [x] Exports: repository object, clone URL (HTTPS), repository ARN
- [x] CDK assertion test: CodeCommit repository resource with expected name

### STR-003 [P]: SQS Queue with DLQ
**Description:** Create the SQS standard queue and dead-letter queue per spec FR-3.
**Files:**
- `lib/constructs/storage.ts` — SQS queue + DLQ
- `test/constructs/storage.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [x] Standard queue (not FIFO)
- [x] Visibility timeout: 5 minutes (300 seconds)
- [x] Message retention: 14 days (1,209,600 seconds)
- [x] DLQ configured with `maxReceiveCount: 3`
- [x] DLQ has 14-day retention
- [x] Exports: queue object, queue URL, queue ARN, DLQ object, DLQ ARN
- [x] CDK assertion test: queue has correct visibility timeout and retention; DLQ exists; redrive policy configured with maxReceiveCount 3

### STR-004: Stack Outputs — Storage
**Description:** Add CloudFormation outputs for storage resources per spec Stack Outputs.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput additions
**Dependencies:** STR-001, STR-002, STR-003
**Acceptance Criteria:**
- [x] Output `BucketName`: S3 bucket name
- [x] Output `RepoCloneUrl`: CodeCommit HTTPS clone URL
- [x] CDK assertion test: outputs exist with expected logical IDs

---

## Phase 3: Search Infrastructure

_Corresponds to plan.md Phase C. Builds OpenSearch Serverless + Bedrock KB._

### SRC-001: OpenSearch Serverless Collection
**Description:** Create the OpenSearch Serverless collection of type VECTORSEARCH per spec FR-7 and research.md R-1.
**Files:**
- `lib/constructs/search.ts` — `SearchConstruct`: `CfnCollection` from `aws-cdk-lib/aws-opensearchserverless`
- `test/constructs/search.test.ts`
**Dependencies:** ENV-002, SRC-002 (encryption policy must exist before collection)
**Acceptance Criteria:**
- [x] Uses `CfnCollection` (L1) from `aws-cdk-lib/aws-opensearchserverless`
- [x] Type: `VECTORSEARCH`
- [x] Collection name: configurable, must match `^[a-z][a-z0-9-]{2,27}$`
- [x] `standbyReplicas: 'DISABLED'` for MVP cost savings
- [x] **`collection.addDependency(encryptionPolicy)`** — collection FAILS without this
- [x] **`collection.addDependency(networkPolicy)`** — recommended so network access is ready when collection comes online
- [x] Exports: `collection.attrArn`, `collection.attrCollectionEndpoint`, `collection.attrId`, collection name
- [x] CDK assertion test: collection resource with type `VECTORSEARCH`; `DependsOn` includes encryption policy

### SRC-002: Encryption Policy
**Description:** Create the OpenSearch Serverless encryption policy per spec FR-7 and research.md R-1.
**Files:**
- `lib/constructs/search.ts` — `CfnSecurityPolicy` from `aws-cdk-lib/aws-opensearchserverless`
- `test/constructs/search.test.ts`
**Dependencies:** None (must be created BEFORE SRC-001 collection)
**Acceptance Criteria:**
- [x] Uses `CfnSecurityPolicy` with `type: 'encryption'`
- [x] Policy name must match `^[a-z][a-z0-9-]{2,31}$`
- [x] Policy JSON is a **single object** (NOT an array — unlike network and data access policies)
- [x] Policy specifies `AWSOwnedKey: true`
- [x] `Rules` target the collection by name: `[{ "ResourceType": "collection", "Resource": ["collection/<name>"] }]`
- [x] Policy is `JSON.stringify()`-ed into the `policy` string prop
- [x] SRC-001 must call `collection.addDependency(encryptionPolicy)` — collection creation FAILS without encryption policy
- [x] CDK assertion test: security policy resource with type `encryption` and `AWSOwnedKey: true`

### SRC-003: Network Policy (Dual Access)
**Description:** Create the OpenSearch Serverless network policy allowing both VPC and Bedrock service access per research.md R-6.
**Files:**
- `lib/constructs/search.ts` — `CfnSecurityPolicy` with type `network`
- `test/constructs/search.test.ts`
**Dependencies:** NET-003 (for AOSS VPC endpoint ID)
**Acceptance Criteria:**
- [x] Uses `CfnSecurityPolicy` with type `network`
- [x] **`AllowFromPublic: false`** — do NOT set to `true` (it silently overrides `SourceVPCEs` and `SourceServices`, making the collection fully public)
- [x] `SourceVPCEs: [<aoss-vpc-endpoint-id>]` — enables EC2 direct access via VPC endpoint. Note: field name is `SourceVPCEs` (NOT `SourceVPCEndpoints`)
- [x] `SourceServices: ["bedrock.amazonaws.com"]` — enables Bedrock service access via AWS internal networking
- [x] All field names are case-sensitive: `AllowFromPublic`, `SourceVPCEs`, `SourceServices`, `ResourceType`, `Resource`
- [x] Policy JSON is an **array** (unlike encryption policy which is a single object)
- [x] Both access paths work: EC2 via VPC endpoint, Bedrock via service private networking
- [x] Three-layer security model: network policy (origin) + data access policy (principal) + IAM
- [x] CDK assertion test: network security policy exists with `AllowFromPublic: false`, `SourceVPCEs`, and `SourceServices`

### SRC-004: Data Access Policy
**Description:** Create the OpenSearch Serverless data access policy granting access to EC2 role, Bedrock KB service role, and index creation Lambda role per research.md R-1.
**Files:**
- `lib/constructs/search.ts` — `CfnAccessPolicy` from `aws-cdk-lib/aws-opensearchserverless`
- `test/constructs/search.test.ts`
**Dependencies:** SRC-001 (collection name), CMP-001 (EC2 role ARN), KBS-002 (Bedrock KB service role ARN), SRC-006 (index creation Lambda role ARN)
**Acceptance Criteria:**
- [x] Uses `CfnAccessPolicy` with `type: 'data'`
- [x] Policy name must match `^[a-z][a-z0-9-]{2,31}$`
- [x] **Principals:** EC2 instance role ARN + Bedrock KB service role ARN + index creation Lambda role ARN (all passed as CDK tokens via `JSON.stringify()`)
- [x] CDK tokens in `JSON.stringify()` resolve to `Fn::Join`/`Fn::Sub` intrinsics — this is well-supported
- [x] Index-level permissions: `aoss:ReadDocument`, `aoss:WriteDocument`, `aoss:CreateIndex`, `aoss:DeleteIndex`, `aoss:UpdateIndex`, `aoss:DescribeIndex`
- [x] Collection-level permissions: `aoss:CreateCollectionItems`, `aoss:DeleteCollectionItems`, `aoss:UpdateCollectionItems`, `aoss:DescribeCollectionItems`
- [x] Resources: `index/<collection-name>/*` and `collection/<collection-name>`
- [x] **No circular dependency:** CDK tokens resolve all forward references. Roles reference collection ARN; policy references role ARNs. CloudFormation determines creation order via implicit `DependsOn`.
- [x] **Principals must ALSO have IAM-level `aoss:APIAccessAll`** on the collection ARN in their own IAM policies (AOSS data access policy and IAM policy work together)
- [x] CDK assertion test: access policy exists with correct principals and permissions

### KBS-001: Bedrock Knowledge Base
**Description:** Create the Bedrock Knowledge Base with OpenSearch Serverless as the vector store per spec FR-8 and research.md R-2.
**Files:**
- `lib/constructs/knowledge-base.ts` — `KnowledgeBaseConstruct`: `CfnKnowledgeBase` from `aws-cdk-lib/aws-bedrock`
- `test/constructs/knowledge-base.test.ts`
**Dependencies:** SRC-001 (collection ARN), KBS-002 (service role), **SRC-006 (vector index must exist before KB creation)**
**Acceptance Criteria:**
- [x] Uses `CfnKnowledgeBase` (L1) from `aws-cdk-lib/aws-bedrock`
- [x] `knowledgeBaseConfiguration.type`: `'VECTOR'`
- [x] Embedding model: `embeddingModelArn` constructed from configurable model ID (default: `arn:aws:bedrock:{region}::foundation-model/amazon.titan-embed-text-v2:0`)
- [x] `storageConfiguration.type`: `'OPENSEARCH_SERVERLESS'`
- [x] `opensearchServerlessConfiguration`: `collectionArn`, `vectorIndexName`, and `fieldMapping` (all three fields required)
- [x] **Field mappings must match the pre-created index schema (SRC-006):**
  - `vectorField`: `'bedrock-knowledge-base-default-vector'`
  - `textField`: `'AMAZON_BEDROCK_TEXT_CHUNK'`
  - `metadataField`: `'AMAZON_BEDROCK_METADATA'`
- [x] Role ARN: Bedrock KB service role (KBS-002)
- [x] KB depends on SRC-006 custom resource (index must exist before KB)
- [x] Exports: `attrKnowledgeBaseId`, `attrKnowledgeBaseArn`
- [x] CDK assertion test: KB resource with correct embedding model ARN, storage type, collection ARN, and field mappings

### KBS-002: Bedrock KB Service Role
**Description:** Create the IAM role assumed by Bedrock to access S3, OpenSearch, and the embedding model per research.md R-2.
**Files:**
- `lib/constructs/knowledge-base.ts` — service role for Bedrock KB
- `test/constructs/knowledge-base.test.ts`
**Dependencies:** STR-001 (S3 bucket ARN), SRC-001 (collection ARN), ENV-002 (embedding model ID)
**Acceptance Criteria:**
- [x] IAM role with trust policy for `bedrock.amazonaws.com` service principal
- [x] Trust policy conditions: `aws:SourceAccount` (account ID) + `ArnLike` condition on `arn:aws:bedrock:{region}:{account}:knowledge-base/*`
- [x] Permissions: `s3:GetObject` on `{bucket-arn}/*` + `s3:ListBucket` on bucket ARN (both with `aws:ResourceAccount` condition)
- [x] Permissions: `aoss:APIAccessAll` on the collection ARN
- [x] **Permissions: `bedrock:InvokeModel` on the foundation model ARN** (e.g., `arn:aws:bedrock:{region}::foundation-model/amazon.titan-embed-text-v2:0`). Note: model ARN uses empty account ID (`::`) because foundation models are AWS-owned.
- [x] Least-privilege: no wildcard permissions, all scoped to specific ARNs
- [x] Exports: role, role ARN (needed by SRC-004 data access policy and KBS-001)
- [x] CDK assertion test: role with correct trust policy (including conditions), 4 permission statements on specific ARNs

### KBS-003: Bedrock KB Data Source (S3)
**Description:** Create the Bedrock Knowledge Base data source pointing to the S3 bucket with "no chunking" strategy per spec FR-8.
**Files:**
- `lib/constructs/knowledge-base.ts` — CfnDataSource
- `test/constructs/knowledge-base.test.ts`
**Dependencies:** KBS-001 (KB ID), STR-001 (S3 bucket ARN)
**Acceptance Criteria:**
- [x] Uses `CfnDataSource` (L1) from `aws-bedrock`
- [x] Linked to the Knowledge Base (KBS-001)
- [x] S3 configuration: points to the KB bucket
- [x] Chunking strategy: `NONE` (each note is its own chunk)
- [x] Exports: data source ID (needed by EC2 config for StartIngestionJob)
- [x] CDK assertion test: data source resource with chunking strategy `NONE` and correct S3 bucket

### SRC-006: Custom Resource — OpenSearch Vector Index Creation
**Description:** Create a CDK custom resource (Lambda-backed) that pre-creates the OpenSearch vector index. Bedrock KB does NOT auto-create the index via CloudFormation — this is a console-only feature (research.md R-2).
**Files:**
- `lib/constructs/search.ts` — custom resource addition to `SearchConstruct`
- `lambda/custom-resource/create-index.ts` — custom resource handler
- `test/lambda/custom-resource/create-index.test.ts` — handler unit tests
**Dependencies:** SRC-001 (collection endpoint, collection name), SRC-004 (data access policy — index creation Lambda needs access), NET-003 (AOSS VPC endpoint — Lambda must be in VPC to reach collection)
**Acceptance Criteria:**
- [x] CDK `Provider` + `CustomResource` construct backed by a Lambda function
- [x] Lambda sends PUT request to OpenSearch collection endpoint to create index with schema:
  - `settings.index.knn`: `true`
  - `bedrock-knowledge-base-default-vector` field: `knn_vector`, 1024 dimensions, `faiss` engine, `hnsw` method, `l2` space type
  - `AMAZON_BEDROCK_TEXT_CHUNK` field: `text`, `index: true`
  - `AMAZON_BEDROCK_METADATA` field: `text`, `index: false`
- [x] Index name configurable (default: `'bedrock-kb-index'`)
- [x] Lambda is VPC-attached (needs to reach collection via AOSS VPC endpoint)
- [x] Lambda security group: same EC2 SG or a separate SG with outbound 443 to endpoint SG
- [x] Lambda IAM role: `aoss:APIAccessAll` on collection ARN
- [x] Lambda role must be listed in AOSS data access policy (SRC-004) principals
- [x] **Dependency ordering:** The Lambda IAM role is created as part of this construct (before the custom resource executes). The role ARN is passed to SRC-004 for inclusion in the data access policy. The custom resource execution (`CustomResource` node) must depend on SRC-004 being created — use `customResource.node.addDependency(dataAccessPolicy)` to ensure the policy grants access before the Lambda attempts to create the index.
- [x] On Create: creates the index; on Update: no-op (index schema is immutable); on Delete: optionally deletes the index
- [x] Idempotent: if index already exists, succeeds without error
- [x] Vector dimension (1024) matches the embedding model configuration in KBS-001
- [x] CDK assertion test: custom resource exists; Lambda is VPC-attached; IAM role has `aoss:APIAccessAll`

### SRC-005: Stack Outputs — Search
**Description:** Add CloudFormation outputs for search infrastructure.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput additions
**Dependencies:** SRC-001, KBS-001, KBS-003 (data source ID)
**Acceptance Criteria:**
- [x] Output `CollectionEndpoint`: OpenSearch Serverless collection endpoint
- [x] Output `KnowledgeBaseId`: Bedrock KB ID
- [x] Output `DataSourceId`: Bedrock KB data source ID (from KBS-003) — required by EC2 config.yaml per [server-config.md](contracts/server-config.md)
- [x] CDK assertion test: outputs exist (including `DataSourceId`)

---

## Phase 4: Lambda Functions

_Corresponds to plan.md Phase D. Builds API handler functions._

### LMB-001: Shared Lambda Utilities
**Description:** Implement shared utilities used by both Lambda handlers per plan.md lambda/shared/.
**Files:**
- `lambda/shared/uid.ts` — Crockford base32 UID generation (per [research.md R-5](research.md#r-5-crockford-base32-uid-generation-in-nodejs))
- `lambda/shared/response.ts` — API Gateway Lambda proxy response helpers (per research.md R-7)
- `lambda/shared/validation.ts` — field validation utilities
- `test/lambda/shared/uid.test.ts`
- `test/lambda/shared/response.test.ts`
- `test/lambda/shared/validation.test.ts`
**Dependencies:** ENV-001
**Acceptance Criteria:**
- [x] **UID generation (R-5):** `crypto.randomBytes(10)` → 16-char Crockford base32 (alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ`); uppercase; exactly 16 chars
- [x] Uses bit-buffer encoding algorithm (R-5): accumulate 8 bits per byte, extract 5-bit groups MSB-first via `(buf >>> bits) & 0x1F` (unsigned right shift)
- [x] Zero npm dependencies — uses only Node.js built-in `crypto`
- [x] `encodeCrockford(Buffer)` exported separately from `generateUid()` for deterministic testing
- [x] **Response helpers (R-7):** Four functions in `lambda/shared/response.ts`: `success(statusCode, body)` for 200/202 responses, `error(statusCode, body)` for arbitrary errors, `validationError(errors: Record<string, string>)` convenience for HTTP 400 `{ errors: { field: reason } }`, `internalError()` convenience for HTTP 500 with generic message. All auto-stringify body and set `Content-Type: application/json` via shared `JSON_HEADERS` constant. Body parameter is `unknown` (helper calls `JSON.stringify()` internally). Uses `APIGatewayProxyResult` type from `@types/aws-lambda` (dev dependency — erased at compile time).
- [x] **Handler wrapper pattern (R-7):** Both Lambda handlers wrap logic in top-level try/catch returning `internalError()`. This guarantees well-formed responses (never 502 from malformed response) and preserves 500 vs 502 distinction for debugging. Guard `event.body` with `JSON.parse(event.body ?? '{}')` — catch `SyntaxError` to return 400.
- [x] **HTTP 401/403 not handled by Lambda (R-7):** API Gateway handles auth errors before Lambda invocation for `AWS_IAM` auth. Lambda does not need 401/403 response paths.
- [x] **Validation:** `validateSubmitKnowledge(body)` returns `{ valid: true, data }` or `{ valid: false, errors: {} }`; validates title (present, non-empty, ≤255), content (present, non-empty, ≤100K), author (present, non-empty, ≤100)
- [x] Test: UID deterministic encoding of 5 shared test vectors from R-5:
  - `Buffer.from([0x00 × 10])` → `"0000000000000000"`
  - `Buffer.from([0xFF × 10])` → `"ZZZZZZZZZZZZZZZZ"`
  - `Buffer.from([0x00..0x09])` → `"000G40R40M30E209"`
  - `Buffer.from([0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x42])` → `"VTPVXVYAZTXBW022"`
  - `Buffer.from("HelloWorld")` → `"91JPRV3FAXQQ4V34"`
- [x] Test: UID format (length=16, valid alphabet, no I/L/O/U, uniqueness over 1K), response shape, all validation rules from contracts/submit-knowledge.md table

### LMB-002: submitKnowledge Lambda Handler
**Description:** Implement the submitKnowledge Lambda per spec FR-2 and contracts/submit-knowledge.md.
**Files:**
- `lambda/submit/index.ts` — handler function
- `test/lambda/submit.test.ts` — handler unit tests
**Dependencies:** LMB-001
**Acceptance Criteria:**
- [x] Parses `event.body` (JSON string from API Gateway proxy)
- [x] Calls validation; returns HTTP 400 with `{ errors: {...} }` on failure (only failed fields)
- [x] Generates UID (16-char Crockford base32) and `submitted_at` (ISO 8601 current time)
- [x] Sends SQS message: `{ uid, title, content, author, submitted_at }` serialized as JSON body
- [x] Returns HTTP 202: `{ uid, request_id: event.requestContext.requestId }`
- [x] On SQS failure: returns HTTP 500 with generic message; logs full error
- [x] Reads `SQS_QUEUE_URL` from `process.env`
- [x] Test (mocked SQS): valid submission, each field validation failure (6 cases), whitespace-only fields, SQS send failure

### LMB-003: submitKnowledge CDK Construct
**Description:** Create the CDK construct that provisions the submitKnowledge Lambda with correct runtime, permissions, and environment.
**Files:**
- `lib/constructs/submit-lambda.ts` — `SubmitLambdaConstruct`: NodejsFunction, IAM role
- `test/constructs/submit-lambda.test.ts`
**Dependencies:** LMB-002, STR-003 (SQS queue)
**Acceptance Criteria:**
- [x] Uses `NodejsFunction` with esbuild bundling
- [x] Runtime: `nodejs22.x`, architecture: ARM64
- [x] Memory: 256 MB, timeout: 10 seconds
- [x] Environment variable: `SQS_QUEUE_URL` from queue construct
- [x] IAM: `sqs:SendMessage` scoped to queue ARN only
- [x] Exports: Lambda function (for API Gateway integration)
- [x] CDK assertion test: Lambda runtime, memory, timeout; IAM policy with `sqs:SendMessage` on specific ARN; environment variable set

### LMB-004: recallKnowledge Lambda Handler
**Description:** Implement the recallKnowledge Lambda per spec FR-9 and contracts/recall-knowledge.md.
**Files:**
- `lambda/recall/index.ts` — handler function
- `test/lambda/recall.test.ts` — handler unit tests
**Dependencies:** LMB-001, R-2 research (Bedrock KB metadata extraction — determines how uid/title are extracted from Retrieve results), PRM-003 (coverage assessment prompt)
**Acceptance Criteria:**
- [x] Parses `event.body` — extracts `query` (required, string) and `limit` (optional, integer, default 10)
- [x] Validates `query`: present, non-empty string; returns HTTP 400 on failure
- [x] Validates `limit`: must be integer >= 1 and <= 100; returns HTTP 400 with `{ errors: { limit: "must be an integer between 1 and 100" } }` if out of range. Non-integer values return 400. Default 10 if omitted.
- [x] Calls Bedrock Retrieve API with `knowledgeBaseId`, `retrievalQuery.text = query`
- [x] Optionally filters to `status: active` when `EXCLUDE_PENDING` is `true`
- [x] Maps Retrieve response to `[{ uid, title, content, score }]` sorted by descending score
- [x] Coverage assessment: if top score < `COVERAGE_SCORE_THRESHOLD`:
  - Calls InvokeModel (coverage LLM) with query + result summaries
  - If gap detected: follow-up Retrieve query
  - Deduplicates by UID, sorts by score, truncates to `limit`
  - On any coverage failure: falls back to original results silently
- [x] Coverage model ARN construction: `arn:aws:bedrock:${AWS_REGION}::foundation-model/${COVERAGE_MODEL_ID}` (empty account ID for AWS-owned foundation models; see [contracts/recall-knowledge.md](contracts/recall-knowledge.md#coverage-model-arn-construction))
- [x] Writes recall log to S3 synchronously: `recall-logs/<YYYY-MM-DD>/<request-id>.json` (date partition is UTC)
  - Best-effort: S3 failure logged but doesn't affect response
- [x] Returns HTTP 200 with results array (or empty array)
- [x] Reads env vars: `KNOWLEDGE_BASE_ID`, `BUCKET_NAME`, `COVERAGE_MODEL_ID`, `COVERAGE_SCORE_THRESHOLD`, `EXCLUDE_PENDING`
- [x] Test (mocked Bedrock/S3): successful recall, empty results, coverage assessment trigger, coverage fallback, S3 write failure, validation error, limit=0 → 400, limit=-1 → 400, limit=101 → 400, limit=50 → 200, limit omitted → uses default 10

### LMB-005: recallKnowledge CDK Construct
**Description:** Create the CDK construct that provisions the recallKnowledge Lambda with correct runtime, permissions, and environment.
**Files:**
- `lib/constructs/recall-lambda.ts` — `RecallLambdaConstruct`: NodejsFunction, IAM role
- `test/constructs/recall-lambda.test.ts`
**Dependencies:** LMB-004, KBS-001 (KB ID), STR-001 (S3 bucket)
**Acceptance Criteria:**
- [x] Uses `NodejsFunction` with esbuild bundling
- [x] Runtime: `nodejs22.x`, architecture: ARM64
- [x] Memory: 1024 MB, timeout: 30 seconds
- [x] Environment variables: `KNOWLEDGE_BASE_ID`, `BUCKET_NAME`, `COVERAGE_MODEL_ID`, `COVERAGE_SCORE_THRESHOLD`, `EXCLUDE_PENDING`
- [x] IAM: `bedrock:Retrieve` on KB ARN; `bedrock:InvokeModel` on `arn:aws:bedrock:{region}::foundation-model/{coverageModelId}` (empty account ID for AWS-owned foundation models); `s3:PutObject` on `{bucket-arn}/recall-logs/*`
- [x] Lambda is NOT VPC-attached (calls public Bedrock endpoints)
- [x] Exports: Lambda function (for API Gateway integration)
- [x] CDK assertion test: Lambda runtime, memory, timeout; IAM with three permission sets scoped to specific ARNs; environment variables; NOT in VPC

---

## Phase 5: API Gateway

_Corresponds to plan.md Phase E._

### API-001: REST API and Stage
**Description:** Create the API Gateway REST API with `prod` stage per spec FR-1.
**Files:**
- `lib/constructs/api.ts` — `ApiConstruct`: RestApi, deployment, stage
- `test/constructs/api.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [x] REST API (not HTTP API) — required for `AWS_IAM` auth
- [x] `prod` stage deployed
- [x] Access logging enabled (CloudWatch Logs)
- [x] CORS not enabled (spec: "CLI is not a browser client")
- [x] Exports: API object, endpoint URL
- [x] CDK assertion test: REST API resource; stage named `prod`; access log destination configured

### API-002: submitKnowledge Endpoint
**Description:** Create the `POST /submitKnowledge` resource, method, and Lambda proxy integration per spec FR-1, FR-2.
**Files:**
- `lib/constructs/api.ts` — resource + method + integration
- `test/constructs/api.test.ts`
**Dependencies:** API-001, LMB-003
**Acceptance Criteria:**
- [x] Resource: `/submitKnowledge`
- [x] Method: `POST` with `AWS_IAM` authorization
- [x] Lambda proxy integration with submitKnowledge function
- [x] Unauthorized requests receive HTTP 401; insufficient permissions receive HTTP 403
- [x] CDK assertion test: API resource with `POST` method; IAM auth type; Lambda integration

### API-003 [P]: recallKnowledge Endpoint
**Description:** Create the `POST /recallKnowledge` resource, method, and Lambda proxy integration per spec FR-1, FR-9.
**Files:**
- `lib/constructs/api.ts` — resource + method + integration
- `test/constructs/api.test.ts`
**Dependencies:** API-001, LMB-005
**Acceptance Criteria:**
- [x] Resource: `/recallKnowledge`
- [x] Method: `POST` with `AWS_IAM` authorization
- [x] Lambda proxy integration with recallKnowledge function
- [x] CDK assertion test: API resource with `POST` method; IAM auth type; Lambda integration

### API-004: Stack Output — API Endpoint
**Description:** Add CloudFormation outputs for API Gateway.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput additions
**Dependencies:** API-001
**Acceptance Criteria:**
- [x] Output `ApiEndpoint`: full API Gateway endpoint URL (e.g., `https://{api-id}.execute-api.{region}.amazonaws.com/prod`)
- [x] Output `ApiId`: API Gateway REST API ID
- [x] CDK assertion test: outputs exist

---

## Phase 6: EC2 Compute

_Corresponds to plan.md Phase F._

### CMP-001: EC2 IAM Role
**Description:** Create the EC2 instance role with all required permissions per spec FR-4 and data-model.md IAM Role Summary.
**Files:**
- `lib/constructs/compute.ts` — `ComputeConstruct`: IAM role + instance profile
- `test/constructs/compute.test.ts`
**Dependencies:** STR-001 (bucket ARN), STR-002 (repo ARN), STR-003 (queue ARN), SRC-001 (collection ARN), KBS-001 (KB ARN), KBS-003 (data source ID), ENV-002 (CLI binary S3 URI, model IDs)
**Acceptance Criteria:**
- [x] IAM role with trust policy for `ec2.amazonaws.com`
- [x] Permissions (all scoped to specific resource ARNs):
  - `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes` on queue ARN
  - `codecommit:GitPull`, `codecommit:GitPush` on repo ARN
  - `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket` on KB bucket ARN
  - `s3:GetObject` on CLI binary S3 object ARN (parsed from `cliBinaryS3Uri`)
  - `aoss:APIAccessAll` on OpenSearch collection ARN
  - `bedrock:InvokeModel` on consolidation model ARN
  - `bedrock-agent:StartIngestionJob`, `bedrock-agent:GetIngestionJob` (scoped to KB/data source)
  - SSM Session Manager permissions (`ssm:UpdateInstanceInformation`, `ssmmessages:*`, `ec2messages:*`)
- [x] Instance profile created from role
- [x] Exports: role, role ARN, instance profile
- [x] CDK assertion test: role trust policy; each permission statement verified for specific ARN (no wildcards except SSM messages); instance profile exists

### CMP-002: Launch Template
**Description:** Create the EC2 launch template with Amazon Linux 2023, instance type, and security group per spec FR-4.
**Files:**
- `lib/constructs/compute.ts` — launch template
- `test/constructs/compute.test.ts`
**Dependencies:** CMP-001, NET-004 (EC2 SG)
**Acceptance Criteria:**
- [x] Amazon Linux 2023 AMI (latest, ARM64 or x86_64 matching instance type)
- [x] Instance type from `ec2InstanceType` prop (default: `t3.small`)
- [x] Instance profile from CMP-001
- [x] Security group: EC2 SG from NET-004
- [x] No public IP (private subnet only)
- [x] User data script from CMP-003
- [x] CDK assertion test: launch template with correct instance type; no associate public IP

### CMP-003: User Data Script
**Description:** Implement the EC2 user data script that bootstraps the instance per spec FR-4 and research.md R-3.
**Files:**
- `lib/constructs/compute.ts` — user data script generation
**Dependencies:** CMP-001, CMP-004 (for `addSignalOnExitCommand`), STR-001, STR-002, STR-003, SRC-001, KBS-001, KBS-003, ENV-002, OBS-001 (log group name)
**Contract:** [server-config.md](contracts/server-config.md) — defines the exact config.yaml fields to template and their CDK output sources
**Acceptance Criteria:**
- [x] Uses `UserData.forLinux()` with `set -euxo pipefail` as first command
- [x] Step 1: Install packages — `dnf install -y amazon-cloudwatch-agent` (git is pre-installed on AL2023; do NOT install git separately to avoid ambiguity)
- [x] Step 2: Download CLI binary — wrap in retry loop (3 attempts, exponential backoff: 1s/2s/4s): `aws s3 cp ${cliBinaryS3Uri} /usr/local/bin/multi-kb && chmod +x /usr/local/bin/multi-kb` (raw `addCommands`, NOT `addS3DownloadCommand` — S3 URI is a string prop, not an `IBucket` reference)
- [x] Step 3: Git credential helper — `git config --system credential.helper '!aws codecommit credential-helper $@'` and `git config --system credential.UseHttpPath true` (use `--system` not `--global` so it applies to all users)
- [x] Step 4: Clone CodeCommit repo — wrap in retry loop (3 attempts, exponential backoff: 1s/2s/4s): `git clone https://git-codecommit.{region}.amazonaws.com/v1/repos/{repoName} /opt/multi-kb/repo` with `|| { git init ... }` fallback for empty repos on first deploy
- [x] Step 5: Template `config.yaml` at `/opt/multi-kb/config.yaml` — interpolate all CDK-resolved values per [server-config.md](contracts/server-config.md) field mapping table. Use line-by-line `addCommands()` with heredoc and template literal interpolation for CDK tokens.
- [x] Step 6: Configure CloudWatch agent — use `Stack.toJsonString()` to safely serialize JSON config containing CDK token (`logGroupName`). Config at `/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json`. Log stream name uses `{instance_id}` (CloudWatch agent variable, NOT a CDK token). Start agent with `amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:...`.
- [x] Step 7: Create systemd unit file at `/etc/systemd/system/multi-kb.service` — `Type=simple`, `Restart=on-failure`, `RestartSec=5`, `WorkingDirectory=/opt/multi-kb/repo`, `StandardOutput=append:/var/log/multi-kb/server.log`, `StandardError=append:/var/log/multi-kb/server.log`, `Environment=AWS_REGION={region}`, `Environment=HOME=/root`. Create `/var/log/multi-kb/` directory first.
- [x] Step 8: Start services — `systemctl daemon-reload`, start CloudWatch agent via `amazon-cloudwatch-agent-ctl`, then `systemctl enable --now multi-kb`
- [x] **cfn-signal integration:** Call `userData.addSignalOnExitCommand(asg)` AFTER all other `addCommands()` calls. ASG uses `Signals.waitForAll({ timeout: Duration.minutes(15) })`.
- [x] All `${...}` values resolved from CDK construct outputs at synthesis time via template literal interpolation in `addCommands()`
- [x] Launch template sets `requireImdsv2: true` (security best practice; AWS CLI v2 on AL2023 supports IMDSv2)
- [x] Process runs as root for MVP (no dedicated user)
- [x] CDK assertion test: user data is non-empty; launch template has IMDSv2 enforced

### CMP-004: Auto Scaling Group
**Description:** Create the ASG with min/max/desired=1, pinned to single AZ per spec FR-4.
**Files:**
- `lib/constructs/compute.ts` — ASG configuration
- `test/constructs/compute.test.ts`
**Dependencies:** CMP-002, NET-001 (subnet)
**Acceptance Criteria:**
- [x] Min capacity: 1, max capacity: 1, desired capacity: 1
- [x] Pinned to single AZ (same as VPC endpoints)
- [x] Uses launch template from CMP-002
- [x] Health check: EC2 status checks (default)
- [x] Instance replacement on termination: ASG launches new instance, user data re-bootstraps
- [x] **Signals integration (R-3):** `signals: Signals.waitForAll({ timeout: Duration.minutes(15) })` — CloudFormation waits for cfn-signal from user data script before marking resource as created. On script failure, stack rolls back.
- [x] CDK assertion test: ASG with min=max=desired=1; subnet specified; CreationPolicy present with 15-minute timeout

### CMP-005: Stack Output — Compute
**Description:** Add CloudFormation output for EC2 instance.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput
**Dependencies:** CMP-004
**Acceptance Criteria:**
- [x] Output `Ec2InstanceId`: EC2 instance ID (note: may need to be a custom resource or reference since ASG manages the instance)
- [x] CDK assertion test: output exists

---

## Phase 7: Observability

_Corresponds to plan.md Phase G._

### OBS-001: CloudWatch Log Groups
**Description:** Create log groups for API Gateway, Lambda functions, and EC2 CLI process per spec NFR-4.
**Files:**
- `lib/constructs/observability.ts` — `ObservabilityConstruct`: log groups
- `test/constructs/observability.test.ts`
**Dependencies:** API-001, LMB-003, LMB-005
**Acceptance Criteria:**
- [x] API Gateway access log group (referenced by API stage)
- [x] Lambda function log groups (auto-created by Lambda, but set retention)
- [x] EC2 CLI process log group (for CloudWatch agent to ship to)
- [x] Retention: 30 days (configurable, reasonable default for MVP)
- [x] CDK assertion test: log group resources exist with retention policy

### OBS-002 [P]: CloudWatch Alarms
**Description:** Create CloudWatch alarms per spec NFR-4.
**Files:**
- `lib/constructs/observability.ts` — alarm definitions
- `test/constructs/observability.test.ts`
**Dependencies:** STR-003 (DLQ), CMP-004 (ASG)
**Acceptance Criteria:**
- [x] Alarm: DLQ `ApproximateNumberOfMessagesVisible` > 0 (indicates processing failures)
- [x] Alarm: ASG `GroupInServiceInstances` < 1 (EC2 instance unhealthy)
- [x] Alarm: Dream cycle lock held > 60 minutes (custom metric from CLI logs — metric filter on log group)
- [x] No alarm actions (metrics only for MVP — operators poll console)
- [x] CDK assertion test: 3 alarm resources; no action configuration; correct metric references

---

## Phase 8: Stack Wiring

_Bring all constructs together in the main stack._

### WIR-001: Main Stack Assembly
**Description:** Wire all constructs together in `MultiKbStack`, passing outputs between constructs per plan.md Construct Dependency Graph.
**Files:**
- `lib/multi-kb-stack.ts` — complete wiring
**Dependencies:** All Phase 1-7 construct tasks
**Acceptance Criteria:**
- [x] Instantiates all constructs in dependency order: Networking → Storage → Search → KnowledgeBase → Lambdas → API → Compute → Observability
- [x] Passes correct references between constructs (queue to Lambda, bucket to Lambda, KB ID to Lambda, VPC to endpoints, etc.)
- [x] Resolves the circular dependency between OpenSearch data access policy ↔ EC2 role / Bedrock KB role (CDK `addDependency()` or post-creation policy update)
- [x] All 7 stack outputs defined: `ApiEndpoint`, `ApiId`, `RepoCloneUrl`, `KnowledgeBaseId`, `BucketName`, `CollectionEndpoint`, `Ec2InstanceId`
- [x] `cdk synth` produces valid CloudFormation template
- [x] Template contains all expected resource types

### WIR-002: Stack Snapshot Test
**Description:** Create a snapshot test for the fully wired stack to catch unintended changes.
**Files:**
- `test/multi-kb-stack.test.ts` — snapshot test + key assertions
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [x] Snapshot test captures synthesized template
- [x] Fine-grained assertions verify critical cross-construct relationships:
  - submitKnowledge Lambda env var references the actual SQS queue URL
  - recallKnowledge Lambda env var references the actual KB ID
  - EC2 IAM role has permissions on the actual SQS/S3/CodeCommit/OpenSearch ARNs
  - API Gateway methods reference the actual Lambda functions
  - VPC endpoints are in the same subnet as the ASG
- [x] Test: `npm test` passes with snapshot matching

---

## Research Ordering

_Research items must complete before their dependent implementation phases._

| Research | Status | Must Complete Before | Blocks |
|----------|--------|---------------------|--------|
| **R-4:** VPC endpoint security groups | ✅ Resolved | Phase 1 (Networking) | NET-003, NET-004 (AOSS endpoint construct, `open: false`, SG config) |
| **R-6:** OpenSearch dual-access network policy | ✅ Resolved | Phase 3 (Search Infrastructure) | SRC-003 (`AllowFromPublic: false` + `SourceVPCEs` + `SourceServices`) |
| **R-1:** OpenSearch Serverless CDK setup | ✅ Resolved | Phase 3 (Search Infrastructure) | SRC-001, SRC-002, SRC-003, SRC-004 (L1 constructs, policy formats, dependency ordering) |
| **R-2:** Bedrock KB CDK construct | ✅ Resolved | Phase 3 (Search Infrastructure) + Phase 4 (Lambda Functions) | KBS-001, KBS-002, KBS-003, SRC-006 (field mappings, service role permissions, index pre-creation) + LMB-004 (uid/title extraction from Retrieve results) |
| **R-5:** Crockford base32 UID | ✅ Resolved | Phase 4 (Lambda Functions) | LMB-001 (UID generation — reshaped with bit-buffer algorithm and 5 shared test vectors) |
| **R-7:** Lambda proxy integration format | ✅ Resolved | Phase 4 (Lambda Functions) | LMB-001 (response helpers — 4 helper functions: `success()`, `error()`, `validationError()`, `internalError()`) |
| **R-3:** EC2 user data script | ✅ Resolved | Phase 6 (EC2 Compute) | CMP-003 (user data script — full bash script, CDK TypeScript code pattern, CloudWatch agent config, systemd unit, cfn-signal integration) |

**All Phase 3 research is resolved.** Implementation can now proceed through Phase 3 (Search Infrastructure) without blocking.

**R-5 is now resolved.** LMB-001 UID acceptance criteria updated with bit-buffer encoding algorithm and 5 authoritative test vectors shared with CLI R-7.

**R-7 is now resolved.** LMB-001 response helper acceptance criteria updated with 4 helper functions (`success()`, `error()`, `validationError()`, `internalError()`), handler wrapper pattern, and `@types/aws-lambda` dev dependency for the `APIGatewayProxyResult` type.

**R-3 is now resolved.** CMP-003 and CMP-004 acceptance criteria updated with: `UserData.forLinux()` + `addCommands()` pattern, `Stack.toJsonString()` for CloudWatch agent JSON, `addSignalOnExitCommand()` + `Signals.waitForAll()` for ASG integration, systemd unit with `StandardOutput=append:`, `requireImdsv2: true`, and 10 known gotchas documented.

**All research is now resolved (R-1 through R-7).** All phases (0-8) can proceed without blocking on research.

---

## Phase 9: Quality, Security & Integration Testing

### QAT-001: CDK Assertion Test Coverage
**Description:** Ensure all constructs have comprehensive CDK assertion tests beyond snapshot tests.
**Files:** All `test/constructs/*.test.ts` files
**Dependencies:** All construct tasks
**Acceptance Criteria:**
- [x] Every construct file has a corresponding test file
- [x] IAM policies tested for least-privilege (specific ARNs, no wildcards beyond SSM)
- [x] Security groups tested for correct rules (no overly permissive rules)
- [x] VPC endpoints tested for correct service names and subnet placement
- [x] Lambda configurations tested (runtime, memory, timeout, environment, architecture)
- [x] SQS tested for DLQ configuration
- [x] S3 tested for encryption and public access block
- [x] API Gateway tested for IAM authorization on both methods
- [x] `npm test` passes with all assertions green

### QAT-002 [P]: Lambda Handler Unit Tests
**Description:** Ensure comprehensive unit test coverage for Lambda handler business logic.
**Files:**
- `test/lambda/submit.test.ts`
- `test/lambda/recall.test.ts`
- `test/lambda/shared/*.test.ts`
**Dependencies:** LMB-001, LMB-002, LMB-004
**Acceptance Criteria:**
- [x] **submitKnowledge tests:** valid input → 202, missing title → 400, empty title → 400, long title → 400, missing content → 400, empty content → 400, long content → 400, missing author → 400, empty author → 400, long author → 400, SQS failure → 500, multiple validation errors → single 400 with all errors
- [x] **recallKnowledge tests:** valid query → 200 with results, empty query → 400, empty results → 200 with `[]`, coverage trigger (low score) → follow-up query, coverage fallback on error, S3 write failure → still returns results, limit parameter respected
- [x] **UID tests:** length=16, valid Crockford alphabet, no I/L/O/U, uniqueness, deterministic encoding of 5 shared test vectors from R-5 (matching CLI R-7 Go implementation)
- [x] All AWS SDK calls mocked (no real AWS calls in unit tests)
- [x] `npm test` passes

### QAT-003 [P]: Security Review
**Description:** Validate security requirements per spec NFR-3.
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [x] API Gateway: both methods require `AWS_IAM` auth (no anonymous access)
- [x] EC2 instance: private subnet, no public IP
- [x] EC2 IAM role: least-privilege (verify each policy statement scoped to specific ARNs)
- [x] S3 bucket: public access blocked, SSE-S3 encryption
- [x] OpenSearch: data access policy restricts to specific principals only
- [x] Lambda IAM roles: minimum permissions per function
- [x] No secrets in code, environment variables, or CDK context (only IAM role-based auth)
- [x] VPC endpoint security groups: only HTTPS/443 from EC2 SG

### QAT-004 [P]: Multi-Tenancy Validation
**Description:** Verify that the same CDK code can deploy independent instances with different stack names per spec success criteria.
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [x] `cdk synth --context repoName=team-a-kb ...` and `cdk synth --context repoName=team-b-kb ...` produce independent templates
- [x] Resource names derived from props, not hardcoded (S3 bucket, CodeCommit repo, SQS queue, OpenSearch collection)
- [x] Two stacks can coexist in the same account/region without resource name conflicts
- [x] Stack outputs are unique per deployment

### QAT-005: Post-Deploy Integration Checklist
**Description:** Manual integration test checklist for validating a deployed stack end-to-end per spec User Scenarios. Automated test script at `test/integration/qat-005-post-deploy.sh`.
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [x] **Submit flow:** `aws apigateway test-invoke-method` on submitKnowledge → verify SQS message arrives → verify EC2 picks up message → verify CodeCommit commit → verify S3 sync → verify note appears in OpenSearch after Bedrock KB sync
- [x] **Recall flow:** `aws apigateway test-invoke-method` on recallKnowledge with query matching submitted note → verify results returned → verify recall log in S3
- [x] **Dream cycle:** Wait for dream cycle tick → verify pending notes processed → verify status changed to active → verify S3 sync + reindex
- [x] **EC2 recovery:** Terminate EC2 instance → verify ASG launches replacement → verify new instance boots, clones CodeCommit, starts CLI process → verify periodic tick resumes
- [x] **SSM access:** Verify `aws ssm start-session --target <instance-id>` connects
- [x] **CloudWatch:** Verify Lambda logs, EC2 CLI logs, and API access logs visible in CloudWatch
- [x] **Alarms:** Verify DLQ alarm fires when a test message is sent to DLQ
- [x] All validation within 30-minute success criterion (single `cdk deploy` → working KB in 12.5 min)

### QAT-006: Bedrock KB Metadata Extraction Verification
**Description:** Verify that Bedrock KB correctly extracts YAML frontmatter fields (`uid`, `title`) from Markdown notes as queryable metadata in the Retrieve API response. This is a critical assumption that must be validated before implementation proceeds past Phase 0. Automated test script at `test/integration/qat-006-metadata-extraction.sh`.
**Dependencies:** KBS-003 (data source created), STR-001 (S3 bucket)
**Acceptance Criteria:**
- [x] Deploy a minimal Bedrock KB with the CDK stack's data source configuration (S3 bucket + OpenSearch collection)
- [x] Upload a test Markdown note with YAML frontmatter containing `uid` and `title` fields to S3
- [x] Trigger a data source sync (`StartIngestionJob`) and wait for completion
- [x] Call `bedrock-agent-runtime:Retrieve` with a query matching the test note content
- [x] Confirm `retrievalResults[].metadata.uid` contains the expected UID value
- [x] Confirm `retrievalResults[].metadata.title` contains the expected title value
- [x] If metadata extraction does NOT work as expected, document the actual response structure and update `contracts/recall-knowledge.md` field mapping accordingly
- [x] Document findings in research.md R-2

---

## Dependency Graph (Critical Path)

```
ENV-001 → ENV-002 → NET-001 → NET-002
                            → NET-003 ────────────────────────→ SRC-003
                            → NET-004 ────────────────────────→ CMP-002
                  → STR-001 ──→ STR-004
                            ──→ KBS-002 ──→ KBS-001 ──→ KBS-003 ──→ SRC-005
                                                      ──→ LMB-005
                  → STR-002 ──→ STR-004
                            ──→ CMP-001
                  → STR-003 ──→ STR-004
                            ──→ LMB-003 ──→ API-002
                  → SRC-001 ──→ SRC-002
                            ──→ SRC-003
                            ──→ KBS-002
                            ──→ SRC-004 (needs CMP-001 + KBS-002 + SRC-006 Lambda role)
                            ──→ SRC-006 (needs SRC-004 + NET-003) ──→ KBS-001
       → ENV-003
       → LMB-001 → LMB-002 → LMB-003 ──→ API-002 ──┐
                 → LMB-004 → LMB-005 ──→ API-003 ──┤
                                                     ├→ API-004
       → API-001 ─────────────────────→ API-002 ────┘
                                      → API-003

       → CMP-001 → CMP-002 → CMP-004 → CMP-005
                 → CMP-003

       → OBS-001
       → OBS-002

       All constructs → WIR-001 → WIR-002
```

**Critical Path:** ENV-001 → ENV-002 → STR-001 → KBS-002 → SRC-004 → SRC-006 → KBS-001 → KBS-003 → LMB-005 → API-003 → WIR-001 → WIR-002

This is the longest dependency chain (~12 sequential tasks) running through storage → search → index creation → KB → recall Lambda → API → wiring. The critical path is now 2 tasks longer due to SRC-006 (index pre-creation) which must complete between SRC-004 and KBS-001. The networking chain (NET-001 → NET-003 → SRC-003) runs in parallel and typically completes faster.

**Note on SRC-004 / SRC-006 ordering:** SRC-004 (data access policy) and SRC-006 (index creation) have a mutual dependency: SRC-004 needs SRC-006's Lambda role ARN, and SRC-006 needs the data access policy to grant access. Resolution: create the SRC-006 Lambda role first (as part of SRC-006 construct), pass its ARN to SRC-004, then have the custom resource execution depend on both the data access policy and the collection being ready.

## Parallel Execution Groups

| Group | Tasks | Can Start After |
|-------|-------|-----------------|
| Setup Parallel | ENV-002, ENV-003 | ENV-001 |
| Storage Parallel | STR-001, STR-002, STR-003 | ENV-002 |
| Networking + Search Parallel | NET-001..NET-004, SRC-001, SRC-002 | ENV-002 |
| Lambda Shared + API Shell | LMB-001, API-001 | ENV-001 / ENV-002 |
| Lambda Handlers | LMB-002, LMB-004 | LMB-001 |
| Lambda Constructs | LMB-003, LMB-005 | LMB-002/LMB-004 + storage/KB |
| API Endpoints | API-002, API-003 | API-001 + Lambda constructs |
| Compute Chain | CMP-001 → CMP-002/CMP-003 → CMP-004 | Storage + Search + KB |
| Observability | OBS-001, OBS-002 | API + Storage (DLQ) |
| KB Chain | KBS-002 → SRC-004 → SRC-006 → KBS-001 → KBS-003 | STR-001 + SRC-001 + CMP-001 |
| Quality Parallel | QAT-001, QAT-002, QAT-003, QAT-004 | WIR-001 |
| Stack Outputs | STR-004, SRC-005, API-004, CMP-005 | Respective constructs |
