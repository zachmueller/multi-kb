import * as cdk from "aws-cdk-lib";
import * as codecommit from "aws-cdk-lib/aws-codecommit";
import * as s3 from "aws-cdk-lib/aws-s3";
import * as sqs from "aws-cdk-lib/aws-sqs";
import { Construct } from "constructs";

export interface StorageProps {
  readonly bucketPrefix: string;
  readonly repoName: string;
}

export class Storage extends Construct {
  readonly bucket: s3.Bucket;
  readonly bucketName: string;
  readonly bucketArn: string;
  readonly repository: codecommit.Repository;
  readonly repoCloneUrl: string;
  readonly repoArn: string;
  readonly queue: sqs.Queue;
  readonly queueUrl: string;
  readonly queueArn: string;
  readonly dlq: sqs.Queue;
  readonly dlqArn: string;

  constructor(scope: Construct, id: string, props: StorageProps) {
    super(scope, id);

    const stack = cdk.Stack.of(this);

    // S3 bucket — note replication and recall logs
    this.bucket = new s3.Bucket(this, "Bucket", {
      bucketName: `${props.bucketPrefix}-${stack.account}-${stack.region}`,
      encryption: s3.BucketEncryption.S3_MANAGED,
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      versioned: false,
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });
    this.bucketName = this.bucket.bucketName;
    this.bucketArn = this.bucket.bucketArn;

    // CodeCommit repository
    this.repository = new codecommit.Repository(this, "Repository", {
      repositoryName: props.repoName,
    });
    this.repoCloneUrl = this.repository.repositoryCloneUrlHttp;
    this.repoArn = this.repository.repositoryArn;

    // Dead-letter queue
    this.dlq = new sqs.Queue(this, "Dlq", {
      retentionPeriod: cdk.Duration.days(14),
    });
    this.dlqArn = this.dlq.queueArn;

    // Main SQS standard queue with DLQ
    this.queue = new sqs.Queue(this, "Queue", {
      visibilityTimeout: cdk.Duration.seconds(300),
      retentionPeriod: cdk.Duration.days(14),
      deadLetterQueue: {
        queue: this.dlq,
        maxReceiveCount: 3,
      },
    });
    this.queueUrl = this.queue.queueUrl;
    this.queueArn = this.queue.queueArn;

    // CloudFormation outputs — storage
    new cdk.CfnOutput(this, "BucketNameOutput", {
      exportName: "BucketName",
      value: this.bucketName,
    });
    new cdk.CfnOutput(this, "RepoCloneUrlOutput", {
      exportName: "RepoCloneUrl",
      value: this.repoCloneUrl,
    });
  }
}
