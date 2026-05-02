import * as cdk from "aws-cdk-lib";
import * as s3 from "aws-cdk-lib/aws-s3";
import { Match, Template } from "aws-cdk-lib/assertions";
import { RecallLambda } from "../../lib/constructs/recall-lambda";

function createTemplate(): Template {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  const bucket = new s3.Bucket(stack, "Bucket");
  new RecallLambda(stack, "RecallLambda", {
    knowledgeBaseId: "KB-12345",
    knowledgeBaseArn:
      "arn:aws:bedrock:us-east-1:123456789012:knowledge-base/KB-12345",
    bucket,
    coverageModelId: "anthropic.claude-haiku-4-5-20251001",
    coverageScoreThreshold: 0.3,
    excludePendingFromRecall: true,
  });
  return Template.fromStack(stack);
}

describe("RecallLambda Construct", () => {
  test("creates Lambda function with Node.js 22.x", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Runtime: "nodejs22.x",
    });
  });

  test("Lambda uses ARM64 architecture", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Architectures: ["arm64"],
    });
  });

  test("Lambda has 1024 MB memory", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      MemorySize: 1024,
    });
  });

  test("Lambda has 30 second timeout", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Timeout: 30,
    });
  });

  test("Lambda has required environment variables", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Environment: {
        Variables: {
          KNOWLEDGE_BASE_ID: Match.anyValue(),
          BUCKET_NAME: Match.anyValue(),
          COVERAGE_MODEL_ID: "anthropic.claude-haiku-4-5-20251001",
          COVERAGE_SCORE_THRESHOLD: "0.3",
          EXCLUDE_PENDING: "true",
        },
      },
    });
  });

  test("IAM policy grants bedrock:Retrieve scoped to KB ARN", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "bedrock:Retrieve",
            Resource:
              "arn:aws:bedrock:us-east-1:123456789012:knowledge-base/KB-12345",
          }),
        ]),
      }),
    });
  });

  test("IAM policy grants bedrock:InvokeModel scoped to coverage model ARN", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "bedrock:InvokeModel",
            Resource:
              "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-haiku-4-5-20251001",
          }),
        ]),
      }),
    });
  });

  test("IAM policy grants s3:PutObject scoped to recall-logs/* path", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const policies = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::IAM::Policy",
    );
    let found = false;
    for (const policy of policies) {
      const statements = (policy as any).Properties.PolicyDocument.Statement;
      for (const stmt of statements) {
        if (stmt.Action === "s3:PutObject") {
          const resourceStr = JSON.stringify(stmt.Resource);
          expect(resourceStr).toContain("/recall-logs/*");
          found = true;
        }
      }
    }
    expect(found).toBe(true);
  });

  test("IAM policy does not contain wildcard * resources for Bedrock or S3", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const policies = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::IAM::Policy",
    );
    for (const policy of policies) {
      const statements = (policy as any).Properties.PolicyDocument.Statement;
      for (const stmt of statements) {
        const actions = Array.isArray(stmt.Action)
          ? stmt.Action
          : [stmt.Action];
        for (const action of actions) {
          if (
            action.startsWith("bedrock:") ||
            action.startsWith("s3:")
          ) {
            expect(stmt.Resource).not.toBe("*");
          }
        }
      }
    }
  });
});
