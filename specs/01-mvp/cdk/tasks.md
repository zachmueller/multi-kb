# Task Breakdown: Multi-KB CDK Infrastructure — MVP

**Created:** 2026-05-01
**Implementation Plan:** [plan.md](plan.md)
**Specification:** [spec.md](spec.md)
**Status:** Planning

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
- [ ] `cdk init app --language typescript` or equivalent manual setup complete
- [ ] All directories from plan.md CDK Stack Architecture exist
- [ ] `npx tsc --noEmit` succeeds
- [ ] `cdk synth` produces a valid (empty) CloudFormation template
- [ ] `npm test` runs Jest with no failures

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
- [ ] `MultiKbStackProps` interface defines all 12 parameters from spec: `repoName`, `bucketPrefix`, `ec2InstanceType`, `embeddingModelId`, `consolidationModelId`, `coverageModelId`, `tickInterval`, `dreamCycleInterval`, `excludePendingFromRecall`, `coverageScoreThreshold`, `cliBinaryS3Uri`, `vpcId`
- [ ] Defaults applied for all optional parameters per spec table
- [ ] `cliBinaryS3Uri` is required — synthesis fails with clear error if missing
- [ ] Context values resolved from `cdk.json` and CLI `--context` flags
- [ ] Test: synth succeeds with only `cliBinaryS3Uri` provided; synth fails without it

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
- [ ] `npm run build` compiles TypeScript
- [ ] `npm test` runs Jest with CDK assertions module available
- [ ] `npm run lint` runs ESLint with no errors on scaffold
- [ ] Snapshot test infrastructure ready (`jest --updateSnapshot` works)
- [ ] `cdk.json` configures app entry point and default context

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
- [ ] When `vpcId` is not provided: creates new VPC with a single private subnet in one AZ
- [ ] When `vpcId` is provided: imports existing VPC via `Vpc.fromLookup()`
- [ ] No public subnets, no NAT gateway
- [ ] Single AZ for cost optimization (all endpoints and ASG pinned to same AZ)
- [ ] Exports: VPC, subnet, availability zone
- [ ] CDK assertion test: VPC resource exists with expected CIDR; no NAT gateway resource; single subnet

### NET-002: S3 Gateway Endpoint
**Description:** Create the S3 gateway VPC endpoint per spec FR-4.
**Files:**
- `lib/constructs/networking.ts` — S3 gateway endpoint addition
- `test/constructs/networking.test.ts` — additional assertions
**Dependencies:** NET-001
**Acceptance Criteria:**
- [ ] Gateway endpoint for `com.amazonaws.{region}.s3`
- [ ] Associated with the private subnet's route table
- [ ] Free resource (no hourly cost)
- [ ] CDK assertion test: VPC endpoint resource with `s3` service name and `Gateway` type

### NET-003: Interface VPC Endpoints (9 endpoints)
**Description:** Create all 9 interface VPC endpoints per spec FR-4 VPC endpoint list and research.md R-4.
**Files:**
- `lib/constructs/networking.ts` — interface endpoint creation
- `test/constructs/networking.test.ts` — endpoint assertions
**Dependencies:** NET-001, NET-004
**Acceptance Criteria:**
- [ ] Creates **8 standard interface endpoints** via `ec2.InterfaceVpcEndpoint`:
  1. `com.amazonaws.{region}.sqs`
  2. `com.amazonaws.{region}.git-codecommit`
  3. `com.amazonaws.{region}.bedrock-runtime`
  4. `com.amazonaws.{region}.bedrock-agent`
  5. `com.amazonaws.{region}.ssm`
  6. `com.amazonaws.{region}.ssmmessages`
  7. `com.amazonaws.{region}.ec2messages`
  8. `com.amazonaws.{region}.logs` (CloudWatch Logs)
