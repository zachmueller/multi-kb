# Research: Multi-KB CDK Infrastructure — MVP

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)
**Status:** R-1 through R-7 all resolved

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

### QAT-006 Findings: Metadata Extraction (Validated 2026-05-03)

**Bedrock KB does NOT extract YAML frontmatter fields as queryable metadata.** The `metadata` field in Retrieve API responses only contains Bedrock system fields:

```json
{
  "metadata": {
    "x-amz-bedrock-kb-source-uri": "s3://bucket/UID.md",
    "x-amz-bedrock-kb-source-file-modality": "TEXT",
    "x-amz-bedrock-kb-chunk-id": "1%3A0%3AcGsr7J0Bd5XQMU3FgTx2",
    "x-amz-bedrock-kb-data-source-id": "KGW4ZLEBZV"
  }
}
```

**However, the YAML frontmatter IS preserved in `content.text`** as raw text (the entire Markdown file including frontmatter block). The actual field mapping for `recallKnowledge`:

| Desired Field | Source | Extraction Method |
|---------------|--------|-------------------|
| `uid` | `metadata["x-amz-bedrock-kb-source-uri"]` or `location.s3Location.uri` | Parse S3 key filename, strip `.md` suffix |
| `title` | `content.text` | Regex parse `^title:\s*"?([^"\n]*)"?` from YAML frontmatter |
| `content` | `content.text` | Direct use (includes frontmatter) |
| `score` | `score` | Direct use |
| `status` (for exclude_pending) | `content.text` | Regex parse `^status:\s*(.*)` — filter client-side, not via Bedrock metadata filter |

**Impact:** The Bedrock metadata filter (`filter: { equals: { key: "status", value: "active" } }`) does NOT work for YAML frontmatter fields. The `excludePending` filtering must be done client-side after retrieval by parsing the `status` field from frontmatter in `content.text`. This means Bedrock returns all results regardless of status, and the Lambda filters post-retrieval.

---

## R-3: EC2 User Data Script Best Practices

**Question:** How to reliably bootstrap an Amazon Linux 2023 EC2 instance with the CLI binary, CloudWatch agent, and server-mode config?

**Status:** Resolved

**Findings:**

### CDK UserData API

#### `UserData.forLinux()` and `addCommands()`

CDK provides `UserData.forLinux()` which creates a Linux user data script. Commands are added via `addCommands(...commands: string[])`, which appends shell commands to the script body. The script is automatically wrapped with `#!/bin/bash` by CDK.

```typescript
const userData = ec2.UserData.forLinux();
userData.addCommands(
  'set -euxo pipefail',
  `aws s3 cp s3://${bucket.bucketName}/binary /usr/local/bin/binary`,
);
```

#### CDK Token Interpolation in User Data

CDK tokens (e.g., `bucket.bucketName`, `queue.queueUrl`) can be used directly in TypeScript template literals inside `addCommands()`. At synthesis time, CDK resolves these tokens to CloudFormation intrinsic functions (`Fn::Join`, `Fn::Ref`, etc.). The resulting user data script in the CloudFormation template contains the correct CloudFormation references that resolve at deployment time.

```typescript
// This works - CDK tokens resolve at synthesis time
userData.addCommands(
  `echo "Queue URL: ${queue.queueUrl}"`,           // Token resolves to Fn::Ref or Fn::GetAtt
  `aws s3 cp ${props.cliBinaryS3Uri} /usr/local/bin/multi-kb`,  // String prop, not a token
);
```

**Important caveats:**
- **Do NOT use `JSON.stringify()` on objects containing CDK tokens.** JavaScript's `JSON.stringify()` will encode the token placeholder string literally, not the CloudFormation intrinsic. Use `Stack.of(this).toJsonString(obj)` instead, which correctly resolves tokens to CloudFormation intrinsics.
- **Do NOT perform string operations** (substring, split, parse) on token values. This breaks the token encoding.
- **Template literals work correctly** with CDK tokens: `` `${bucket.bucketName}` `` produces `Fn::Join` in CloudFormation.
- CDK does not perform shell escaping of interpolated values. If a token value could contain shell-special characters, wrap in single quotes. In practice, AWS resource names/URLs do not contain shell metacharacters, so this is not a concern for our use case.

#### `addS3DownloadCommand()` — Not Used

CDK provides `addS3DownloadCommand(params: S3DownloadOptions)` which generates `aws s3 cp` commands:

```typescript
interface S3DownloadOptions {
  bucket: IBucket;       // Required: S3 bucket reference
  bucketKey: string;     // Required: object key
  localFile?: string;    // Optional: local path (default: /tmp/<bucketKey>)
  region?: string;       // Optional: needed for S3 gateway VPC endpoint
}
```

However, **this requires an `IBucket` CDK construct reference**, not an S3 URI string. Since `cliBinaryS3Uri` is a user-provided string (e.g., `s3://my-artifacts/multi-kb-cli/latest/binary`), we cannot directly use `addS3DownloadCommand()`. We have two options:

1. **Parse the S3 URI** into bucket name + key, then use `Bucket.fromBucketName()` to get an `IBucket` reference. This works but adds complexity.
2. **Use `addCommands()` with a raw `aws s3 cp` command.** Simpler and more transparent.

**Decision: Use `addCommands()` with raw `aws s3 cp`.** The S3 URI is a user-provided string prop, not a CDK construct. Using a raw command is simpler and the behavior is immediately visible in the synthesized template.

#### Multiline Heredocs in User Data for Config Files

For writing config files (config.yaml, systemd unit, CloudWatch agent config), use `addCommands()` with `cat << EOF` heredoc syntax. Pass each line of the heredoc as a separate string to `addCommands()`. CDK joins them with newlines in the synthesized template.

CDK tokens inside these lines are resolved at synthesis time. The resulting CloudFormation template contains literal values or `Fn::Join` intrinsics, not shell variables. Therefore, shell-level heredoc quoting (`<< 'EOF'` vs `<< EOF`) is irrelevant for CDK token resolution.

#### `addSignalOnExitCommand()` for ASG Integration

CDK provides `addSignalOnExitCommand(resource: Resource)` which adds a `cfn-signal` command that runs when the user data script exits (via a bash `trap`). Combined with the `signals` prop on `AutoScalingGroup`, this enables CloudFormation to wait for the instance to fully bootstrap before considering the stack creation successful.

```typescript
const asg = new autoscaling.AutoScalingGroup(this, 'ASG', {
  vpc,
  launchTemplate,
  minCapacity: 1,
  maxCapacity: 1,
  signals: autoscaling.Signals.waitForAll({
    timeout: cdk.Duration.minutes(15),
  }),
});
userData.addSignalOnExitCommand(asg);
```

