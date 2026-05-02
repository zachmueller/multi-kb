import * as cdk from "aws-cdk-lib";
import { Template } from "aws-cdk-lib/assertions";
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
    consolidationModelId: "anthropic.claude-sonnet-4-20250514",
    coverageModelId: "anthropic.claude-haiku-4-5-20251001",
    tickInterval: "5m",
    dreamCycleInterval: "3h",
    excludePendingFromRecall: true,
    coverageScoreThreshold: 0.3,
    cliBinaryS3Uri: "s3://test-bucket/multi-kb/cli-linux-arm64",
    ...overrides,
  };
}

describe("MultiKbStack", () => {
  test("synthesizes without errors", () => {
    const app = new cdk.App();
    const stack = new MultiKbStack(app, "TestStack", defaultProps());
    const template = Template.fromStack(stack);
    expect(template.toJSON()).toBeDefined();
  });

  test("resolveProps applies defaults when only cliBinaryS3Uri is set", () => {
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

  test("resolveProps throws when cliBinaryS3Uri is missing", () => {
    const app = new cdk.App({ context: {} });
    expect(() => resolveProps(app)).toThrow("cliBinaryS3Uri");
  });

  test("resolveProps accepts context overrides", () => {
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