- [ ] Creates **1 AOSS VPC endpoint** via `opensearchserverless.CfnVpcEndpoint` (L1 construct) — NOT `ec2.InterfaceVpcEndpoint`. This uses `AWS::OpenSearchServerless::VpcEndpoint`, not `AWS::EC2::VPCEndpoint`.
  - AOSS endpoint props: `name` (must match `^[a-z][a-z0-9-]{2,31}$`), `vpcId`, `subnetIds`, `securityGroupIds`
  - AOSS endpoint returns `attrId` (the VPC endpoint ID used in the network policy's `SourceVPCEs` field)
- [ ] All endpoints placed in the single private subnet (same AZ)
- [ ] All endpoints (8 standard + 1 AOSS) share the VPC endpoint security group (NET-004)
- [ ] **CRITICAL: `open: false`** on all `InterfaceVpcEndpoint` constructs to prevent CDK from auto-adding permissive `0.0.0.0/0` ingress rule
- [ ] Private DNS enabled on all 8 standard interface endpoints
- [ ] Exports: AOSS VPC endpoint ID (needed by SRC-003 network policy `SourceVPCEs`)
- [ ] CDK assertion test: 8 `AWS::EC2::VPCEndpoint` resources + 1 `AWS::OpenSearchServerless::VpcEndpoint` resource; all reference the endpoint SG

### NET-004: Security Groups
**Description:** Create security groups for EC2 instance and VPC endpoints per research.md R-4.
**Files:**
- `lib/constructs/networking.ts` — EC2 SG, endpoint SG, rules
- `test/constructs/networking.test.ts`
**Dependencies:** NET-001
**Acceptance Criteria:**
- [ ] EC2 security group: `allowAllOutbound: false`, explicit egress TCP 443 to endpoint SG only
- [ ] VPC endpoint security group: `allowAllOutbound: false`, explicit ingress TCP 443 from EC2 SG only
- [ ] No public inbound rules on either SG
- [ ] Both SGs shared by all 9 interface endpoints (8 standard + 1 AOSS)
- [ ] AOSS `CfnVpcEndpoint` receives `[endpointSg.securityGroupId]` as `securityGroupIds` (string, not construct reference)
- [ ] Exports: EC2 SG (for compute construct), endpoint SG (for endpoints)
- [ ] CDK assertion test: two SG resources with correct ingress/egress rules; no `0.0.0.0/0` rules

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
- [ ] Bucket name derived from `bucketPrefix` prop + account/region suffix for uniqueness
- [ ] Server-side encryption: SSE-S3 (AES-256)
- [ ] Public access blocked (all four block settings enabled)
- [ ] Versioning disabled
- [ ] No lifecycle rules (MVP)
- [ ] `removalPolicy: RemovalPolicy.RETAIN` (protect against accidental `cdk destroy`)
- [ ] Exports: bucket object, bucket name, bucket ARN
- [ ] CDK assertion test: bucket has encryption, public access blocked, versioning off

### STR-002 [P]: CodeCommit Repository
**Description:** Create the CodeCommit git repository per spec FR-5.
**Files:**
- `lib/constructs/storage.ts` — CodeCommit repository addition
- `test/constructs/storage.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [ ] Repository name from `repoName` prop (default: `"multi-kb"`)
- [ ] Exports: repository object, clone URL (HTTPS), repository ARN
- [ ] CDK assertion test: CodeCommit repository resource with expected name

### STR-003 [P]: SQS Queue with DLQ
**Description:** Create the SQS standard queue and dead-letter queue per spec FR-3.
**Files:**
- `lib/constructs/storage.ts` — SQS queue + DLQ
- `test/constructs/storage.test.ts`
**Dependencies:** ENV-002
**Acceptance Criteria:**
- [ ] Standard queue (not FIFO)
- [ ] Visibility timeout: 5 minutes (300 seconds)
- [ ] Message retention: 14 days (1,209,600 seconds)
- [ ] DLQ configured with `maxReceiveCount: 3`
- [ ] DLQ has 14-day retention
- [ ] Exports: queue object, queue URL, queue ARN, DLQ object, DLQ ARN
- [ ] CDK assertion test: queue has correct visibility timeout and retention; DLQ exists; redrive policy configured with maxReceiveCount 3

### STR-004: Stack Outputs — Storage
**Description:** Add CloudFormation outputs for storage resources per spec Stack Outputs.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput additions
**Dependencies:** STR-001, STR-002, STR-003
**Acceptance Criteria:**
- [ ] Output `BucketName`: S3 bucket name
- [ ] Output `RepoCloneUrl`: CodeCommit HTTPS clone URL
- [ ] CDK assertion test: outputs exist with expected logical IDs

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
- [ ] Uses `CfnCollection` (L1) from `aws-cdk-lib/aws-opensearchserverless`
- [ ] Type: `VECTORSEARCH`
- [ ] Collection name: configurable, must match `^[a-z][a-z0-9-]{2,27}$`
- [ ] `standbyReplicas: 'DISABLED'` for MVP cost savings
- [ ] **`collection.addDependency(encryptionPolicy)`** — collection FAILS without this
- [ ] **`collection.addDependency(networkPolicy)`** — recommended so network access is ready when collection comes online
- [ ] Exports: `collection.attrArn`, `collection.attrCollectionEndpoint`, `collection.attrId`, collection name
- [ ] CDK assertion test: collection resource with type `VECTORSEARCH`; `DependsOn` includes encryption policy

### SRC-002: Encryption Policy
**Description:** Create the OpenSearch Serverless encryption policy per spec FR-7 and research.md R-1.
**Files:**
- `lib/constructs/search.ts` — `CfnSecurityPolicy` from `aws-cdk-lib/aws-opensearchserverless`
- `test/constructs/search.test.ts`
**Dependencies:** None (must be created BEFORE SRC-001 collection)
**Acceptance Criteria:**
- [ ] Uses `CfnSecurityPolicy` with `type: 'encryption'`
- [ ] Policy name must match `^[a-z][a-z0-9-]{2,31}$`
- [ ] Policy JSON is a **single object** (NOT an array — unlike network and data access policies)
- [ ] Policy specifies `AWSOwnedKey: true`
- [ ] `Rules` target the collection by name: `[{ "ResourceType": "collection", "Resource": ["collection/<name>"] }]`
- [ ] Policy is `JSON.stringify()`-ed into the `policy` string prop
- [ ] SRC-001 must call `collection.addDependency(encryptionPolicy)` — collection creation FAILS without encryption policy
- [ ] CDK assertion test: security policy resource with type `encryption` and `AWSOwnedKey: true`

### SRC-003: Network Policy (Dual Access)
**Description:** Create the OpenSearch Serverless network policy allowing both VPC and Bedrock service access per research.md R-6.
**Files:**
- `lib/constructs/search.ts` — `CfnSecurityPolicy` with type `network`
- `test/constructs/search.test.ts`
**Dependencies:** NET-003 (for AOSS VPC endpoint ID)
**Acceptance Criteria:**
- [ ] Uses `CfnSecurityPolicy` with type `network`
- [ ] **`AllowFromPublic: false`** — do NOT set to `true` (it silently overrides `SourceVPCEs` and `SourceServices`, making the collection fully public)
- [ ] `SourceVPCEs: [<aoss-vpc-endpoint-id>]` — enables EC2 direct access via VPC endpoint. Note: field name is `SourceVPCEs` (NOT `SourceVPCEndpoints`)
- [ ] `SourceServices: ["bedrock.amazonaws.com"]` — enables Bedrock service access via AWS internal networking
- [ ] All field names are case-sensitive: `AllowFromPublic`, `SourceVPCEs`, `SourceServices`, `ResourceType`, `Resource`
- [ ] Policy JSON is an **array** (unlike encryption policy which is a single object)
- [ ] Both access paths work: EC2 via VPC endpoint, Bedrock via service private networking
- [ ] Three-layer security model: network policy (origin) + data access policy (principal) + IAM
- [ ] CDK assertion test: network security policy exists with `AllowFromPublic: false`, `SourceVPCEs`, and `SourceServices`

### SRC-004: Data Access Policy
**Description:** Create the OpenSearch Serverless data access policy granting access to EC2 role, Bedrock KB service role, and index creation Lambda role per research.md R-1.
**Files:**
- `lib/constructs/search.ts` — `CfnAccessPolicy` from `aws-cdk-lib/aws-opensearchserverless`
- `test/constructs/search.test.ts`
**Dependencies:** SRC-001 (collection name), CMP-001 (EC2 role ARN), KBS-002 (Bedrock KB service role ARN), SRC-006 (index creation Lambda role ARN)
**Acceptance Criteria:**
- [ ] Uses `CfnAccessPolicy` with `type: 'data'`
- [ ] Policy name must match `^[a-z][a-z0-9-]{2,31}$`
- [ ] **Principals:** EC2 instance role ARN + Bedrock KB service role ARN + index creation Lambda role ARN (all passed as CDK tokens via `JSON.stringify()`)
- [ ] CDK tokens in `JSON.stringify()` resolve to `Fn::Join`/`Fn::Sub` intrinsics — this is well-supported
- [ ] Index-level permissions: `aoss:ReadDocument`, `aoss:WriteDocument`, `aoss:CreateIndex`, `aoss:DeleteIndex`, `aoss:UpdateIndex`, `aoss:DescribeIndex`
- [ ] Collection-level permissions: `aoss:CreateCollectionItems`, `aoss:DeleteCollectionItems`, `aoss:UpdateCollectionItems`, `aoss:DescribeCollectionItems`
- [ ] Resources: `index/<collection-name>/*` and `collection/<collection-name>`
- [ ] **No circular dependency:** CDK tokens resolve all forward references. Roles reference collection ARN; policy references role ARNs. CloudFormation determines creation order via implicit `DependsOn`.
- [ ] **Principals must ALSO have IAM-level `aoss:APIAccessAll`** on the collection ARN in their own IAM policies (AOSS data access policy and IAM policy work together)
- [ ] CDK assertion test: access policy exists with correct principals and permissions

### KBS-001: Bedrock Knowledge Base
**Description:** Create the Bedrock Knowledge Base with OpenSearch Serverless as the vector store per spec FR-8 and research.md R-2.
**Files:**
- `lib/constructs/knowledge-base.ts` — `KnowledgeBaseConstruct`: `CfnKnowledgeBase` from `aws-cdk-lib/aws-bedrock`
- `test/constructs/knowledge-base.test.ts`
**Dependencies:** SRC-001 (collection ARN), KBS-002 (service role), **SRC-006 (vector index must exist before KB creation)**
**Acceptance Criteria:**
- [ ] Uses `CfnKnowledgeBase` (L1) from `aws-cdk-lib/aws-bedrock`
- [ ] `knowledgeBaseConfiguration.type`: `'VECTOR'`
- [ ] Embedding model: `embeddingModelArn` constructed from configurable model ID (default: `arn:aws:bedrock:{region}::foundation-model/amazon.titan-embed-text-v2:0`)
- [ ] `storageConfiguration.type`: `'OPENSEARCH_SERVERLESS'`
- [ ] `opensearchServerlessConfiguration`: `collectionArn`, `vectorIndexName`, and `fieldMapping` (all three fields required)
- [ ] **Field mappings must match the pre-created index schema (SRC-006):**
  - `vectorField`: `'bedrock-knowledge-base-default-vector'`
  - `textField`: `'AMAZON_BEDROCK_TEXT_CHUNK'`
  - `metadataField`: `'AMAZON_BEDROCK_METADATA'`
- [ ] Role ARN: Bedrock KB service role (KBS-002)
- [ ] KB depends on SRC-006 custom resource (index must exist before KB)
- [ ] Exports: `attrKnowledgeBaseId`, `attrKnowledgeBaseArn`
- [ ] CDK assertion test: KB resource with correct embedding model ARN, storage type, collection ARN, and field mappings

### KBS-002: Bedrock KB Service Role
**Description:** Create the IAM role assumed by Bedrock to access S3, OpenSearch, and the embedding model per research.md R-2.
**Files:**
- `lib/constructs/knowledge-base.ts` — service role for Bedrock KB
- `test/constructs/knowledge-base.test.ts`
**Dependencies:** STR-001 (S3 bucket ARN), SRC-001 (collection ARN), ENV-002 (embedding model ID)
**Acceptance Criteria:**
- [ ] IAM role with trust policy for `bedrock.amazonaws.com` service principal
- [ ] Trust policy conditions: `aws:SourceAccount` (account ID) + `ArnLike` condition on `arn:aws:bedrock:{region}:{account}:knowledge-base/*`
- [ ] Permissions: `s3:GetObject` on `{bucket-arn}/*` + `s3:ListBucket` on bucket ARN (both with `aws:ResourceAccount` condition)
- [ ] Permissions: `aoss:APIAccessAll` on the collection ARN
- [ ] **Permissions: `bedrock:InvokeModel` on the foundation model ARN** (e.g., `arn:aws:bedrock:{region}::foundation-model/amazon.titan-embed-text-v2:0`). Note: model ARN uses empty account ID (`::`) because foundation models are AWS-owned.
- [ ] Least-privilege: no wildcard permissions, all scoped to specific ARNs
- [ ] Exports: role, role ARN (needed by SRC-004 data access policy and KBS-001)
- [ ] CDK assertion test: role with correct trust policy (including conditions), 4 permission statements on specific ARNs

### KBS-003: Bedrock KB Data Source (S3)
**Description:** Create the Bedrock Knowledge Base data source pointing to the S3 bucket with "no chunking" strategy per spec FR-8.
**Files:**
- `lib/constructs/knowledge-base.ts` — CfnDataSource
- `test/constructs/knowledge-base.test.ts`
**Dependencies:** KBS-001 (KB ID), STR-001 (S3 bucket ARN)
**Acceptance Criteria:**
- [ ] Uses `CfnDataSource` (L1) from `aws-bedrock`
- [ ] Linked to the Knowledge Base (KBS-001)
- [ ] S3 configuration: points to the KB bucket
- [ ] Chunking strategy: `NONE` (each note is its own chunk)
- [ ] Exports: data source ID (needed by EC2 config for StartIngestionJob)
- [ ] CDK assertion test: data source resource with chunking strategy `NONE` and correct S3 bucket

### SRC-006: Custom Resource — OpenSearch Vector Index Creation
**Description:** Create a CDK custom resource (Lambda-backed) that pre-creates the OpenSearch vector index. Bedrock KB does NOT auto-create the index via CloudFormation — this is a console-only feature (research.md R-2).
**Files:**
- `lib/constructs/search.ts` — custom resource addition to `SearchConstruct`
- `lambda/custom-resource/create-index.ts` — custom resource handler
- `test/lambda/custom-resource/create-index.test.ts` — handler unit tests
**Dependencies:** SRC-001 (collection endpoint, collection name), SRC-004 (data access policy — index creation Lambda needs access), NET-003 (AOSS VPC endpoint — Lambda must be in VPC to reach collection)
**Acceptance Criteria:**
- [ ] CDK `Provider` + `CustomResource` construct backed by a Lambda function
- [ ] Lambda sends PUT request to OpenSearch collection endpoint to create index with schema:
  - `settings.index.knn`: `true`
  - `bedrock-knowledge-base-default-vector` field: `knn_vector`, 1024 dimensions, `faiss` engine, `hnsw` method, `l2` space type
  - `AMAZON_BEDROCK_TEXT_CHUNK` field: `text`, `index: true`
  - `AMAZON_BEDROCK_METADATA` field: `text`, `index: false`
- [ ] Index name configurable (default: `'bedrock-kb-index'`)
- [ ] Lambda is VPC-attached (needs to reach collection via AOSS VPC endpoint)
- [ ] Lambda security group: same EC2 SG or a separate SG with outbound 443 to endpoint SG
- [ ] Lambda IAM role: `aoss:APIAccessAll` on collection ARN
- [ ] Lambda role must be listed in AOSS data access policy (SRC-004) principals
- [ ] On Create: creates the index; on Update: no-op (index schema is immutable); on Delete: optionally deletes the index
- [ ] Idempotent: if index already exists, succeeds without error
- [ ] Vector dimension (1024) matches the embedding model configuration in KBS-001
- [ ] CDK assertion test: custom resource exists; Lambda is VPC-attached; IAM role has `aoss:APIAccessAll`

### SRC-005: Stack Outputs — Search
**Description:** Add CloudFormation outputs for search infrastructure.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput additions
**Dependencies:** SRC-001, KBS-001
**Acceptance Criteria:**
- [ ] Output `CollectionEndpoint`: OpenSearch Serverless collection endpoint
- [ ] Output `KnowledgeBaseId`: Bedrock KB ID
- [ ] CDK assertion test: outputs exist

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
- [ ] **UID generation (R-5):** `crypto.randomBytes(10)` → 16-char Crockford base32 (alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ`); uppercase; exactly 16 chars
- [ ] Uses bit-buffer encoding algorithm (R-5): accumulate 8 bits per byte, extract 5-bit groups MSB-first via `(buf >>> bits) & 0x1F` (unsigned right shift)
- [ ] Zero npm dependencies — uses only Node.js built-in `crypto`
- [ ] `encodeCrockford(Buffer)` exported separately from `generateUid()` for deterministic testing
- [ ] **Response helpers (R-7):** Four functions in `lambda/shared/response.ts`: `success(statusCode, body)` for 200/202 responses, `error(statusCode, body)` for arbitrary errors, `validationError(errors: Record<string, string>)` convenience for HTTP 400 `{ errors: { field: reason } }`, `internalError()` convenience for HTTP 500 with generic message. All auto-stringify body and set `Content-Type: application/json` via shared `JSON_HEADERS` constant. Body parameter is `unknown` (helper calls `JSON.stringify()` internally). Uses `APIGatewayProxyResult` type from `@types/aws-lambda` (dev dependency — erased at compile time).
- [ ] **Handler wrapper pattern (R-7):** Both Lambda handlers wrap logic in top-level try/catch returning `internalError()`. This guarantees well-formed responses (never 502 from malformed response) and preserves 500 vs 502 distinction for debugging. Guard `event.body` with `JSON.parse(event.body ?? '{}')` — catch `SyntaxError` to return 400.
- [ ] **HTTP 401/403 not handled by Lambda (R-7):** API Gateway handles auth errors before Lambda invocation for `AWS_IAM` auth. Lambda does not need 401/403 response paths.
- [ ] **Validation:** `validateSubmitKnowledge(body)` returns `{ valid: true, data }` or `{ valid: false, errors: {} }`; validates title (present, non-empty, ≤255), content (present, non-empty, ≤100K), author (present, non-empty, ≤100)
- [ ] Test: UID deterministic encoding of 5 shared test vectors from R-5:
  - `Buffer.from([0x00 × 10])` → `"0000000000000000"`
  - `Buffer.from([0xFF × 10])` → `"ZZZZZZZZZZZZZZZZ"`
  - `Buffer.from([0x00..0x09])` → `"000G40R40M30E209"`
  - `Buffer.from([0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x42])` → `"VTPVXVYAZTXBW022"`
  - `Buffer.from("HelloWorld")` → `"91JPRV3FAXQQ4V34"`
- [ ] Test: UID format (length=16, valid alphabet, no I/L/O/U, uniqueness over 1K), response shape, all validation rules from contracts/submit-knowledge.md table

### LMB-002: submitKnowledge Lambda Handler
**Description:** Implement the submitKnowledge Lambda per spec FR-2 and contracts/submit-knowledge.md.
**Files:**
- `lambda/submit/index.ts` — handler function
- `test/lambda/submit.test.ts` — handler unit tests
**Dependencies:** LMB-001
**Acceptance Criteria:**
- [ ] Parses `event.body` (JSON string from API Gateway proxy)
- [ ] Calls validation; returns HTTP 400 with `{ errors: {...} }` on failure (only failed fields)
- [ ] Generates UID (16-char Crockford base32) and `submitted_at` (ISO 8601 current time)
- [ ] Sends SQS message: `{ uid, title, content, author, submitted_at }` serialized as JSON body
- [ ] Returns HTTP 202: `{ uid, request_id: event.requestContext.requestId }`
- [ ] On SQS failure: returns HTTP 500 with generic message; logs full error
- [ ] Reads `SQS_QUEUE_URL` from `process.env`
- [ ] Test (mocked SQS): valid submission, each field validation failure (6 cases), whitespace-only fields, SQS send failure

### LMB-003: submitKnowledge CDK Construct
**Description:** Create the CDK construct that provisions the submitKnowledge Lambda with correct runtime, permissions, and environment.
**Files:**
- `lib/constructs/submit-lambda.ts` — `SubmitLambdaConstruct`: NodejsFunction, IAM role
- `test/constructs/submit-lambda.test.ts`
**Dependencies:** LMB-002, STR-003 (SQS queue)
**Acceptance Criteria:**
- [ ] Uses `NodejsFunction` with esbuild bundling
- [ ] Runtime: `nodejs22.x`, architecture: ARM64
- [ ] Memory: 256 MB, timeout: 10 seconds
- [ ] Environment variable: `SQS_QUEUE_URL` from queue construct
- [ ] IAM: `sqs:SendMessage` scoped to queue ARN only
- [ ] Exports: Lambda function (for API Gateway integration)
- [ ] CDK assertion test: Lambda runtime, memory, timeout; IAM policy with `sqs:SendMessage` on specific ARN; environment variable set

### LMB-004: recallKnowledge Lambda Handler
**Description:** Implement the recallKnowledge Lambda per spec FR-9 and contracts/recall-knowledge.md.
**Files:**
- `lambda/recall/index.ts` — handler function
- `test/lambda/recall.test.ts` — handler unit tests
**Dependencies:** LMB-001, R-2 research (Bedrock KB metadata extraction — determines how uid/title are extracted from Retrieve results), PRM-003 (coverage assessment prompt)
**Acceptance Criteria:**
- [ ] Parses `event.body` — extracts `query` (required, string) and `limit` (optional, integer, default 10)
- [ ] Validates `query`: present, non-empty string; returns HTTP 400 on failure
- [ ] Validates `limit`: must be integer >= 1 and <= 100; returns HTTP 400 with `{ errors: { limit: "must be an integer between 1 and 100" } }` if out of range. Non-integer values return 400. Default 10 if omitted.
- [ ] Calls Bedrock Retrieve API with `knowledgeBaseId`, `retrievalQuery.text = query`
- [ ] Optionally filters to `status: active` when `EXCLUDE_PENDING` is `true`
- [ ] Maps Retrieve response to `[{ uid, title, content, score }]` sorted by descending score
- [ ] Coverage assessment: if top score < `COVERAGE_SCORE_THRESHOLD`:
  - Calls InvokeModel (coverage LLM) with query + result summaries
  - If gap detected: follow-up Retrieve query
  - Deduplicates by UID, sorts by score, truncates to `limit`
  - On any coverage failure: falls back to original results silently
- [ ] Writes recall log to S3 synchronously: `recall-logs/<YYYY-MM-DD>/<request-id>.json`
  - Best-effort: S3 failure logged but doesn't affect response
- [ ] Returns HTTP 200 with results array (or empty array)
- [ ] Reads env vars: `KNOWLEDGE_BASE_ID`, `BUCKET_NAME`, `COVERAGE_MODEL_ID`, `COVERAGE_SCORE_THRESHOLD`, `EXCLUDE_PENDING`
- [ ] Test (mocked Bedrock/S3): successful recall, empty results, coverage assessment trigger, coverage fallback, S3 write failure, validation error, limit=0 → 400, limit=-1 → 400, limit=101 → 400, limit=50 → 200, limit omitted → uses default 10

### LMB-005: recallKnowledge CDK Construct
**Description:** Create the CDK construct that provisions the recallKnowledge Lambda with correct runtime, permissions, and environment.
**Files:**
- `lib/constructs/recall-lambda.ts` — `RecallLambdaConstruct`: NodejsFunction, IAM role
- `test/constructs/recall-lambda.test.ts`
**Dependencies:** LMB-004, KBS-001 (KB ID), STR-001 (S3 bucket)
**Acceptance Criteria:**
- [ ] Uses `NodejsFunction` with esbuild bundling
- [ ] Runtime: `nodejs22.x`, architecture: ARM64
- [ ] Memory: 1024 MB, timeout: 30 seconds
- [ ] Environment variables: `KNOWLEDGE_BASE_ID`, `BUCKET_NAME`, `COVERAGE_MODEL_ID`, `COVERAGE_SCORE_THRESHOLD`, `EXCLUDE_PENDING`
- [ ] IAM: `bedrock:Retrieve` on KB ARN; `bedrock:InvokeModel` on coverage model ARN; `s3:PutObject` on `{bucket-arn}/recall-logs/*`
- [ ] Lambda is NOT VPC-attached (calls public Bedrock endpoints)
- [ ] Exports: Lambda function (for API Gateway integration)
- [ ] CDK assertion test: Lambda runtime, memory, timeout; IAM with three permission sets scoped to specific ARNs; environment variables; NOT in VPC

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
- [ ] REST API (not HTTP API) — required for `AWS_IAM` auth
- [ ] `prod` stage deployed
- [ ] Access logging enabled (CloudWatch Logs)
- [ ] CORS not enabled (spec: "CLI is not a browser client")
- [ ] Exports: API object, endpoint URL
- [ ] CDK assertion test: REST API resource; stage named `prod`; access log destination configured

### API-002: submitKnowledge Endpoint
**Description:** Create the `POST /submitKnowledge` resource, method, and Lambda proxy integration per spec FR-1, FR-2.
**Files:**
- `lib/constructs/api.ts` — resource + method + integration
- `test/constructs/api.test.ts`
**Dependencies:** API-001, LMB-003
**Acceptance Criteria:**
- [ ] Resource: `/submitKnowledge`
- [ ] Method: `POST` with `AWS_IAM` authorization
- [ ] Lambda proxy integration with submitKnowledge function
- [ ] Unauthorized requests receive HTTP 401; insufficient permissions receive HTTP 403
- [ ] CDK assertion test: API resource with `POST` method; IAM auth type; Lambda integration

### API-003 [P]: recallKnowledge Endpoint
**Description:** Create the `POST /recallKnowledge` resource, method, and Lambda proxy integration per spec FR-1, FR-9.
**Files:**
- `lib/constructs/api.ts` — resource + method + integration
- `test/constructs/api.test.ts`
**Dependencies:** API-001, LMB-005
**Acceptance Criteria:**
- [ ] Resource: `/recallKnowledge`
- [ ] Method: `POST` with `AWS_IAM` authorization
- [ ] Lambda proxy integration with recallKnowledge function
- [ ] CDK assertion test: API resource with `POST` method; IAM auth type; Lambda integration

### API-004: Stack Output — API Endpoint
**Description:** Add CloudFormation outputs for API Gateway.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput additions
**Dependencies:** API-001
**Acceptance Criteria:**
- [ ] Output `ApiEndpoint`: full API Gateway endpoint URL (e.g., `https://{api-id}.execute-api.{region}.amazonaws.com/prod`)
- [ ] Output `ApiId`: API Gateway REST API ID
- [ ] CDK assertion test: outputs exist

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
- [ ] IAM role with trust policy for `ec2.amazonaws.com`
- [ ] Permissions (all scoped to specific resource ARNs):
  - `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes` on queue ARN
  - `codecommit:GitPull`, `codecommit:GitPush` on repo ARN
  - `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket` on KB bucket ARN
  - `s3:GetObject` on CLI binary S3 object ARN (parsed from `cliBinaryS3Uri`)
  - `aoss:APIAccessAll` on OpenSearch collection ARN
  - `bedrock:InvokeModel` on consolidation model ARN
  - `bedrock-agent:StartIngestionJob`, `bedrock-agent:GetIngestionJob` (scoped to KB/data source)
  - SSM Session Manager permissions (`ssm:UpdateInstanceInformation`, `ssmmessages:*`, `ec2messages:*`)
- [ ] Instance profile created from role
- [ ] Exports: role, role ARN, instance profile
- [ ] CDK assertion test: role trust policy; each permission statement verified for specific ARN (no wildcards except SSM messages); instance profile exists

### CMP-002: Launch Template
**Description:** Create the EC2 launch template with Amazon Linux 2023, instance type, and security group per spec FR-4.
**Files:**
- `lib/constructs/compute.ts` — launch template
- `test/constructs/compute.test.ts`
**Dependencies:** CMP-001, NET-004 (EC2 SG)
**Acceptance Criteria:**
- [ ] Amazon Linux 2023 AMI (latest, ARM64 or x86_64 matching instance type)
- [ ] Instance type from `ec2InstanceType` prop (default: `t3.small`)
- [ ] Instance profile from CMP-001
- [ ] Security group: EC2 SG from NET-004
- [ ] No public IP (private subnet only)
- [ ] User data script from CMP-003
- [ ] CDK assertion test: launch template with correct instance type; no associate public IP

### CMP-003: User Data Script
**Description:** Implement the EC2 user data script that bootstraps the instance per spec FR-4 and research.md R-3.
**Files:**
- `lib/constructs/compute.ts` — user data script generation
**Dependencies:** CMP-001, CMP-004 (for `addSignalOnExitCommand`), STR-001, STR-002, STR-003, SRC-001, KBS-001, KBS-003, ENV-002, OBS-001 (log group name)
**Contract:** [server-config.md](contracts/server-config.md) — defines the exact config.yaml fields to template and their CDK output sources
**Acceptance Criteria:**
- [ ] Uses `UserData.forLinux()` with `set -euxo pipefail` as first command
- [ ] Step 1: Install packages — `dnf install -y amazon-cloudwatch-agent` (git is pre-installed on AL2023; do NOT install git separately to avoid ambiguity)
- [ ] Step 2: Download CLI binary — `aws s3 cp ${cliBinaryS3Uri} /usr/local/bin/multi-kb && chmod +x /usr/local/bin/multi-kb` (raw `addCommands`, NOT `addS3DownloadCommand` — S3 URI is a string prop, not an `IBucket` reference)
- [ ] Step 3: Git credential helper — `git config --system credential.helper '!aws codecommit credential-helper $@'` and `git config --system credential.UseHttpPath true` (use `--system` not `--global` so it applies to all users)
- [ ] Step 4: Clone CodeCommit repo — `git clone https://git-codecommit.{region}.amazonaws.com/v1/repos/{repoName} /opt/multi-kb/repo` with `|| { git init ... }` fallback for empty repos on first deploy
- [ ] Step 5: Template `config.yaml` at `/opt/multi-kb/config.yaml` — interpolate all CDK-resolved values per [server-config.md](contracts/server-config.md) field mapping table. Use line-by-line `addCommands()` with heredoc and template literal interpolation for CDK tokens.
- [ ] Step 6: Configure CloudWatch agent — use `Stack.toJsonString()` to safely serialize JSON config containing CDK token (`logGroupName`). Config at `/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json`. Log stream name uses `{instance_id}` (CloudWatch agent variable, NOT a CDK token). Start agent with `amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:...`.
- [ ] Step 7: Create systemd unit file at `/etc/systemd/system/multi-kb.service` — `Type=simple`, `Restart=on-failure`, `RestartSec=5`, `WorkingDirectory=/opt/multi-kb/repo`, `StandardOutput=append:/var/log/multi-kb/server.log`, `StandardError=append:/var/log/multi-kb/server.log`, `Environment=AWS_REGION={region}`, `Environment=HOME=/root`. Create `/var/log/multi-kb/` directory first.
- [ ] Step 8: Start services — `systemctl daemon-reload`, start CloudWatch agent via `amazon-cloudwatch-agent-ctl`, then `systemctl enable --now multi-kb`
- [ ] **cfn-signal integration:** Call `userData.addSignalOnExitCommand(asg)` AFTER all other `addCommands()` calls. ASG uses `Signals.waitForAll({ timeout: Duration.minutes(15) })`.
- [ ] All `${...}` values resolved from CDK construct outputs at synthesis time via template literal interpolation in `addCommands()`
- [ ] Launch template sets `requireImdsv2: true` (security best practice; AWS CLI v2 on AL2023 supports IMDSv2)
- [ ] Process runs as root for MVP (no dedicated user)
- [ ] CDK assertion test: user data is non-empty; launch template has IMDSv2 enforced

### CMP-004: Auto Scaling Group
**Description:** Create the ASG with min/max/desired=1, pinned to single AZ per spec FR-4.
**Files:**
- `lib/constructs/compute.ts` — ASG configuration
- `test/constructs/compute.test.ts`
**Dependencies:** CMP-002, NET-001 (subnet)
**Acceptance Criteria:**
- [ ] Min capacity: 1, max capacity: 1, desired capacity: 1
- [ ] Pinned to single AZ (same as VPC endpoints)
- [ ] Uses launch template from CMP-002
- [ ] Health check: EC2 status checks (default)
- [ ] Instance replacement on termination: ASG launches new instance, user data re-bootstraps
- [ ] **Signals integration (R-3):** `signals: Signals.waitForAll({ timeout: Duration.minutes(15) })` — CloudFormation waits for cfn-signal from user data script before marking resource as created. On script failure, stack rolls back.
- [ ] CDK assertion test: ASG with min=max=desired=1; subnet specified; CreationPolicy present with 15-minute timeout

### CMP-005: Stack Output — Compute
**Description:** Add CloudFormation output for EC2 instance.
**Files:**
- `lib/multi-kb-stack.ts` — CfnOutput
**Dependencies:** CMP-004
**Acceptance Criteria:**
- [ ] Output `Ec2InstanceId`: EC2 instance ID (note: may need to be a custom resource or reference since ASG manages the instance)
- [ ] CDK assertion test: output exists

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
- [ ] API Gateway access log group (referenced by API stage)
- [ ] Lambda function log groups (auto-created by Lambda, but set retention)
- [ ] EC2 CLI process log group (for CloudWatch agent to ship to)
- [ ] Retention: 30 days (configurable, reasonable default for MVP)
- [ ] CDK assertion test: log group resources exist with retention policy

### OBS-002 [P]: CloudWatch Alarms
**Description:** Create CloudWatch alarms per spec NFR-4.
**Files:**
- `lib/constructs/observability.ts` — alarm definitions
- `test/constructs/observability.test.ts`
**Dependencies:** STR-003 (DLQ), CMP-004 (ASG)
**Acceptance Criteria:**
- [ ] Alarm: DLQ `ApproximateNumberOfMessagesVisible` > 0 (indicates processing failures)
- [ ] Alarm: ASG `GroupInServiceInstances` < 1 (EC2 instance unhealthy)
- [ ] Alarm: Dream cycle lock held > 60 minutes (custom metric from CLI logs — metric filter on log group)
- [ ] No alarm actions (metrics only for MVP — operators poll console)
- [ ] CDK assertion test: 3 alarm resources; no action configuration; correct metric references

---

## Phase 8: Stack Wiring

_Bring all constructs together in the main stack._

### WIR-001: Main Stack Assembly
**Description:** Wire all constructs together in `MultiKbStack`, passing outputs between constructs per plan.md Construct Dependency Graph.
**Files:**
- `lib/multi-kb-stack.ts` — complete wiring
**Dependencies:** All Phase 1-7 construct tasks
**Acceptance Criteria:**
- [ ] Instantiates all constructs in dependency order: Networking → Storage → Search → KnowledgeBase → Lambdas → API → Compute → Observability
- [ ] Passes correct references between constructs (queue to Lambda, bucket to Lambda, KB ID to Lambda, VPC to endpoints, etc.)
- [ ] Resolves the circular dependency between OpenSearch data access policy ↔ EC2 role / Bedrock KB role (CDK `addDependency()` or post-creation policy update)
- [ ] All 7 stack outputs defined: `ApiEndpoint`, `ApiId`, `RepoCloneUrl`, `KnowledgeBaseId`, `BucketName`, `CollectionEndpoint`, `Ec2InstanceId`
- [ ] `cdk synth` produces valid CloudFormation template
- [ ] Template contains all expected resource types

### WIR-002: Stack Snapshot Test
**Description:** Create a snapshot test for the fully wired stack to catch unintended changes.
**Files:**
- `test/multi-kb-stack.test.ts` — snapshot test + key assertions
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [ ] Snapshot test captures synthesized template
- [ ] Fine-grained assertions verify critical cross-construct relationships:
  - submitKnowledge Lambda env var references the actual SQS queue URL
  - recallKnowledge Lambda env var references the actual KB ID
  - EC2 IAM role has permissions on the actual SQS/S3/CodeCommit/OpenSearch ARNs
  - API Gateway methods reference the actual Lambda functions
  - VPC endpoints are in the same subnet as the ASG
- [ ] Test: `npm test` passes with snapshot matching

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
- [ ] Every construct file has a corresponding test file
- [ ] IAM policies tested for least-privilege (specific ARNs, no wildcards beyond SSM)
- [ ] Security groups tested for correct rules (no overly permissive rules)
- [ ] VPC endpoints tested for correct service names and subnet placement
- [ ] Lambda configurations tested (runtime, memory, timeout, environment, architecture)
- [ ] SQS tested for DLQ configuration
- [ ] S3 tested for encryption and public access block
- [ ] API Gateway tested for IAM authorization on both methods
- [ ] `npm test` passes with all assertions green

### QAT-002 [P]: Lambda Handler Unit Tests
**Description:** Ensure comprehensive unit test coverage for Lambda handler business logic.
**Files:**
- `test/lambda/submit.test.ts`
- `test/lambda/recall.test.ts`
- `test/lambda/shared/*.test.ts`
**Dependencies:** LMB-001, LMB-002, LMB-004
**Acceptance Criteria:**
- [ ] **submitKnowledge tests:** valid input → 202, missing title → 400, empty title → 400, long title → 400, missing content → 400, empty content → 400, long content → 400, missing author → 400, empty author → 400, long author → 400, SQS failure → 500, multiple validation errors → single 400 with all errors
- [ ] **recallKnowledge tests:** valid query → 200 with results, empty query → 400, empty results → 200 with `[]`, coverage trigger (low score) → follow-up query, coverage fallback on error, S3 write failure → still returns results, limit parameter respected
- [ ] **UID tests:** length=16, valid Crockford alphabet, no I/L/O/U, uniqueness, deterministic encoding of 5 shared test vectors from R-5 (matching CLI R-7 Go implementation)
- [ ] All AWS SDK calls mocked (no real AWS calls in unit tests)
- [ ] `npm test` passes

### QAT-003 [P]: Security Review
**Description:** Validate security requirements per spec NFR-3.
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [ ] API Gateway: both methods require `AWS_IAM` auth (no anonymous access)
- [ ] EC2 instance: private subnet, no public IP
- [ ] EC2 IAM role: least-privilege (verify each policy statement scoped to specific ARNs)
- [ ] S3 bucket: public access blocked, SSE-S3 encryption
- [ ] OpenSearch: data access policy restricts to specific principals only
- [ ] Lambda IAM roles: minimum permissions per function
- [ ] No secrets in code, environment variables, or CDK context (only IAM role-based auth)
- [ ] VPC endpoint security groups: only HTTPS/443 from EC2 SG

### QAT-004 [P]: Multi-Tenancy Validation
**Description:** Verify that the same CDK code can deploy independent instances with different stack names per spec success criteria.
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [ ] `cdk synth --context repoName=team-a-kb ...` and `cdk synth --context repoName=team-b-kb ...` produce independent templates
- [ ] Resource names derived from props, not hardcoded (S3 bucket, CodeCommit repo, SQS queue, OpenSearch collection)
- [ ] Two stacks can coexist in the same account/region without resource name conflicts
- [ ] Stack outputs are unique per deployment

### QAT-005: Post-Deploy Integration Checklist
**Description:** Manual integration test checklist for validating a deployed stack end-to-end per spec User Scenarios.
**Dependencies:** WIR-001
**Acceptance Criteria:**
- [ ] **Submit flow:** `aws apigateway test-invoke-method` on submitKnowledge → verify SQS message arrives → verify EC2 picks up message → verify CodeCommit commit → verify S3 sync → verify note appears in OpenSearch after Bedrock KB sync
- [ ] **Recall flow:** `aws apigateway test-invoke-method` on recallKnowledge with query matching submitted note → verify results returned → verify recall log in S3
- [ ] **Dream cycle:** Wait for dream cycle tick → verify pending notes processed → verify status changed to active → verify S3 sync + reindex
- [ ] **EC2 recovery:** Terminate EC2 instance → verify ASG launches replacement → verify new instance boots, clones CodeCommit, starts CLI process → verify periodic tick resumes
- [ ] **SSM access:** Verify `aws ssm start-session --target <instance-id>` connects
- [ ] **CloudWatch:** Verify Lambda logs, EC2 CLI logs, and API access logs visible in CloudWatch
- [ ] **Alarms:** Verify DLQ alarm fires when a test message is sent to DLQ
- [ ] All validation within 30-minute success criterion (single `cdk deploy` → working KB)

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