This adds a `trap` at the end of the user data script that calls `cfn-signal -e $?` with the script's exit code. If the script fails (due to `set -e`), `$?` is non-zero, and CloudFormation receives a FAILURE signal, causing stack rollback. If the script succeeds, CloudFormation receives SUCCESS and proceeds.

**`addSignalOnExitCommand()` must be called AFTER all other `addCommands()` calls**, because it installs the trap handler.

### Script Ordering and Error Handling

#### Error Handling with `set -euxo pipefail`

The script MUST begin with `set -euxo pipefail`:
- **`-e`**: Exit immediately on any command failure. Critical because a partially bootstrapped instance is worse than a failed one (CloudFormation can roll back and retry).
- **`-u`**: Treat unset variables as errors.
- **`-x`**: Print each command before executing (debug output goes to `/var/log/cloud-init-output.log`).
- **`-o pipefail`**: Fail on any command in a pipeline, not just the last one.

#### cfn-signal and ASG CreationPolicy

Use `Signals.waitForAll()` on the ASG with a 15-minute timeout. CDK's `addSignalOnExitCommand()` handles the `cfn-signal` call automatically.

**What happens on script failure:**
1. User data script fails (e.g., `aws s3 cp` fails because binary is missing).
2. `set -e` causes the script to exit with non-zero code.
3. The `trap` handler calls `cfn-signal -e $?` with the failure code.
4. CloudFormation receives FAILURE signal.
5. CloudFormation rolls back the stack (or the ASG update).

**What happens on timeout:**
1. If the script hangs or takes too long (>15 min), CloudFormation times out.
2. Stack creation/update fails.
3. The instance is left running but CloudFormation marks the resource as failed.

#### First-Boot vs. Subsequent Boots

**User data scripts run ONLY on first boot by default.** On Amazon Linux 2023 with cloud-init, the user data script is executed once during the initial instance launch. It does NOT re-run on reboot.

This is the correct behavior for our use case:
- On first boot: install packages, download binary, clone repo, template configs, start services.
- On reboot: systemd automatically restarts the `multi-kb` service (configured with `Restart=on-failure`). No need to re-run user data.
- On ASG replacement: the new instance runs user data fresh (it is a new instance, not a reboot).

**Idempotency is not strictly required** because user data runs only once. However, defensive checks are free and prevent confusion during development:
- `dnf install -y` is idempotent (skips already-installed packages).
- `aws s3 cp` overwrites existing files.
- `git clone` should check if the directory exists first (though on first boot it never will).

#### User Data Size Limit

EC2 user data has a **16 KB raw limit** (before base64 encoding). Our script is well within this limit. If it grows too large, compress with gzip or move logic to a separate script downloaded from S3.

### Package Installation on Amazon Linux 2023

#### Package Manager

Amazon Linux 2023 uses **`dnf`** (successor to `yum`). All package installation commands must use `dnf`:

```bash
dnf install -y amazon-cloudwatch-agent
```

#### Pre-Installed Packages on AL2023

| Package | Pre-installed? | Notes |
|---------|---------------|-------|
| **git** | Yes | Available by default on standard AMI |
| **aws-cli** | Yes | AWS CLI v2 included |
| **amazon-ssm-agent** | Yes | Pre-installed and enabled, starts on boot |
| **python3** | Yes | Python 3 is the default |
| **cloud-init** | Yes | Handles user data execution |
| **systemd** | Yes | Version 252+; uses systemd-networkd for networking |
| **curl/wget** | Yes | Available for downloads |
| **amazon-cloudwatch-agent** | **No** | Must be installed via `dnf install -y amazon-cloudwatch-agent` |

Since git is pre-installed, we do not need to install it. However, including `git` in the `dnf install` command is harmless (dnf skips already-installed packages) and serves as defensive documentation.

#### AL2023 cloud-init Behavior Change

**Critical change:** In AL2023, unavailable remote resources cause a **fatal error** and fail cloud-init execution (AL2 treated this as a warning). This means if the S3 binary download fails, cloud-init will fail the instance, which is actually the behavior we want (fail fast, signal ASG).

#### Python 2.7 Removed

AL2023 removes Python 2.7. Only Python 3 is available. This does not affect our script (we do not use Python), but `cfn-bootstrap` scripts use Python and are compatible with Python 3 on AL2023.

### CLI Binary Download

#### Pattern

```bash
aws s3 cp "${cliBinaryS3Uri}" /usr/local/bin/multi-kb
chmod +x /usr/local/bin/multi-kb
```

#### S3 Gateway VPC Endpoint

Yes, `aws s3 cp` works through the S3 gateway VPC endpoint. The gateway endpoint is a route table entry that routes S3 traffic within the AWS network. The AWS CLI automatically uses this route. No special configuration needed on the CLI side.

#### Checksum Verification

For MVP, **skip checksum verification.** The S3 download is integrity-protected by S3's built-in checksums (Content-MD5 on upload, ETag verification on download). If the binary is corrupted, the CLI process will fail to start, and the systemd unit will report failure.

Post-MVP, consider publishing a `.sha256` sidecar file and verifying after download.

### Git Credential Helper for CodeCommit

#### Configuration

```bash
git config --system credential.helper '!aws codecommit credential-helper $@'
git config --system credential.UseHttpPath true
```

**Use `--system` instead of `--global`** because the user data script runs as root, and the systemd service may run as a different user. `--system` applies to all users on the machine.

Both settings are required:
- **`credential.helper`**: Invokes the AWS CLI's CodeCommit credential helper, which generates temporary Git credentials from the instance's IAM role.
- **`credential.UseHttpPath`**: Required so the credential helper receives the full repository path (needed to differentiate CodeCommit repositories).

#### IAM Role Integration

The credential helper uses the EC2 instance's IAM role (via instance metadata) to generate temporary credentials. No static credentials or SSH keys are needed. The IAM role must have `codecommit:GitPull` and `codecommit:GitPush` permissions on the repository ARN.

#### AL2023 Considerations

No AL2023-specific issues. The AWS CLI v2 (pre-installed) includes the `codecommit credential-helper` subcommand. Git (pre-installed) supports the credential helper interface.

#### Clone Command

```bash
git clone https://git-codecommit.${REGION}.amazonaws.com/v1/repos/${REPO_NAME} /opt/multi-kb/repo
```

The git-codecommit VPC endpoint enables this clone to work from the private subnet without NAT. Private DNS on the endpoint resolves `git-codecommit.{region}.amazonaws.com` to the VPC endpoint IP.

### CloudWatch Agent Configuration

#### Installation

```bash
dnf install -y amazon-cloudwatch-agent
```

This installs from the Amazon Linux 2023 package repository (available by default). No need to download from S3.

#### Configuration File

Place the configuration at `/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json`:

