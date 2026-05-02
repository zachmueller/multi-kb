import * as cdk from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import { Match, Template } from "aws-cdk-lib/assertions";
import { Search } from "../../lib/constructs/search";

function createTestStack(): {
  stack: cdk.Stack;
  vpc: ec2.Vpc;
  subnet: ec2.ISubnet;
  endpointSg: ec2.SecurityGroup;
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
  const endpointSg = new ec2.SecurityGroup(stack, "EndpointSg", {
    vpc,
    allowAllOutbound: false,
  });
  return { stack, vpc, subnet: vpc.isolatedSubnets[0], endpointSg };
}

function createTemplate(): Template {
  const { stack, vpc, subnet, endpointSg } = createTestStack();
  new Search(stack, "Search", {
    collectionName: "multi-kb",
    aossVpcEndpointId: "vpce-0123456789abcdef0",
    vpc,
    subnet,
    endpointSecurityGroup: endpointSg,
  });
  return Template.fromStack(stack);
}

describe("Search Construct", () => {
  test("creates encryption policy for the collection", () => {
    const template = createTemplate();
    template.hasResourceProperties(
      "AWS::OpenSearchServerless::SecurityPolicy",
      {
        Name: "multi-kb-enc",
        Type: "encryption",
      },
    );
  });

  test("encryption policy uses AWSOwnedKey", () => {
    const template = createTemplate();
    const policies = template.findResources(
      "AWS::OpenSearchServerless::SecurityPolicy",
    );
    const encPolicy = Object.values(policies).find(
      (p: any) => p.Properties.Type === "encryption",
    ) as any;
    expect(encPolicy).toBeDefined();
    const parsed = JSON.parse(encPolicy.Properties.Policy);
    expect(parsed.AWSOwnedKey).toBe(true);
  });

  test("creates network policy with AllowFromPublic=false", () => {
    const template = createTemplate();
    template.hasResourceProperties(
      "AWS::OpenSearchServerless::SecurityPolicy",
      {
        Name: "multi-kb-net",
        Type: "network",
      },
    );
    const policies = template.findResources(
      "AWS::OpenSearchServerless::SecurityPolicy",
    );
    const netPolicy = Object.values(policies).find(
      (p: any) => p.Properties.Type === "network",
    ) as any;
    const parsed = JSON.parse(netPolicy.Properties.Policy);
    expect(parsed[0].AllowFromPublic).toBe(false);
  });

  test("network policy includes bedrock.amazonaws.com in SourceServices", () => {
    const template = createTemplate();
    const policies = template.findResources(
      "AWS::OpenSearchServerless::SecurityPolicy",
    );
    const netPolicy = Object.values(policies).find(
      (p: any) => p.Properties.Type === "network",
    ) as any;
    const parsed = JSON.parse(netPolicy.Properties.Policy);
    expect(parsed[0].SourceServices).toContain("bedrock.amazonaws.com");
  });

  test("creates VECTORSEARCH collection", () => {
    const template = createTemplate();
    template.hasResourceProperties(
      "AWS::OpenSearchServerless::Collection",
      {
        Name: "multi-kb",
        Type: "VECTORSEARCH",
        StandbyReplicas: "DISABLED",
      },
    );
  });

  test("collection depends on encryption policy", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const collectionKey = Object.keys(json.Resources).find(
      (k) =>
        json.Resources[k].Type ===
        "AWS::OpenSearchServerless::Collection",
    )!;
    const collection = json.Resources[collectionKey];
    expect(collection.DependsOn).toBeDefined();
    // DependsOn should include encryption policy logical ID
    const depStr = JSON.stringify(collection.DependsOn);
    expect(depStr).toContain("EncryptionPolicy");
  });

  test("creates index creation Lambda role with AOSSAccess", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      AssumeRolePolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Principal: { Service: "lambda.amazonaws.com" },
          }),
        ]),
      }),
      Policies: Match.arrayWith([
        Match.objectLike({
          PolicyName: "AOSSAccess",
        }),
      ]),
    });
  });

  test("index creation Lambda has VPCAccessExecutionRole managed policy", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      ManagedPolicyArns: Match.arrayWith([
        Match.objectLike({
          "Fn::Join": Match.arrayWith([
            Match.arrayWith([
              Match.stringLikeRegexp("AWSLambdaVPCAccessExecutionRole"),
            ]),
          ]),
        }),
      ]),
    });
  });

  test("index creation Lambda uses Node.js 22.x ARM64", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Lambda::Function", {
      Runtime: "nodejs22.x",
      Architectures: ["arm64"],
    });
  });

  test("creates CollectionEndpoint CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasOutput = Object.keys(outputs).some((k) =>
      k.includes("CollectionEndpoint"),
    );
    expect(hasOutput).toBe(true);
  });

  test("creates custom resource for index creation", () => {
    const template = createTemplate();
    const json = template.toJSON();
    const customResources = Object.values(json.Resources).filter(
      (r: any) => r.Type === "AWS::CloudFormation::CustomResource",
    );
    expect(customResources.length).toBeGreaterThanOrEqual(1);
  });
});
