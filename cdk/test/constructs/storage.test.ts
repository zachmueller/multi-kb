import * as cdk from "aws-cdk-lib";
import { Match, Template } from "aws-cdk-lib/assertions";
import { Storage } from "../../lib/constructs/storage";

function createTemplate(
  overrides?: Partial<{ bucketPrefix: string; repoName: string }>,
): Template {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  new Storage(stack, "Storage", {
    bucketPrefix: overrides?.bucketPrefix ?? "multi-kb",
    repoName: overrides?.repoName ?? "multi-kb",
  });
  return Template.fromStack(stack);
}

describe("Storage Construct", () => {
  test("creates S3 bucket with S3_MANAGED encryption", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::S3::Bucket", {
      BucketEncryption: {
        ServerSideEncryptionConfiguration: [
          {
            ServerSideEncryptionByDefault: {
              SSEAlgorithm: "AES256",
            },
          },
        ],
      },
    });
  });

  test("S3 bucket has BlockPublicAccess.BLOCK_ALL", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::S3::Bucket", {
      PublicAccessBlockConfiguration: {
        BlockPublicAcls: true,
        BlockPublicPolicy: true,
        IgnorePublicAcls: true,
        RestrictPublicBuckets: true,
      },
    });
  });

  test("S3 bucket has RETAIN removal policy", () => {
    const template = createTemplate();
    const buckets = template.findResources("AWS::S3::Bucket");
    const bucketKeys = Object.keys(buckets);
    expect(bucketKeys.length).toBe(1);
    expect(buckets[bucketKeys[0]].DeletionPolicy).toBe("Retain");
  });

  test("S3 bucket name includes account and region", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::S3::Bucket", {
      BucketName: "multi-kb-123456789012-us-east-1",
    });
  });

  test("creates CodeCommit repository", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::CodeCommit::Repository", {
      RepositoryName: "multi-kb",
    });
  });

  test("creates main SQS queue with 300s visibility timeout", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::SQS::Queue", {
      VisibilityTimeout: 300,
    });
  });

  test("creates DLQ with 14-day retention", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::SQS::Queue", {
      MessageRetentionPeriod: 1209600, // 14 days in seconds
    });
  });

  test("main queue has DLQ configured with maxReceiveCount=3", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::SQS::Queue", {
      RedrivePolicy: Match.objectLike({
        maxReceiveCount: 3,
      }),
    });
  });

  test("creates exactly 2 queues (main + DLQ)", () => {
    const template = createTemplate();
    template.resourceCountIs("AWS::SQS::Queue", 2);
  });

  test("creates BucketName CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasBucketOutput = Object.keys(outputs).some((k) =>
      k.includes("BucketNameOutput"),
    );
    expect(hasBucketOutput).toBe(true);
  });

  test("creates RepoCloneUrl CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasRepoOutput = Object.keys(outputs).some((k) =>
      k.includes("RepoCloneUrlOutput"),
    );
    expect(hasRepoOutput).toBe(true);
  });
});