```json
{
  "agent": {
    "logfile": "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
  },
  "logs": {
    "logs_collected": {
      "files": {
        "collect_list": [
          {
            "file_path": "/var/log/multi-kb/server.log",
            "log_group_name": "<CDK_LOG_GROUP_NAME>",
            "log_stream_name": "{instance_id}",
            "timestamp_format": "%Y-%m-%dT%H:%M:%S",
            "timezone": "UTC"
          }
        ]
      }
    },
    "force_flush_interval": 15
  }
}
```

Key configuration choices:
- **`file_path`**: The log file that the CLI process writes to (via systemd `StandardOutput=append:`).
- **`log_group_name`**: Set from CDK construct output. The log group is created by the CDK stack (OBS-001), and the name is interpolated into the config at synthesis time.
- **`log_stream_name`**: `{instance_id}` is a built-in CloudWatch agent variable that resolves to the EC2 instance ID at runtime. This ensures each ASG replacement instance gets its own log stream.
- **`timestamp_format`**: Matches ISO 8601 prefix format. The CLI outputs JSON lines with timestamps in this format.
- **`force_flush_interval`**: 15 seconds. Ensures logs appear in CloudWatch within 15 seconds of being written.

#### Structured JSON Log Shipping

The CloudWatch agent ships log lines as-is to CloudWatch Logs. It does NOT parse JSON internally. However, when the CLI outputs JSON lines (one JSON object per line), CloudWatch Logs stores each line as a separate log event. CloudWatch Logs Insights can then query the JSON fields directly using its built-in JSON parsing:

```
fields @timestamp, @message
| parse @message '{"level":"*","msg":"*"' as level, msg
| filter level = "ERROR"
```

This approach (ship raw JSON lines, query with Insights) is the standard pattern for structured logging on EC2 with CloudWatch.

#### Starting the Agent

```bash
/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl \
  -a fetch-config \
  -m ec2 \
  -s \
  -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json
```

Flags:
- **`-a fetch-config`**: Load configuration and start.
- **`-m ec2`**: Running on EC2 (uses instance metadata for region/credentials).
- **`-s`**: Start the agent after loading config.
- **`-c file:...`**: Configuration file location (note the `file:` prefix).

The agent runs as a systemd service (`amazon-cloudwatch-agent`). After the initial `fetch-config`, it persists the configuration and auto-starts on reboot. The agent uses the EC2 instance's IAM role for CloudWatch Logs API calls via the `com.amazonaws.{region}.logs` VPC endpoint.

### Systemd Unit File

#### Complete Unit File

```ini
[Unit]
Description=Multi-KB Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/multi-kb server --config /opt/multi-kb/config.yaml
WorkingDirectory=/opt/multi-kb/repo
Restart=on-failure
RestartSec=5
StandardOutput=append:/var/log/multi-kb/server.log
StandardError=append:/var/log/multi-kb/server.log
Environment=AWS_REGION=<REGION>
Environment=HOME=/root

[Install]
WantedBy=multi-user.target
```

#### Key Directives Explained

**`Type=simple`**: The process started by `ExecStart` is the main process. systemd considers the service started as soon as the process is forked. This is correct for a long-running CLI binary that does not daemonize itself.

**`Restart=on-failure`**: Restart the service only when it exits with a non-zero exit code, is killed by a signal, or times out. Preferred over `Restart=always` because:
- If the process exits cleanly (code 0), it was intentional and should not restart.
- If the process crashes (non-zero exit), it should restart automatically.
- On a normal `systemctl stop`, the service receives SIGTERM which counts as a clean stop with `on-failure`, so it will NOT restart.

**`RestartSec=5`**: Wait 5 seconds between restart attempts. Prevents tight restart loops that could overwhelm downstream services (SQS, CodeCommit).

**`WorkingDirectory=/opt/multi-kb/repo`**: The cloned CodeCommit repository. The CLI process runs git commands relative to this directory.

**`StandardOutput=append:/var/log/multi-kb/server.log`** and **`StandardError=append:/var/log/multi-kb/server.log`**: Redirect both stdout and stderr to a log file that CloudWatch agent monitors. The `append:` prefix (available since systemd 240; AL2023 ships systemd 252+) appends to the file rather than truncating on service restart. Both streams go to the same file for unified logging.

**Important:** The log directory `/var/log/multi-kb/` must be created before the service starts. Add `mkdir -p /var/log/multi-kb` to the user data script.

**`Environment=AWS_REGION=<REGION>`**: Set the AWS region for SDK calls. Setting it explicitly is more reliable and faster than falling back to instance metadata.

**`Environment=HOME=/root`**: Ensures git can find the system-level git config and that the AWS CLI can find credentials.

#### Running as Root vs. Dedicated User

For MVP, **run as root**. Rationale:
- The process needs git operations, credential helper access, and writes to `/var/log/multi-kb/` and `/opt/multi-kb/`.
- Creating a dedicated user adds complexity (file permissions, credential helper, HOME setup) with minimal security benefit for a single-purpose instance in a private subnet.

Post-MVP, consider creating a `multi-kb` system user with appropriate permissions.

#### Enabling the Service

```bash
systemctl daemon-reload
systemctl enable --now multi-kb
```

`enable --now` both enables the service (auto-start on boot) and starts it immediately.

### Complete User Data Script (Bash)

The `${...}` placeholders below represent values that CDK resolves at synthesis time via token interpolation. See the CDK code pattern section for the exact TypeScript implementation.

