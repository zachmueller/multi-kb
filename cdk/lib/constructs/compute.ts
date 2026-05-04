import * as cdk from "aws-cdk-lib";
import * as autoscaling from "aws-cdk-lib/aws-autoscaling";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as iam from "aws-cdk-lib/aws-iam";
import { Construct } from "constructs";

function bedrockModelArn(region: string, account: string, modelId: string): string {
  if (/^[a-z]{2}\./.test(modelId)) {
    return `arn:aws:bedrock:${region}:${account}:inference-profile/${modelId}`;
  }
  return `arn:aws:bedrock:${region}::foundation-model/${modelId}`;
}

export interface ComputeProps {
  readonly vpc: ec2.IVpc;
  readonly subnet: ec2.ISubnet;
  readonly availabilityZone: string;
  readonly ec2SecurityGroup: ec2.SecurityGroup;
  readonly ec2InstanceType: string;
  readonly cliBinaryS3Uri: string;
  readonly consolidationModelId: string;
  readonly tickInterval: string;
  readonly dreamCycleInterval: string;
  readonly repoName: string;
  // Storage
  readonly queueArn: string;
  readonly queueUrl: string;
  readonly bucketArn: string;
  readonly bucketName: string;
  readonly repoArn: string;
  // Search
  readonly collectionArn: string;
  readonly collectionEndpoint: string;
  // Knowledge Base
  readonly knowledgeBaseId: string;
  readonly knowledgeBaseArn: string;
  readonly dataSourceId: string;
  // Observability
  readonly ec2LogGroupName: string;
}

export class Compute extends Construct {
  readonly role: iam.Role;
  readonly roleArn: string;
  readonly asg: autoscaling.AutoScalingGroup;

