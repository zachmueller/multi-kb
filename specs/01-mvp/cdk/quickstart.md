# Quickstart: Multi-KB CDK Infrastructure Development

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)

## Prerequisites

- **Node.js 22+** — `node --version` should report v22 or later
- **npm 10+** — `npm --version`
- **AWS CDK v2** — `npm install -g aws-cdk` → `cdk --version`
- **AWS CLI v2** — `aws --version`
- **AWS account** with:
  - CDK bootstrapped (`cdk bootstrap aws://<account>/<region>`)
  - Bedrock model access granted (Titan Embeddings V2, Claude Sonnet, Claude Haiku)
  - OpenSearch Serverless available in target region
  - IAM permissions to create all resource types in the stack
- **CLI binary** published to an S3 path accessible from the target account

## Repository Setup

```bash
# Clone the repo
git clone <repo-url> multi-kb-cdk
cd multi-kb-cdk

# Install dependencies
npm install

# Verify TypeScript compilation
npx tsc --noEmit

# Verify CDK synthesis
cdk synth
```

## Project Structure

```
bin/
└── multi-kb.ts                    # CDK app entry point

lib/
├── multi-kb-stack.ts              # Main stack
└── constructs/
    ├── networking.ts              # VPC, subnets, endpoints, SGs
    ├── storage.ts                 # S3, CodeCommit, SQS
    ├── search.ts                  # OpenSearch Serverless
    ├── knowledge-base.ts          # Bedrock KB + data source
    ├── api.ts                     # API Gateway
    ├── submit-lambda.ts           # submitKnowledge Lambda
    ├── recall-lambda.ts           # recallKnowledge Lambda
    ├── compute.ts                 # EC2 ASG, launch template
    └── observability.ts           # CloudWatch logs, alarms

lambda/
├── submit/
│   └── index.ts                   # submitKnowledge handler
├── recall/
│   └── index.ts                   # recallKnowledge handler
└── shared/
    ├── uid.ts                     # Crockford base32 UID
    ├── response.ts                # API GW response helpers
    └── validation.ts              # Field validation

test/
├── multi-kb-stack.test.ts         # Stack snapshot + assertions
├── constructs/                    # Per-construct tests
└── lambda/                        # Handler unit tests
```

## Build & Deploy Commands

```bash
# Compile TypeScript
npx tsc

# Run tests
npm test

# Synthesize CloudFormation template
cdk synth

# Diff against deployed stack
cdk diff

# Deploy (first time — will create all resources)
cdk deploy --context cliBinaryS3Uri=s3://my-bucket/multi-kb-cli/latest/multi-kb-linux-amd64

# Deploy with custom parameters
cdk deploy \
  --context cliBinaryS3Uri=s3://my-bucket/multi-kb-cli/latest/multi-kb-linux-amd64 \
  --context repoName=my-team-kb \
  --context bucketPrefix=my-team-kb \
  --context ec2InstanceType=t3.medium \
  --context consolidationModelId=anthropic.claude-sonnet-4-20250514

# Deploy into an existing VPC
cdk deploy \
  --context cliBinaryS3Uri=s3://my-bucket/multi-kb-cli/latest/multi-kb-linux-amd64 \
  --context vpcId=vpc-0123456789abcdef0

# Destroy stack (removes all resources)
cdk destroy
```

## Testing Strategy

### CDK Unit Tests (Jest + CDK Assertions)

```bash
# Run all tests
npm test

# Run specific test file
npx jest test/constructs/networking.test.ts

# Update snapshots
npx jest --updateSnapshot
```

**Test categories:**

1. **Snapshot tests:** Verify synthesized template hasn't changed unexpectedly
2. **Fine-grained assertions:** Verify critical properties:
   - IAM policies are least-privilege (specific ARNs, not wildcards)
   - Security groups have correct inbound/outbound rules
   - VPC endpoints are in the correct subnet and AZ
   - Lambda functions have correct runtime, memory, timeout
   - SQS queue has DLQ configured with maxReceiveCount 3
   - S3 bucket blocks public access and has encryption
   - API Gateway has IAM authorization on both methods
3. **Construct isolation tests:** Each construct can be instantiated independently with mock dependencies

### Lambda Handler Tests

```bash
# Run Lambda tests
npx jest test/lambda/
```

Test Lambda handlers in isolation (mock AWS SDK calls):
- submitKnowledge: valid input, each validation failure, SQS send failure
- recallKnowledge: valid query, empty results, coverage assessment trigger, S3 write failure (should not affect response)

### Integration Tests (Post-Deploy)

Manual verification after deployment:
1. `aws apigateway test-invoke-method` for both endpoints
2. Submit a test note → verify SQS message → verify CodeCommit commit → verify S3 sync
3. Wait for Bedrock KB index sync → recall the submitted note
4. Verify CloudWatch logs from Lambda and EC2
5. SSM Session Manager access to EC2 instance

## Configuration Reference

See [spec.md](spec.md) CDK Stack Structure section for full parameter table and stack outputs.

**Required parameter:** `cliBinaryS3Uri` — must be set on every deploy.

## Key Dependencies

| Dependency | Purpose |
|------------|---------|
| `aws-cdk-lib` | CDK v2 construct library |
| `constructs` | CDK constructs base |
| `@aws-sdk/client-sqs` | Lambda: SQS message sending |
| `@aws-sdk/client-bedrock-agent-runtime` | Lambda: Bedrock Retrieve API |
| `@aws-sdk/client-bedrock-runtime` | Lambda: InvokeModel for coverage |
| `@aws-sdk/client-s3` | Lambda: recall log writing |
| `esbuild` | Lambda bundling (via NodejsFunction) |
| `jest` | Testing framework |
| `ts-jest` | TypeScript Jest transformer |

## Estimated Costs (Single Deployment)

| Resource | Monthly Cost | Notes |
|----------|-------------|-------|
| VPC Interface Endpoints (9) | ~$65.70 | Single AZ |
| OpenSearch Serverless (4 OCUs) | ~$700 | Minimum config |
| EC2 t3.small | ~$15 | On-demand |
| S3 | ~$1 | Small storage |
| SQS | ~$0 | Free tier likely covers |
| API Gateway | ~$3.50/million calls | Pay per use |
| Lambda | ~$0.50 | Pay per use |
| CloudWatch Logs | ~$5 | Varies with log volume |
| **Total** | **~$790/month** | Dominated by OpenSearch Serverless |
