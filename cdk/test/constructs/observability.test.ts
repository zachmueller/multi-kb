import * as cdk from "aws-cdk-lib";
import * as sqs from "aws-cdk-lib/aws-sqs";
import { Match, Template } from "aws-cdk-lib/assertions";
import {
  Observability,
  ObservabilityProps,
} from "../../lib/constructs/observability";

function createTestStack(): { stack: cdk.Stack; dlq: sqs.Queue } {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  const dlq = new sqs.Queue(stack, "Dlq");
  return { stack, dlq };
}

function defaultObsProps(dlq: sqs.Queue): ObservabilityProps {
  return {
    dlq,
    submitLambdaFunctionName: "multi-kb-submit",
    recallLambdaFunctionName: "multi-kb-recall",
    asgName: "multi-kb-asg",
  };
}

describe("Observability Construct", () => {
  test("creates EC2 CLI log group with 30-day retention", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::Logs::LogGroup", {
      LogGroupName: "/multi-kb/ec2/server",
      RetentionInDays: 30,
    });
  });

  test("creates submit Lambda log group with 30-day retention", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::Logs::LogGroup", {
      LogGroupName: "/aws/lambda/multi-kb-submit",
      RetentionInDays: 30,
    });
  });

  test("creates recall Lambda log group with 30-day retention", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::Logs::LogGroup", {
      LogGroupName: "/aws/lambda/multi-kb-recall",
      RetentionInDays: 30,
    });
  });

  test("creates 3 log groups total", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.resourceCountIs("AWS::Logs::LogGroup", 3);
  });

  test("creates DLQ alarm with threshold > 0", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::CloudWatch::Alarm", {
      AlarmName: "multi-kb-dlq-messages",
      MetricName: "ApproximateNumberOfMessagesVisible",
      Threshold: 0,
      ComparisonOperator: "GreaterThanThreshold",
    });
  });

  test("creates ASG unhealthy alarm", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::CloudWatch::Alarm", {
      AlarmName: "multi-kb-ec2-unhealthy",
      MetricName: "GroupInServiceInstances",
      Threshold: 1,
      ComparisonOperator: "LessThanThreshold",
    });
  });

  test("creates dream cycle lock alarm", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::CloudWatch::Alarm", {
      AlarmName: "multi-kb-dream-cycle-lock",
    });
  });

  test("creates 3 alarms total", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.resourceCountIs("AWS::CloudWatch::Alarm", 3);
  });

  test("no alarm has actions configured (metrics only for MVP)", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    const alarms = template.findResources("AWS::CloudWatch::Alarm");
    for (const [, alarm] of Object.entries(alarms)) {
      const props = alarm.Properties;
      expect(props.AlarmActions ?? []).toHaveLength(0);
      expect(props.OKActions ?? []).toHaveLength(0);
      expect(props.InsufficientDataActions ?? []).toHaveLength(0);
    }
  });

  test("creates metric filter for dream cycle lock", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::Logs::MetricFilter", {
      MetricTransformations: Match.arrayWith([
        Match.objectLike({
          MetricNamespace: "MultiKB",
          MetricName: "DreamCycleLockHeldMinutes",
        }),
      ]),
    });
  });

  test("skips ASG alarm when asgName is undefined", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", {
      ...defaultObsProps(dlq),
      asgName: undefined,
    });
    const template = Template.fromStack(stack);

    // Should only have DLQ alarm + dream cycle lock alarm (no ASG alarm)
    template.resourceCountIs("AWS::CloudWatch::Alarm", 2);
  });

  test("EC2 log group has DESTROY removal policy", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    const logGroups = template.findResources("AWS::Logs::LogGroup");
    const ec2LogGroup = Object.entries(logGroups).find(
      ([, r]: [string, any]) =>
        r.Properties?.LogGroupName === "/multi-kb/ec2/server",
    );
    expect(ec2LogGroup).toBeDefined();
    expect(ec2LogGroup![1].DeletionPolicy).toBe("Delete");
  });

  test("DLQ alarm has TreatMissingData NOT_BREACHING", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::CloudWatch::Alarm", {
      AlarmName: "multi-kb-dlq-messages",
      TreatMissingData: "notBreaching",
    });
  });

  test("ASG alarm has TreatMissingData BREACHING", () => {
    const { stack, dlq } = createTestStack();
    new Observability(stack, "Obs", defaultObsProps(dlq));
    const template = Template.fromStack(stack);

    template.hasResourceProperties("AWS::CloudWatch::Alarm", {
      AlarmName: "multi-kb-ec2-unhealthy",
      TreatMissingData: "breaching",
    });
  });
});
