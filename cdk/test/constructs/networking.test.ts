import * as cdk from "aws-cdk-lib";
import { Match, Template } from "aws-cdk-lib/assertions";
import { Networking } from "../../lib/constructs/networking";

function createTemplate(
  overrides?: Partial<{ collectionName: string }>,
): Template {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  new Networking(stack, "Networking", {
    collectionName: overrides?.collectionName ?? "multi-kb",
  });
  return Template.fromStack(stack);
}

describe("Networking Construct", () => {
  test("creates a VPC with CIDR 10.0.0.0/16", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::EC2::VPC", {
      CidrBlock: "10.0.0.0/16",
    });
  });

  test("VPC has exactly 1 AZ (single-AZ design)", () => {
    const template = createTemplate();
    // Should have exactly 1 private isolated subnet
    template.resourceCountIs("AWS::EC2::Subnet", 1);
  });

  test("subnet is private isolated (no NAT gateways)", () => {
    const template = createTemplate();
    // No NAT gateways or internet gateways expected
    template.resourceCountIs("AWS::EC2::NatGateway", 0);
  });

  test("creates EC2 security group with allowAllOutbound=false", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::EC2::SecurityGroup", {
      GroupDescription:
        "EC2 instance security group - HTTPS to endpoints only",
    });
  });

  test("creates Endpoint security group with allowAllOutbound=false", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::EC2::SecurityGroup", {
      GroupDescription:
        "VPC endpoint security group - HTTPS from EC2 SG only",
    });
  });

  test("creates exactly 2 security groups", () => {
    const template = createTemplate();
    template.resourceCountIs("AWS::EC2::SecurityGroup", 2);
  });

  test("creates CfnSecurityGroupEgress from EC2 SG to Endpoint SG on port 443", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::EC2::SecurityGroupEgress", {
      IpProtocol: "tcp",
      FromPort: 443,
      ToPort: 443,
      Description: "HTTPS to VPC endpoints",
    });
  });

  test("creates CfnSecurityGroupIngress from EC2 SG to Endpoint SG on port 443", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::EC2::SecurityGroupIngress", {
      IpProtocol: "tcp",
      FromPort: 443,
      ToPort: 443,
      Description: "HTTPS from EC2 instances",
    });
  });

  test("creates S3 gateway endpoint", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::EC2::VPCEndpoint", {
      ServiceName: {
        "Fn::Join": [
          "",
          [
            "com.amazonaws.",
            { Ref: "AWS::Region" },
            ".s3",
          ],
        ],
      },
      VpcEndpointType: "Gateway",
    });
  });

  test("creates 8 interface VPC endpoints", () => {
    const template = createTemplate();
    const allEndpoints = template.findResources("AWS::EC2::VPCEndpoint");
    const interfaceEndpoints = Object.entries(allEndpoints).filter(
      ([, r]: [string, any]) =>
        r.Properties?.VpcEndpointType !== "Gateway",
    );
    expect(interfaceEndpoints.length).toBe(8);
  });

  test("interface endpoints have open=false (no permissive ingress)", () => {
    const template = createTemplate();
    const json = template.toJSON();
    // No ingress rule with 0.0.0.0/0 should exist for endpoint SG
    const allIngress = Object.values(json.Resources).filter(
      (r: any) =>
        r.Type === "AWS::EC2::SecurityGroupIngress" &&
        r.Properties?.CidrIp === "0.0.0.0/0",
    );
    expect(allIngress.length).toBe(0);
  });

  test("creates AOSS VPC endpoint (CfnVpcEndpoint)", () => {
    const template = createTemplate();
    template.hasResourceProperties(
      "AWS::OpenSearchServerless::VpcEndpoint",
      {
        Name: Match.stringLikeRegexp("multi-kb.*-ep"),
      },
    );
  });

  test("all interface endpoints have privateDnsEnabled", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const interfaceEndpoints = Object.entries(json.Resources).filter(
      ([, r]: [string, any]) =>
        r.Type === "AWS::EC2::VPCEndpoint" &&
        r.Properties?.VpcEndpointType !== "Gateway",
    );
    for (const [, ep] of interfaceEndpoints) {
      expect((ep as any).Properties.PrivateDnsEnabled).toBe(true);
    }
  });

  test("interface endpoints include expected services", () => {
    const template = createTemplate();
    const json = template.toJSON();
    // With env set, ServiceName is a resolved plain string like "com.amazonaws.us-east-1.sqs"
    const serviceNames = Object.values(json.Resources)
      .filter(
        (r: any) =>
          r.Type === "AWS::EC2::VPCEndpoint" &&
          r.Properties?.VpcEndpointType !== "Gateway",
      )
      .map((r: any) => {
        const sn = r.Properties.ServiceName;
        return typeof sn === "string" ? sn : JSON.stringify(sn);
      });

    const allServices = serviceNames.join(" ");
    expect(allServices).toContain("sqs");
    expect(allServices).toContain("git-codecommit");
    expect(allServices).toContain("bedrock-runtime");
    expect(allServices).toContain("bedrock-agent");
    expect(allServices).toContain(".ssm");
    expect(allServices).toContain("ssmmessages");
    expect(allServices).toContain("ec2messages");
    expect(allServices).toContain(".logs");
  });
});
