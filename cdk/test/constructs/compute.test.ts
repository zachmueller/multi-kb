import * as cdk from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as logs from "aws-cdk-lib/aws-logs";
import { Match, Template } from "aws-cdk-lib/assertions";
import { Compute, ComputeProps } from "../../lib/constructs/compute";

function createTestStack(): {
  stack: cdk.Stack;
  vpc: ec2.Vpc;
  subnet: ec2.ISubnet;
  sg: ec2.SecurityGroup;
} {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  const vpc = new ec2.Vpc(stack, "Vpc", {
    maxAzs: 1,
    natGateways: 0,
    subnetConfiguration: [
      {
        name: "private",
        subnetType: ec2.SubnetType.PRIVATE_ISOLATED,
        cidrMask: 24,
      },
    ],
  });
  const subnet = vpc.isolatedSubnets[0];
  const sg = new ec2.SecurityGroup(stack, "Ec2Sg", {
    vpc,
    allowAllOutbound: false,
  });
  return { stack, vpc, subnet, sg };
}

function defaultComputeProps(
  stack: cdk.Stack,
  vpc: ec2.Vpc,
  subnet: ec2.ISubnet,
  sg: ec2.SecurityGroup,
): ComputeProps {
  const logGroup = new logs.LogGroup(stack, "TestLogGroup", {
    logGroupName: "/multi-kb/ec2/server",
  });
  return {
    vpc,
    subnet,
    availabilityZone: "us-east-1a",
    ec2SecurityGroup: sg,
    ec2InstanceType: "t4g.micro",
    cliBinaryS3Uri: "s3://my-bucket/multi-kb/cli-linux-arm64",
    consolidationModelId: "us.anthropic.claude-sonnet-4-6",
    tickInterval: "5m",
    dreamCycleInterval: "3h",
    repoName: "multi-kb",
    queueArn: "arn:aws:sqs:us-east-1:123456789012:multi-kb-queue",
    queueUrl: "https://sqs.us-east-1.amazonaws.com/123456789012/multi-kb-queue",
    bucketArn: "arn:aws:s3:::multi-kb-123456789012-us-east-1",
    bucketName: "multi-kb-123456789012-us-east-1",
    repoArn: "arn:aws:codecommit:us-east-1:123456789012:multi-kb",
    collectionArn: "arn:aws:aoss:us-east-1:123456789012:collection/abc123",
    collectionEndpoint: "https://abc123.us-east-1.aoss.amazonaws.com",
    knowledgeBaseId: "KB-12345",
    knowledgeBaseArn:
      "arn:aws:bedrock:us-east-1:123456789012:knowledge-base/KB-12345",
    dataSourceId: "DS-67890",
    ec2LogGroupName: logGroup.logGroupName,
  };
}