  constructor(scope: Construct, id: string, props: ComputeProps) {
    super(scope, id);

    const stack = cdk.Stack.of(this);
    const region = stack.region;
    const consolidationModelArn = bedrockModelArn(region, stack.account, props.consolidationModelId);

    // Parse S3 bucket and key from cliBinaryS3Uri for scoped IAM
    // Format: s3://bucket-name/path/to/binary
    const s3UriParts = props.cliBinaryS3Uri.replace("s3://", "").split("/");
    const cliBinaryBucket = s3UriParts[0];
    const cliBinaryKey = s3UriParts.slice(1).join("/");

    // --- CMP-001: EC2 IAM Role ---

    this.role = new iam.Role(this, "Ec2Role", {
      assumedBy: new iam.ServicePrincipal("ec2.amazonaws.com"),
    });

    // SQS: receive + delete + get attributes
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes",
        ],
        resources: [props.queueArn],
      }),
    );

    // CodeCommit: git pull + push
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["codecommit:GitPull", "codecommit:GitPush"],
        resources: [props.repoArn],
      }),
    );

    // S3: KB bucket read/write/delete/list
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
        resources: [`${props.bucketArn}/*`],
      }),
    );
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["s3:ListBucket"],
        resources: [props.bucketArn],
      }),
    );

    // S3: CLI binary download (separate bucket, read-only)
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["s3:GetObject"],
        resources: [
          `arn:aws:s3:::${cliBinaryBucket}/${cliBinaryKey}`,
        ],
      }),
    );

    // OpenSearch Serverless: data plane access
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["aoss:APIAccessAll"],
        resources: [props.collectionArn],
      }),
    );

    // Bedrock: invoke consolidation model
    // Cross-region inference profiles require permission on both the
    // inference-profile ARN and the underlying foundation-model ARN.
    const consolidationResources = [consolidationModelArn];
    const profilePrefix = props.consolidationModelId.match(/^([a-z]{2})\./);
    if (profilePrefix) {
      const baseModelId = props.consolidationModelId.slice(profilePrefix[0].length);
      consolidationResources.push(
        `arn:aws:bedrock:*::foundation-model/${baseModelId}`,
      );
    }
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["bedrock:InvokeModel"],
        resources: consolidationResources,
      }),
    );

    // Bedrock Agent: start/get ingestion jobs + retrieve, scoped to KB
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "bedrock:StartIngestionJob",
          "bedrock:GetIngestionJob",
          "bedrock:Retrieve",
        ],
        resources: [props.knowledgeBaseArn],
      }),
    );

    // SSM Session Manager permissions
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: ["ssm:UpdateInstanceInformation"],
        resources: ["*"],
      }),
    );
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "ssmmessages:CreateControlChannel",
          "ssmmessages:CreateDataChannel",
          "ssmmessages:OpenControlChannel",
          "ssmmessages:OpenDataChannel",
        ],
        resources: ["*"],
      }),
    );
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "ec2messages:AcknowledgeMessage",
          "ec2messages:DeleteMessage",
          "ec2messages:FailMessage",
          "ec2messages:GetEndpoint",
          "ec2messages:GetMessages",
          "ec2messages:SendReply",
        ],
        resources: ["*"],
      }),
    );

    // CloudWatch Logs: allow agent to push logs
    this.role.addToPolicy(
      new iam.PolicyStatement({
        actions: [
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "logs:DescribeLogStreams",
        ],
        resources: [
          `arn:aws:logs:${region}:${stack.account}:log-group:${props.ec2LogGroupName}:*`,
        ],
      }),
    );

    this.roleArn = this.role.roleArn;

    // --- CMP-003: User Data Script ---

    const userData = ec2.UserData.forLinux();
    userData.addCommands("set -euxo pipefail");

    // Step 1: Install packages
    userData.addCommands("dnf install -y amazon-cloudwatch-agent git");

    // Step 2: Download CLI binary with retry (3 attempts, exponential backoff)
    userData.addCommands(
      "for i in 1 2 3; do",
      `  aws s3 cp ${props.cliBinaryS3Uri} /usr/local/bin/multi-kb && chmod +x /usr/local/bin/multi-kb && break`,
      '  echo "CLI binary download attempt $i failed, retrying..."',
      "  sleep $((2 ** (i - 1)))",
      "done",
      "test -x /usr/local/bin/multi-kb",
    );

    // Step 3: Git credential helper (--system scope)
    userData.addCommands(
      "git config --system credential.helper '!aws codecommit credential-helper $@'",
      "git config --system credential.UseHttpPath true",
    );

    // Step 4: Clone CodeCommit repo with retry + empty repo fallback
    userData.addCommands(
      "mkdir -p /opt/multi-kb",
      "for i in 1 2 3; do",
      `  git clone https://git-codecommit.${region}.amazonaws.com/v1/repos/${props.repoName} /opt/multi-kb/repo && break`,
      '  echo "CodeCommit clone attempt $i failed, retrying..."',
      "  sleep $((2 ** (i - 1)))",
      "done",
      'if [ ! -d /opt/multi-kb/repo/.git ]; then',
      "  mkdir -p /opt/multi-kb/repo",
      "  cd /opt/multi-kb/repo",
      "  git init",
      `  git remote add origin https://git-codecommit.${region}.amazonaws.com/v1/repos/${props.repoName}`,
      "fi",
    );

    // Step 5: Template config.yaml per server-config.md contract
    userData.addCommands(
      "cat > /opt/multi-kb/config.yaml << 'CONFIGEOF'",
      "mode: server",
      'author: "multi-kb-server"',
      "",
      "sqs:",
      `  queue_url: "${props.queueUrl}"`,
      "  batch_size: 10",
      "",
      "codecommit:",
      `  repo_name: "${props.repoName}"`,
      `  region: "${region}"`,
      "",
      "s3:",
      `  bucket: "${props.bucketName}"`,
      `  region: "${region}"`,
      "",
      "opensearch:",
      `  endpoint: "${props.collectionEndpoint}"`,
      `  region: "${region}"`,
      "",
      "bedrock_kb:",
      `  knowledge_base_id: "${props.knowledgeBaseId}"`,
      `  data_source_id: "${props.dataSourceId}"`,
      "",
      `tick_interval: "${props.tickInterval}"`,
      "",
      "dream_cycle:",
      `  interval: "${props.dreamCycleInterval}"`,
      `  model_id: "${props.consolidationModelId}"`,
      "",
      "recall_log:",
      '  schedule: "02:00"',
      "CONFIGEOF",
    );

    // Step 6: Configure CloudWatch agent
    // Use Stack.toJsonString() for safe serialization of CDK tokens
    const cwAgentConfig = {
      logs: {
        logs_collected: {
          files: {
            collect_list: [
              {
                file_path: "/var/log/multi-kb/server.log",
                log_group_name: props.ec2LogGroupName,
                log_stream_name: "{instance_id}",
              },
            ],
          },
        },
      },
    };

    userData.addCommands(
      "mkdir -p /opt/aws/amazon-cloudwatch-agent/etc",
      `cat > /opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json << 'CWEOF'`,
      stack.toJsonString(cwAgentConfig),
      "CWEOF",
      "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.json",
    );

    // Step 7: Create systemd unit
    userData.addCommands(
      "mkdir -p /var/log/multi-kb",
      "cat > /etc/systemd/system/multi-kb.service << 'UNITEOF'",
      "[Unit]",
      "Description=Multi-KB Server",
      "After=network-online.target",
      "Wants=network-online.target",
      "",
      "[Service]",
      "Type=simple",
      `ExecStart=/usr/local/bin/multi-kb server --config /opt/multi-kb/config.yaml`,
      "WorkingDirectory=/opt/multi-kb/repo",
      "Restart=on-failure",
      "RestartSec=5",
      "StandardOutput=append:/var/log/multi-kb/server.log",
      "StandardError=append:/var/log/multi-kb/server.log",
      `Environment=AWS_REGION=${region}`,
      "Environment=HOME=/root",
      "",
      "[Install]",
      "WantedBy=multi-user.target",
      "UNITEOF",
    );

    // Step 8: Start services
    userData.addCommands(
      "systemctl daemon-reload",
      "systemctl enable --now multi-kb",
    );

    // --- CMP-002: Launch Template ---

    // Determine architecture from instance type prefix
    const isGraviton =
      props.ec2InstanceType.startsWith("t4g") ||
      props.ec2InstanceType.startsWith("m6g") ||
      props.ec2InstanceType.startsWith("m7g") ||
      props.ec2InstanceType.startsWith("c6g") ||
      props.ec2InstanceType.startsWith("c7g") ||
      props.ec2InstanceType.startsWith("r6g") ||
      props.ec2InstanceType.startsWith("r7g");

    const machineImage = ec2.MachineImage.latestAmazonLinux2023({
      cpuType: isGraviton ? ec2.AmazonLinuxCpuType.ARM_64 : ec2.AmazonLinuxCpuType.X86_64,
    });

    const launchTemplate = new ec2.LaunchTemplate(this, "LaunchTemplate", {
      instanceType: new ec2.InstanceType(props.ec2InstanceType),
      machineImage,
      role: this.role,
      securityGroup: props.ec2SecurityGroup,
      userData,
      requireImdsv2: true,
      associatePublicIpAddress: false,
    });

    // --- CMP-004: Auto Scaling Group ---

    this.asg = new autoscaling.AutoScalingGroup(this, "Asg", {
      vpc: props.vpc,
      vpcSubnets: { subnets: [props.subnet] },
      launchTemplate,
      minCapacity: 1,
      maxCapacity: 1,
      signals: autoscaling.Signals.waitForAll({
        timeout: cdk.Duration.minutes(15),
      }),
    });

    // cfn-signal: user data signals ASG on successful boot
    userData.addSignalOnExitCommand(this.asg);

    // --- CMP-005: Stack Output — Compute ---

    new cdk.CfnOutput(this, "AsgName", {
      value: this.asg.autoScalingGroupName,
      description: "Auto Scaling Group name (manages the single EC2 instance)",
    });
  }
}
