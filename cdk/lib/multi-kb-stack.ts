import * as cdk from "aws-cdk-lib";
import { Construct } from "constructs";
import { Networking } from "./constructs/networking";
import { Storage } from "./constructs/storage";
import { Search } from "./constructs/search";
import { KnowledgeBase } from "./constructs/knowledge-base";
import { SubmitLambda } from "./constructs/submit-lambda";
import { RecallLambda } from "./constructs/recall-lambda";
import { Api } from "./constructs/api";
import { Compute } from "./constructs/compute";
import { Observability } from "./constructs/observability";

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
      "us.anthropic.claude-sonnet-4-6",
    coverageModelId:
      app.node.tryGetContext("coverageModelId") ??
      "us.anthropic.claude-haiku-4-5-20251001-v1:0",
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

    const collectionName = props.repoName;

    // --- Phase 1: Networking ---
    const networking = new Networking(this, "Networking", {
      vpcId: props.vpcId,
      collectionName,
    });

    // --- Phase 2: Storage ---
    const storage = new Storage(this, "Storage", {
      bucketPrefix: props.bucketPrefix,
      repoName: props.repoName,
    });

    // --- Phase 3: Search Infrastructure ---
    const search = new Search(this, "Search", {
      collectionName,
      aossVpcEndpointId: networking.aossVpcEndpointId,
      vpc: networking.vpc,
      subnet: networking.subnet,
      endpointSecurityGroup: networking.endpointSecurityGroup,
      ec2SecurityGroup: networking.ec2SecurityGroup,
    });

    // --- Phase 3: Knowledge Base ---
    const kb = new KnowledgeBase(this, "KnowledgeBase", {
      collectionArn: search.collectionArn,
      collectionName,
      vectorIndexName: search.indexName,
      bucketArn: storage.bucketArn,
      embeddingModelId: props.embeddingModelId,
    });

    // KB creation depends on the vector index existing — wired below via data access policy chain

    // --- Phase 4: Lambda Functions ---
    const submitLambda = new SubmitLambda(this, "SubmitLambda", {
      queue: storage.queue,
    });

    const recallLambda = new RecallLambda(this, "RecallLambda", {
      knowledgeBaseId: kb.knowledgeBaseId,
      knowledgeBaseArn: kb.knowledgeBaseArn,
      bucket: storage.bucket,
      coverageModelId: props.coverageModelId,
      coverageScoreThreshold: props.coverageScoreThreshold,
      excludePendingFromRecall: props.excludePendingFromRecall,
    });

    // --- Phase 7: Observability (before Compute, since Compute needs ec2LogGroupName) ---
    const observability = new Observability(this, "Observability", {
      dlq: storage.dlq,
      submitLambdaFunctionName: submitLambda.fn.functionName,
      recallLambdaFunctionName: recallLambda.fn.functionName,
    });

    // --- Phase 5: API Gateway ---
    new Api(this, "Api", {
      submitLambda: submitLambda.fn,
      recallLambda: recallLambda.fn,
    });

    // --- Phase 6: Compute ---
    const compute = new Compute(this, "Compute", {
      vpc: networking.vpc,
      subnet: networking.subnet,
      availabilityZone: networking.availabilityZone,
      ec2SecurityGroup: networking.ec2SecurityGroup,
      ec2InstanceType: props.ec2InstanceType,
      cliBinaryS3Uri: props.cliBinaryS3Uri,
      consolidationModelId: props.consolidationModelId,
      tickInterval: props.tickInterval,
      dreamCycleInterval: props.dreamCycleInterval,
      repoName: props.repoName,
      queueArn: storage.queueArn,
      queueUrl: storage.queueUrl,
      bucketArn: storage.bucketArn,
      bucketName: storage.bucketName,
      repoArn: storage.repoArn,
      collectionArn: search.collectionArn,
      collectionEndpoint: search.collectionEndpoint,
      knowledgeBaseId: kb.knowledgeBaseId,
      knowledgeBaseArn: kb.knowledgeBaseArn,
      dataSourceId: kb.dataSourceId,
      ec2LogGroupName: observability.ec2LogGroupName,
    });

    // Wire ASG name back to observability for the ASG alarm
    // (Observability was created before Compute, so we add the alarm post-hoc)
    new cdk.aws_cloudwatch.Alarm(this, "AsgUnhealthyAlarm", {
      alarmName: "multi-kb-ec2-unhealthy",
      alarmDescription:
        "No EC2 instances in service - server mode is down",
      metric: new cdk.aws_cloudwatch.Metric({
        namespace: "AWS/AutoScaling",
        metricName: "GroupInServiceInstances",
        dimensionsMap: {
          AutoScalingGroupName: compute.asg.autoScalingGroupName,
        },
        period: cdk.Duration.minutes(5),
        statistic: "Minimum",
      }),
      threshold: 1,
      comparisonOperator:
        cdk.aws_cloudwatch.ComparisonOperator.LESS_THAN_THRESHOLD,
      evaluationPeriods: 1,
      treatMissingData: cdk.aws_cloudwatch.TreatMissingData.BREACHING,
    });

    // --- SRC-004: Data Access Policy ---
    // Created at stack scope (not inside Search) to avoid cyclic dependencies:
    // Search ← KB depends on index → DataAccessPolicy references KB role → KB
    const dataAccessPolicy = search.createDataAccessPolicy(
      this,
      compute.roleArn,
      kb.serviceRoleArn,
    );

    // Index custom resource needs data access policy before it can create the index
    search.node.findChild("IndexCustomResource").node.addDependency(
      dataAccessPolicy,
    );

    // KB must wait for the index custom resource to complete
    kb.knowledgeBase.node.addDependency(
      search.node.findChild("IndexCustomResource"),
    );

    // Stack outputs are created by each construct:
    // Storage: BucketName, RepoCloneUrl
    // Search: CollectionEndpoint
    // KnowledgeBase: KnowledgeBaseId, DataSourceId
    // Api: ApiEndpoint, ApiId
    // Compute: AsgName
  }
}