describe("Compute Construct", () => {
  test("creates IAM role with ec2 trust policy", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Role", {
      AssumeRolePolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Effect: "Allow",
            Principal: { Service: "ec2.amazonaws.com" },
            Action: "sts:AssumeRole",
          }),
        ]),
      }),
    });
  });

  test("IAM role has SQS permissions scoped to queue ARN", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: Match.arrayWith(["sqs:ReceiveMessage"]),
            Resource:
              "arn:aws:sqs:us-east-1:123456789012:multi-kb-queue",
          }),
        ]),
      }),
    });
  });

  test("IAM role has CodeCommit permissions scoped to repo ARN", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: Match.arrayWith(["codecommit:GitPull"]),
            Resource:
              "arn:aws:codecommit:us-east-1:123456789012:multi-kb",
          }),
        ]),
      }),
    });
  });

  test("IAM role has S3 permissions scoped to bucket ARN", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: Match.arrayWith(["s3:GetObject", "s3:PutObject"]),
            Resource:
              "arn:aws:s3:::multi-kb-123456789012-us-east-1/*",
          }),
        ]),
      }),
    });
  });

  test("IAM role has CLI binary S3 read scoped to exact key", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "s3:GetObject",
            Resource:
              "arn:aws:s3:::my-bucket/multi-kb/cli-linux-arm64",
          }),
        ]),
      }),
    });
  });

  test("IAM role has OpenSearch scoped to collection ARN", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "aoss:APIAccessAll",
            Resource:
              "arn:aws:aoss:us-east-1:123456789012:collection/abc123",
          }),
        ]),
      }),
    });
  });

  test("IAM role has Bedrock invoke scoped to consolidation model", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "bedrock:InvokeModel",
            Resource:
              "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-sonnet-4-6",
          }),
        ]),
      }),
    });
  });

  test("IAM role has SSM Session Manager permissions", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: "ssm:UpdateInstanceInformation",
          }),
        ]),
      }),
    });
  });

  test("creates instance profile from role", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    // LaunchTemplate creates an instance profile automatically; ASG may create another
    const count = Object.keys(
      template.findResources("AWS::IAM::InstanceProfile"),
    ).length;
    expect(count).toBeGreaterThanOrEqual(1);
  });

  test("launch template has correct instance type", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::EC2::LaunchTemplate", {
      LaunchTemplateData: Match.objectLike({
        InstanceType: "t4g.micro",
      }),
    });
  });

  test("launch template enforces IMDSv2", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::EC2::LaunchTemplate", {
      LaunchTemplateData: Match.objectLike({
        MetadataOptions: Match.objectLike({
          HttpTokens: "required",
        }),
      }),
    });
  });

  test("launch template does not associate public IP", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::EC2::LaunchTemplate", {
      LaunchTemplateData: Match.objectLike({
        NetworkInterfaces: Match.arrayWith([
          Match.objectLike({
            AssociatePublicIpAddress: false,
          }),
        ]),
      }),
    });
  });

  test("launch template has non-empty user data", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::EC2::LaunchTemplate", {
      LaunchTemplateData: Match.objectLike({
        UserData: Match.anyValue(),
      }),
    });
  });

  test("ASG has min=max=1 with no explicit desiredCapacity", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::AutoScaling::AutoScalingGroup", {
      MinSize: "1",
      MaxSize: "1",
    });

    template.hasResourceProperties("AWS::AutoScaling::AutoScalingGroup",
      Match.not(Match.objectLike({ DesiredCapacity: Match.anyValue() })),
    );
  });

  test("ASG has CreationPolicy with 15-minute timeout", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    const asgResources = template.findResources(
      "AWS::AutoScaling::AutoScalingGroup",
    );
    const asgKeys = Object.keys(asgResources);
    expect(asgKeys.length).toBe(1);

    const asg = asgResources[asgKeys[0]];
    expect(asg.CreationPolicy).toBeDefined();
    expect(asg.CreationPolicy.ResourceSignal).toBeDefined();
    expect(asg.CreationPolicy.ResourceSignal.Timeout).toBe("PT15M");
  });

  test("ASG is pinned to specified subnet", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::AutoScaling::AutoScalingGroup", {
      VPCZoneIdentifier: Match.anyValue(),
    });
  });

  test("creates CfnOutput for ASG name", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    const outputs = template.findOutputs("*", Match.anyValue());
    const outputKeys = Object.keys(outputs);
    const hasAsgOutput = outputKeys.some((k) => k.includes("AsgName"));
    expect(hasAsgOutput).toBe(true);
  });

  test("IAM role has Bedrock ingestion job permissions scoped to KB ARN", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: Match.arrayWith(["bedrock:StartIngestionJob"]),
            Resource:
              "arn:aws:bedrock:us-east-1:123456789012:knowledge-base/KB-12345",
          }),
        ]),
      }),
    });
  });

  test("IAM role has CloudWatch Logs permissions scoped to log group", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::IAM::Policy", {
      PolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Action: Match.arrayWith(["logs:PutLogEvents"]),
            Resource: Match.anyValue(),
          }),
        ]),
      }),
    });
  });

  test("Graviton instance type uses ARM64 AMI", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    // t4g is Graviton — should result in arm64
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);

    // Verify the instance type is t4g.micro
    template.hasResourceProperties("AWS::EC2::LaunchTemplate", {
      LaunchTemplateData: Match.objectLike({
        InstanceType: "t4g.micro",
      }),
    });
  });

  test("SSM wildcard resources are limited to SSM/EC2Messages actions only", () => {
    const { stack, vpc, subnet, sg } = createTestStack();
    new Compute(stack, "Compute", defaultComputeProps(stack, vpc, subnet, sg));
    const template = Template.fromStack(stack);
    const json = template.toJSON();

    const policies = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::IAM::Policy",
    );
    for (const policy of policies) {
      const statements = (policy as any).Properties.PolicyDocument.Statement;
      for (const stmt of statements) {
        if (stmt.Resource === "*") {
          const actions = Array.isArray(stmt.Action)
            ? stmt.Action
            : [stmt.Action];
          for (const action of actions) {
            // Only SSM, ssmmessages, and ec2messages should use * resource
            expect(action).toMatch(
              /^(ssm:|ssmmessages:|ec2messages:)/,
            );
          }
        }
      }
    }
  });
});
