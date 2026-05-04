import * as cdk from "aws-cdk-lib";
import { Match, Template } from "aws-cdk-lib/assertions";
import {
  MultiKbStack,
  MultiKbStackProps,
  resolveProps,
} from "../lib/multi-kb-stack";

function defaultProps(
  overrides?: Partial<MultiKbStackProps>,
): MultiKbStackProps {
  return {
    repoName: "multi-kb",
    bucketPrefix: "multi-kb",
    ec2InstanceType: "t4g.micro",
    embeddingModelId: "amazon.titan-embed-text-v2:0",
    consolidationModelId: "us.anthropic.claude-sonnet-4-6",
    coverageModelId: "us.anthropic.claude-haiku-4-5-20251001-v1:0",
    tickInterval: "5m",
    dreamCycleInterval: "3h",
    excludePendingFromRecall: true,
    coverageScoreThreshold: 0.3,
    cliBinaryS3Uri: "s3://test-bucket/multi-kb/cli-linux-arm64",
    env: { account: "123456789012", region: "us-east-1" },
    ...overrides,
  };
}

function createTemplate(
  overrides?: Partial<MultiKbStackProps>,
): Template {
  const app = new cdk.App();
  const stack = new MultiKbStack(app, "TestStack", defaultProps(overrides));
  return Template.fromStack(stack);
}

describe("resolveProps", () => {
  test("applies defaults when only cliBinaryS3Uri is set", () => {
    const app = new cdk.App({
      context: { cliBinaryS3Uri: "s3://bucket/key" },
    });
    const props = resolveProps(app);

    expect(props.repoName).toBe("multi-kb");
    expect(props.bucketPrefix).toBe("multi-kb");
    expect(props.ec2InstanceType).toBe("t4g.micro");
    expect(props.embeddingModelId).toBe("amazon.titan-embed-text-v2:0");
    expect(props.tickInterval).toBe("5m");
    expect(props.dreamCycleInterval).toBe("3h");
    expect(props.excludePendingFromRecall).toBe(true);
    expect(props.coverageScoreThreshold).toBe(0.3);
    expect(props.cliBinaryS3Uri).toBe("s3://bucket/key");
    expect(props.vpcId).toBeUndefined();
  });

  test("throws when cliBinaryS3Uri is missing", () => {
    const app = new cdk.App({ context: {} });
    expect(() => resolveProps(app)).toThrow("cliBinaryS3Uri");
  });

  test("accepts context overrides", () => {
    const app = new cdk.App({
      context: {
        cliBinaryS3Uri: "s3://bucket/key",
        repoName: "my-kb",
        ec2InstanceType: "t4g.small",
        excludePendingFromRecall: "false",
        coverageScoreThreshold: "0.5",
        vpcId: "vpc-12345",
      },
    });
    const props = resolveProps(app);

    expect(props.repoName).toBe("my-kb");
    expect(props.ec2InstanceType).toBe("t4g.small");
    expect(props.excludePendingFromRecall).toBe(false);
    expect(props.coverageScoreThreshold).toBe(0.5);
    expect(props.vpcId).toBe("vpc-12345");
  });
});

describe("MultiKbStack - synthesis", () => {
  test("synthesizes without errors", () => {
    const template = createTemplate();
    expect(template.toJSON()).toBeDefined();
  });

  test("snapshot", () => {
    const template = createTemplate();
    expect(template.toJSON()).toMatchSnapshot();
  });
});