```bash
#!/bin/bash
set -euxo pipefail

# --- Step 1: Install packages ---
dnf install -y amazon-cloudwatch-agent

# --- Step 2: Download CLI binary from S3 ---
aws s3 cp "${CLI_BINARY_S3_URI}" /usr/local/bin/multi-kb
chmod +x /usr/local/bin/multi-kb

# --- Step 3: Configure git credential helper for CodeCommit ---
git config --system credential.helper '!aws codecommit credential-helper $@'
git config --system credential.UseHttpPath true

# --- Step 4: Clone CodeCommit repository ---
mkdir -p /opt/multi-kb
git clone "https://git-codecommit.${REGION}.amazonaws.com/v1/repos/${REPO_NAME}" \
  /opt/multi-kb/repo || {
  # If repo is empty (first deploy), initialize it
  mkdir -p /opt/multi-kb/repo
  cd /opt/multi-kb/repo
  git init
  git remote add origin \
    "https://git-codecommit.${REGION}.amazonaws.com/v1/repos/${REPO_NAME}"
}

# --- Step 5: Template config.yaml ---
cat > /opt/multi-kb/config.yaml << CONFIGEOF
mode: server
author: "multi-kb-server"

sqs:
  queue_url: "${SQS_QUEUE_URL}"
  batch_size: 10

codecommit:
  repo_name: "${REPO_NAME}"
  region: "${REGION}"

s3:
  bucket: "${BUCKET_NAME}"
  region: "${REGION}"

opensearch:
  endpoint: "${COLLECTION_ENDPOINT}"
  region: "${REGION}"

bedrock_kb:
  knowledge_base_id: "${KB_ID}"
  data_source_id: "${DATA_SOURCE_ID}"

tick_interval: "${TICK_INTERVAL}"

dream_cycle:
  interval: "${DREAM_CYCLE_INTERVAL}"
  model_id: "${CONSOLIDATION_MODEL_ID}"

recall_log:
  schedule: "02:00"
CONFIGEOF

# --- Step 6: Configure CloudWatch agent ---
mkdir -p /var/log/multi-kb
cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json << 'CWEOF'
{
  "agent": {
    "logfile": "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
  },
  "logs": {
    "logs_collected": {
      "files": {
        "collect_list": [
          {
            "file_path": "/var/log/multi-kb/server.log",
            "log_group_name": "${LOG_GROUP_NAME}",
            "log_stream_name": "{instance_id}",
            "timestamp_format": "%Y-%m-%dT%H:%M:%S",
            "timezone": "UTC"
          }
        ]
      }
    },
    "force_flush_interval": 15
  }
}
CWEOF

# --- Step 7: Create systemd unit file ---
cat > /etc/systemd/system/multi-kb.service << UNITEOF
[Unit]
Description=Multi-KB Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/multi-kb server --config /opt/multi-kb/config.yaml
WorkingDirectory=/opt/multi-kb/repo
Restart=on-failure
RestartSec=5
StandardOutput=append:/var/log/multi-kb/server.log
StandardError=append:/var/log/multi-kb/server.log
Environment=AWS_REGION=${REGION}
Environment=HOME=/root

[Install]
WantedBy=multi-user.target
UNITEOF

# --- Step 8: Start services ---
systemctl daemon-reload

# Start CloudWatch agent with config
/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl \
  -a fetch-config -m ec2 -s \
  -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json

# Enable and start multi-kb service
systemctl enable --now multi-kb
```

### CDK TypeScript Code Pattern

The following shows how the user data script is generated in the CDK compute construct, with all CDK token interpolation:

```typescript
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as autoscaling from 'aws-cdk-lib/aws-autoscaling';
import * as cdk from 'aws-cdk-lib';

// Props received from other constructs
interface ComputeProps {
  vpc: ec2.IVpc;
  subnet: ec2.ISubnet;
  ec2Sg: ec2.ISecurityGroup;
  cliBinaryS3Uri: string;
  region: string;
  repoName: string;
  queueUrl: string;           // CDK token from SQS queue
  bucketName: string;          // CDK token from S3 bucket
  collectionEndpoint: string;  // CDK token from OpenSearch collection
  knowledgeBaseId: string;     // CDK token from Bedrock KB
  dataSourceId: string;        // CDK token from Bedrock data source
  tickInterval: string;
  dreamCycleInterval: string;
  consolidationModelId: string;
  logGroupName: string;        // CDK token from CloudWatch log group
  instanceType: ec2.InstanceType;
  instanceProfile: iam.IInstanceProfile;
}

// Build user data
const userData = ec2.UserData.forLinux();

// Step 0: Error handling
userData.addCommands('set -euxo pipefail');

// Step 1: Install packages
userData.addCommands('dnf install -y amazon-cloudwatch-agent');

// Step 2: Download CLI binary
userData.addCommands(
  `aws s3 cp ${props.cliBinaryS3Uri} /usr/local/bin/multi-kb`,
  'chmod +x /usr/local/bin/multi-kb',
);

// Step 3: Git credential helper
userData.addCommands(
  "git config --system credential.helper '!aws codecommit credential-helper $@'",
  'git config --system credential.UseHttpPath true',
);

// Step 4: Clone repository (with empty-repo fallback)
userData.addCommands(
  'mkdir -p /opt/multi-kb',
  [
    `git clone https://git-codecommit.${props.region}.amazonaws.com`,
    `/v1/repos/${props.repoName} /opt/multi-kb/repo || {`,
  ].join(''),
  '  mkdir -p /opt/multi-kb/repo',
  '  cd /opt/multi-kb/repo',
  '  git init',
  [
    '  git remote add origin https://git-codecommit.',
    `${props.region}.amazonaws.com/v1/repos/${props.repoName}`,
  ].join(''),
  '}',
);

// Step 5: Template config.yaml (using CDK tokens via template literals)
userData.addCommands(
  'cat > /opt/multi-kb/config.yaml << CONFIGEOF',
  'mode: server',
  'author: "multi-kb-server"',
  '',
  'sqs:',
  `  queue_url: "${props.queueUrl}"`,
  '  batch_size: 10',
  '',
  'codecommit:',
  `  repo_name: "${props.repoName}"`,
  `  region: "${props.region}"`,
  '',
  's3:',
  `  bucket: "${props.bucketName}"`,
  `  region: "${props.region}"`,
  '',
  'opensearch:',
  `  endpoint: "${props.collectionEndpoint}"`,
  `  region: "${props.region}"`,
  '',
  'bedrock_kb:',
  `  knowledge_base_id: "${props.knowledgeBaseId}"`,
  `  data_source_id: "${props.dataSourceId}"`,
  '',
  `tick_interval: "${props.tickInterval}"`,
  '',
  'dream_cycle:',
  `  interval: "${props.dreamCycleInterval}"`,
  `  model_id: "${props.consolidationModelId}"`,
  '',
  'recall_log:',
  '  schedule: "02:00"',
  'CONFIGEOF',
);

// Step 6: CloudWatch agent config
// Use Stack.toJsonString() for safe CDK token serialization in JSON
const stack = cdk.Stack.of(this);
const cwAgentConfig = stack.toJsonString({
  agent: {
    logfile: '/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log',
  },
  logs: {
    logs_collected: {
      files: {
        collect_list: [{
          file_path: '/var/log/multi-kb/server.log',
          log_group_name: props.logGroupName,  // CDK token - safely serialized
          log_stream_name: '{instance_id}',
          timestamp_format: '%Y-%m-%dT%H:%M:%S',
          timezone: 'UTC',
        }],
      },
    },
    force_flush_interval: 15,
  },
});

userData.addCommands(
  'mkdir -p /var/log/multi-kb',
  'mkdir -p /opt/aws/amazon-cloudwatch-agent/etc',
  `cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json << 'CWEOF'`,
  cwAgentConfig,
  'CWEOF',
);

