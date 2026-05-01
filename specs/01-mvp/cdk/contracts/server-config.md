# Contract: Server-Mode config.yaml

**Source:** CLI data-model.md Server Config Extensions, CDK spec FR-4 (CMP-003)

## Overview

When the CDK stack deploys, the EC2 user data script (CMP-003) templates a `config.yaml` file for the CLI running in server mode. This contract defines the exact YAML fields that CMP-003 must generate, ensuring the CDK-templated config matches what the CLI expects (SRV-001).

**Both sides must reference this contract:**
- **CDK CMP-003** — generates this file from stack outputs during instance boot
- **CLI SRV-001** — validates and loads this file at server startup

If either side changes the schema, the other must be updated to match.

## config.yaml Template

The user data script must generate a file at `/opt/multi-kb/config.yaml` with the following structure. Values in `${...}` are resolved from CDK construct outputs at synthesis time.

```yaml
mode: server
author: "multi-kb-server"

sqs:
  queue_url: "${sqsQueueUrl}"
  batch_size: 10

codecommit:
  repo_name: "${repoName}"
  region: "${region}"

s3:
  bucket: "${bucketName}"
  region: "${region}"

opensearch:
  endpoint: "${collectionEndpoint}"
  region: "${region}"

bedrock_kb:
  knowledge_base_id: "${knowledgeBaseId}"
  data_source_id: "${dataSourceId}"

tick_interval: "${tickInterval}"

dream_cycle:
  interval: "${dreamCycleInterval}"
  model_id: "${consolidationModelId}"

recall_log:
  schedule: "02:00"
```

## Field Mapping: CDK Output → config.yaml

| config.yaml field | CDK source | Required |
|-------------------|------------|----------|
| `sqs.queue_url` | StorageConstruct → queue URL | yes |
| `sqs.batch_size` | Hardcoded `10` (or from stack props) | yes |
| `codecommit.repo_name` | Stack prop `repoName` | yes |
| `codecommit.region` | Stack region | yes |
| `s3.bucket` | StorageConstruct → bucket name | yes |
| `s3.region` | Stack region | yes |
| `opensearch.endpoint` | SearchConstruct → collection endpoint | yes |
| `opensearch.region` | Stack region | yes |
| `bedrock_kb.knowledge_base_id` | KnowledgeBaseConstruct → KB ID | yes |
| `bedrock_kb.data_source_id` | KnowledgeBaseConstruct → data source ID | yes |
| `tick_interval` | Stack prop `tickInterval` (default: `5m`) | yes |
| `dream_cycle.interval` | Stack prop `dreamCycleInterval` (default: `3h`) | yes |
| `dream_cycle.model_id` | Stack prop `consolidationModelId` | yes |
| `recall_log.schedule` | Hardcoded `"02:00"` | yes |
| `mode` | Hardcoded `"server"` | yes |
| `author` | Hardcoded `"multi-kb-server"` | yes |

## File Location

- **Path on EC2:** `/opt/multi-kb/config.yaml`
- **CLI binary invocation:** `multi-kb server --config /opt/multi-kb/config.yaml`
- **Working directory:** `/opt/multi-kb/repo` (cloned CodeCommit repository)

## Systemd Unit Reference

The systemd unit file should invoke:
```
ExecStart=/usr/local/bin/multi-kb server --config /opt/multi-kb/config.yaml
WorkingDirectory=/opt/multi-kb/repo
```

## Validation

The CLI (SRV-001) validates all server-mode fields on startup. If any required field is missing or invalid, the CLI exits with a clear error message identifying the missing field. This provides fast feedback if the CDK template and CLI schema diverge.
