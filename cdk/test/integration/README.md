# Integration Tests

Post-deploy integration test scripts for validating a live Multi-KB stack.

## Prerequisites

- Stack deployed via `cdk deploy --context cliBinaryS3Uri=s3://...`
- AWS CLI configured with credentials that have access to the stack
- `jq` installed

## Test Order

Run in this order (Wave 7 execution plan):

### 1. QAT-006: Metadata Extraction (Run First)

**Highest-risk integration point.** Validates that Bedrock KB extracts YAML frontmatter (`uid`, `title`) as queryable metadata.

```bash
./qat-006-metadata-extraction.sh <stack-name>
```

If this fails, the `recallKnowledge` field mapping needs rework before proceeding.

### 2. QAT-005: Post-Deploy Smoke Tests

Full end-to-end validation: submit flow, recall flow, server config, EC2 health, recovery, SSM, observability, alarms.

```bash
./qat-005-post-deploy.sh <stack-name>

# Skip destructive EC2 recovery test:
./qat-005-post-deploy.sh <stack-name> --skip-recovery
```

The submit flow test requires waiting for the EC2 tick loop (~5 min) to process the SQS message.