// Step 7: Systemd unit file
userData.addCommands(
  'cat > /etc/systemd/system/multi-kb.service << UNITEOF',
  '[Unit]',
  'Description=Multi-KB Server',
  'After=network-online.target',
  'Wants=network-online.target',
  '',
  '[Service]',
  'Type=simple',
  'ExecStart=/usr/local/bin/multi-kb server --config /opt/multi-kb/config.yaml',
  'WorkingDirectory=/opt/multi-kb/repo',
  'Restart=on-failure',
  'RestartSec=5',
  'StandardOutput=append:/var/log/multi-kb/server.log',
  'StandardError=append:/var/log/multi-kb/server.log',
  `Environment=AWS_REGION=${props.region}`,
  'Environment=HOME=/root',
  '',
  '[Install]',
  'WantedBy=multi-user.target',
  'UNITEOF',
);

// Step 8: Start services
userData.addCommands(
  'systemctl daemon-reload',
  [
    '/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl',
    ' -a fetch-config -m ec2 -s',
    ' -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json',
  ].join(''),
  'systemctl enable --now multi-kb',
);

// Create launch template
const launchTemplate = new ec2.LaunchTemplate(this, 'LaunchTemplate', {
  machineImage: ec2.MachineImage.latestAmazonLinux2023(),
  instanceType: props.instanceType,
  userData,
  securityGroup: props.ec2Sg,
  instanceProfile: props.instanceProfile,
  requireImdsv2: true,  // Security best practice
});

// Create ASG with signaling
const asg = new autoscaling.AutoScalingGroup(this, 'ASG', {
  vpc: props.vpc,
  launchTemplate,
  minCapacity: 1,
  maxCapacity: 1,
  vpcSubnets: { subnets: [props.subnet] },
  signals: autoscaling.Signals.waitForAll({
    timeout: cdk.Duration.minutes(15),
  }),
});

// Signal CloudFormation on script exit — MUST be called last
userData.addSignalOnExitCommand(asg);
```

**Key CDK pattern notes:**

1. **`Stack.toJsonString()`** is used for the CloudWatch agent JSON config because it contains CDK token values (`logGroupName`). Regular `JSON.stringify()` would break the token.

2. **Template literals** (`${props.queueUrl}`) work in `addCommands()` because CDK tokens resolve to `Fn::Join` intrinsics in the synthesized CloudFormation template.

3. **`addSignalOnExitCommand(asg)`** must be called AFTER all other `addCommands()` calls, because it adds a `trap` that should be the last thing in the script.

4. **`requireImdsv2: true`** on the launch template enforces IMDSv2 (Instance Metadata Service v2), a security best practice. AWS CLI v2 on AL2023 supports IMDSv2.

5. **Unquoted heredoc delimiters** (`<< CONFIGEOF` not `<< 'CONFIGEOF'`) for config.yaml and systemd unit. CDK resolves all tokens before the shell sees the script. The single exception is the CloudWatch agent config (`<< 'CWEOF'`) which is pre-rendered via `toJsonString()` and should not be subject to shell expansion.

### Known Gotchas and AL2023-Specific Issues

1. **AL2023 cloud-init fatal on remote resource failure.** Unlike AL2, AL2023 fails cloud-init execution if any remote resource is unavailable. This is desirable: if the S3 binary download fails, the instance fails fast and signals ASG.

2. **`dnf` not `yum`.** AL2023 uses `dnf`. Using `yum` may work (it is aliased) but `dnf` is canonical.

3. **`StandardOutput=append:` requires systemd 240+.** AL2023 ships systemd 252+, so this is safe. Older distros would need `StandardOutput=file:` (which truncates on restart) or a wrapper script.

4. **CloudWatch agent `{instance_id}` in `log_stream_name`.** This is a CloudWatch agent variable, NOT a shell variable or CDK token. It must be passed literally (not interpolated). The agent resolves it at runtime. Use `Stack.toJsonString()` to render it as a literal string in the JSON.

5. **Git clone of empty repository.** On first deployment, the CodeCommit repo may be empty (no default branch). `git clone` of an empty repo outputs a warning but succeeds, creating an empty directory with `.git`. The `|| { git init ... }` fallback handles edge cases where the clone fails entirely.

6. **User data 16 KB limit.** Our script is within this limit, but monitor it as configuration grows. If needed, move the CloudWatch agent config to a separate file downloaded from S3.

7. **IMDSv2 and AWS CLI.** With `requireImdsv2: true`, the instance only accepts token-based IMDSv2 requests. AWS CLI v2 (pre-installed on AL2023) supports this. Older CLIs may fail.

8. **`Signals.waitForAll()` timeout.** Set to 15 minutes. Must be long enough for `dnf install` + S3 download + git clone + service start. Typically completes in 2-3 minutes; 15 minutes provides generous headroom.

9. **Heredoc lines in `addCommands()`.** Each line of the heredoc is a separate string argument. CDK joins them with newlines. The heredoc start marker and end marker must each be their own string.

10. **`addSignalOnExitCommand()` must be called last.** It installs a bash `trap EXIT` handler. If called before other `addCommands()`, the trap would not cover subsequent commands.

11. **EC2 IAM role permissions for CloudWatch agent.** The instance role needs `logs:CreateLogGroup`, `logs:CreateLogStream`, `logs:PutLogEvents`, and `logs:DescribeLogStreams` for the CloudWatch agent to ship logs. These should be scoped to the specific log group ARN.

**Decision:**

1. **Use `UserData.forLinux()` with `addCommands()` for the entire script.** No `CloudFormationInit` (cfn-init) needed; a bash script is simpler and more debuggable.

2. **Use `addSignalOnExitCommand()` + `Signals.waitForAll()` for ASG integration.** CloudFormation waits for bootstrap; on failure, the stack rolls back cleanly.

3. **Use `Stack.toJsonString()` for the CloudWatch agent JSON config.** Safely serializes CDK tokens. For YAML config and systemd units, use line-by-line `addCommands()` with template literal interpolation.

4. **Use `StandardOutput=append:` in the systemd unit** to write to a log file that CloudWatch agent monitors. Both stdout and stderr go to the same file.

5. **Run as root for MVP.** Simplifies credential helper access, file permissions, and git operations.

6. **`dnf install -y amazon-cloudwatch-agent`** is the only required package installation. git, aws-cli, and SSM agent are pre-installed on AL2023.

7. **No idempotency guard needed.** User data runs only on first boot. ASG replacement instances are fresh.

8. **15-minute signal timeout.** Generous but bounded.

9. **`requireImdsv2: true`** on the launch template. Security best practice with no downside on AL2023.

10. **Unquoted heredoc delimiters** for configs where CDK has pre-resolved all tokens.

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

## R-5: Crockford Base32 UID Generation in Node.js ✅

**Question:** How to generate 16-character Crockford base32 UIDs in the submitKnowledge Lambda?

**Status:** Resolved

**Areas to Investigate:**
- Crockford base32 alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ` (32 chars, excludes I, L, O, U)
- Encoding: 10 bytes (80 bits) from `crypto.randomBytes(10)` → 16 characters (5 bits per char)
- Existing npm packages: `base32-encode` (supports Crockford), `crockford-base32`
- Zero-dependency implementation (preferred for Lambda — fewer dependencies = faster cold starts)
- Must match CLI's Go implementation format exactly (CLI plan R-7)

