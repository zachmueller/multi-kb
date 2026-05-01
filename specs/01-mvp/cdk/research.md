# Research: Multi-KB CDK Infrastructure — MVP

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)
**Status:** Open (findings to be populated during Phase 0)

## R-1: OpenSearch Serverless Collection Setup via CDK

**Question:** What CDK constructs are needed to create an OpenSearch Serverless VECTORSEARCH collection with all three policy types?

**Areas to Investigate:**
- L2 construct availability (or need for L1 `CfnCollection`, `CfnSecurityPolicy`, `CfnAccessPolicy`, `CfnNetworkSecurityPolicy` from `aws-cdk-lib/aws-opensearchserverless`)
- Encryption policy creation (AWS-owned key)
- Network policy creation (dual access: VPC endpoint + public for Bedrock)
- Data access policy creation (principals: EC2 role + Bedrock KB service role)
- Dependency ordering: collection must wait for encryption policy; data access policy needs role ARNs that may not exist yet
- How to handle the circular dependency between OpenSearch data access policy and Bedrock KB service role

**Success Criteria:** CDK code that synthesizes a working collection with all three policies, accessible by both EC2 and Bedrock service.

**Findings:**

### 1. L2 vs L1 Constructs

**No L2 constructs exist.** The CDK v2 module `aws-cdk-lib/aws-opensearchserverless` explicitly states: "There are no official hand-written (L2) constructs for this service yet." All constructs are L1 (CloudFormation wrappers).

**Complete list of available constructs in `aws-cdk-lib/aws-opensearchserverless`:**

| Construct | CloudFormation Type | Purpose |
|-----------|-------------------|---------|
| `CfnCollection` | `AWS::OpenSearchServerless::Collection` | Creates a collection |
| `CfnSecurityPolicy` | `AWS::OpenSearchServerless::SecurityPolicy` | Creates encryption or network policy |
| `CfnAccessPolicy` | `AWS::OpenSearchServerless::AccessPolicy` | Creates data access policy |
| `CfnVpcEndpoint` | `AWS::OpenSearchServerless::VpcEndpoint` | Creates AOSS-specific VPC endpoint |
| `CfnLifecyclePolicy` | `AWS::OpenSearchServerless::LifecyclePolicy` | Creates lifecycle policy |

There is no `CfnNetworkSecurityPolicy` -- network policies use `CfnSecurityPolicy` with `type: 'network'`.

### 2. CfnCollection for VECTORSEARCH

**CfnCollectionProps interface:**

```typescript
interface CfnCollectionProps {
  name: string;                    // REQUIRED
  type?: string;                   // 'SEARCH' | 'TIMESERIES' | 'VECTORSEARCH'
  description?: string;
  standbyReplicas?: string;        // 'ENABLED' | 'DISABLED' (immutable after creation)
  tags?: CfnTag[];
  collectionGroupName?: string;
  encryptionConfig?: CfnCollection.EncryptionConfigProperty;
  vectorOptions?: CfnCollection.VectorOptionsProperty;
}
```

**Name constraints:**
- Must start with a lowercase letter
- 3-28 characters long
- Only lowercase a-z, digits 0-9, and hyphen (-)
- Must be unique within the account and region
- Pattern: `^[a-z][a-z0-9-]{2,27}$`

**Attributes exposed (read-only after creation):**

| CDK Attribute | CloudFormation Attribute | Example Value |
|---------------|------------------------|---------------|
| `attrArn` | `Arn` | `arn:aws:aoss:us-east-1:123456789012:collection/07tjusf2h91cunochc` |
| `attrCollectionEndpoint` | `CollectionEndpoint` | `https://07tjusf2h91cunochc.us-east-1.aoss.amazonaws.com` |
| `attrDashboardEndpoint` | `DashboardEndpoint` | `https://07tjusf2h91cunochc.us-east-1.aoss.amazonaws.com/_dashboards` |
| `attrId` | `Id` | `07tjusf2h91cunochc` |
| `attrKmsKeyArn` | `KmsKeyArn` | KMS key ARN used for encryption |

**Encryption dependency: YES, mandatory.** The collection will FAIL to create if no encryption configuration exists. Either:
- Specify `encryptionConfig` inline on the collection, OR
- Create a matching `CfnSecurityPolicy` (type: `encryption`) first and use CDK `addDependency()`

