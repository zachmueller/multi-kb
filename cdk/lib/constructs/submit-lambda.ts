import * as cdk from "aws-cdk-lib";
import * as iam from "aws-cdk-lib/aws-iam";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as sqs from "aws-cdk-lib/aws-sqs";
import { NodejsFunction } from "aws-cdk-lib/aws-lambda-nodejs";
import { Construct } from "constructs";
import * as path from "path";

export interface SubmitLambdaProps {
  readonly queue: sqs.Queue;
}

export class SubmitLambda extends Construct {
  readonly fn: lambda.Function;

  constructor(scope: Construct, id: string, props: SubmitLambdaProps) {
    super(scope, id);

    this.fn = new NodejsFunction(this, "Function", {
      runtime: lambda.Runtime.NODEJS_22_X,
      architecture: lambda.Architecture.ARM_64,
      entry: path.join(__dirname, "../../lambda/submit/index.ts"),
      handler: "handler",
      memorySize: 256,
      timeout: cdk.Duration.seconds(10),
      environment: {
        SQS_QUEUE_URL: props.queue.queueUrl,
      },
      bundling: {
        minify: true,
        sourceMap: false,
        externalModules: [],
      },
    });

    // Least-privilege: SendMessage scoped to this queue only
    this.fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ["sqs:SendMessage"],
        resources: [props.queue.queueArn],
      }),
    );
  }
}
