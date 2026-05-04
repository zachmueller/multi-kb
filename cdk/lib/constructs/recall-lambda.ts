import * as cdk from "aws-cdk-lib";
import * as iam from "aws-cdk-lib/aws-iam";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as s3 from "aws-cdk-lib/aws-s3";
import { NodejsFunction } from "aws-cdk-lib/aws-lambda-nodejs";
import { Construct } from "constructs";
import * as path from "path";

export interface RecallLambdaProps {
  readonly knowledgeBaseId: string;
  readonly knowledgeBaseArn: string;
  readonly bucket: s3.Bucket;
  readonly coverageModelId: string;
  readonly coverageScoreThreshold: number;
  readonly excludePendingFromRecall: boolean;
}

function bedrockModelArn(region: string, account: string, modelId: string): string {
  if (/^[a-z]{2}\./.test(modelId)) {
    return `arn:aws:bedrock:${region}:${account}:inference-profile/${modelId}`;
  }
  return `arn:aws:bedrock:${region}::foundation-model/${modelId}`;
}

export class RecallLambda extends Construct {
  readonly fn: lambda.Function;

  constructor(scope: Construct, id: string, props: RecallLambdaProps) {
    super(scope, id);

    const stack = cdk.Stack.of(this);
    const coverageModelArn = bedrockModelArn(stack.region, stack.account, props.coverageModelId);

    this.fn = new NodejsFunction(this, "Function", {
      runtime: lambda.Runtime.NODEJS_22_X,
      architecture: lambda.Architecture.ARM_64,
      entry: path.join(__dirname, "../../lambda/recall/index.ts"),
      handler: "handler",
      memorySize: 1024,
      timeout: cdk.Duration.seconds(30),
      // NOT VPC-attached — calls public Bedrock endpoints
      environment: {
        KNOWLEDGE_BASE_ID: props.knowledgeBaseId,
        BUCKET_NAME: props.bucket.bucketName,
        COVERAGE_MODEL_ID: props.coverageModelId,
        COVERAGE_SCORE_THRESHOLD: props.coverageScoreThreshold.toString(),
        EXCLUDE_PENDING: props.excludePendingFromRecall.toString(),
      },
      bundling: {
        minify: true,
        sourceMap: false,
        externalModules: [],
      },
    });

    // Least-privilege IAM permissions
    this.fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ["bedrock:Retrieve"],
        resources: [props.knowledgeBaseArn],
      }),
    );

    const coverageResources = [coverageModelArn];
    const coverageProfilePrefix = props.coverageModelId.match(/^([a-z]{2})\./);
    if (coverageProfilePrefix) {
      const baseModelId = props.coverageModelId.slice(coverageProfilePrefix[0].length);
      coverageResources.push(
        `arn:aws:bedrock:*::foundation-model/${baseModelId}`,
      );
    }
    this.fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ["bedrock:InvokeModel"],
        resources: coverageResources,
      }),
    );

    this.fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ["s3:PutObject"],
        resources: [`${props.bucket.bucketArn}/recall-logs/*`],
      }),
    );
  }
}
