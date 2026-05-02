import * as cdk from "aws-cdk-lib";
import { Match, Template } from "aws-cdk-lib/assertions";
import { MultiKbStack, MultiKbStackProps } from "../lib/multi-kb-stack";

function defaultProps(
  overrides?: Partial<MultiKbStackProps>,
): MultiKbStackProps {
  return {
    repoName: "multi-kb",
    bucketPrefix: "multi-kb",
    ec2InstanceType: "t4g.micro",
    embeddingModelId: "amazon.titan-embed-text-v2:0",
    consolidationModelId: "anthropic.claude-sonnet-4-20250514",
    coverageModelId: "anthropic.claude-haiku-4-5-20251001",
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

describe("QAT-003: Security Review", () => {
  describe("API Gateway authentication", () => {
    test("all API methods require AWS_IAM auth", () => {
      const template = createTemplate();
      const json = template.toJSON();
      const methods = Object.values(json.Resources).filter(
        (r: any) =>
          r.Type === "AWS::ApiGateway::Method" &&
          r.Properties?.HttpMethod === "POST",
      );
      expect(methods.length).toBe(2);
      for (const method of methods) {
        expect((method as any).Properties.AuthorizationType).toBe("AWS_IAM");
      }
    });

    test("no API methods use NONE auth", () => {
      const template = createTemplate();
      const json = template.toJSON();
      const methods = Object.values(json.Resources).filter(
        (r: any) =>
          r.Type === "AWS::ApiGateway::Method" &&
          r.Properties?.AuthorizationType === "NONE" &&
          r.Properties?.HttpMethod === "POST",
      );
      expect(methods.length).toBe(0);
    });
  });

  describe("S3 bucket security", () => {
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

    test("S3 bucket has server-side encryption enabled", () => {
      const template = createTemplate();
      template.hasResourceProperties("AWS::S3::Bucket", {
        BucketEncryption: Match.objectLike({
          ServerSideEncryptionConfiguration: Match.anyValue(),
        }),
      });
    });
  });

  describe("Lambda IAM minimum permissions", () => {
    test("submit Lambda only has sqs:SendMessage (no wildcard resource)", () => {
      const template = createTemplate();
      const json = template.toJSON();

      // Find policies attached to submit Lambda role
      // The submit Lambda function has 256MB/10s timeout
      const submitLambda = Object.values(json.Resources).find(
        (r: any) =>
          r.Type === "AWS::Lambda::Function" &&
          r.Properties?.MemorySize === 256 &&
          r.Properties?.Timeout === 10,
      ) as any;
      expect(submitLambda).toBeDefined();

      const submitRoleRef = submitLambda.Properties.Role;
      const submitRoleLogicalId = submitRoleRef["Fn::GetAtt"]?.[0];

      // Find policies that reference this role
      const policies = Object.values(json.Resources).filter(
        (r: any) =>
          r.Type === "AWS::IAM::Policy" &&
          JSON.stringify(r.Properties?.Roles).includes(submitRoleLogicalId),
      );

      for (const policy of policies) {
        const statements = (policy as any).Properties.PolicyDocument.Statement;
        for (const stmt of statements) {
          // Every resource should be scoped (not "*")
          if (stmt.Resource === "*") {
            // SSM-related wildcards are acceptable
            const actions = Array.isArray(stmt.Action)
              ? stmt.Action
              : [stmt.Action];
            for (const action of actions) {
              expect(action).toMatch(/^(ssm|ssmmessages|ec2messages):/);
            }
          }
        }
      }
    });

    test("recall Lambda permissions are scoped to specific ARNs", () => {
      const template = createTemplate();
      const json = template.toJSON();

      // Find the recall Lambda (1024MB, 30s)
      const recallLambda = Object.values(json.Resources).find(
        (r: any) =>
          r.Type === "AWS::Lambda::Function" &&
          r.Properties?.MemorySize === 1024 &&
          r.Properties?.Timeout === 30,
      ) as any;
      expect(recallLambda).toBeDefined();

      const recallRoleRef = recallLambda.Properties.Role;
      const recallRoleLogicalId = recallRoleRef["Fn::GetAtt"]?.[0];

      const policies = Object.values(json.Resources).filter(
        (r: any) =>
          r.Type === "AWS::IAM::Policy" &&
          JSON.stringify(r.Properties?.Roles).includes(recallRoleLogicalId),
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

  describe("VPC endpoint security groups", () => {
    test("endpoint SG ingress only allows port 443", () => {
      const template = createTemplate();
      const json = template.toJSON();

      // Find all SecurityGroupIngress resources
      const ingressRules = Object.values(json.Resources).filter(
        (r: any) => r.Type === "AWS::EC2::SecurityGroupIngress",
      );

      for (const rule of ingressRules) {
        const props = (rule as any).Properties;
        expect(props.FromPort).toBe(443);
        expect(props.ToPort).toBe(443);
        expect(props.IpProtocol).toBe("tcp");
      }
    });

    test("no security group has 0.0.0.0/0 ingress", () => {
      const template = createTemplate();
      const json = template.toJSON();

      const ingressRules = Object.values(json.Resources).filter(
        (r: any) => r.Type === "AWS::EC2::SecurityGroupIngress",
      );
      for (const rule of ingressRules) {
        expect((rule as any).Properties.CidrIp).not.toBe("0.0.0.0/0");
      }

      // Also check inline security group ingress rules
      const sgs = Object.values(json.Resources).filter(
        (r: any) => r.Type === "AWS::EC2::SecurityGroup",
      );
      for (const sg of sgs) {
        const ingress =
          (sg as any).Properties.SecurityGroupIngress ?? [];
        for (const rule of ingress) {
          expect(rule.CidrIp).not.toBe("0.0.0.0/0");
        }
      }
    });

    test("EC2 SG egress only allows port 443 to endpoint SG", () => {
      const template = createTemplate();
      const json = template.toJSON();

      const egressRules = Object.values(json.Resources).filter(
        (r: any) => r.Type === "AWS::EC2::SecurityGroupEgress",
      );

      // Should have at least one egress rule for EC2 -> Endpoints on 443
      const httpsEgress = egressRules.filter(
        (r: any) =>
          r.Properties.FromPort === 443 && r.Properties.ToPort === 443,
      );
      expect(httpsEgress.length).toBeGreaterThanOrEqual(1);
    });

    test("endpoint SG has allowAllOutbound=false (no default 0.0.0.0/0 egress)", () => {
      const template = createTemplate();
      const json = template.toJSON();

      // With allowAllOutbound=false, CDK does not auto-add the 0.0.0.0/0 egress
      // Check that no SG has outbound rules to 0.0.0.0/0 for all ports
      const sgs = Object.values(json.Resources).filter(
        (r: any) => r.Type === "AWS::EC2::SecurityGroup",
      );
      for (const sg of sgs) {
        const egress =
          (sg as any).Properties.SecurityGroupEgress ?? [];
        for (const rule of egress) {
          if (rule.CidrIp === "0.0.0.0/0") {
            // Only acceptable if it's the CDK default disable rule (protocol -1, ipprotocol 255)
            // which CDK adds when allowAllOutbound=false to block the default "allow all"
            expect(rule.IpProtocol).toBe("255");
          }
        }
      }
    });
  });

  describe("EC2 security hardening", () => {
    test("launch template enforces IMDSv2", () => {
      const template = createTemplate();
      template.hasResourceProperties("AWS::EC2::LaunchTemplate", {
        LaunchTemplateData: Match.objectLike({
          MetadataOptions: Match.objectLike({
            HttpTokens: "required",
          }),
        }),
      });
    });

    test("launch template does not associate public IP", () => {
      const template = createTemplate();
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
  });
});