**Prototype Task:** Implement the function, verify it produces exactly 16 uppercase chars from the Crockford alphabet, and confirm round-trip compatibility with the Go implementation.

**Findings:**

### Crockford Base32 Alphabet

The Crockford Base32 alphabet uses 32 symbols: `0123456789ABCDEFGHJKMNPQRSTVWXYZ`. It deliberately excludes four letters that are visually ambiguous: **I** (confused with 1), **L** (confused with 1), **O** (confused with 0), and **U** (accidental obscenity). The canonical form is uppercase.

### Encoding Algorithm

10 random bytes (80 bits) map to exactly 16 Crockford Base32 characters (5 bits per character, 80/5 = 16). No padding is needed — the math is exact.

**Bit-buffer approach (recommended for Node.js Lambda):**

```typescript
const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

export function generateUid(): string {
  const bytes = crypto.randomBytes(10);
  return encodeCrockford(bytes);
}

export function encodeCrockford(data: Buffer): string {
  let out = '';
  let buf = 0;
  let bits = 0;
  for (const b of data) {
    buf = (buf << 8) | b;
    bits += 8;
    while (bits >= 5) {
      bits -= 5;
      out += ALPHABET[(buf >>> bits) & 0x1F];
    }
  }
  return out;
}
```

**JavaScript integer safety note:** The bit-buffer value never exceeds 2^15 (accumulates at most 12 bits before extraction: 8 new + 4 remaining, and 12 < 32). All operations stay within the 32-bit safe zone for JavaScript bitwise operators. Use `>>>` (unsigned right shift) instead of `>>` (signed) for correctness, though for values this small both produce identical results.

### No npm Package Needed

While `base32-encode` and `crockford-base32` packages exist on npm, the encoding is a ~12-line zero-dependency function. For Lambda cold start optimization, zero external dependencies is strongly preferred — each dependency adds to the deployment package size and module resolution time.

Node.js's built-in `crypto.randomBytes(10)` provides cryptographically secure entropy. No external random library needed.

### Collision Probability

80 bits of entropy provides ~1.2 × 10²⁴ possible values. The birthday paradox threshold is ~2⁴⁰ ≈ 1 trillion UIDs before a 50% collision probability. For a knowledge base system generating hundreds to thousands of notes, collision is astronomically unlikely.

### Authoritative Test Vectors

These vectors are shared between Node.js (CDK R-5) and Go (CLI R-7) to ensure cross-implementation compatibility:

| Input (hex) | Input (bytes) | Output |
|---|---|---|
| `00000000000000000000` | `[0x00 × 10]` | `0000000000000000` |
| `FFFFFFFFFFFFFFFFFFFF` | `[0xFF × 10]` | `ZZZZZZZZZZZZZZZZ` |
| `00010203040506070809` | `[0x00..0x09]` | `000G40R40M30E209` |
| `DEADBEEFCAFEBABE0042` | `[0xDE, 0xAD, ...]` | `VTPVXVYAZTXBW022` |
| `48656C6C6F576F726C64` | `"HelloWorld"` | `91JPRV3FAXQQ4V34` |

### Integration with submitKnowledge Lambda

The UID is generated synchronously on each `submitKnowledge` request. The flow is:

1. Validate request body → generate UID → generate `submitted_at` → enqueue to SQS → return `{ uid, request_id }`

The UID generation adds negligible latency (~0.1ms for 10 random bytes + encoding).

**Decision:**

1. **Zero-dependency implementation.** Use the bit-buffer encoding algorithm with `crypto.randomBytes(10)`. No npm packages. Place in `lambda/shared/uid.ts`.

2. **Export `encodeCrockford()` separately from `generateUid()`.** This enables deterministic testing: `encodeCrockford(Buffer.from(...))` should produce the test vectors above. `generateUid()` calls `encodeCrockford()` with random bytes.

3. **Canonical uppercase output.** The alphabet constant is uppercase; no post-processing needed.

4. **Use `>>>` (unsigned right shift)** in the bit extraction to avoid signed integer issues, even though the accumulated value is small enough that `>>` would also work.

5. **Test vectors shared with CLI R-7.** The 5 test vectors above must pass in both the Node.js and Go implementations. This ensures UIDs generated by the Lambda and by the CLI are format-compatible (though they are never correlated — each side generates independent UIDs).

6. **No decoding in MVP.** UIDs are write-once identifiers. No need for Crockford decode or error correction.

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

**Status:** Resolved

**Findings:**

### Required Response Shape

The Lambda proxy integration (REST API v1, payload format 1.0) requires Lambda to return a JSON object with this structure:

```json
{
  "isBase64Encoded": false,
  "statusCode": 200,
  "headers": { "Content-Type": "application/json" },
  "multiValueHeaders": { "X-Custom": ["value1", "value2"] },
  "body": "{\"key\": \"value\"}"
}
```

### TypeScript Interface (Authoritative -- from @types/aws-lambda)

```typescript
// From DefinitelyTyped: @types/aws-lambda
interface APIGatewayProxyResult {
  statusCode: number;                                              // REQUIRED
  headers?: { [header: string]: boolean | number | string };       // optional
  multiValueHeaders?: { [header: string]: Array<boolean | number | string> }; // optional
  body: string;                                                    // REQUIRED
  isBase64Encoded?: boolean;                                       // optional
}
```

**Important:** This is the REST API v1 (payload format 1.0) type. The v2 type (`APIGatewayProxyStructuredResultV2`) has different rules where `statusCode` and `body` are both optional -- but this project uses REST API, not HTTP API, so v1 applies.

### Field-by-Field Analysis

| Field | Required | Type | Notes |
|-------|----------|------|-------|
| `statusCode` | **Yes** | `number` | Integer HTTP status code. If missing or if the entire response is malformed, API Gateway returns **502 Bad Gateway** to the client. |
| `body` | **Yes** | `string` | Must be a JSON-serialized string (via `JSON.stringify()`). If `body` is an object instead of a string, API Gateway returns **502 Bad Gateway** -- it does not auto-serialize. |
| `headers` | No | `Record<string, string \| number \| boolean>` | Single-value headers. Can be omitted entirely. API Gateway converts these to HTTP response headers. |
| `multiValueHeaders` | No | `Record<string, Array<string \| number \| boolean>>` | For headers that need multiple values (e.g., `Set-Cookie`). If both `headers` and `multiValueHeaders` contain the same key, `multiValueHeaders` values take precedence in the merged output. |
| `isBase64Encoded` | No | `boolean` | Defaults to `false` if omitted. Set to `true` only when returning binary content AND `*/*` is configured as a Binary Media Type in API Gateway. **Not needed for JSON responses.** |

