# Research: Multi-KB CDK Infrastructure — MVP

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)
**Status:** R-1, R-2, R-4, R-6 resolved; R-3, R-5, R-7 open

## R-1: OpenSearch Serverless Collection Setup via CDK

**Question:** What CDK constructs are needed to create an OpenSearch Serverless VECTORSEARCH collection with all three policy types?

**Status:** Resolved

**Findings:**

### No L2 Constructs

`aws-cdk-lib/aws-opensearchserverless` contains only L1 constructs:

| Construct | Purpose |
|-----------|---------|
| `CfnCollection` | Creates a collection |
| `CfnSecurityPolicy` | Creates encryption or network policy (type field selects) |
| `CfnAccessPolicy` | Creates data access policy |
| `CfnVpcEndpoint` | Creates AOSS-specific VPC endpoint (NOT a standard EC2 VPC endpoint) |
| `CfnLifecyclePolicy` | Creates lifecycle policy |

There is no `CfnNetworkSecurityPolicy` — network policies use `CfnSecurityPolicy` with `type: 'network'`.

### CfnCollection

```typescript
new opensearchserverless.CfnCollection(this, 'Collection', {
  name: collectionName,       // REQUIRED: ^[a-z][a-z0-9-]{2,27}$
  type: 'VECTORSEARCH',
  standbyReplicas: 'DISABLED', // MVP cost savings
});
```

Attributes: `attrArn`, `attrCollectionEndpoint`, `attrId`.

**Collection FAILS to create without an encryption policy.** Must use `collection.addDependency(encryptionPolicy)`.

### CfnSecurityPolicy — Encryption

Policy is a **single JSON object** (not an array):

```json
{
  "Rules": [{ "ResourceType": "collection", "Resource": ["collection/<name>"] }],
  "AWSOwnedKey": true
}
```

Must be created BEFORE the collection.

### CfnSecurityPolicy — Network

See R-6 findings below.

### CfnAccessPolicy — Data Access

Policy is a **JSON array of objects**:

```json
[{
  "Rules": [
    {
      "ResourceType": "index",
      "Resource": ["index/<collection-name>/*"],
      "Permission": ["aoss:ReadDocument", "aoss:WriteDocument", "aoss:CreateIndex",
                      "aoss:DeleteIndex", "aoss:UpdateIndex", "aoss:DescribeIndex"]
    },
    {
      "ResourceType": "collection",
      "Resource": ["collection/<collection-name>"],
      "Permission": ["aoss:CreateCollectionItems", "aoss:DeleteCollectionItems",
                      "aoss:UpdateCollectionItems", "aoss:DescribeCollectionItems"]
    }
  ],
  "Principal": ["<role-arn-1>", "<role-arn-2>"]
}]
```

Principals must ALSO have IAM-level `aoss:APIAccessAll` on the collection ARN. Both the AOSS data access policy and IAM policy must grant access.

CDK tokens (e.g., `role.roleArn`) work inside `JSON.stringify()` — CDK resolves them to `Fn::Join`/`Fn::Sub` intrinsics in the synthesized template.

Policy name constraint: `^[a-z][a-z0-9-]{2,31}$`.

### No Circular Dependency

The dependency chain resolves cleanly with CDK tokens:

1. Encryption policy (no deps)
2. Collection (depends on encryption policy via `addDependency`)
3. EC2 role + Bedrock KB role (use `collection.attrArn` token for IAM policies)
4. Data access policy (uses role ARN tokens + collection name)

CDK tokens produce CloudFormation intrinsics at synthesis time. CloudFormation determines creation order via implicit `DependsOn`. Only the encryption policy → collection dependency needs explicit `addDependency()`.

**Decision:** Use L1 constructs with explicit `addDependency()` for encryption → collection. Pass role ARNs as CDK tokens in the data access policy JSON. The AOSS VPC endpoint uses `CfnVpcEndpoint` from the opensearchserverless module, not `ec2.InterfaceVpcEndpoint`.

---

## R-2: Bedrock Knowledge Base CDK Construct

**Question:** How to create a Bedrock Knowledge Base with S3 data source and "no chunking" strategy via CDK?

**Status:** Resolved

**Findings:**

### No L2 Constructs

`aws-cdk-lib/aws-bedrock` has only two L2 constructs: `FoundationModel` and `ProvisionedModel`. Knowledge Base and DataSource require L1 constructs: `CfnKnowledgeBase` and `CfnDataSource`.

The `@aws-cdk/aws-bedrock-alpha` package also lacks KB/DataSource L2s.

### CfnKnowledgeBase

