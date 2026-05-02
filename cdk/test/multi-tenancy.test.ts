import * as cdk from "aws-cdk-lib";
import { Template } from "aws-cdk-lib/assertions";
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

describe("QAT-004: Multi-Tenancy Validation", () => {
  let templateA: Template;
  let templateB: Template;
  let jsonA: any;
  let jsonB: any;

  beforeAll(() => {
    const appA = new cdk.App();
    const stackA = new MultiKbStack(
      appA,
      "TenantA",
      defaultProps({
        repoName: "tenant-a-kb",
        bucketPrefix: "tenant-a",
      }),
    );
    templateA = Template.fromStack(stackA);
    jsonA = templateA.toJSON();

    const appB = new cdk.App();
    const stackB = new MultiKbStack(
      appB,
      "TenantB",
      defaultProps({
        repoName: "tenant-b-kb",
        bucketPrefix: "tenant-b",
      }),
    );
    templateB = Template.fromStack(stackB);
    jsonB = templateB.toJSON();
  });

  test("both stacks synthesize without errors", () => {
    expect(jsonA).toBeDefined();
    expect(jsonB).toBeDefined();
  });

  test("S3 bucket names are different between tenants", () => {
    const bucketsA = Object.values(jsonA.Resources).filter(
      (r: any) => r.Type === "AWS::S3::Bucket",
    );
    const bucketsB = Object.values(jsonB.Resources).filter(
      (r: any) => r.Type === "AWS::S3::Bucket",
    );
    expect(bucketsA.length).toBe(1);
    expect(bucketsB.length).toBe(1);

    const nameA = JSON.stringify(
      (bucketsA[0] as any).Properties.BucketName,
    );
    const nameB = JSON.stringify(
      (bucketsB[0] as any).Properties.BucketName,
    );
    expect(nameA).toContain("tenant-a");
    expect(nameB).toContain("tenant-b");
    expect(nameA).not.toEqual(nameB);
  });

  test("CodeCommit repository names are different between tenants", () => {
    const reposA = Object.values(jsonA.Resources).filter(
      (r: any) => r.Type === "AWS::CodeCommit::Repository",
    );
    const reposB = Object.values(jsonB.Resources).filter(
      (r: any) => r.Type === "AWS::CodeCommit::Repository",
    );

    const repoNameA = (reposA[0] as any).Properties.RepositoryName;
    const repoNameB = (reposB[0] as any).Properties.RepositoryName;
    expect(repoNameA).toBe("tenant-a-kb");
    expect(repoNameB).toBe("tenant-b-kb");
    expect(repoNameA).not.toEqual(repoNameB);
  });

  test("OpenSearch collection names are different between tenants", () => {
    const collectionsA = Object.values(jsonA.Resources).filter(
      (r: any) =>
        r.Type === "AWS::OpenSearchServerless::Collection",
    );
    const collectionsB = Object.values(jsonB.Resources).filter(
      (r: any) =>
        r.Type === "AWS::OpenSearchServerless::Collection",
    );

    const nameA = (collectionsA[0] as any).Properties.Name;
    const nameB = (collectionsB[0] as any).Properties.Name;
    expect(nameA).toBe("tenant-a-kb");
    expect(nameB).toBe("tenant-b-kb");
    expect(nameA).not.toEqual(nameB);
  });

  test("OpenSearch encryption policy names are different between tenants", () => {
    const encPolA = Object.values(jsonA.Resources).filter(
      (r: any) =>
        r.Type === "AWS::OpenSearchServerless::SecurityPolicy" &&
        r.Properties?.Type === "encryption",
    );
    const encPolB = Object.values(jsonB.Resources).filter(
      (r: any) =>
        r.Type === "AWS::OpenSearchServerless::SecurityPolicy" &&
        r.Properties?.Type === "encryption",
    );

    const nameA = (encPolA[0] as any).Properties.Name;
    const nameB = (encPolB[0] as any).Properties.Name;
    expect(nameA).toContain("tenant-a");
    expect(nameB).toContain("tenant-b");
    expect(nameA).not.toEqual(nameB);
  });

  test("each tenant has the same number of resources", () => {
    const resourceTypesA = new Map<string, number>();
    const resourceTypesB = new Map<string, number>();

    for (const r of Object.values(jsonA.Resources) as any[]) {
      resourceTypesA.set(r.Type, (resourceTypesA.get(r.Type) ?? 0) + 1);
    }
    for (const r of Object.values(jsonB.Resources) as any[]) {
      resourceTypesB.set(r.Type, (resourceTypesB.get(r.Type) ?? 0) + 1);
    }

    // Both stacks should have the same resource type counts
    expect(resourceTypesA.size).toBe(resourceTypesB.size);
    for (const [type, count] of resourceTypesA) {
      expect(resourceTypesB.get(type)).toBe(count);
    }
  });

  test("globally-unique resource names differ between tenants", () => {
    // These resource types require globally or regionally unique names:
    // S3 buckets, CodeCommit repos, OpenSearch collections,
    // AOSS policies, AOSS VPC endpoints
    const globallyUniqueTypes = new Set([
      "AWS::S3::Bucket",
      "AWS::CodeCommit::Repository",
      "AWS::OpenSearchServerless::Collection",
      "AWS::OpenSearchServerless::SecurityPolicy",
      "AWS::OpenSearchServerless::AccessPolicy",
      "AWS::OpenSearchServerless::VpcEndpoint",
    ]);

    function extractNames(json: any): string[] {
      const names: string[] = [];
      for (const r of Object.values(json.Resources) as any[]) {
        if (globallyUniqueTypes.has(r.Type)) {
          const name =
            r.Properties?.Name ??
            r.Properties?.RepositoryName ??
            r.Properties?.BucketName;
          if (typeof name === "string") {
            names.push(name);
          }
        }
      }
      return names;
    }

    const namesA = extractNames(jsonA);
    const namesB = extractNames(jsonB);
    const setB = new Set(namesB);
    const overlap = namesA.filter((n) => setB.has(n));
    expect(overlap).toEqual([]);
  });

  test("API Gateway REST API names could be the same (not globally unique)", () => {
    // API names are not globally unique resources, so this is informational
    const apisA = Object.values(jsonA.Resources).filter(
      (r: any) => r.Type === "AWS::ApiGateway::RestApi",
    );
    const apisB = Object.values(jsonB.Resources).filter(
      (r: any) => r.Type === "AWS::ApiGateway::RestApi",
    );
    expect(apisA.length).toBe(1);
    expect(apisB.length).toBe(1);
  });
});
