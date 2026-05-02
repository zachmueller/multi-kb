import * as cdk from "aws-cdk-lib";
import * as lambda from "aws-cdk-lib/aws-lambda";
import { Template } from "aws-cdk-lib/assertions";
import { Api } from "../../lib/constructs/api";

function createTemplate(): Template {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  const submitFn = new lambda.Function(stack, "SubmitFn", {
    runtime: lambda.Runtime.NODEJS_22_X,
    handler: "index.handler",
    code: lambda.Code.fromInline("exports.handler = async () => {}"),
  });
  const recallFn = new lambda.Function(stack, "RecallFn", {
    runtime: lambda.Runtime.NODEJS_22_X,
    handler: "index.handler",
    code: lambda.Code.fromInline("exports.handler = async () => {}"),
  });
  new Api(stack, "Api", {
    submitLambda: submitFn,
    recallLambda: recallFn,
  });
  return Template.fromStack(stack);
}

describe("Api Construct", () => {
  test("creates REST API with name multi-kb", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::ApiGateway::RestApi", {
      Name: "multi-kb",
    });
  });

  test("creates exactly 2 POST methods", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const postMethods = Object.values(json.Resources).filter(
      (r: any) =>
        r.Type === "AWS::ApiGateway::Method" &&
        r.Properties?.HttpMethod === "POST",
    );
    expect(postMethods.length).toBe(2);
  });

  test("all POST methods require AWS_IAM auth", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const postMethods = Object.values(json.Resources).filter(
      (r: any) =>
        r.Type === "AWS::ApiGateway::Method" &&
        r.Properties?.HttpMethod === "POST",
    );
    for (const method of postMethods) {
      expect((method as any).Properties.AuthorizationType).toBe("AWS_IAM");
    }
  });

  test("all POST methods use AWS_PROXY integration", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const postMethods = Object.values(json.Resources).filter(
      (r: any) =>
        r.Type === "AWS::ApiGateway::Method" &&
        r.Properties?.HttpMethod === "POST",
    );
    for (const method of postMethods) {
      expect((method as any).Properties.Integration.Type).toBe("AWS_PROXY");
    }
  });

  test("creates submitKnowledge resource", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::ApiGateway::Resource", {
      PathPart: "submitKnowledge",
    });
  });

  test("creates recallKnowledge resource", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::ApiGateway::Resource", {
      PathPart: "recallKnowledge",
    });
  });

  test("deployment stage is prod", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::ApiGateway::Stage", {
      StageName: "prod",
    });
  });

  test("creates access log group for API Gateway", () => {
    const template = createTemplate();
    template.resourceCountIs("AWS::Logs::LogGroup", 1);
  });

  test("creates ApiEndpoint CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasOutput = Object.keys(outputs).some((k) =>
      k.includes("ApiEndpoint"),
    );
    expect(hasOutput).toBe(true);
  });

  test("creates ApiId CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasOutput = Object.keys(outputs).some((k) =>
      k.includes("ApiId"),
    );
    expect(hasOutput).toBe(true);
  });
});