### 3. CfnSecurityPolicy for Encryption

**CfnSecurityPolicyProps interface:**

```typescript
interface CfnSecurityPolicyProps {
  name: string;         // REQUIRED, pattern: ^[a-z][a-z0-9-]{2,31}$
  policy: string;       // REQUIRED, JSON string (minified, no whitespace)
  type: string;         // REQUIRED, 'encryption' | 'network'
  description?: string;
}
```

**Encryption policy JSON format (single object, NOT an array):**

```json
{
  "Rules": [
    {
      "ResourceType": "collection",
      "Resource": ["collection/<collection-name>"]
    }
  ],
  "AWSOwnedKey": true
}
```

The collection is referenced by name pattern in the `Resource` array. Wildcards are supported (e.g., `collection/my-kb*`).

**Must be created BEFORE the collection.** Use CDK `addDependency()` to enforce this.

### 4. CfnAccessPolicy for Data Access

**CfnAccessPolicyProps interface:**

```typescript
interface CfnAccessPolicyProps {
  name: string;         // REQUIRED, pattern: ^[a-z][a-z0-9-]{2,31}$
  policy: string;       // REQUIRED, JSON string (minified)
  type: string;         // REQUIRED, only valid value: 'data'
  description?: string;
}
```

**Data access policy JSON format (array of objects):**

```json
[
  {
    "Description": "Access for KB roles",
    "Rules": [
      {
        "ResourceType": "index",
        "Resource": ["index/<collection-name>/*"],
        "Permission": [
          "aoss:ReadDocument",
          "aoss:WriteDocument",
          "aoss:CreateIndex",
          "aoss:DeleteIndex",
          "aoss:UpdateIndex",
          "aoss:DescribeIndex"
        ]
      },
      {
        "ResourceType": "collection",
        "Resource": ["collection/<collection-name>"],
        "Permission": [
          "aoss:CreateCollectionItems",
          "aoss:DeleteCollectionItems",
          "aoss:UpdateCollectionItems",
          "aoss:DescribeCollectionItems"
        ]
      }
    ],
    "Principal": [
      "arn:aws:iam::123456789012:role/ec2-role",
      "arn:aws:iam::123456789012:role/bedrock-kb-role"
    ]
  }
]
```

**Complete permission lists:**

Index-level permissions (ResourceType: `index`):
- `aoss:ReadDocument`
- `aoss:WriteDocument`
- `aoss:CreateIndex`
- `aoss:DeleteIndex`
- `aoss:UpdateIndex`
- `aoss:DescribeIndex`
- `aoss:*` (wildcard)

Collection-level permissions (ResourceType: `collection`):
- `aoss:CreateCollectionItems`
- `aoss:DeleteCollectionItems`
- `aoss:UpdateCollectionItems`
- `aoss:DescribeCollectionItems`
- `aoss:*` (wildcard)

**No separate VECTORSEARCH-specific permissions exist** -- the same permissions apply to all collection types.

**Principals:** Specified as full IAM role/user ARNs. SAML identities also supported but not needed for this use case.

**CDK token references in policy strings:** YES, CDK tokens work. Use `JSON.stringify()` to build the policy, and CDK tokens (e.g., `role.roleArn`) resolve at synthesis time within the string. Example:

```typescript
policy: JSON.stringify([{
  Rules: [...],
  Principal: [ec2Role.roleArn, bedrockKbRole.roleArn]
}])
```

CDK tokens like `role.roleArn` produce CloudFormation `{ Ref }` or `{ Fn::GetAtt }` intrinsics in the synthesized template. When used inside `JSON.stringify()`, CDK's token resolution system replaces them with the appropriate CloudFormation intrinsics during synthesis. The resulting template uses `Fn::Sub` or `Fn::Join` to compose the final JSON string at deploy time.

**Important IAM prerequisite:** Principals listed in the data access policy must ALSO have IAM permissions (`aoss:APIAccessAll`) on the collection ARN in their own IAM policy. The AOSS data access policy and IAM policy work together -- both must grant access.

### 5. Circular Dependency Handling

The dependency chain is:

```
Bedrock KB Service Role → needs collection ARN for IAM policy
OpenSearch Data Access Policy → needs Bedrock KB Role ARN + EC2 Role ARN
Collection → needs encryption policy (but NOT data access or network policy)
```