```typescript
new bedrock.CfnKnowledgeBase(this, 'KnowledgeBase', {
  name: kbName,
  roleArn: serviceRole.roleArn,
  knowledgeBaseConfiguration: {
    type: 'VECTOR',
    vectorKnowledgeBaseConfiguration: {
      embeddingModelArn: `arn:aws:bedrock:${region}::foundation-model/amazon.titan-embed-text-v2:0`,
    },
  },
  storageConfiguration: {
    type: 'OPENSEARCH_SERVERLESS',
    opensearchServerlessConfiguration: {
      collectionArn: collection.attrArn,
      vectorIndexName: 'bedrock-kb-index',
      fieldMapping: {
        vectorField: 'bedrock-knowledge-base-default-vector',
        textField: 'AMAZON_BEDROCK_TEXT_CHUNK',
        metadataField: 'AMAZON_BEDROCK_METADATA',
      },
    },
  },
});
```

All three field mappings (`vectorField`, `textField`, `metadataField`) are **REQUIRED**. The field names must match the pre-created OpenSearch index schema exactly.

Attributes: `attrKnowledgeBaseId`, `attrKnowledgeBaseArn`, `attrStatus`.

### CfnDataSource — S3 with No Chunking

```typescript
new bedrock.CfnDataSource(this, 'DataSource', {
  name: `${kbName}-s3`,
  knowledgeBaseId: kb.attrKnowledgeBaseId,
  dataSourceConfiguration: {
    type: 'S3',
    s3Configuration: { bucketArn: bucket.bucketArn },
  },
  vectorIngestionConfiguration: {
    chunkingConfiguration: { chunkingStrategy: 'NONE' },
  },
  dataDeletionPolicy: 'DELETE',
});
```

Attributes: `attrDataSourceId`, `attrDataSourceStatus`.

### Service Role — Missing Permission Found

The Bedrock KB service role needs **4 permission sets** (not 3 as originally planned):

