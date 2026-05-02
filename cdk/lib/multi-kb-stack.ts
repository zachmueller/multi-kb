import * as cdk from "aws-cdk-lib";
import { Construct } from "constructs";

export interface MultiKbStackProps extends cdk.StackProps {
  readonly repoName: string;
  readonly bucketPrefix: string;
  readonly ec2InstanceType: string;
  readonly embeddingModelId: string;
  readonly consolidationModelId: string;
  readonly coverageModelId: string;
  readonly tickInterval: string;
  readonly dreamCycleInterval: string;
  readonly excludePendingFromRecall: boolean;
  readonly coverageScoreThreshold: number;
  readonly cliBinaryS3Uri: string;
  readonly vpcId?: string;
}

export function resolveProps(app: cdk.App): MultiKbStackProps {
  const cliBinaryS3Uri = app.node.tryGetContext("cliBinaryS3Uri");
  if (!cliBinaryS3Uri) {
    throw new Error(
      "Required context variable 'cliBinaryS3Uri' is missing. " +
        "Provide it via cdk.json or --context cliBinaryS3Uri=s3://...",
    );
  }

  return {
    repoName: app.node.tryGetContext("repoName") ?? "multi-kb",
    bucketPrefix: app.node.tryGetContext("bucketPrefix") ?? "multi-kb",
    ec2InstanceType:
      app.node.tryGetContext("ec2InstanceType") ?? "t4g.micro",
    embeddingModelId:
      app.node.tryGetContext("embeddingModelId") ??
      "amazon.titan-embed-text-v2:0",
    consolidationModelId:
      app.node.tryGetContext("consolidationModelId") ??
      "anthropic.claude-sonnet-4-20250514",
    coverageModelId:
      app.node.tryGetContext("coverageModelId") ??
      "anthropic.claude-haiku-4-5-20251001",
    tickInterval: app.node.tryGetContext("tickInterval") ?? "5m",
    dreamCycleInterval:
      app.node.tryGetContext("dreamCycleInterval") ?? "3h",
    excludePendingFromRecall:
      app.node.tryGetContext("excludePendingFromRecall") !== "false",
    coverageScoreThreshold: parseFloat(
      app.node.tryGetContext("coverageScoreThreshold") ?? "0.3",
    ),
    cliBinaryS3Uri,
    vpcId: app.node.tryGetContext("vpcId"),
    env: {
      account: process.env.CDK_DEFAULT_ACCOUNT,
      region: process.env.CDK_DEFAULT_REGION,
    },
  };
}

export class MultiKbStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props: MultiKbStackProps) {
    super(scope, id, props);

    // Construct instantiation is handled in future waves when all
    // constructs are wired together (WIR-001). Individual constructs
    // are developed and tested independently.
    //
    // Stack outputs are added by each construct or in this file
    // once constructs are wired.
  }
}