**This is NOT a true circular dependency.** Here is why:

- The **collection** only depends on the encryption policy
- The **Bedrock KB service role** needs `aoss:APIAccessAll` on the collection ARN in its IAM policy
- The **data access policy** needs the role ARNs of both EC2 and Bedrock KB roles
- The **collection** does NOT depend on the data access policy

The resolution order is:

1. Create encryption policy (no dependencies)
2. Create collection (depends on encryption policy)
3. Create EC2 role and Bedrock KB service role in parallel (both need collection ARN for IAM policy -- this is a forward reference that CDK tokens handle automatically)
4. Create data access policy (needs both role ARNs and collection name -- all available as CDK tokens)

**CDK tokens resolve this cleanly.** When you write `collection.attrArn` in a role's policy statement or `role.roleArn` in the access policy JSON, CDK generates CloudFormation intrinsics that resolve at deploy time. There is no actual circular dependency because:
- The collection does not reference the roles
- The roles reference the collection (forward reference, resolved by CloudFormation)
- The access policy references both roles and the collection name (all resolved by CloudFormation)

CDK will automatically add the correct `DependsOn` relationships in the synthesized template. You may need explicit `addDependency()` only for the encryption policy -> collection relationship (since CDK may not detect this implicit dependency).

### 6. Dependency Ordering

| Resource | Must Be Created Before | Reason |
|----------|----------------------|--------|
| Encryption policy | Collection | Collection creation FAILS without it |
| Network policy | No strict ordering | Can be created before or after collection; collection is accessible per the policy once both exist |
| Data access policy | No strict ordering | Can be created before or after collection; access is granted once both exist |
| Collection | Bedrock KB, EC2 role policies | Roles need collection ARN; KB needs collection endpoint |

**Recommended CDK dependency setup:**
- `collection.addDependency(encryptionPolicy)` -- REQUIRED
- `collection.addDependency(networkPolicy)` -- RECOMMENDED (ensures network access is ready when collection comes online)
- No dependency needed: data access policy can be created in parallel with collection

### 7. Complete CDK TypeScript Code for SearchConstruct