### What Happens When Things Go Wrong

#### Missing `statusCode`

API Gateway returns **HTTP 502** with a body like:

```json
{ "message": "Internal server error" }
```

The CloudWatch execution log records: `Execution failed due to configuration error: Malformed Lambda proxy response`. This is a gateway response of type `DEFAULT_5XX`.

#### `body` Is an Object (Not a String)

Same as missing `statusCode` -- API Gateway returns **HTTP 502** with `{ "message": "Internal server error" }`. The response fails format validation before any header or status code processing occurs.

#### Lambda Throws an Unhandled Exception

When a Lambda function throws (or rejects a promise) without catching, the Lambda service returns an error response to API Gateway. Since this error response does not conform to the proxy integration format (it is a Lambda error envelope with `errorMessage`, `errorType`, `stackTrace`), API Gateway returns **HTTP 502** to the client:

```json
{ "message": "Internal server error" }
```

The Lambda error details (stack trace, error message) are logged to CloudWatch Logs but **never exposed to the client**. This is a security feature -- internal error details do not leak.

#### Lambda Returns `{ statusCode: 500, body: "..." }` (Intentional 500)

API Gateway faithfully passes this to the client as HTTP 500 with the provided body. This is a **Lambda-returned error**, not a Lambda execution error. The client sees exactly what Lambda returned.

**Key distinction:**
- **Lambda crash** (unhandled exception) -> client sees **502** with generic message
- **Lambda returns 500** (intentional) -> client sees **500** with Lambda's body
- **Lambda returns malformed response** -> client sees **502** with generic message

### HTTP 401/403 -- API Gateway Handles These

With `AWS_IAM` authorization, API Gateway evaluates SigV4 credentials **before invoking Lambda**:

- **HTTP 401 (Unauthorized):** Missing or invalid SigV4 signature. Lambda is never invoked. Response body: `{ "message": "Missing Authentication Token" }` or `{ "message": "Unauthorized" }`.
- **HTTP 403 (Forbidden):** Valid SigV4 signature but IAM policy denies `execute-api:Invoke`. Lambda is never invoked. Response body: `{ "message": "User: arn:aws:iam::... is not authorized to perform: execute-api:Invoke on resource: ..." }`.

These are **gateway responses**, not Lambda responses. The Lambda handler does not need to handle 401/403 -- API Gateway handles them at the auth layer.

### CORS Headers

The spec says CORS is not enabled (CLI is not a browser client). **No CORS headers needed.** The only header we set is `Content-Type: application/json` for consistency and to help debugging tools parse the response body correctly.

If CORS were needed in the future, the Lambda would need to add `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods`, and `Access-Control-Allow-Headers` to the response headers. API Gateway does not auto-add these for proxy integrations -- they must come from Lambda.

### `multiValueHeaders` Field

Used when a response needs to include multiple values for the same header name (e.g., multiple `Set-Cookie` headers). For this project's JSON responses, `multiValueHeaders` is not needed. If both `headers` and `multiValueHeaders` are present with the same key, `multiValueHeaders` wins.

### Response Helper Module Design

#### Goals

1. **Guarantee well-formed responses** -- every code path returns a valid proxy integration response
2. **Consistent Content-Type** -- all responses include `Content-Type: application/json`
3. **Type-safe** -- TypeScript catches misuse at compile time
4. **Minimal** -- no external dependencies, ~30 lines of code

#### Implementation: `lambda/shared/response.ts`

```typescript
import type { APIGatewayProxyResult } from 'aws-lambda';

const JSON_HEADERS = { 'Content-Type': 'application/json' } as const;

/**
 * Build a success response with the given status code and body.
 * Body is automatically JSON.stringify'd.
 */
export function success(statusCode: number, body: unknown): APIGatewayProxyResult {
  return {
    statusCode,
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  };
}

/**
 * Build an error response with the given status code and error body.
 * Body is automatically JSON.stringify'd.
 */
export function error(statusCode: number, body: unknown): APIGatewayProxyResult {
  return {
    statusCode,
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  };
}

/**
 * Build a validation error response (HTTP 400) with field-level errors.
 * Matches the contract: { "errors": { "<field>": "<reason>", ... } }
 */
export function validationError(errors: Record<string, string>): APIGatewayProxyResult {
  return error(400, { errors });
}

/**
 * Build a generic internal server error response (HTTP 500).
 * Never exposes internal error details to the client.
 */
export function internalError(): APIGatewayProxyResult {
  return error(500, { message: 'Internal server error' });
}
```

**Design decisions:**

- `success()` and `error()` are structurally identical (both wrap in the proxy format). The semantic separation makes handler code self-documenting: `return success(202, { uid, request_id })` vs. `return error(500, { message: '...' })`.
- `validationError()` is a convenience for the `{ errors: { field: reason } }` pattern used by both endpoints.
- `internalError()` returns a generic message -- never expose stack traces or internal details.
- `JSON_HEADERS` is a shared constant to avoid object allocation per response.
- The `body` parameter is `unknown` (not `string`) because the helper calls `JSON.stringify()` internally. Callers pass plain objects.

#### Handler Wrapper Pattern

Every handler should wrap its logic in a top-level try/catch to guarantee a well-formed response even on unexpected errors:

```typescript
import type { APIGatewayProxyEvent, APIGatewayProxyResult } from 'aws-lambda';
import { success, validationError, internalError } from './shared/response';

export async function handler(event: APIGatewayProxyEvent): Promise<APIGatewayProxyResult> {
  try {
    // Parse body
    const body = JSON.parse(event.body ?? '{}');

    // Validate
    const validation = validate(body);
    if (!validation.valid) {
      return validationError(validation.errors);
    }

    // Business logic...
    const result = await doWork(body);

    return success(200, result);
  } catch (err) {
    // Log full error for debugging -- never expose to client
    console.error('Unhandled error:', err);
    return internalError();
  }
}
```

**Why this pattern is critical:** Without the try/catch, an unhandled exception causes Lambda to return a Lambda error envelope (not the proxy integration format), and API Gateway returns **502 Bad Gateway** with `{ "message": "Internal server error" }`. While the client sees a similar message either way, the 502 vs 500 distinction matters:

- **502** suggests an infrastructure problem (API Gateway could not communicate with Lambda properly)
- **500** suggests an application error (Lambda ran but something went wrong)

Returning 500 from the catch block gives operators better signal about where the problem is. It also allows the Lambda to log the error with structured JSON (CloudWatch) before returning, rather than relying on Lambda's default error logging.

### Concrete Response Examples

#### submitKnowledge -- HTTP 202 (Accepted)

