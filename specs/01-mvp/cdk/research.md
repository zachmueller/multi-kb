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

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

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

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

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

**Findings:** _(to be populated)_

**Decision:** _(to be populated)_

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