```typescript
import { Construct } from 'constructs';
import { Stack } from 'aws-cdk-lib';
import * as opensearchserverless from 'aws-cdk-lib/aws-opensearchserverless';

export interface SearchConstructProps {
  /** Collection name (must be lowercase, 3-28 chars, a-z0-9 and hyphens) */
  collectionName: string;
  /** AOSS VPC endpoint ID for network policy */
  vpcEndpointId: string;
  /** IAM role ARNs that need data access to the collection */
  dataAccessPrincipalArns: string[];
  /** Optional: disable standby replicas to save cost (default: DISABLED for MVP) */
  standbyReplicas?: string;
}

export class SearchConstruct extends Construct {
  public readonly collection: opensearchserverless.CfnCollection;
  public readonly collectionName: string;
  public readonly collectionArn: string;
  public readonly collectionEndpoint: string;
  public readonly collectionId: string;

  constructor(scope: Construct, id: string, props: SearchConstructProps) {
    super(scope, id);

    this.collectionName = props.collectionName;

    // --- Encryption Policy (must exist before collection) ---
    const encryptionPolicy = new opensearchserverless.CfnSecurityPolicy(
      this,
      'EncryptionPolicy',
      {
        name: `${props.collectionName}-enc`,
        type: 'encryption',
        description: `Encryption policy for ${props.collectionName} collection`,
        policy: JSON.stringify({
          Rules: [
            {
              ResourceType: 'collection',
              Resource: [`collection/${props.collectionName}`],
            },
          ],
          AWSOwnedKey: true,
        }),
      },
    );

    // --- Network Policy (dual access: VPC + Bedrock service) ---
    const networkPolicy = new opensearchserverless.CfnSecurityPolicy(
      this,
      'NetworkPolicy',
      {
        name: `${props.collectionName}-net`,
        type: 'network',
        description: `Network policy for ${props.collectionName} collection`,
        policy: JSON.stringify([
          {
            Description: 'VPC endpoint and Bedrock service access',
            Rules: [
              {
                ResourceType: 'collection',
                Resource: [`collection/${props.collectionName}`],
              },
            ],
            AllowFromPublic: false,
            SourceVPCEs: [props.vpcEndpointId],
            SourceServices: ['bedrock.amazonaws.com'],
          },
        ]),
      },
    );

    // --- Collection ---
    this.collection = new opensearchserverless.CfnCollection(
      this,
      'Collection',
      {
        name: props.collectionName,
        type: 'VECTORSEARCH',
        description: `VECTORSEARCH collection for ${props.collectionName}`,
        standbyReplicas: props.standbyReplicas ?? 'DISABLED',
      },
    );

    // REQUIRED: collection fails without encryption policy
    this.collection.addDependency(encryptionPolicy);
    // RECOMMENDED: network access ready when collection comes online
    this.collection.addDependency(networkPolicy);

    // --- Data Access Policy ---
    const accessPolicy = new opensearchserverless.CfnAccessPolicy(
      this,
      'DataAccessPolicy',
      {
        name: `${props.collectionName}-access`,
        type: 'data',
        description: `Data access policy for ${props.collectionName} collection`,
        policy: JSON.stringify([
          {
            Description: `Data access for ${props.collectionName}`,
            Rules: [
              {
                ResourceType: 'index',
                Resource: [`index/${props.collectionName}/*`],
                Permission: [
                  'aoss:ReadDocument',
                  'aoss:WriteDocument',
                  'aoss:CreateIndex',
                  'aoss:DeleteIndex',
                  'aoss:UpdateIndex',
                  'aoss:DescribeIndex',
                ],
              },
              {
                ResourceType: 'collection',
                Resource: [`collection/${props.collectionName}`],
                Permission: [
                  'aoss:CreateCollectionItems',
                  'aoss:DeleteCollectionItems',
                  'aoss:UpdateCollectionItems',
                  'aoss:DescribeCollectionItems',
                ],
              },
            ],
            Principal: props.dataAccessPrincipalArns,
          },
        ]),
      },
    );

    // Expose attributes
    this.collectionArn = this.collection.attrArn;
    this.collectionEndpoint = this.collection.attrCollectionEndpoint;
    this.collectionId = this.collection.attrId;
  }
}
```

**Usage in the main stack (showing dependency resolution):**

```typescript
// 1. Create networking (VPC, AOSS VPC endpoint)
const networking = new NetworkingConstruct(this, 'Networking', { ... });

// 2. Create storage (S3, CodeCommit, SQS)
const storage = new StorageConstruct(this, 'Storage', { ... });

// 3. Create Bedrock KB service role first (needs collection ARN via token)
const bedrockKbRole = new iam.Role(this, 'BedrockKbRole', {
  assumedBy: new iam.ServicePrincipal('bedrock.amazonaws.com'),
});

// 4. Create EC2 role (needs collection ARN via token)
const ec2Role = new iam.Role(this, 'Ec2Role', {
  assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
});

// 5. Create search construct -- pass role ARNs as CDK tokens
const search = new SearchConstruct(this, 'Search', {
  collectionName: `${stackName}-kb`,
  vpcEndpointId: networking.aossVpcEndpoint.attrId,
  dataAccessPrincipalArns: [
    ec2Role.roleArn,        // CDK token, resolved at deploy time
    bedrockKbRole.roleArn,  // CDK token, resolved at deploy time
  ],
});

// 6. Add IAM permissions to roles (using collection ARN token)
bedrockKbRole.addToPolicy(new iam.PolicyStatement({
  actions: ['aoss:APIAccessAll'],
  resources: [search.collectionArn],  // CDK token
}));

ec2Role.addToPolicy(new iam.PolicyStatement({
  actions: ['aoss:APIAccessAll'],
  resources: [search.collectionArn],  // CDK token
}));
```

**Key insight on CDK tokens in JSON.stringify:** When `props.dataAccessPrincipalArns` contains CDK tokens (like `role.roleArn`), `JSON.stringify()` produces a string with unresolved token placeholders. CDK's token resolution system then transforms the entire string into a `Fn::Join` or `Fn::Sub` expression in the synthesized CloudFormation template, which CloudFormation resolves at deploy time. This is well-supported behavior.

**Decision:** Use L1 constructs (`CfnCollection`, `CfnSecurityPolicy`, `CfnAccessPolicy`) with explicit `addDependency()` for the encryption policy. Pass role ARNs as CDK tokens in the data access policy JSON. Create the AOSS VPC endpoint using `CfnVpcEndpoint` from the opensearchserverless module, not `ec2.InterfaceVpcEndpoint`.

---

## R-2: Bedrock Knowledge Base CDK Construct

**Question:** How to create a Bedrock Knowledge Base with S3 data source and "no chunking" strategy via CDK?

**Areas to Investigate:**
- L1 constructs: `CfnKnowledgeBase`, `CfnDataSource` from `aws-cdk-lib/aws-bedrock`
- KB configuration: embedding model ID, storage config pointing to OpenSearch Serverless collection
- Data source configuration: S3 bucket, "no chunking" strategy (`chunkingStrategy: { chunkingStrategy: 'NONE' }`)
- Service role creation: role that Bedrock assumes to read S3 and write OpenSearch
- How to wire the OpenSearch collection endpoint and index name into the KB storage config
- Whether the KB auto-creates the OpenSearch index or if we need to pre-create it

**Success Criteria:** CDK code that creates a KB + data source, with a service role that has correct permissions for both S3 and OpenSearch.

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

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

**Areas to Investigate:**
- Can all 9 interface endpoints share a single security group? (Yes — they all need the same inbound rule: TCP 443 from EC2 SG)
- S3 gateway endpoint: no security group needed, but route table association required
- OpenSearch Serverless VPC endpoint: uses a separate `CfnVpcEndpoint` from the `aws-opensearchserverless` module (not a standard `ec2.InterfaceVpcEndpoint`)
- Whether the Lambda functions (NOT in VPC) need any security group rules (no — they call public AWS API endpoints)
- Outbound rules on EC2 SG: need to allow HTTPS (443) to VPC endpoint SG

**Success Criteria:** Minimal security group configuration where EC2 can reach all services via VPC endpoints.

**Findings:**

### VPC Endpoint Architecture

**10 endpoints total:**
- 1 S3 gateway endpoint (via `ec2.GatewayVpcEndpoint`) -- no security group, route table association only
- 8 standard interface endpoints (via `ec2.InterfaceVpcEndpoint`) -- share one security group
- 1 AOSS VPC endpoint (via `opensearchserverless.CfnVpcEndpoint`) -- uses its own security group parameter

**The 8 standard interface endpoints** (all created with `ec2.InterfaceVpcEndpoint`):
1. `com.amazonaws.{region}.sqs`
2. `com.amazonaws.{region}.git-codecommit`
3. `com.amazonaws.{region}.bedrock-runtime`
4. `com.amazonaws.{region}.bedrock-agent`
5. `com.amazonaws.{region}.ssm`
6. `com.amazonaws.{region}.ssmmessages`
7. `com.amazonaws.{region}.ec2messages`
8. `com.amazonaws.{region}.logs`

**The AOSS VPC endpoint** (created with `opensearchserverless.CfnVpcEndpoint`):
- This is NOT a standard interface endpoint. It uses `AWS::OpenSearchServerless::VpcEndpoint`, not `AWS::EC2::VPCEndpoint`
- `CfnVpcEndpointProps`: `name` (required), `subnetIds` (required), `vpcId` (required), `securityGroupIds` (optional)
- Returns `attrId` which is the VPC endpoint ID (e.g., `vpce-050f79086ee71ac05`) -- this ID is used in the network policy's `SourceVPCEs` field

### Security Group Configuration

**Two security groups:**
1. **EC2 SG**: Outbound HTTPS (443) to endpoint SG
2. **VPC Endpoint SG**: Inbound HTTPS (443) from EC2 SG

**Both the standard interface endpoints and the AOSS VPC endpoint can share the same endpoint security group.** The AOSS `CfnVpcEndpoint` accepts a `securityGroupIds` array.

**Lambda functions (NOT in VPC):** No security group rules needed. They call public AWS API endpoints (Bedrock, S3).

**Decision:** Use two security groups. The EC2 SG allows outbound 443 to the endpoint SG. The endpoint SG allows inbound 443 from the EC2 SG. All 9 interface endpoints (8 standard + 1 AOSS) share the endpoint SG. The S3 gateway endpoint needs route table association but no SG. The AOSS VPC endpoint is created via `opensearchserverless.CfnVpcEndpoint` with the shared endpoint SG in `securityGroupIds`.

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

**Areas to Investigate:**
- Network policy JSON schema: `AllowFromPublic`, `SourceVPCEndpoints`, `SourceServices`
- Does `AllowFromPublic: true` allow Bedrock service access? (Bedrock Retrieve API accesses OpenSearch from the Bedrock service network, not from the customer's VPC)
- Can a single policy combine VPC endpoint source AND public access?
- Alternative: `SourceServices: ["bedrock.amazonaws.com"]` — does this work?
- Security implications of `AllowFromPublic: true` (mitigated by data access policy requiring specific IAM principals)

**Success Criteria:** Network policy that allows both EC2 (via VPC endpoint) and Bedrock (via service network) to reach the collection.

**Key Concern:** If we use VPC-only access, the recallKnowledge Lambda (not in VPC) → Bedrock Retrieve API → OpenSearch path will fail because Bedrock's internal access won't be able to reach a VPC-only collection.

**Findings:**

### Network Policy JSON Format (Definitive)

The network policy is an **array of objects** (unlike encryption policy which is a single object).

**Complete field reference (all field names are case-sensitive):**

```json
[
  {
    "Description": "string",
    "Rules": [
      {
        "ResourceType": "collection" | "dashboard",
        "Resource": ["collection/<name-or-pattern>"]
      }
    ],
    "AllowFromPublic": true | false,
    "SourceVPCEs": ["vpce-xxxxxxxxx"],
    "SourceServices": ["bedrock.amazonaws.com"]
  }
]
```

**Field names confirmed (case-sensitive):**
- `AllowFromPublic` (not `allowFromPublic`)
- `SourceVPCEs` (not `SourceVPCEndpoints` or `SourceVpcEndpoints`)
- `SourceServices` (not `sourceServices`)
- `ResourceType` (not `resourceType`)
- `Resource` (not `resource`)
- `Description` (not `description`)

**Critical behavioral rules:**
1. If `AllowFromPublic: true`, it **overrides** any `SourceVPCEs` or `SourceServices` settings
2. `SourceServices` private access **only applies to collection endpoints**, not to Dashboards
3. Multiple VPC endpoints in overlapping rules are **additive**
4. Currently the only valid `SourceServices` value is `"bedrock.amazonaws.com"`

### Dual Access Pattern (Recommended)

Use `AllowFromPublic: false` with both `SourceVPCEs` and `SourceServices` for maximum security:

```json
[
  {
    "Description": "VPC endpoint access for EC2 + Bedrock service access",
    "Rules": [
      {
        "ResourceType": "collection",
        "Resource": ["collection/<collection-name>"]
      }
    ],
    "AllowFromPublic": false,
    "SourceVPCEs": ["vpce-050f79086ee71ac05"],
    "SourceServices": ["bedrock.amazonaws.com"]
  }
]
```

This is more secure than `AllowFromPublic: true` because:
- EC2 access is restricted to the specific VPC endpoint
- Bedrock service access is restricted to the `bedrock.amazonaws.com` service principal
- No public internet access to the collection

### Alternative: AllowFromPublic (Simpler but Less Secure)

```json
[
  {
    "Rules": [
      {
        "ResourceType": "collection",
        "Resource": ["collection/<collection-name>"]
      }
    ],
    "AllowFromPublic": true
  }
]
```

This also works because `AllowFromPublic: true` allows Bedrock service access. However, it opens the collection endpoint to the public internet. The data access policy (requiring specific IAM principals) still restricts who can actually read/write data, so the security impact is mitigated. But the `SourceVPCEs` + `SourceServices` approach is strictly better.

### Prior Research Context Confirmed

The context from the user is correct:
- Field names ARE case-sensitive: `SourceVPCEs`, `SourceServices`, `AllowFromPublic`
- `AllowFromPublic: false` with `SourceVPCEs` + `SourceServices` is the recommended pattern
- The AOSS VPC endpoint uses `opensearchserverless.CfnVpcEndpoint`, NOT `ec2.InterfaceVpcEndpoint`

**Decision:** Use the dual-access pattern with `AllowFromPublic: false`, `SourceVPCEs: [vpcEndpointId]`, and `SourceServices: ["bedrock.amazonaws.com"]`. This provides both EC2 access via VPC endpoint and Bedrock service access without exposing the collection to the public internet. The data-model.md Entity 6 should be updated to reflect the corrected field name `SourceVPCEs` (not `SourceVPCEndpoints`) and to use the private dual-access pattern instead of `AllowFromPublic: true`.

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