```json
{
  "statusCode": 202,
  "headers": { "Content-Type": "application/json" },
  "body": "{\"uid\":\"01H5K9QZXNM8V3PW\",\"request_id\":\"a1b2c3d4-e5f6-7890-abcd-ef1234567890\"}"
}
```

Client receives:
```
HTTP/1.1 202 Accepted
Content-Type: application/json

{"uid":"01H5K9QZXNM8V3PW","request_id":"a1b2c3d4-e5f6-7890-abcd-ef1234567890"}
```

#### submitKnowledge -- HTTP 400 (Validation Error, Single Field)

```json
{
  "statusCode": 400,
  "headers": { "Content-Type": "application/json" },
  "body": "{\"errors\":{\"title\":\"must be present and non-empty\"}}"
}
```

#### submitKnowledge -- HTTP 400 (Validation Error, Multiple Fields)

```json
{
  "statusCode": 400,
  "headers": { "Content-Type": "application/json" },
  "body": "{\"errors\":{\"title\":\"must be present and non-empty\",\"content\":\"must not exceed 100,000 characters\"}}"
}
```

#### recallKnowledge -- HTTP 200 (Results)

```json
{
  "statusCode": 200,
  "headers": { "Content-Type": "application/json" },
  "body": "[{\"uid\":\"01H5K9QZXNM8V3PW\",\"title\":\"DynamoDB Global Tables Configuration\",\"content\":\"## Overview\\nGlobal Tables provide...\",\"score\":0.87},{\"uid\":\"01H5KABCDEF12345\",\"title\":\"Cross-Region Replication Patterns\",\"content\":\"## Replication Strategies\\n...\",\"score\":0.72}]"
}
```

#### recallKnowledge -- HTTP 200 (Empty Results)

```json
{
  "statusCode": 200,
  "headers": { "Content-Type": "application/json" },
  "body": "[]"
}
```

#### recallKnowledge -- HTTP 400 (Missing Query)

```json
{
  "statusCode": 400,
  "headers": { "Content-Type": "application/json" },
  "body": "{\"errors\":{\"query\":\"must be present and non-empty\"}}"
}
```

#### Any Endpoint -- HTTP 500 (Internal Error)

```json
{
  "statusCode": 500,
  "headers": { "Content-Type": "application/json" },
  "body": "{\"message\":\"Internal server error\"}"
}
```

#### Lambda Crash -- HTTP 502 (Gateway Returns This, Not Lambda)

The client sees:
```
HTTP/1.1 502 Bad Gateway
Content-Type: application/json

{"message": "Internal server error"}
```

This happens when Lambda throws an unhandled exception OR returns a malformed response. The handler wrapper pattern (above) prevents this by catching all errors and returning a well-formed 500 instead.

### Known Gotchas

1. **`body` must be a string.** The single most common mistake. `body: { key: "value" }` causes 502. Always use `body: JSON.stringify({ key: "value" })`. The response helper module eliminates this by calling `JSON.stringify()` internally.

2. **`statusCode` must be a number.** `statusCode: "200"` (string) causes 502. The TypeScript type catches this at compile time.

3. **`null` body causes 502.** If you need an empty body, use `body: ""` or `body: JSON.stringify(null)`. The response helper module always calls `JSON.stringify()`, which handles `null` -> `"null"`.

4. **`event.body` may be `null`.** API Gateway sets `event.body` to `null` (not `""`) when the request has no body. Always use `JSON.parse(event.body ?? '{}')` or guard with a null check.

5. **`event.body` is a string, not an object.** API Gateway passes the raw request body as a JSON string in `event.body`. You must `JSON.parse()` it. If the client sends malformed JSON, `JSON.parse()` throws -- catch it and return 400.

6. **`JSON.parse()` on non-JSON body throws.** If the client sends a request with `Content-Type: text/plain` or malformed JSON, the handler's `JSON.parse(event.body)` will throw. The try/catch wrapper handles this gracefully.

7. **502 vs 500 distinction matters for debugging.** 502 means API Gateway got a malformed response from Lambda (infrastructure problem). 500 means Lambda deliberately returned an error (application problem). Always wrap handlers in try/catch to ensure 500, not 502.

8. **Headers are case-insensitive in HTTP but case-sensitive in the response object.** API Gateway preserves the casing you provide. Use consistent casing (`Content-Type`, not `content-type`) for readability.

9. **Empty `headers` object is fine.** `headers: {}` works. Omitting `headers` entirely also works. But including `Content-Type: application/json` is recommended for all JSON responses -- it helps debugging tools and client libraries parse the response correctly.

10. **`isBase64Encoded` defaults to `false`.** Omitting it is equivalent to `isBase64Encoded: false`. Only set it to `true` when returning binary content with a Binary Media Type configured in API Gateway. For this project (JSON only), never set it.

### npm Type Dependency

The helper module uses `import type { APIGatewayProxyResult } from 'aws-lambda'`. This requires `@types/aws-lambda` as a dev dependency:

```bash
npm install -D @types/aws-lambda
```

Since this is a `import type` (type-only import), it is erased at compile time and does not add to the Lambda deployment package. The esbuild bundler (`NodejsFunction`) also tree-shakes type-only imports.

**Decision:**

1. **Use the `APIGatewayProxyResult` type from `@types/aws-lambda`.** This is the canonical TypeScript type for REST API v1 proxy integration responses. Install as a dev dependency only.

2. **Both `statusCode` (number) and `body` (string) are required.** Omitting either causes API Gateway to return 502 Bad Gateway. The `headers`, `multiValueHeaders`, and `isBase64Encoded` fields are all optional.

3. **Always set `Content-Type: application/json`.** While technically optional, it is necessary for client libraries and debugging tools to correctly parse the response. Use a shared `JSON_HEADERS` constant.

4. **Never set `isBase64Encoded`.** All responses are JSON text. Omitting the field defaults to `false`.

5. **Implement four helper functions in `lambda/shared/response.ts`:**
   - `success(statusCode, body)` -- for 200/202 responses
   - `error(statusCode, body)` -- for arbitrary error responses
   - `validationError(errors)` -- convenience for HTTP 400 with `{ errors: { field: reason } }` format
   - `internalError()` -- convenience for HTTP 500 with generic message

6. **Wrap every handler in a top-level try/catch** that returns `internalError()`. This guarantees Lambda never returns a malformed response (which would cause 502) and preserves the 500 vs 502 distinction for debugging.

7. **Guard `event.body` parsing.** Use `JSON.parse(event.body ?? '{}')` and catch `SyntaxError` to return 400 for malformed request bodies.

8. **No CORS headers.** The spec says CORS is not enabled. Only `Content-Type` is set.

9. **`multiValueHeaders` not used.** No response in this project requires multi-value headers.