| Action | Resource | Purpose |
|--------|----------|---------|
| `s3:ListBucket` | Bucket ARN | List objects in data source |
| `s3:GetObject` | Bucket ARN/* | Read documents |
| `aoss:APIAccessAll` | Collection ARN | Read/write OpenSearch |
| **`bedrock:InvokeModel`** | Foundation model ARN | **Generate embeddings** |

The `bedrock:InvokeModel` permission was missing from the original plan. The model ARN uses an empty account ID: `arn:aws:bedrock:{region}::foundation-model/amazon.titan-embed-text-v2:0`.

Trust policy: `bedrock.amazonaws.com` with `aws:SourceAccount` and `ArnLike` conditions.

### OpenSearch Index MUST Be Pre-Created (New Task Required)

**Bedrock KB does NOT auto-create the OpenSearch vector index via CloudFormation/CDK.** Auto-creation is a console-only convenience.

For CDK deployments, a **custom resource Lambda** must create the index before the KB is created. The index requires:

```json
{
  "settings": { "index": { "knn": true } },
  "mappings": {
    "properties": {
      "bedrock-knowledge-base-default-vector": {
        "type": "knn_vector",
        "dimension": 1024,
        "method": { "engine": "faiss", "name": "hnsw", "parameters": {}, "space_type": "l2" }
      },
      "AMAZON_BEDROCK_TEXT_CHUNK": { "type": "text", "index": true },
      "AMAZON_BEDROCK_METADATA": { "type": "text", "index": false }
    }
  }
}
```

Key details:
- `amazon.titan-embed-text-v2:0` produces **1024 dimensions** by default (also supports 512, 256)
- Field names in the index must match the KB's `fieldMapping` configuration
- The custom resource Lambda needs network access to the collection endpoint (via VPC + AOSS endpoint) and data access policy permission
- `StartIngestionJob` does NOT create the index — it only ingests into an existing index

**This requires a new implementation task** (SRC-006) for the custom resource Lambda.

### Circular Dependency — Non-Issue

Same as R-1: CDK tokens resolve all forward references. The creation order is:

1. Bedrock KB service role (trust policy only, no collection ref needed)
2. OpenSearch collection (depends on encryption policy)
3. Add IAM policies to role using `collection.attrArn` tokens
4. AOSS data access policy using `role.roleArn` tokens
5. OpenSearch index via custom resource (depends on collection + data access policy)
6. CfnKnowledgeBase (depends on role + collection + index)
7. CfnDataSource (depends on KB)

**Decision:** Use L1 constructs `CfnKnowledgeBase` and `CfnDataSource`. Add `bedrock:InvokeModel` to the service role (was missing). Add a new custom resource task (SRC-006) to pre-create the OpenSearch vector index before KB creation. Use 1024-dimension vectors with `faiss`/`hnsw` engine. All three field mappings are required and must match between index schema and KB config.

---

## R-3: EC2 User Data Script Best Practices

**Question:** How to reliably bootstrap an Amazon Linux 2023 EC2 instance with the CLI binary, CloudWatch agent, and server-mode config?

**Areas to Investigate:**
- User data script ordering: (1) install packages, (2) download CLI binary, (3) install CloudWatch agent, (4) clone CodeCommit repo, (5) template config.yaml, (6) create systemd unit, (7) start services
- CDK `UserData.forLinux()` API for templating values from CDK constructs
- Error handling: `set -euxo pipefail`, cfn-signal for ASG lifecycle hooks
- CloudWatch agent config: structured JSON log shipping from CLI process stdout/stderr
- Systemd unit configuration: restart policy, environment variables, working directory
- Git credential helper for CodeCommit: `git config --global credential.helper '!aws codecommit credential-helper $@'`
- How to handle first-boot vs. subsequent boots (idempotency of user data script)

**Prototype Task:** Write the full user data script with CDK value interpolation. Test on a fresh Amazon Linux 2023 AMI.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-4: VPC Endpoint Security Group Configuration

**Question:** What security group rules are needed for 9 interface VPC endpoints + 1 gateway endpoint?

**Status:** Resolved

**Findings:**

### Endpoint Architecture (10 Total)

| Type | Count | CDK Construct | Security Group |
|------|-------|---------------|----------------|
| S3 gateway | 1 | `ec2.GatewayVpcEndpoint` | None (route table only) |
| Standard interface | 8 | `ec2.InterfaceVpcEndpoint` | Shared endpoint SG |
| AOSS interface | 1 | `opensearchserverless.CfnVpcEndpoint` | Shared endpoint SG |

**8 standard interface endpoints** (all via `ec2.InterfaceVpcEndpoint`):
1. `sqs` 2. `git-codecommit` 3. `bedrock-runtime` 4. `bedrock-agent` 5. `ssm` 6. `ssmmessages` 7. `ec2messages` 8. `logs`

**1 AOSS endpoint** (via `opensearchserverless.CfnVpcEndpoint`): This is NOT a standard interface endpoint — it uses `AWS::OpenSearchServerless::VpcEndpoint`, not `AWS::EC2::VPCEndpoint`. It accepts `securityGroupIds` as `string[]`, so it CAN share the same endpoint SG.

### Security Group Rules (Minimal)

Two security groups:

**EC2 SG:** Outbound TCP 443 → endpoint SG
**Endpoint SG:** Inbound TCP 443 ← EC2 SG

No additional rules needed. Stateful SG tracking handles return traffic.

### CDK Code Pattern

```typescript
const ec2Sg = new ec2.SecurityGroup(this, 'Ec2Sg', {
  vpc, description: 'EC2 instance', allowAllOutbound: false,
});
const endpointSg = new ec2.SecurityGroup(this, 'EndpointSg', {
  vpc, description: 'VPC interface endpoints', allowAllOutbound: false,
});
ec2Sg.addEgressRule(endpointSg, ec2.Port.tcp(443), 'HTTPS to endpoints');
endpointSg.addIngressRule(ec2Sg, ec2.Port.tcp(443), 'HTTPS from EC2');

// S3 gateway (no SG)
vpc.addGatewayEndpoint('S3', {
  service: ec2.GatewayVpcEndpointAwsService.S3,
  subnets: [{ subnetType: ec2.SubnetType.PRIVATE_ISOLATED }],
});

// Standard interface endpoints — CRITICAL: open: false prevents auto-permissive ingress
new ec2.InterfaceVpcEndpoint(this, 'SqsEndpoint', {
  vpc, service: ec2.InterfaceVpcEndpointAwsService.SQS,
  securityGroups: [endpointSg], privateDnsEnabled: true,
  open: false, // Prevents CDK from adding 0.0.0.0/0 ingress rule
  subnets: { subnetType: ec2.SubnetType.PRIVATE_ISOLATED },
});

// AOSS endpoint (L1 construct, takes string IDs)
new opensearchserverless.CfnVpcEndpoint(this, 'AossEndpoint', {
  name: 'aoss-endpoint', // ^[a-z][a-z0-9-]{2,31}$
  vpcId: vpc.vpcId,
  subnetIds: [subnet.subnetId],
  securityGroupIds: [endpointSg.securityGroupId],
});
```

**Critical: `open: false`** on `InterfaceVpcEndpoint` prevents CDK from auto-adding a permissive `0.0.0.0/0` ingress rule. Without this, the tight SG-to-SG rules are undermined.

**Decision:** Two SGs (EC2 + endpoint), shared across all 9 interface endpoints. AOSS uses `opensearchserverless.CfnVpcEndpoint` with `securityGroupIds`. Always set `open: false` on standard interface endpoints. S3 gateway needs route table association only.

---

## R-5: Crockford Base32 UID Generation in Node.js

**Question:** How to generate 16-character Crockford base32 UIDs in the submitKnowledge Lambda?

**Areas to Investigate:**
- Crockford base32 alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ` (32 chars, excludes I, L, O, U)
- Encoding: 10 bytes (80 bits) from `crypto.randomBytes(10)` → 16 characters (5 bits per char)
- Existing npm packages: `base32-encode` (supports Crockford), `crockford-base32`
- Zero-dependency implementation (preferred for Lambda — fewer dependencies = faster cold starts)
- Must match CLI's Go implementation format exactly (CLI plan R-7)

**Prototype Task:** Implement the function, verify it produces exactly 16 uppercase chars from the Crockford alphabet, and confirm round-trip compatibility with the Go implementation.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

---

## R-6: OpenSearch Serverless Network Policy for Dual Access

**Question:** How to configure the network policy so both VPC (EC2) and Bedrock service can access the collection?

**Status:** Resolved

**Findings:**

### Network Policy JSON Format

The policy is a **JSON array** (unlike encryption policy which is a single object). Field names are **case-sensitive**:

```json
[{
  "Description": "string",
  "Rules": [{ "ResourceType": "collection", "Resource": ["collection/<name>"] }],
  "AllowFromPublic": false,
  "SourceVPCEs": ["vpce-xxx"],
  "SourceServices": ["bedrock.amazonaws.com"]
}]
```

Correct field names: `SourceVPCEs` (NOT `SourceVPCEndpoints`), `SourceServices`, `AllowFromPublic`.

### Critical: Do NOT Use AllowFromPublic

**If `AllowFromPublic: true`, it OVERRIDES `SourceVPCEs` and `SourceServices`.** The collection becomes fully public and the VPC/service restrictions are silently ignored.

### Recommended Dual-Access Pattern

```json
[{
  "Rules": [{ "ResourceType": "collection", "Resource": ["collection/<name>"] }],
  "AllowFromPublic": false,
  "SourceVPCEs": ["<aoss-vpc-endpoint-id>"],
  "SourceServices": ["bedrock.amazonaws.com"]
}]
```

This enables:
- **EC2 access:** Via VPC endpoint (restricted to the specific VPCE)
- **Bedrock service access:** Via AWS service private network (`SourceServices`)
- **No public internet access** to the collection

### Bedrock Service Access Path

When Lambda calls `bedrock:Retrieve`, Bedrock reaches OpenSearch via **AWS internal service-to-service networking**, not the customer's VPC. `SourceServices: ["bedrock.amazonaws.com"]` enables this internal path. Putting Lambda in the VPC would NOT change this — Bedrock's internal path is separate.

### Three-Layer Security Model

```
Request → Network Policy (origin) → Data Access Policy (principal) → IAM Policy → Access
```

With `AllowFromPublic: false` + `SourceVPCEs` + `SourceServices`, all three layers enforce restrictions. This is strictly more secure than `AllowFromPublic: true` (which collapses the network layer).

### Corrections to Existing Documentation

The plan and data-model files should be updated:
- Change `AllowFromPublic: true` → `AllowFromPublic: false` with `SourceServices`
- Change `SourceVPCEndpoints` → `SourceVPCEs` (correct field name)

**Decision:** Use `AllowFromPublic: false` with `SourceVPCEs` + `SourceServices: ["bedrock.amazonaws.com"]`. Do NOT use `AllowFromPublic: true`. Update task SRC-003 acceptance criteria accordingly.

---

## R-7: Lambda Proxy Integration Response Format

**Question:** What is the exact response format required by API Gateway Lambda proxy integration?

**Areas to Investigate:**
- Required response shape for Lambda proxy integration:
  ```json
  {
    "statusCode": 200,
    "headers": { "Content-Type": "application/json" },
    "body": "{\"key\": \"value\"}"
  }
  ```
- The `body` must be a **string** (JSON-serialized), not an object
- How API Gateway translates Lambda errors (thrown exceptions) vs. returned error status codes
- Whether `isBase64Encoded` is needed (no — all responses are JSON)
- How to return proper CORS headers (not needed — spec says CORS is not enabled)

**Success Criteria:** Helper function that wraps responses in the correct format for all status codes.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_