describe("MultiKbStack - cross-construct wiring", () => {
  test("submitKnowledge Lambda env references actual SQS queue URL", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Handler: "index.handler",
      MemorySize: 256,
      Timeout: 10,
      Environment: {
        Variables: {
          SQS_QUEUE_URL: Match.objectLike({
            Ref: Match.stringLikeRegexp("StorageQueue"),
          }),
        },
      },
    });
  });

  test("recallKnowledge Lambda env references actual KB ID", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const lambdas = Object.entries(json.Resources).filter(
      ([_, r]: [string, any]) =>
        r.Type === "AWS::Lambda::Function" &&
        r.Properties?.MemorySize === 1024 &&
        r.Properties?.Timeout === 30,
    );
    expect(lambdas.length).toBe(1);
    const [, recallLambda] = lambdas[0];
    const kbId = (recallLambda as any).Properties.Environment.Variables
      .KNOWLEDGE_BASE_ID;
    expect(kbId).toBeDefined();
    expect(JSON.stringify(kbId)).toContain("KnowledgeBase");
  });

  test("EC2 IAM role has SQS permissions on actual queue ARN", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const policies = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::IAM::Policy",
    );
    const ec2Policy = policies.find((p: any) =>
      JSON.stringify(p).includes("sqs:ReceiveMessage"),
    ) as any;
    expect(ec2Policy).toBeDefined();
    const sqsStatement = ec2Policy.Properties.PolicyDocument.Statement.find(
      (s: any) =>
        Array.isArray(s.Action)
          ? s.Action.includes("sqs:ReceiveMessage")
          : s.Action === "sqs:ReceiveMessage",
    );
    expect(sqsStatement).toBeDefined();
    expect(JSON.stringify(sqsStatement.Resource)).toContain("StorageQueue");
  });

  test("EC2 IAM role has S3 permissions on actual bucket ARN", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const policies = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::IAM::Policy",
    );
    const ec2Policy = policies.find((p: any) =>
      JSON.stringify(p).includes("s3:PutObject"),
    ) as any;
    expect(ec2Policy).toBeDefined();
    const s3Statement = ec2Policy.Properties.PolicyDocument.Statement.find(
      (s: any) =>
        Array.isArray(s.Action)
          ? s.Action.includes("s3:PutObject")
          : s.Action === "s3:PutObject",
    );
    expect(s3Statement).toBeDefined();
    expect(JSON.stringify(s3Statement.Resource)).toContain("StorageBucket");
  });

  test("API Gateway methods reference actual Lambda functions", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const methods = Object.entries(json.Resources).filter(
      ([_, r]: [string, any]) =>
        r.Type === "AWS::ApiGateway::Method" &&
        r.Properties?.HttpMethod === "POST",
    );
    expect(methods.length).toBe(2);
    for (const [, method] of methods) {
      const props = (method as any).Properties;
      expect(props.AuthorizationType).toBe("AWS_IAM");
      expect(props.Integration.Type).toBe("AWS_PROXY");
      const uriStr = JSON.stringify(props.Integration.Uri);
      expect(uriStr).toMatch(/Lambda/);
    }
  });

  test("VPC endpoints are in the same subnet as ASG", () => {
    const template = createTemplate();
    const json = template.toJSON();

    // Find the subnet ID used by the ASG (via launch template)
    const asg = Object.values(json.Resources).find(
      (r: any) => r.Type === "AWS::AutoScaling::AutoScalingGroup",
    ) as any;
    expect(asg).toBeDefined();
    const asgSubnets = asg.Properties.VPCZoneIdentifier;
    expect(asgSubnets).toBeDefined();

    // Find interface VPC endpoints and check they reference the same subnet
    const endpoints = Object.entries(json.Resources).filter(
      ([_, r]: [string, any]) =>
        r.Type === "AWS::EC2::VPCEndpoint" &&
        r.Properties?.VpcEndpointType !== "Gateway",
    );
    expect(endpoints.length).toBeGreaterThanOrEqual(8);
    for (const [, ep] of endpoints) {
      const epSubnets = (ep as any).Properties.SubnetIds;
      expect(epSubnets).toEqual(asgSubnets);
    }
  });
});

describe("MultiKbStack - resource counts", () => {
  test("has expected resource types", () => {
    const template = createTemplate();
    template.resourceCountIs("AWS::S3::Bucket", 1);
    template.resourceCountIs("AWS::SQS::Queue", 2); // main + DLQ
    template.resourceCountIs("AWS::CodeCommit::Repository", 1);
    template.resourceCountIs("AWS::OpenSearchServerless::Collection", 1);
    template.resourceCountIs("AWS::Bedrock::KnowledgeBase", 1);
    template.resourceCountIs("AWS::Bedrock::DataSource", 1);
    template.resourceCountIs("AWS::ApiGateway::RestApi", 1);
    template.resourceCountIs("AWS::ApiGateway::Method", 2);
    template.resourceCountIs("AWS::AutoScaling::AutoScalingGroup", 1);
    template.resourceCountIs("AWS::EC2::LaunchTemplate", 1);
    template.resourceCountIs("AWS::CloudWatch::Alarm", 3);
  });
});
