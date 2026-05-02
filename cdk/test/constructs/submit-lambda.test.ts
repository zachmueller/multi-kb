import * as cdk from "aws-cdk-lib";
import * as sqs from "aws-cdk-lib/aws-sqs";
import { Match, Template } from "aws-cdk-lib/assertions";
import { SubmitLambda } from "../../lib/constructs/submit-lambda";

function createTemplate(): Template {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  const queue = new sqs.Queue(stack, "Queue");
  new SubmitLambda(stack, "SubmitLambda", { queue });
  return Template.fromStack(stack);
}

describe("SubmitLambda Construct", () => {
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

  test("Lambda has 256 MB memory", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      MemorySize: 256,
    });
  });

  test("Lambda has 10 second timeout", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Timeout: 10,
    });
  });

  test("Lambda has SQS_QUEUE_URL environment variable", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Environment: {
        Variables: {
          SQS_QUEUE_URL: Match.anyValue(),
        },
      },
    });
  });

  test("IAM policy grants sqs:SendMessage scoped to queue ARN", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "sqs:SendMessage",
            Resource: Match.objectLike({
              "Fn::GetAtt": Match.arrayWith([
                Match.stringLikeRegexp("Queue"),
              ]),
            }),
          }),
        ]),
      }),
    });
  });

  test("IAM policy does not have wildcard resources (beyond managed policies)", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const policies = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::IAM::Policy",
    );
    for (const policy of policies) {
      const statements = (policy as any).Properties.PolicyDocument.Statement;
      for (const stmt of statements) {
        // Only sqs:SendMessage should be there, always scoped
        if (stmt.Action === "sqs:SendMessage" || (Array.isArray(stmt.Action) && stmt.Action.includes("sqs:SendMessage"))) {
          expect(stmt.Resource).not.toBe("*");
        }
      }
    }
  });
});
